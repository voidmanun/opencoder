package bridge

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"dingtalk-bridge/internal/config"
	"dingtalk-bridge/internal/dingtalk"
	"dingtalk-bridge/internal/logger"
	"dingtalk-bridge/internal/opencode"
	"dingtalk-bridge/internal/session"
)

type Router struct {
	config         *config.Config
	dingClient     *dingtalk.Client
	cardClient     *dingtalk.CardClient
	opencodeClient *opencode.ServerClient
	sessionStore   *session.Store
	replier        *dingtalk.Replier
	activeStreams  sync.Map
	// recentMsgIDs caches recently seen message IDs to deduplicate incoming messages
	recentMsgIDs sync.Map
}

func NewRouter(cfg *config.Config, dingClient *dingtalk.Client, cardClient *dingtalk.CardClient, opencodeClient *opencode.ServerClient) *Router {
	r := &Router{
		config:         cfg,
		dingClient:     dingClient,
		cardClient:     cardClient,
		opencodeClient: opencodeClient,
		sessionStore:   session.NewStore(),
		replier:        dingtalk.NewReplier(),
	}

	// Optional: start background cleanup for deduplication cache
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			// Simple eviction: keep the most recent 1000 message IDs by counting through the map
			count := 0
			r.recentMsgIDs.Range(func(key, value interface{}) bool {
				count++
				if count > 1000 {
					r.recentMsgIDs.Delete(key)
				}
				return true
			})
		}
	}()

	return r
}

func (r *Router) HandleMessage(ctx context.Context, msg *dingtalk.ReceivedMessage) error {
	// Enforce user whitelist at entrypoint if configured
	if r.config.UserWhitelist != nil && r.config.UserWhitelist.Enabled {
		allowed := false
		for _, uid := range r.config.UserWhitelist.Users {
			if uid == msg.SenderStaffID {
				allowed = true
				break
			}
		}
		if !allowed {
			logger.Warnf("Blocked non-whitelist user: %s", msg.SenderStaffID)
			return r.replier.ReplyText(ctx, msg.SessionWebhook, "Access denied: you are not authorized to use this bot.")
		}
	}
	// After whitelist check, prepare sessionKey for potential stream handling
	// sessionKey is declared earlier to enable dedup/cancellation logic

	// Declare sessionKey early to be shared with dedup/cancellation logic
	sessionKey := msg.SessionKey()

	// 1) Add deduplication for incoming messages
	if _, seen := r.recentMsgIDs.Load(msg.MsgID); seen {
		logger.Debugf("Ignoring duplicate message: %s", msg.MsgID)
		return nil
	}
	r.recentMsgIDs.Store(msg.MsgID, true)

	// 2) Cancel existing stream for this session before starting a new one
	if cancel, ok := r.activeStreams.Load(sessionKey); ok {
		logger.Infof("Cancelling existing stream for session: %s", sessionKey)
		cancel.(context.CancelFunc)()
		// Brief cleanup
		time.Sleep(100 * time.Millisecond)
	}

	content := strings.TrimSpace(msg.Content)
	if content == "" {
		return nil
	}

	if msg.IsGroupChat() && !msg.IsInAtList {
		logger.Debugf("Ignoring group message without @ mention")
		return nil
	}
	content = r.stripMention(content)

	// sessionKey already defined above
	sess, exists := r.sessionStore.Get(sessionKey)
	if !exists {
		sess = &session.Session{
			DingConversationID: msg.ConversationID,
			DingUserID:         msg.SenderStaffID,
			DingSessionWebhook: msg.SessionWebhook,
			ProjectID:          r.config.BridgeWorkDir,
			Mode:               r.config.BridgeMode,
		}
		r.sessionStore.Set(sessionKey, sess)
		// Log sanitized webhook for debugging without leaking sensitive data
		logger.Debugf("Initialized session: webhook=%s", sanitizeWebhook(sess.DingSessionWebhook))
	}

	// Log a concise summary of the incoming message at info level,
	// with the full content logged at debug level if needed.
	logger.Infof("Processing message from %s (%d chars)", sessionKey, len(content))
	logger.Debugf("Message content from %s: %s", sessionKey, content)

	if r.config.BridgeMode == "advanced" {
		return r.handleAdvancedMode(ctx, sess, msg, content)
	}
	return r.handleMVPMode(ctx, sess, msg, content)
}

func (r *Router) stripMention(content string) string {
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if strings.Contains(line, "@") {
			parts := strings.SplitN(line, " ", 2)
			if len(parts) > 1 {
				lines[i] = parts[1]
			} else {
				lines[i] = ""
			}
		}
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func (r *Router) handleAdvancedMode(ctx context.Context, sess *session.Session, msg *dingtalk.ReceivedMessage, content string) error {
	logger.Infof("handleAdvancedMode: sending initial card...")
	cardBizID, err := r.sendInitialCard(msg)
	if err != nil {
		logger.Errorf("Failed to send initial card: %v", err)
		return r.handleMVPMode(ctx, sess, msg, content)
	}
	logger.Infof("handleAdvancedMode: card sent, bizID=%s", cardBizID)
	sess.CardBizID = cardBizID
	r.sessionStore.Set(msg.SessionKey(), sess)

	if sess.OpenCodeSessionID == "" {
		logger.Infof("handleAdvancedMode: creating OpenCode session...")
		ocSession, err := r.opencodeClient.CreateSession(ctx, "", "/Users/voidman/Documents/git_dir/opencoder")
		if err != nil {
			r.updateCardWithError(cardBizID, err.Error())
			return fmt.Errorf("failed to create opencode session: %w", err)
		}
		logger.Infof("handleAdvancedMode: OpenCode session created, id=%s", ocSession.ID)
		sess.OpenCodeSessionID = ocSession.ID
		r.sessionStore.Set(msg.SessionKey(), sess)
	} else {
		logger.Infof("handleAdvancedMode: reusing existing OpenCode session, id=%s", sess.OpenCodeSessionID)
	}

	streamCtx, cancel := context.WithCancel(context.Background())
	streamKey := msg.SessionKey()
	r.activeStreams.Store(streamKey, cancel)

	logger.Infof("handleAdvancedMode: starting streamResponse goroutine...")
	go r.streamResponse(streamCtx, sess, content, cardBizID)

	return nil
}

func (r *Router) handleMVPMode(ctx context.Context, sess *session.Session, msg *dingtalk.ReceivedMessage, content string) error {
	if sess.OpenCodeSessionID == "" {
		ocSession, err := r.opencodeClient.CreateSession(ctx, "", "/Users/voidman/Documents/git_dir/opencoder")
		if err != nil {
			r.replier.ReplyText(ctx, msg.SessionWebhook, fmt.Sprintf("Error: %v", err))
			return err
		}
		sess.OpenCodeSessionID = ocSession.ID
		r.sessionStore.Set(msg.SessionKey(), sess)
	}

	respMsg, err := r.opencodeClient.SendMessage(ctx, sess.OpenCodeSessionID, content)
	if err != nil {
		r.replier.ReplyText(ctx, msg.SessionWebhook, fmt.Sprintf("Error: %v", err))
		return err
	}

	var responseText strings.Builder
	for _, part := range respMsg.Parts {
		if part.Type == "text" && part.Text != "" {
			responseText.WriteString(part.Text)
		}
	}

	if responseText.Len() > 0 {
		return r.replier.ReplyMarkdown(ctx, msg.SessionWebhook, "OpenCode Response", responseText.String())
	}
	return r.replier.ReplyText(ctx, msg.SessionWebhook, "Task completed (no text output)")
}

func (r *Router) sendInitialCard(msg *dingtalk.ReceivedMessage) (string, error) {
	cardData := dingtalk.RenderStreamingCard(&dingtalk.StreamingCard{
		Title:      "🤖 OpenCode AI",
		Subtitle:   "智能助手",
		Content:    "_等待响应中..._",
		Status:     dingtalk.FormatStatus("thinking"),
		Tokens:     "",
		SessionKey: msg.SessionKey(),
	})

	return r.cardClient.SendCard(&dingtalk.SendCardOptions{
		ConversationID:   msg.ConversationID,
		ConversationType: msg.ConversationType,
		SenderStaffID:    msg.SenderStaffID,
		CardData:         cardData,
	})
}

func (r *Router) streamResponse(ctx context.Context, sess *session.Session, content, cardBizID string) {
	defer r.activeStreams.Delete(sess.SessionKey())

	logger.Infof("streamResponse: connecting to event stream FIRST...")
	resp, err := r.opencodeClient.EventStream(ctx)
	if err != nil {
		logger.Errorf("streamResponse: EventStream failed: %v", err)
		r.updateCardWithError(cardBizID, err.Error())
		return
	}
	defer resp.Body.Close()

	logger.Infof("streamResponse: NOW sending message async to session %s", sess.OpenCodeSessionID)
	err = r.opencodeClient.SendMessageAsync(ctx, sess.OpenCodeSessionID, content)
	if err != nil {
		logger.Errorf("streamResponse: SendMessageAsync failed: %v", err)
		r.updateCardWithError(cardBizID, err.Error())
		return
	}

	logger.Infof("streamResponse: reading SSE events...")
	reader := opencode.NewSSEReader(resp.Body)

	var accumulatedText strings.Builder
	var lastThinking strings.Builder
	var lastToolName string
	tokenCount := 0
	currentStatus := "streaming"
	lastUpdate := time.Time{}
	updateInterval := 150 * time.Millisecond

	for {
		event, err := reader.ReadEvent()
		if err != nil {
			logger.Infof("SSE read result: err=%v (type: %T)", err, err)
			if err == io.EOF || err.Error() == "EOF" {
				logger.Infof("SSE stream ended normally (EOF)")
				break
			}
			logger.Errorf("Error reading SSE event: %v", err)
			break
		}

		logger.Debugf("Received SSE event: type=%s, content=%s", event.Type, event.Content)

		select {
		case <-ctx.Done():
			r.updateCardFinal(cardBizID, accumulatedText.String(), "cancelled", tokenCount)
			return
		default:
		}

		if event.SessionID != "" && event.SessionID != sess.OpenCodeSessionID {
			logger.Debugf("Skipping event for different session: %s (current: %s)", event.SessionID, sess.OpenCodeSessionID)
			continue
		}

		if event.SessionID == "" && event.Type != opencode.EventServerConnected {
			logger.Debugf("Event without sessionID: type=%s", event.Type)
		}

		switch event.Type {
		case opencode.EventText, opencode.EventMessageDelta:
			if event.Content != "" {
				accumulatedText.WriteString(event.Content)
				tokenCount++
				if time.Since(lastUpdate) > updateInterval {
					r.updateCard(cardBizID, accumulatedText.String(), currentStatus, tokenCount, lastThinking.String(), lastToolName)
					lastUpdate = time.Now()
				}
			}

		case opencode.EventMessagePart:
			if event.Content != "" {
				accumulatedText.WriteString(event.Content)
				tokenCount++
				r.updateCard(cardBizID, accumulatedText.String(), currentStatus, tokenCount, lastThinking.String(), lastToolName)
				lastUpdate = time.Now()
			}

		case opencode.EventReasoning:
			currentStatus = "thinking"
			if event.Content != "" {
				lastThinking.Reset()
				lastThinking.WriteString(event.Content)
			}
			r.updateCard(cardBizID, accumulatedText.String(), currentStatus, tokenCount, lastThinking.String(), lastToolName)

		case opencode.EventToolUse:
			currentStatus = "tool"
			lastToolName = event.ToolName
			r.updateCard(cardBizID, accumulatedText.String(), currentStatus, tokenCount, lastThinking.String(), lastToolName)

		case opencode.EventStepFinish, opencode.EventSessionIdle:
			currentStatus = "completed"
			if accumulatedText.Len() > 0 {
				r.updateCardFinal(cardBizID, accumulatedText.String(), currentStatus, tokenCount)
			}
			return

		case opencode.EventError:
			r.updateCardWithError(cardBizID, event.Error)
			return
		}
	}

	if accumulatedText.Len() > 0 {
		r.updateCardFinal(cardBizID, accumulatedText.String(), "completed", tokenCount)
	}
}

func (r *Router) updateCard(cardBizID, content, status string, tokenCount int, thinking string, toolName string) {
	thinkingLabel, thinkingContent := dingtalk.FormatThinking(thinking, status == "thinking")
	toolsLabel, toolsContent := dingtalk.FormatTool(toolName, status == "tool")

	cardData := dingtalk.RenderStreamingCard(&dingtalk.StreamingCard{
		Title:           "🤖 OpenCode AI",
		Subtitle:        "智能助手",
		Content:         dingtalk.TruncateContent(content, 6000),
		Status:          dingtalk.FormatStatus(status),
		Tokens:          dingtalk.FormatTokens(tokenCount),
		ThinkingLabel:   thinkingLabel,
		ThinkingContent: thinkingContent,
		ToolsLabel:      toolsLabel,
		ToolsContent:    toolsContent,
		SessionKey:      cardBizID,
	})

	if err := r.cardClient.UpdateCard(&dingtalk.UpdateCardOptions{
		CardBizID: cardBizID,
		CardData:  cardData,
	}); err != nil {
		logger.Errorf("Failed to update card: %v", err)
	}
}

func (r *Router) updateCardFinal(cardBizID, content, status string, tokenCount int) {
	cardData := dingtalk.RenderStreamingCard(&dingtalk.StreamingCard{
		Title:           "🤖 OpenCode AI",
		Subtitle:        "智能助手 - 已完成",
		Content:         content,
		Status:          dingtalk.FormatStatus(status),
		Tokens:          dingtalk.FormatTokens(tokenCount),
		ThinkingLabel:   "",
		ThinkingContent: "",
		ToolsLabel:      "",
		ToolsContent:    "",
		SessionKey:      cardBizID,
	})

	if err := r.cardClient.UpdateCard(&dingtalk.UpdateCardOptions{
		CardBizID: cardBizID,
		CardData:  cardData,
	}); err != nil {
		logger.Errorf("Failed to update final card: %v", err)
	}
}

func (r *Router) updateCardWithError(cardBizID, errMsg string) {
	cardData := dingtalk.RenderStreamingCard(&dingtalk.StreamingCard{
		Title:           "🤖 OpenCode AI",
		Subtitle:        "智能助手 - 出错",
		Content:         fmt.Sprintf("❌ **发生错误**\n\n```\n%s\n```", errMsg),
		Status:          dingtalk.FormatStatus("error"),
		Tokens:          "",
		ThinkingLabel:   "",
		ThinkingContent: "",
		ToolsLabel:      "",
		ToolsContent:    "",
		SessionKey:      cardBizID,
	})

	if err := r.cardClient.UpdateCard(&dingtalk.UpdateCardOptions{
		CardBizID: cardBizID,
		CardData:  cardData,
	}); err != nil {
		logger.Errorf("Failed to update error card: %v", err)
	}
}

func (r *Router) CancelStream(sessionKey string) bool {
	if cancel, ok := r.activeStreams.Load(sessionKey); ok {
		cancel.(context.CancelFunc)()
		return true
	}
	return false
}

// HandleCardAction processes interactive card button actions
func (r *Router) HandleCardAction(ctx context.Context, sessionKey, action string) error {
	if action != "cancel" {
		logger.Warnf("Unknown card action: %s", action)
		return fmt.Errorf("unknown action: %s", action)
	}

	logger.Infof("Cancel requested for session: %s", sessionKey)

	if r.CancelStream(sessionKey) {
		logger.Infof("Stream cancelled for session: %s", sessionKey)
	}

	sess, exists := r.sessionStore.Get(sessionKey)
	if exists && sess.OpenCodeSessionID != "" {
		if err := r.opencodeClient.AbortSession(ctx, sess.OpenCodeSessionID); err != nil {
			logger.Errorf("Failed to abort OpenCode session: %v", err)
		} else {
			logger.Infof("OpenCode session aborted: %s", sess.OpenCodeSessionID)
		}
	}

	if exists && sess.CardBizID != "" {
		r.updateCardFinal(sess.CardBizID, "Task cancelled by user", "cancelled", 0)
	}

	return nil
}

func (r *Router) GetSessionStore() *session.Store {
	return r.sessionStore
}

// sanitizeWebhook truncates webhook URLs for safe logging
func sanitizeWebhook(webhook string) string {
	if len(webhook) > 30 {
		return webhook[:15] + "..." + webhook[len(webhook)-15:]
	}
	return webhook
}

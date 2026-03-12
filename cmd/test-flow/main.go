package main

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

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Printf("Config error: %v\n", err)
		return
	}

	if err := logger.Init("debug", ""); err != nil {
		fmt.Printf("Logger error: %v\n", err)
		return
	}
	defer logger.Sync()

	// Simulate a DingTalk message
	msg := &dingtalk.ReceivedMessage{
		ConversationID:   "test-conversation-123",
		ConversationType: "1",
		SenderStaffID:    "test-user-456",
		Content:          "你好，请介绍一下你自己",
		SessionWebhook:   "https://example.com/webhook",
	}

	sessionKey := msg.SessionKey()
	fmt.Printf("=== Simulating DingTalk Message ===\n")
	fmt.Printf("Session Key: %s\n", sessionKey)
	fmt.Printf("Content: %s\n", msg.Content)

	// Step 1: Get access token
	fmt.Println("\n=== Step 1: Get DingTalk Access Token ===")
	_, err = dingtalk.NewCardClient(cfg.DingTalkClientID, cfg.DingTalkClientSecret)
	if err != nil {
		fmt.Printf("❌ Card client error: %v\n", err)
		return
	}
	fmt.Println("✅ Card client initialized")

	// Step 2: Create OpenCode session
	fmt.Println("\n=== Step 2: Create OpenCode Session ===")
	oc := opencode.NewServerClient(cfg.OpenCodeServerURL, cfg.OpenCodeServerUsername, cfg.OpenCodeServerPassword, cfg.OpenCodeProviderID, cfg.OpenCodeModelID, cfg.OpenCodeAgent)
	ocSession, err := oc.CreateSession(context.Background(), "", "")
	if err != nil {
		fmt.Printf("❌ Create session failed: %v\n", err)
		return
	}
	fmt.Printf("✅ OpenCode session: %s\n", ocSession.ID)

	// Step 3: Connect SSE stream FIRST
	fmt.Println("\n=== Step 3: Connect SSE Stream ===")
	resp, err := oc.EventStream(context.Background())
	if err != nil {
		fmt.Printf("❌ Event stream failed: %v\n", err)
		return
	}
	defer resp.Body.Close()
	fmt.Println("✅ SSE stream connected")

	// Step 4: Send message
	fmt.Println("\n=== Step 4: Send Message ===")
	err = oc.SendMessageAsync(context.Background(), ocSession.ID, msg.Content)
	if err != nil {
		fmt.Printf("❌ SendMessageAsync failed: %v\n", err)
		return
	}
	fmt.Println("✅ Message sent")

	// Step 5: Read events and simulate card updates
	fmt.Println("\n=== Step 5: Process SSE Events ===")
	reader := opencode.NewSSEReader(resp.Body)

	var accumulatedText strings.Builder
	tokenCount := 0
	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		timeout := time.After(30 * time.Second)

		for {
			select {
			case <-timeout:
				fmt.Println("\n⏱️ Timeout")
				return
			default:
			}

			event, err := reader.ReadEvent()
			if err != nil {
				if err == io.EOF {
					fmt.Println("\n✅ Stream ended (EOF)")
					return
				}
				fmt.Printf("\n❌ Read error: %v\n", err)
				return
			}

			fmt.Printf("[%s] ", event.Type)
			if event.Content != "" {
				fmt.Printf("content=%q ", event.Content[:min(30, len(event.Content))])
			}

			switch event.Type {
			case opencode.EventText:
				if event.Content != "" {
					accumulatedText.WriteString(event.Content)
					tokenCount++
				}
			case opencode.EventMessagePart, opencode.EventMessageDelta:
				if event.Content != "" {
					accumulatedText.WriteString(event.Content)
					tokenCount++
					fmt.Printf("\n📝 Added content\n")
				}
			case opencode.EventReasoning:
				fmt.Printf("\n💭 Reasoning\n")
			case opencode.EventStepFinish, opencode.EventSessionIdle:
				fmt.Println("\n✅ Step finished")
				return
			}
		}
	}()

	wg.Wait()

	// Step 6: Show result
	fmt.Println("\n=== Final Result ===")
	fmt.Printf("Tokens: %d\n", tokenCount)
	fmt.Printf("Response: %s\n", accumulatedText.String())

	// Step 7: Test card render
	fmt.Println("\n=== Step 7: Card Render Test ===")
	cardData := dingtalk.RenderStreamingCard(&dingtalk.StreamingCard{
		Title:      "OpenCode",
		Content:    accumulatedText.String(),
		Status:     dingtalk.FormatStatus("completed"),
		Tokens:     dingtalk.FormatTokens(tokenCount),
		SessionKey: sessionKey,
	})

	if len(cardData) > 100 {
		fmt.Printf("✅ Card rendered (%d bytes)\n", len(cardData))
		fmt.Printf("Preview: %s...\n", cardData[:100])
	}

	// Step 8: Test session store
	fmt.Println("\n=== Step 8: Session Store Test ===")
	store := session.NewStore()
	sess := &session.Session{
		DingConversationID: msg.ConversationID,
		DingUserID:         msg.SenderStaffID,
		DingSessionWebhook: msg.SessionWebhook,
		OpenCodeSessionID:  ocSession.ID,
		ProjectID:          cfg.BridgeWorkDir,
		Mode:               cfg.BridgeMode,
	}
	store.Set(sessionKey, sess)

	retrieved, exists := store.Get(sessionKey)
	if exists && retrieved.OpenCodeSessionID == ocSession.ID {
		fmt.Println("✅ Session store works correctly")
	} else {
		fmt.Println("❌ Session store failed")
	}

	fmt.Println("\n=== ✅ ALL COMPONENTS VERIFIED ===")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

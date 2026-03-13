package dingtalk

import (
	"context"
	"encoding/json"
	"fmt"

	"dingtalk-bridge/internal/logger"

	"github.com/open-dingtalk/dingtalk-stream-sdk-go/card"
	"github.com/open-dingtalk/dingtalk-stream-sdk-go/chatbot"
	"github.com/open-dingtalk/dingtalk-stream-sdk-go/client"
)

type MessageCallback func(ctx context.Context, msg *ReceivedMessage) error

type CardActionCallback func(ctx context.Context, sessionKey, action string) error

type ReceivedMessage struct {
	ConversationID          string
	ConversationType        string
	SenderStaffID           string
	SenderNick              string
	SenderID                string
	IsAdmin                 bool
	MsgID                   string
	MsgType                 string
	Content                 string
	SessionWebhook          string
	SessionWebhookExpiredAt int64
	ChatbotUserID           string
	ChatbotCorpID           string
	SenderCorpID            string
	CreateAt                int64
	AtUsers                 []string
	IsInAtList              bool
}

type Client struct {
	streamClient *client.StreamClient
	clientID     string
	clientSecret string
	callback     MessageCallback
	cardCallback CardActionCallback
}

func NewClient(clientID, clientSecret string) *Client {
	return &Client{
		clientID:     clientID,
		clientSecret: clientSecret,
	}
}

func (c *Client) SetMessageCallback(callback MessageCallback) {
	c.callback = callback
}

func (c *Client) SetCardActionCallback(callback CardActionCallback) {
	c.cardCallback = callback
}

func (c *Client) Start(ctx context.Context) error {
	cred := client.NewAppCredentialConfig(c.clientID, c.clientSecret)
	c.streamClient = client.NewStreamClient(client.WithAppCredential(cred))

	c.streamClient.RegisterChatBotCallbackRouter(c.handleMessage)
	c.streamClient.RegisterCardCallbackRouter(c.handleCardAction)

	logger.Infof("Starting DingTalk Stream client for app: %s", c.clientID)

	if err := c.streamClient.Start(ctx); err != nil {
		return fmt.Errorf("failed to start stream client: %w", err)
	}

	logger.Info("DingTalk Stream client connected successfully")
	return nil
}

func (c *Client) Close() {
	if c.streamClient != nil {
		c.streamClient.Close()
	}
}

func (c *Client) handleMessage(ctx context.Context, data *chatbot.BotCallbackDataModel) ([]byte, error) {
	msg := c.parseMessage(data)

	logger.Debugf("Received message: conversation=%s, sender=%s, type=%s, content=%s",
		msg.ConversationID, msg.SenderStaffID, msg.MsgType, msg.Content)

	if c.callback != nil {
		if err := c.callback(ctx, msg); err != nil {
			logger.Errorf("Message callback error: %v", err)
			return nil, err
		}
	}

	return []byte{}, nil
}

func (c *Client) handleCardAction(ctx context.Context, request *card.CardRequest) (*card.CardResponse, error) {
	params := request.CardActionData.CardPrivateData.Params
	action, _ := params["action"].(string)
	sessionKey, _ := params["sessionKey"].(string)

	logger.Infof("Card action received: action=%s, sessionKey=%s", action, sessionKey)

	if c.cardCallback != nil && action != "" && sessionKey != "" {
		if err := c.cardCallback(ctx, sessionKey, action); err != nil {
			logger.Errorf("Card action callback error: %v", err)
			return nil, err
		}
	}

	return &card.CardResponse{}, nil
}

func (c *Client) parseMessage(data *chatbot.BotCallbackDataModel) *ReceivedMessage {
	msg := &ReceivedMessage{
		ConversationID:          data.ConversationId,
		ConversationType:        data.ConversationType,
		SenderStaffID:           data.SenderStaffId,
		SenderNick:              data.SenderNick,
		SenderID:                data.SenderId,
		IsAdmin:                 data.IsAdmin,
		MsgID:                   data.MsgId,
		MsgType:                 data.Msgtype,
		SessionWebhook:          data.SessionWebhook,
		SessionWebhookExpiredAt: data.SessionWebhookExpiredTime,
		ChatbotUserID:           data.ChatbotUserId,
		ChatbotCorpID:           data.ChatbotCorpId,
		SenderCorpID:            data.SenderCorpId,
		CreateAt:                data.CreateAt,
		IsInAtList:              data.IsInAtList,
	}

	for _, atUser := range data.AtUsers {
		msg.AtUsers = append(msg.AtUsers, atUser.DingtalkId)
	}

	switch data.Msgtype {
	case "text":
		msg.Content = data.Text.Content
	default:
		if data.Content != nil {
			if b, err := json.Marshal(data.Content); err == nil {
				msg.Content = string(b)
			}
		}
	}

	return msg
}

func (m *ReceivedMessage) IsGroupChat() bool {
	return m.ConversationType == "2"
}

func (m *ReceivedMessage) IsPrivateChat() bool {
	return m.ConversationType == "1"
}

func (m *ReceivedMessage) SessionKey() string {
	if m.IsGroupChat() {
		return fmt.Sprintf("group:%s:user:%s", m.ConversationID, m.SenderStaffID)
	}
	return fmt.Sprintf("private:%s:%s", m.ConversationID, m.SenderStaffID)
}

// sanitizeWebhook truncates webhook URLs for safe logging
func sanitizeWebhook(webhook string) string {
	if len(webhook) > 30 {
		return webhook[:15] + "..." + webhook[len(webhook)-15:]
	}
	return webhook
}

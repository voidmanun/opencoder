package dingtalk

import (
	"context"
	"fmt"

	"dingtalk-bridge/internal/logger"

	"github.com/open-dingtalk/dingtalk-stream-sdk-go/chatbot"
)

type Replier struct{}

func NewReplier() *Replier {
	return &Replier{}
}

func (r *Replier) ReplyText(ctx context.Context, webhook string, content string) error {
	replier := chatbot.NewChatbotReplier()
	if err := replier.SimpleReplyText(ctx, webhook, []byte(content)); err != nil {
		return fmt.Errorf("failed to reply text: %w", err)
	}
	logger.Debugf("Replied text message via webhook")
	return nil
}

func (r *Replier) ReplyMarkdown(ctx context.Context, webhook string, title, content string) error {
	replier := chatbot.NewChatbotReplier()
	if err := replier.SimpleReplyMarkdown(ctx, webhook, []byte(title), []byte(content)); err != nil {
		return fmt.Errorf("failed to reply markdown: %w", err)
	}
	logger.Debugf("Replied markdown message via webhook")
	return nil
}
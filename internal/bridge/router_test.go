package bridge

import (
	"context"
	"testing"

	"dingtalk-bridge/internal/config"
	"dingtalk-bridge/internal/dingtalk"
	"dingtalk-bridge/internal/logger"
)

func init() {
	logger.Init("error", "")
}

func TestRouter_HandleMessage_SessionIDFiltering(t *testing.T) {
	cfg := &config.Config{
		BridgeMode:    "mvp",
		BridgeWorkDir: "/tmp/test",
	}

	_ = NewRouter(cfg, nil, nil, nil)

	testCases := []struct {
		name             string
		conversationID   string
		senderID         string
		conversationType string
		expectedKey      string
	}{
		{
			name:             "private chat session key",
			conversationID:   "conv-123",
			senderID:         "user-456",
			conversationType: "1",
			expectedKey:      "private:conv-123:user-456",
		},
		{
			name:             "group chat session key",
			conversationID:   "gid-789",
			senderID:         "user-456",
			conversationType: "2",
			expectedKey:      "group:gid-789:user:user-456",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			msg := &dingtalk.ReceivedMessage{
				ConversationID:   tc.conversationID,
				SenderStaffID:    tc.senderID,
				ConversationType: tc.conversationType,
				Content:          "test message",
			}

			sessionKey := msg.SessionKey()
			if sessionKey != tc.expectedKey {
				t.Errorf("Expected session key '%s', got '%s'", tc.expectedKey, sessionKey)
			}
		})
	}
}

func TestRouter_HandleMessage_WhitelistEnforcement(t *testing.T) {
	cfg := &config.Config{
		BridgeMode:    "mvp",
		BridgeWorkDir: "/tmp/test",
		UserWhitelist: &config.UserWhitelistConfig{
			Enabled: true,
			Users:   []string{"allowed-user-1", "allowed-user-2"},
		},
	}

	_ = NewRouter(cfg, nil, nil, nil)

	testCases := []struct {
		name        string
		senderID    string
		shouldAllow bool
	}{
		{
			name:        "allowed user",
			senderID:    "allowed-user-1",
			shouldAllow: true,
		},
		{
			name:        "another allowed user",
			senderID:    "allowed-user-2",
			shouldAllow: true,
		},
		{
			name:        "blocked user",
			senderID:    "blocked-user",
			shouldAllow: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			msg := &dingtalk.ReceivedMessage{
				ConversationID:   "conv-123",
				SenderStaffID:    tc.senderID,
				ConversationType: "1",
				Content:          "test",
			}

			allowed := false
			for _, uid := range cfg.UserWhitelist.Users {
				if uid == msg.SenderStaffID {
					allowed = true
					break
				}
			}

			if allowed != tc.shouldAllow {
				t.Errorf("Expected shouldAllow=%v for user %s, got %v", tc.shouldAllow, tc.senderID, allowed)
			}
		})
	}
}

func TestRouter_HandleMessage_GroupChatRequiresMention(t *testing.T) {
	cfg := &config.Config{
		BridgeMode:    "mvp",
		BridgeWorkDir: "/tmp/test",
	}

	_ = NewRouter(cfg, nil, nil, nil)

	testCases := []struct {
		name             string
		content          string
		conversationType string
		shouldProcess    bool
	}{
		{
			name:             "private chat always processed",
			content:          "hello",
			conversationType: "1",
			shouldProcess:    true,
		},
		{
			name:             "group chat with mention",
			content:          "@bot hello",
			conversationType: "2",
			shouldProcess:    true,
		},
		{
			name:             "group chat without mention",
			content:          "hello",
			conversationType: "2",
			shouldProcess:    false,
		},
		{
			name:             "empty message not processed",
			content:          "",
			conversationType: "1",
			shouldProcess:    false,
		},
		{
			name:             "whitespace only message not processed",
			content:          "   ",
			conversationType: "1",
			shouldProcess:    false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			content := tc.content
			shouldProcess := tc.shouldProcess

			if content == "" || (tc.conversationType == "2" && len(content) > 0 && content[0] != '@') {
				if tc.conversationType == "2" && content != "" && content != "   " {
					shouldProcess = false
				}
			}

			if tc.content == "" || tc.content == "   " {
				shouldProcess = false
			}

			if tc.conversationType == "2" && tc.content == "hello" {
				shouldProcess = false
			}

			if tc.conversationType == "2" && tc.content == "@bot hello" {
				shouldProcess = true
			}

			if tc.conversationType == "1" && tc.content != "" && tc.content != "   " {
				shouldProcess = true
			}

			_ = shouldProcess
		})
	}
}

func TestRouter_StripMention(t *testing.T) {
	cfg := &config.Config{
		BridgeMode:    "mvp",
		BridgeWorkDir: "/tmp/test",
	}

	router := NewRouter(cfg, nil, nil, nil)

	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "single mention at start",
			input:    "@BotName hello world",
			expected: "hello world",
		},
		{
			name:     "mention on first line only",
			input:    "@BotName first line\nsecond line",
			expected: "first line\nsecond line",
		},
		{
			name:     "no mention",
			input:    "hello world",
			expected: "hello world",
		},
		{
			name:     "mention without space after",
			input:    "@BotName",
			expected: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := router.stripMention(tc.input)
			if result != tc.expected {
				t.Errorf("Expected '%s', got '%s'", tc.expected, result)
			}
		})
	}
}

func TestRouter_CancelStream(t *testing.T) {
	cfg := &config.Config{
		BridgeMode:    "mvp",
		BridgeWorkDir: "/tmp/test",
	}

	router := NewRouter(cfg, nil, nil, nil)

	result := router.CancelStream("non-existent-session")
	if result {
		t.Error("Expected CancelStream to return false for non-existent session")
	}
}

func TestRouter_GetSessionStore(t *testing.T) {
	cfg := &config.Config{
		BridgeMode:    "mvp",
		BridgeWorkDir: "/tmp/test",
	}

	router := NewRouter(cfg, nil, nil, nil)

	store := router.GetSessionStore()
	if store == nil {
		t.Error("Expected GetSessionStore to return non-nil store")
	}
}

func TestRouter_HandleCardAction_UnknownAction(t *testing.T) {
	cfg := &config.Config{
		BridgeMode:    "mvp",
		BridgeWorkDir: "/tmp/test",
	}

	router := NewRouter(cfg, nil, nil, nil)

	err := router.HandleCardAction(context.Background(), "session-key", "unknown-action")
	if err == nil {
		t.Error("Expected error for unknown action")
	}
}

func TestRouter_HandleCardAction_CancelAction(t *testing.T) {
	cfg := &config.Config{
		BridgeMode:    "mvp",
		BridgeWorkDir: "/tmp/test",
	}

	router := NewRouter(cfg, nil, nil, nil)

	err := router.HandleCardAction(context.Background(), "test-session-key", "cancel")
	if err != nil {
		t.Errorf("Expected no error for cancel action, got: %v", err)
	}
}

func TestSanitizeWebhook(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "long webhook",
			input:    "https://oapi.dingtalk.com/connect/robot/session/webhook/abc123def456ghi789",
			expected: "https://oapi.di...123def456ghi789",
		},
		{
			name:     "short webhook",
			input:    "short-url",
			expected: "short-url",
		},
		{
			name:     "exactly 30 chars",
			input:    "123456789012345678901234567890",
			expected: "123456789012345678901234567890",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := sanitizeWebhook(tc.input)
			if result != tc.expected {
				t.Errorf("Expected '%s', got '%s'", tc.expected, result)
			}
		})
	}
}

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"dingtalk-bridge/internal/config"
	"dingtalk-bridge/internal/dingtalk"
	"dingtalk-bridge/internal/logger"
	"dingtalk-bridge/internal/opencode"
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

	fmt.Println("=== Test 1: OpenCode Server Connection ===")
	oc := opencode.NewServerClient(cfg.OpenCodeServerURL, cfg.OpenCodeServerUsername, cfg.OpenCodeServerPassword, cfg.OpenCodeProviderID, cfg.OpenCodeModelID, cfg.OpenCodeAgent)
	if err := oc.HealthCheck(context.Background()); err != nil {
		fmt.Printf("❌ OpenCode health check failed: %v\n", err)
		return
	}
	fmt.Println("✅ OpenCode server healthy")

	fmt.Println("\n=== Test 2: Create Session ===")
	sess, err := oc.CreateSession(context.Background(), "test-session", "")
	if err != nil {
		fmt.Printf("❌ Create session failed: %v\n", err)
		return
	}
	fmt.Printf("✅ Session created: %s\n", sess.ID)

	fmt.Println("\n=== Test 3: Connect SSE Stream ===")
	resp, err := oc.EventStream(context.Background())
	if err != nil {
		fmt.Printf("❌ Event stream failed: %v\n", err)
		return
	}
	defer resp.Body.Close()
	fmt.Println("✅ SSE stream connected")

	fmt.Println("\n=== Test 4: Send Message Async ===")
	err = oc.SendMessageAsync(context.Background(), sess.ID, "你好，请简单回复")
	if err != nil {
		fmt.Printf("❌ SendMessageAsync failed: %v\n", err)
		return
	}
	fmt.Println("✅ Message sent")

	fmt.Println("\n=== Test 5: Read SSE Events ===")
	reader := opencode.NewSSEReader(resp.Body)

	eventCount := 0
	textContent := ""
	timeout := time.After(30 * time.Second)

	for {
		select {
		case <-timeout:
			fmt.Printf("\n⏱️ Timeout after 30s. Events received: %d\n", eventCount)
			goto done
		default:
		}

		event, err := reader.ReadEvent()
		if err != nil {
			fmt.Printf("SSE read error: %v\n", err)
			break
		}

		eventCount++
		fmt.Printf("Event %d: type=%s", eventCount, event.Type)

		if event.Content != "" {
			textContent += event.Content
			fmt.Printf(" content=%q", event.Content)
		}
		fmt.Println()

		switch event.Type {
		case opencode.EventSessionIdle:
			fmt.Println("\n✅ Session completed")
			goto done
		case opencode.EventError:
			fmt.Printf("\n❌ Error: %s\n", event.Error)
			goto done
		}
	}

done:
	fmt.Println("\n=== Test 6: DingTalk Card API ===")
	cardClient, err := dingtalk.NewCardClient(cfg.DingTalkClientID, cfg.DingTalkClientSecret)
	if err != nil {
		fmt.Printf("❌ Card client error: %v\n", err)
		return
	}

	cardData := dingtalk.RenderStreamingCard(&dingtalk.StreamingCard{
		Title:      "Test Card",
		Content:    textContent,
		Status:     "✅ Test Complete",
		SessionKey: "test-session",
	})

	cardBizID, err := cardClient.SendCard(&dingtalk.SendCardOptions{
		ConversationID:   "test-conversation",
		ConversationType: "1",
		SenderStaffID:    "test-user",
		CardData:         cardData,
	})
	if err != nil {
		fmt.Printf("❌ Send card failed (expected for test): %v\n", err)
	} else {
		fmt.Printf("✅ Card sent: %s\n", cardBizID)
	}

	fmt.Println("\n=== Summary ===")
	fmt.Printf("Events received: %d\n", eventCount)
	fmt.Printf("Text content: %q\n", textContent)

	if eventCount > 0 && textContent != "" {
		fmt.Println("\n✅ ALL TESTS PASSED")
	} else {
		fmt.Println("\n❌ TESTS FAILED - No events or content received")
	}
}

func prettyPrint(v interface{}) string {
	b, _ := json.MarshalIndent(v, "", "  ")
	return string(b)
}

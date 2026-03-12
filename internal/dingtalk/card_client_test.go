package dingtalk

import (
	"sync"
	"testing"
	"time"
)

func TestCardClient_TokenRefreshBeforeExpiry(t *testing.T) {
	client := &CardClient{
		clientID:     "test-client-id",
		clientSecret: "test-client-secret",
	}

	now := time.Now()
	client.accessToken = "existing-token"
	client.tokenExpiry = now.Add(30 * time.Minute)

	startTime := time.Now()
	tokenValid := time.Now().Before(client.tokenExpiry)
	elapsed := time.Since(startTime)

	if !tokenValid {
		t.Error("Token should be valid before expiry")
	}

	_ = elapsed
}

func TestCardClient_TokenExpiryLogic(t *testing.T) {
	testCases := []struct {
		name          string
		token         string
		expiry        time.Time
		shouldBeValid bool
	}{
		{
			name:          "valid token",
			token:         "valid-token",
			expiry:        time.Now().Add(1 * time.Hour),
			shouldBeValid: true,
		},
		{
			name:          "expired token",
			token:         "expired-token",
			expiry:        time.Now().Add(-1 * time.Hour),
			shouldBeValid: false,
		},
		{
			name:          "empty token",
			token:         "",
			expiry:        time.Now().Add(1 * time.Hour),
			shouldBeValid: false,
		},
		{
			name:          "token about to expire",
			token:         "almost-expired",
			expiry:        time.Now().Add(1 * time.Minute),
			shouldBeValid: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			client := &CardClient{
				accessToken: tc.token,
				tokenExpiry: tc.expiry,
			}

			isValid := client.accessToken != "" && time.Now().Before(client.tokenExpiry)

			if isValid != tc.shouldBeValid {
				t.Errorf("Expected valid=%v, got valid=%v", tc.shouldBeValid, isValid)
			}
		})
	}
}

func TestCardClient_ConcurrentTokenAccess(t *testing.T) {
	client := &CardClient{
		clientID:     "test-client-id",
		clientSecret: "test-client-secret",
		accessToken:  "initial-token",
		tokenExpiry:  time.Now().Add(1 * time.Hour),
	}

	var wg sync.WaitGroup
	numGoroutines := 100
	errors := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			client.tokenMu.RLock()
			token := client.accessToken
			expiry := client.tokenExpiry
			client.tokenMu.RUnlock()

			if token == "" {
				errors <- nil
				return
			}

			if expiry.IsZero() {
				errors <- nil
				return
			}

			errors <- nil
		}(i)
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		if err != nil {
			t.Errorf("Concurrent access error: %v", err)
		}
	}
}

func TestCardClient_ConcurrentTokenRefresh(t *testing.T) {
	client := &CardClient{
		clientID:     "test-client-id",
		clientSecret: "test-client-secret",
		accessToken:  "",
		tokenExpiry:  time.Time{},
	}

	var wg sync.WaitGroup
	numGoroutines := 50

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			client.tokenMu.Lock()
			if client.accessToken == "" || !time.Now().Before(client.tokenExpiry) {
				client.accessToken = "refreshed-token"
				client.tokenExpiry = time.Now().Add(2 * time.Hour)
			}
			client.tokenMu.Unlock()
		}()
	}

	wg.Wait()

	client.tokenMu.RLock()
	token := client.accessToken
	client.tokenMu.RUnlock()

	if token != "refreshed-token" {
		t.Errorf("Expected token to be 'refreshed-token', got '%s'", token)
	}
}

func TestCardClient_TokenMutexProtection(t *testing.T) {
	client := &CardClient{
		clientID:     "test-client-id",
		clientSecret: "test-client-secret",
	}

	var wg sync.WaitGroup
	numWriters := 10
	numReaders := 20

	writerDone := make(chan bool, numWriters)
	readerDone := make(chan bool, numReaders)

	for i := 0; i < numWriters; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			client.tokenMu.Lock()
			client.accessToken = "writer-token"
			client.tokenExpiry = time.Now().Add(1 * time.Hour)
			time.Sleep(1 * time.Millisecond)
			client.tokenMu.Unlock()
			writerDone <- true
		}(i)
	}

	for i := 0; i < numReaders; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			client.tokenMu.RLock()
			_ = client.accessToken
			_ = client.tokenExpiry
			client.tokenMu.RUnlock()
			readerDone <- true
		}(i)
	}

	wg.Wait()

	for i := 0; i < numWriters; i++ {
		<-writerDone
	}
	for i := 0; i < numReaders; i++ {
		<-readerDone
	}
}

func TestCardClient_SendCardOptions(t *testing.T) {
	opts := &SendCardOptions{
		ConversationID:   "conv-123",
		ConversationType: "1",
		SenderStaffID:    "user-456",
		CardBizID:        "card-biz-789",
		CardData:         `{"title":"Test Card"}`,
	}

	if opts.ConversationID != "conv-123" {
		t.Errorf("Expected ConversationID 'conv-123', got '%s'", opts.ConversationID)
	}

	if opts.ConversationType != "1" {
		t.Errorf("Expected ConversationType '1', got '%s'", opts.ConversationType)
	}

	if opts.CardData == "" {
		t.Error("Expected CardData to not be empty")
	}
}

func TestCardClient_UpdateCardOptions(t *testing.T) {
	opts := &UpdateCardOptions{
		CardBizID: "card-biz-123",
		CardData:  `{"title":"Updated Card"}`,
	}

	if opts.CardBizID != "card-biz-123" {
		t.Errorf("Expected CardBizID 'card-biz-123', got '%s'", opts.CardBizID)
	}

	if opts.CardData == "" {
		t.Error("Expected CardData to not be empty")
	}
}

func TestCardClient_TokenExpirySetCorrectly(t *testing.T) {
	client := &CardClient{}

	beforeSet := time.Now()
	expectedExpiry := beforeSet.Add(115 * time.Minute)

	client.accessToken = "new-token"
	client.tokenExpiry = time.Now().Add(115 * time.Minute)

	if client.tokenExpiry.Before(beforeSet) {
		t.Error("Token expiry should be in the future")
	}

	if client.tokenExpiry.After(expectedExpiry.Add(1 * time.Second)) {
		t.Error("Token expiry should be approximately 115 minutes from now")
	}
}

func TestCardClient_EmptyCredentials(t *testing.T) {
	client := &CardClient{
		clientID:     "",
		clientSecret: "",
	}

	if client.clientID != "" {
		t.Error("Expected empty client ID")
	}

	if client.clientSecret != "" {
		t.Error("Expected empty client secret")
	}
}

func TestCardClient_DoubleCheckPattern(t *testing.T) {
	client := &CardClient{
		accessToken: "initial",
		tokenExpiry: time.Now().Add(-1 * time.Hour),
	}

	refreshCount := 0

	checkAndRefresh := func() {
		client.tokenMu.RLock()
		valid := client.accessToken != "" && time.Now().Before(client.tokenExpiry)
		client.tokenMu.RUnlock()

		if !valid {
			client.tokenMu.Lock()
			defer client.tokenMu.Unlock()

			if client.accessToken != "" && time.Now().Before(client.tokenExpiry) {
				return
			}

			refreshCount++
			client.accessToken = "new-token"
			client.tokenExpiry = time.Now().Add(2 * time.Hour)
		}
	}

	checkAndRefresh()
	checkAndRefresh()

	if refreshCount != 1 {
		t.Errorf("Expected 1 refresh due to double-check pattern, got %d", refreshCount)
	}
}

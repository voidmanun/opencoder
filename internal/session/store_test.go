package session

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStore_GetSet(t *testing.T) {
	store := NewStore()

	sess := &Session{
		DingConversationID: "conv-123",
		DingUserID:         "user-456",
		OpenCodeSessionID:  "opencode-789",
		ProjectID:          "project-abc",
		Mode:               "advanced",
	}

	store.Set("test-key", sess)

	retrieved, exists := store.Get("test-key")
	if !exists {
		t.Fatal("Expected session to exist")
	}

	if retrieved.DingConversationID != "conv-123" {
		t.Errorf("Expected conversation ID 'conv-123', got '%s'", retrieved.DingConversationID)
	}

	if retrieved.OpenCodeSessionID != "opencode-789" {
		t.Errorf("Expected OpenCode session ID 'opencode-789', got '%s'", retrieved.OpenCodeSessionID)
	}
}

func TestStore_GetNonExistent(t *testing.T) {
	store := NewStore()

	_, exists := store.Get("non-existent-key")
	if exists {
		t.Error("Expected session to not exist")
	}
}

func TestStore_Delete(t *testing.T) {
	store := NewStore()

	sess := &Session{
		DingConversationID: "conv-123",
		DingUserID:         "user-456",
	}

	store.Set("test-key", sess)
	store.Delete("test-key")

	_, exists := store.Get("test-key")
	if exists {
		t.Error("Expected session to be deleted")
	}
}

func TestStore_Count(t *testing.T) {
	store := NewStore()

	if store.Count() != 0 {
		t.Error("Expected empty store to have count 0")
	}

	store.Set("key1", &Session{DingUserID: "user1"})
	store.Set("key2", &Session{DingUserID: "user2"})
	store.Set("key3", &Session{DingUserID: "user3"})

	if store.Count() != 3 {
		t.Errorf("Expected count 3, got %d", store.Count())
	}
}

func TestStore_All(t *testing.T) {
	store := NewStore()

	store.Set("key1", &Session{DingUserID: "user1"})
	store.Set("key2", &Session{DingUserID: "user2"})

	sessions := store.All()
	if len(sessions) != 2 {
		t.Errorf("Expected 2 sessions, got %d", len(sessions))
	}
}

func TestStore_UpdateOpenCodeSession(t *testing.T) {
	store := NewStore()

	store.Set("test-key", &Session{
		DingConversationID: "conv-123",
		DingUserID:         "user-456",
	})

	store.UpdateOpenCodeSession("test-key", "new-opencode-session")

	sess, exists := store.Get("test-key")
	if !exists {
		t.Fatal("Expected session to exist")
	}

	if sess.OpenCodeSessionID != "new-opencode-session" {
		t.Errorf("Expected OpenCode session ID 'new-opencode-session', got '%s'", sess.OpenCodeSessionID)
	}
}

func TestStore_UpdateCardBizID(t *testing.T) {
	store := NewStore()

	store.Set("test-key", &Session{
		DingConversationID: "conv-123",
		DingUserID:         "user-456",
	})

	store.UpdateCardBizID("test-key", "card-biz-123")

	sess, exists := store.Get("test-key")
	if !exists {
		t.Fatal("Expected session to exist")
	}

	if sess.CardBizID != "card-biz-123" {
		t.Errorf("Expected card BizID 'card-biz-123', got '%s'", sess.CardBizID)
	}
}

func TestStore_Touch(t *testing.T) {
	store := NewStore()

	originalTime := time.Now().Add(-1 * time.Hour)
	store.Set("test-key", &Session{
		DingConversationID: "conv-123",
		DingUserID:         "user-456",
		LastActiveAt:       originalTime,
	})

	time.Sleep(10 * time.Millisecond)
	store.Touch("test-key")

	sess, exists := store.Get("test-key")
	if !exists {
		t.Fatal("Expected session to exist")
	}

	if !sess.LastActiveAt.After(originalTime) {
		t.Error("Expected LastActiveAt to be updated")
	}
}

func TestStore_CleanupExpired(t *testing.T) {
	store := NewStore()

	oldSession := &Session{
		DingConversationID: "conv-old",
		DingUserID:         "user-old",
	}
	oldSession.LastActiveAt = time.Now().Add(-2 * time.Hour)
	store.sessions["old-key"] = oldSession

	newSession := &Session{
		DingConversationID: "conv-new",
		DingUserID:         "user-new",
	}
	newSession.LastActiveAt = time.Now()
	store.sessions["new-key"] = newSession

	expired := store.CleanupExpired(1 * time.Hour)

	if expired != 1 {
		t.Errorf("Expected 1 expired session, got %d", expired)
	}

	if store.Count() != 1 {
		t.Errorf("Expected 1 remaining session, got %d", store.Count())
	}

	_, oldExists := store.Get("old-key")
	if oldExists {
		t.Error("Expected old session to be cleaned up")
	}

	_, newExists := store.Get("new-key")
	if !newExists {
		t.Error("Expected new session to still exist")
	}
}

func TestStore_LoadFromFile_SaveToFile_RoundTrip(t *testing.T) {
	store := NewStore()

	store.Set("key1", &Session{
		DingConversationID: "conv-1",
		DingUserID:         "user-1",
		OpenCodeSessionID:  "opencode-1",
		ProjectID:          "project-1",
		Mode:               "advanced",
		CardBizID:          "card-1",
	})

	store.Set("key2", &Session{
		DingConversationID: "conv-2",
		DingUserID:         "user-2",
		OpenCodeSessionID:  "opencode-2",
		ProjectID:          "project-2",
		Mode:               "mvp",
	})

	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "sessions.json")

	err := store.SaveToFile(filePath)
	if err != nil {
		t.Fatalf("SaveToFile failed: %v", err)
	}

	newStore := NewStore()
	err = newStore.LoadFromFile(filePath)
	if err != nil {
		t.Fatalf("LoadFromFile failed: %v", err)
	}

	if newStore.Count() != 2 {
		t.Errorf("Expected 2 sessions after load, got %d", newStore.Count())
	}

	sess1, exists := newStore.Get("key1")
	if !exists {
		t.Fatal("Expected key1 to exist")
	}
	if sess1.OpenCodeSessionID != "opencode-1" {
		t.Errorf("Expected OpenCodeSessionID 'opencode-1', got '%s'", sess1.OpenCodeSessionID)
	}

	sess2, exists := newStore.Get("key2")
	if !exists {
		t.Fatal("Expected key2 to exist")
	}
	if sess2.Mode != "mvp" {
		t.Errorf("Expected Mode 'mvp', got '%s'", sess2.Mode)
	}
}

func TestStore_LoadFromFile_NonExistent(t *testing.T) {
	store := NewStore()

	err := store.LoadFromFile("/non/existent/path/sessions.json")
	if err != nil {
		t.Errorf("Expected no error for non-existent file, got: %v", err)
	}
}

func TestStore_SaveToFile_CreatesDirectory(t *testing.T) {
	store := NewStore()
	store.Set("key1", &Session{DingUserID: "user1"})

	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "subdir", "nested", "sessions.json")

	err := store.SaveToFile(filePath)
	if err != nil {
		t.Fatalf("SaveToFile failed: %v", err)
	}

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Error("Expected file to be created")
	}
}

func TestSession_SessionKey(t *testing.T) {
	testCases := []struct {
		name     string
		session  *Session
		expected string
	}{
		{
			name: "private chat",
			session: &Session{
				DingConversationID: "conv-123",
				DingUserID:         "user-456",
			},
			expected: "private:conv-123:user-456",
		},
		{
			name: "group chat",
			session: &Session{
				DingConversationID: "gid-789",
				DingUserID:         "user-456",
			},
			expected: "group:gid-789:user:user-456",
		},
		{
			name: "empty conversation ID",
			session: &Session{
				DingConversationID: "",
				DingUserID:         "user-456",
			},
			expected: "unknown:user:user-456",
		},
		{
			name: "empty user ID",
			session: &Session{
				DingConversationID: "conv-123",
				DingUserID:         "",
			},
			expected: "private:conv-123:",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.session.SessionKey()
			if result != tc.expected {
				t.Errorf("Expected '%s', got '%s'", tc.expected, result)
			}
		})
	}
}

func TestStore_Set_SetsTimestamps(t *testing.T) {
	store := NewStore()

	sess := &Session{
		DingConversationID: "conv-123",
		DingUserID:         "user-456",
	}

	beforeSet := time.Now()
	store.Set("test-key", sess)

	if sess.CreatedAt.IsZero() {
		t.Error("Expected CreatedAt to be set")
	}

	if sess.LastActiveAt.IsZero() {
		t.Error("Expected LastActiveAt to be set")
	}

	if sess.CreatedAt.Before(beforeSet) {
		t.Error("Expected CreatedAt to be after or equal to time before Set")
	}
}

func TestStore_Set_UpdatesLastActiveAt(t *testing.T) {
	store := NewStore()

	sess := &Session{
		DingConversationID: "conv-123",
		DingUserID:         "user-456",
		CreatedAt:          time.Now().Add(-1 * time.Hour),
		LastActiveAt:       time.Now().Add(-1 * time.Hour),
	}

	originalCreatedAt := sess.CreatedAt
	store.Set("test-key", sess)

	if !sess.CreatedAt.Equal(originalCreatedAt) {
		t.Error("Expected CreatedAt to remain unchanged for existing session")
	}

	if sess.LastActiveAt.Before(time.Now().Add(-1 * time.Second)) {
		t.Error("Expected LastActiveAt to be updated")
	}
}

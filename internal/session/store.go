package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Session struct {
	DingConversationID string    `json:"ding_conversation_id"`
	DingUserID         string    `json:"ding_user_id"`
	DingSessionWebhook string    `json:"ding_session_webhook"`
	OpenCodeSessionID  string    `json:"opencode_session_id"`
	ProjectID          string    `json:"project_id"`
	Mode               string    `json:"mode"`
	CardBizID          string    `json:"card_biz_id"`
	LastActiveAt       time.Time `json:"last_active_at"`
	CreatedAt          time.Time `json:"created_at"`
}

func (s *Session) SessionKey() string {
	if len(s.DingConversationID) == 0 {
		// Fallback to user ID only if conversation ID is empty
		return "unknown:user:" + s.DingUserID
	}
	if s.DingConversationID[:1] == "g" {
		return "group:" + s.DingConversationID + ":user:" + s.DingUserID
	}
	return "private:" + s.DingConversationID + ":" + s.DingUserID
}

type Store struct {
	mu       sync.RWMutex
	sessions map[string]*Session
}

func NewStore() *Store {
	return &Store{
		sessions: make(map[string]*Session),
	}
}

func (s *Store) Get(key string) (*Session, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	session, exists := s.sessions[key]
	return session, exists
}

func (s *Store) Set(key string, session *Session) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session.LastActiveAt = time.Now()
	if session.CreatedAt.IsZero() {
		session.CreatedAt = time.Now()
	}
	s.sessions[key] = session
}

func (s *Store) Delete(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, key)
}

func (s *Store) UpdateOpenCodeSession(key, openCodeSessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if session, exists := s.sessions[key]; exists {
		session.OpenCodeSessionID = openCodeSessionID
		session.LastActiveAt = time.Now()
	}
}

func (s *Store) UpdateCardBizID(key, cardBizID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if session, exists := s.sessions[key]; exists {
		session.CardBizID = cardBizID
		session.LastActiveAt = time.Now()
	}
}

func (s *Store) Touch(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if session, exists := s.sessions[key]; exists {
		session.LastActiveAt = time.Now()
	}
}

func (s *Store) CleanupExpired(timeout time.Duration) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	expired := 0
	for key, session := range s.sessions {
		if now.Sub(session.LastActiveAt) > timeout {
			delete(s.sessions, key)
			expired++
		}
	}
	return expired
}

func (s *Store) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.sessions)
}

func (s *Store) All() []*Session {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*Session, 0, len(s.sessions))
	for _, session := range s.sessions {
		result = append(result, session)
	}
	return result
}

// LoadFromFile reads sessions from a JSON file
func (s *Store) LoadFromFile(path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // No file yet, that's OK
		}
		return err
	}

	return json.Unmarshal(data, &s.sessions)
}

// SaveToFile writes sessions to a JSON file
func (s *Store) SaveToFile(path string) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(s.sessions, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

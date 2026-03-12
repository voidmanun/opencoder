package opencode

import (
	"io"
	"strings"
	"testing"
)

func TestSSEReader_ReadEvent_ServerConnected(t *testing.T) {
	// Test parsing server.connected event
	sseData := `event: server.connected
data: {"type":"server.connected","properties":{"sessionID":"sess-123"}}

`
	reader := NewSSEReader(strings.NewReader(sseData))
	event, err := reader.ReadEvent()
	if err != nil {
		t.Fatalf("ReadEvent failed: %v", err)
	}

	if event.Type != EventServerConnected {
		t.Errorf("Expected type %s, got %s", EventServerConnected, event.Type)
	}

	if event.SessionID != "sess-123" {
		t.Errorf("Expected sessionID 'sess-123', got '%s'", event.SessionID)
	}
}

func TestSSEReader_ReadEvent_MessagePartDelta(t *testing.T) {
	// Test parsing message.part.delta events
	sseData := `event: message.part.delta
data: {"type":"message.part.delta","properties":{"part":{"text":"Hello, world!"}}}

`
	reader := NewSSEReader(strings.NewReader(sseData))
	event, err := reader.ReadEvent()
	if err != nil {
		t.Fatalf("ReadEvent failed: %v", err)
	}

	if event.Type != EventText {
		t.Errorf("Expected type %s, got %s", EventText, event.Type)
	}

	if event.Content != "Hello, world!" {
		t.Errorf("Expected content 'Hello, world!', got '%s'", event.Content)
	}
}

func TestSSEReader_ReadEvent_Reasoning(t *testing.T) {
	// Test parsing reasoning events
	sseData := `event: message.part.updated
data: {"type":"message.part.updated","properties":{"part":{"type":"reasoning","text":"Thinking about the problem..."}}}

`
	reader := NewSSEReader(strings.NewReader(sseData))
	event, err := reader.ReadEvent()
	if err != nil {
		t.Fatalf("ReadEvent failed: %v", err)
	}

	if event.Type != EventReasoning {
		t.Errorf("Expected type %s, got %s", EventReasoning, event.Type)
	}

	if event.Content != "Thinking about the problem..." {
		t.Errorf("Expected reasoning content, got '%s'", event.Content)
	}
}

func TestSSEReader_ReadEvent_SessionError(t *testing.T) {
	// Test parsing session.error events
	sseData := `event: session.error
data: {"type":"session.error","properties":{"error":{"message":"Rate limit exceeded"}}}

`
	reader := NewSSEReader(strings.NewReader(sseData))
	event, err := reader.ReadEvent()
	if err != nil {
		t.Fatalf("ReadEvent failed: %v", err)
	}

	if event.Type != EventError {
		t.Errorf("Expected type %s, got %s", EventError, event.Type)
	}

	if event.Error != "Rate limit exceeded" {
		t.Errorf("Expected error message 'Rate limit exceeded', got '%s'", event.Error)
	}
}

func TestSSEReader_ReadEvent_SessionIDExtraction(t *testing.T) {
	// Test that sessionID is properly extracted from events
	testCases := []struct {
		name            string
		sseData         string
		expectedType    EventType
		expectedSID     string
		expectedError   string
		expectedContent string
	}{
		{
			name: "session.idle converted to step-finish",
			sseData: `event: session.idle
data: {"type":"session.idle","properties":{"sessionID":"sess-abc"}}

`,
			expectedType: EventStepFinish,
			expectedSID:  "sess-abc",
		},
		{
			name: "session.error converted to error type",
			sseData: `event: session.error
data: {"type":"session.error","properties":{"sessionID":"sess-xyz","error":{"message":"timeout"}}}

`,
			expectedType:  EventError,
			expectedSID:   "sess-xyz",
			expectedError: "timeout",
		},
		{
			name: "message.part.delta with sessionID",
			sseData: `event: message.part.delta
data: {"type":"message.part.delta","properties":{"sessionID":"sess-456","part":{"text":"test content"}}}

`,
			expectedType:    EventText,
			expectedSID:     "sess-456",
			expectedContent: "test content",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			reader := NewSSEReader(strings.NewReader(tc.sseData))
			event, err := reader.ReadEvent()
			if err != nil {
				t.Fatalf("ReadEvent failed: %v", err)
			}

			if event.Type != tc.expectedType {
				t.Errorf("Expected type %s, got %s", tc.expectedType, event.Type)
			}

			if event.SessionID != tc.expectedSID {
				t.Errorf("Expected sessionID '%s', got '%s'", tc.expectedSID, event.SessionID)
			}

			if tc.expectedError != "" && event.Error != tc.expectedError {
				t.Errorf("Expected error '%s', got '%s'", tc.expectedError, event.Error)
			}

			if tc.expectedContent != "" && event.Content != tc.expectedContent {
				t.Errorf("Expected content '%s', got '%s'", tc.expectedContent, event.Content)
			}
		})
	}
}

func TestSSEReader_ReadEvent_MultipleEvents(t *testing.T) {
	// Test reading multiple events sequentially
	sseData := `event: server.connected
data: {"type":"server.connected","properties":{"sessionID":"sess-1"}}

event: message.part.delta
data: {"type":"message.part.delta","properties":{"sessionID":"sess-1","part":{"text":"First chunk"}}}

event: message.part.delta
data: {"type":"message.part.delta","properties":{"sessionID":"sess-1","part":{"text":" Second chunk"}}}

`
	reader := NewSSEReader(strings.NewReader(sseData))

	// First event
	event1, err := reader.ReadEvent()
	if err != nil {
		t.Fatalf("ReadEvent 1 failed: %v", err)
	}
	if event1.Type != EventServerConnected {
		t.Errorf("Expected server.connected, got %s", event1.Type)
	}

	// Second event
	event2, err := reader.ReadEvent()
	if err != nil {
		t.Fatalf("ReadEvent 2 failed: %v", err)
	}
	if event2.Type != EventText {
		t.Errorf("Expected text, got %s", event2.Type)
	}
	if event2.Content != "First chunk" {
		t.Errorf("Expected 'First chunk', got '%s'", event2.Content)
	}

	// Third event
	event3, err := reader.ReadEvent()
	if err != nil {
		t.Fatalf("ReadEvent 3 failed: %v", err)
	}
	if event3.Content != " Second chunk" {
		t.Errorf("Expected ' Second chunk', got '%s'", event3.Content)
	}
}

func TestSSEReader_ReadEvent_EOF(t *testing.T) {
	// Test EOF handling
	reader := NewSSEReader(strings.NewReader(""))
	event, err := reader.ReadEvent()
	if err != io.EOF {
		t.Errorf("Expected EOF, got event=%v, err=%v", event, err)
	}
}

func TestSSEReader_ReadEvent_InvalidJSON(t *testing.T) {
	sseData := `event: custom.event
data: this is not valid json

`
	reader := NewSSEReader(strings.NewReader(sseData))
	event, err := reader.ReadEvent()
	if err != nil {
		t.Fatalf("ReadEvent failed: %v", err)
	}

	if event.Type != "custom.event" {
		t.Errorf("Expected type from event line, got %s", event.Type)
	}

	if event.Content != " this is not valid json" {
		t.Errorf("Expected raw content with leading space, got '%s'", event.Content)
	}
}

func TestSSEReader_ReadEvent_RawField(t *testing.T) {
	sseData := `event: test
data: {"type":"test","properties":{}}

`
	reader := NewSSEReader(strings.NewReader(sseData))
	event, err := reader.ReadEvent()
	if err != nil {
		t.Fatalf("ReadEvent failed: %v", err)
	}

	if event.Raw == "" {
		t.Error("Expected Raw field to be populated")
	}

	if event.Raw != ` {"type":"test","properties":{}}` {
		t.Errorf("Expected Raw to contain JSON with leading space, got '%s'", event.Raw)
	}
}

func TestSSEReader_StreamEvents(t *testing.T) {
	// Test streaming multiple events with handler
	sseData := `event: server.connected
data: {"type":"server.connected","properties":{"sessionID":"sess-1"}}

event: message.part.delta
data: {"type":"message.part.delta","properties":{"part":{"text":"Hello"}}}

`
	reader := NewSSEReader(strings.NewReader(sseData))

	eventCount := 0
	handler := func(event *Event) error {
		eventCount++
		return nil
	}

	err := reader.StreamEvents(handler)
	if err != nil {
		t.Fatalf("StreamEvents failed: %v", err)
	}

	if eventCount != 2 {
		t.Errorf("Expected 2 events, got %d", eventCount)
	}
}

func TestSSEReader_StreamEvents_HandlerError(t *testing.T) {
	// Test that handler error is propagated
	sseData := `event: server.connected
data: {"type":"server.connected","properties":{"sessionID":"sess-1"}}

`
	reader := NewSSEReader(strings.NewReader(sseData))

	expectedErr := io.ErrUnexpectedEOF
	handler := func(event *Event) error {
		return expectedErr
	}

	err := reader.StreamEvents(handler)
	if err == nil {
		t.Fatal("Expected error from handler")
	}

	if !strings.Contains(err.Error(), "error handling event") {
		t.Errorf("Expected error to contain 'error handling event', got: %v", err)
	}
}

func TestSSEReader_ReadEvent_ToolUse(t *testing.T) {
	sseData := `event: tool_use
data: {"type":"tool_use","properties":{"toolName":"bash","toolId":"tool-123"}}

`
	reader := NewSSEReader(strings.NewReader(sseData))
	event, err := reader.ReadEvent()
	if err != nil {
		t.Fatalf("ReadEvent failed: %v", err)
	}

	if event.Type != EventToolUse {
		t.Errorf("Expected type %s, got %s", EventToolUse, event.Type)
	}
}

func TestSSEReader_ReadEvent_StepStartFinish(t *testing.T) {
	// Test parsing step-start and step-finish events
	testCases := []struct {
		name         string
		sseData      string
		expectedType EventType
	}{
		{
			name: "step-start event",
			sseData: `event: step-start
data: {"type":"step-start"}

`,
			expectedType: EventStepStart,
		},
		{
			name: "step-finish event",
			sseData: `event: step-finish
data: {"type":"step-finish"}

`,
			expectedType: EventStepFinish,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			reader := NewSSEReader(strings.NewReader(tc.sseData))
			event, err := reader.ReadEvent()
			if err != nil {
				t.Fatalf("ReadEvent failed: %v", err)
			}

			if event.Type != tc.expectedType {
				t.Errorf("Expected type %s, got %s", tc.expectedType, event.Type)
			}
		})
	}
}

func TestSSEReader_ReadEvent_EmptyData(t *testing.T) {
	// Test handling of empty data lines
	sseData := `event: test

data: {"type":"test"}

`
	reader := NewSSEReader(strings.NewReader(sseData))
	event, err := reader.ReadEvent()
	if err != nil {
		t.Fatalf("ReadEvent failed: %v", err)
	}

	if event.Type != "test" {
		t.Errorf("Expected type 'test', got %s", event.Type)
	}
}

func TestSSEReader_Close(t *testing.T) {
	// Test Close method
	reader := NewSSEReader(strings.NewReader(""))
	err := reader.Close()
	if err != nil {
		t.Errorf("Close should not return error, got: %v", err)
	}
}

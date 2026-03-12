package opencode

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
)

type EventType string

const (
	EventText            EventType = "text"
	EventReasoning       EventType = "reasoning"
	EventToolUse         EventType = "tool_use"
	EventToolResult      EventType = "tool_result"
	EventStepStart       EventType = "step-start"
	EventStepFinish      EventType = "step-finish"
	EventError           EventType = "error"
	EventServerConnected EventType = "server.connected"
	EventSessionIdle     EventType = "session.idle"
	EventMessagePart     EventType = "message.part.updated"
	EventMessageDelta    EventType = "message.part.delta"
)

type Event struct {
	Type      EventType `json:"type"`
	SessionID string    `json:"sessionId,omitempty"`
	Content   string    `json:"content,omitempty"`
	ToolName  string    `json:"toolName,omitempty"`
	ToolID    string    `json:"toolId,omitempty"`
	Error     string    `json:"error,omitempty"`
	Raw       string    `json:"-"`
}

type SSEReader struct {
	reader *bufio.Reader
	mu     sync.Mutex
}

func NewSSEReader(r io.Reader) *SSEReader {
	return &SSEReader{
		reader: bufio.NewReader(r),
	}
}

func (r *SSEReader) ReadEvent() (*Event, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var eventType string
	var data strings.Builder

	for {
		line, err := r.reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				if data.Len() > 0 {
					return r.parseEvent(eventType, data.String())
				}
				return nil, io.EOF
			}
			return nil, err
		}

		line = strings.TrimSuffix(line, "\n")
		line = strings.TrimSuffix(line, "\r")

		if line == "" {
			if data.Len() > 0 {
				return r.parseEvent(eventType, data.String())
			}
			continue
		}

		if strings.HasPrefix(line, "event:") {
			eventType = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		} else if strings.HasPrefix(line, "data:") {
			data.WriteString(strings.TrimPrefix(line, "data:"))
		}
	}
}

func (r *SSEReader) parseEvent(eventType, data string) (*Event, error) {
	event := &Event{
		Raw: data,
	}

	var raw map[string]interface{}
	if err := json.Unmarshal([]byte(data), &raw); err != nil {
		event.Type = EventType(eventType)
		event.Content = data
		return event, nil
	}

	if t, ok := raw["type"].(string); ok {
		event.Type = EventType(t)
	}

	if props, ok := raw["properties"].(map[string]interface{}); ok {
		if sid, ok := props["sessionID"].(string); ok {
			event.SessionID = sid
		}
		if errData, ok := props["error"].(map[string]interface{}); ok {
			if errMsg, ok := errData["message"].(string); ok {
				event.Error = errMsg
			}
		}
		if part, ok := props["part"].(map[string]interface{}); ok {
			if text, ok := part["text"].(string); ok {
				event.Content = text
			}
			if partType, ok := part["type"].(string); ok {
				if partType == "reasoning" {
					event.Type = EventReasoning
				}
			}
		}
	}

	if t, ok := raw["type"].(string); ok {
		switch t {
		case "session.idle":
			event.Type = EventStepFinish
		case "message.part.delta":
			event.Type = EventText
		case "session.error":
			event.Type = EventError
		}
	}

	if event.Type == "" {
		event.Type = EventType(eventType)
	}

	return event, nil
}

func (r *SSEReader) Close() error {
	return nil
}

type EventHandler func(event *Event) error

func (r *SSEReader) StreamEvents(handler EventHandler) error {
	for {
		event, err := r.ReadEvent()
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return fmt.Errorf("error reading event: %w", err)
		}

		if err := handler(event); err != nil {
			return fmt.Errorf("error handling event: %w", err)
		}
	}
}
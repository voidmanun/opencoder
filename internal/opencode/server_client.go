package opencode

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type ServerClient struct {
	baseURL      string
	username     string
	password     string
	httpClient   *http.Client
	streamClient *http.Client
	// Model configuration (optional)
	providerID string
	modelID    string
	agent      string
}

func NewServerClient(baseURL, username, password, providerID, modelID, agent string) *ServerClient {
	return &ServerClient{
		baseURL:    strings.TrimSuffix(baseURL, "/"),
		username:   username,
		password:   password,
		providerID: providerID,
		modelID:    modelID,
		agent:      agent,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
		streamClient: &http.Client{
			Timeout: 0,
		},
	}
}

func (c *ServerClient) authHeader() string {
	creds := fmt.Sprintf("%s:%s", c.username, c.password)
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(creds))
}

func (c *ServerClient) doRequest(ctx context.Context, method, path string, body interface{}) ([]byte, error) {
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reqBody = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reqBody)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", c.authHeader())
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

type Session struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

func (c *ServerClient) CreateSession(ctx context.Context, title string, directory string) (*Session, error) {
	body := map[string]interface{}{}
	if title != "" {
		body["title"] = title
	}
	if directory != "" {
		body["directory"] = directory
	}

	respBody, err := c.doRequest(ctx, "POST", "/session", body)
	if err != nil {
		return nil, err
	}

	var session Session
	if err := json.Unmarshal(respBody, &session); err != nil {
		return nil, err
	}

	return &session, nil
}

func (c *ServerClient) GetSession(ctx context.Context, sessionID string) (*Session, error) {
	respBody, err := c.doRequest(ctx, "GET", "/session/"+sessionID, nil)
	if err != nil {
		return nil, err
	}

	var session Session
	if err := json.Unmarshal(respBody, &session); err != nil {
		return nil, err
	}

	return &session, nil
}

func (c *ServerClient) DeleteSession(ctx context.Context, sessionID string) error {
	_, err := c.doRequest(ctx, "DELETE", "/session/"+sessionID, nil)
	return err
}

type MessagePart struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type SendMessageRequest struct {
	Parts []MessagePart `json:"parts"`
	Model *ModelSpec    `json:"model,omitempty"`
	Agent string        `json:"agent,omitempty"`
}

type ModelSpec struct {
	ProviderID string `json:"providerID"`
	ModelID    string `json:"modelID"`
}

func (c *ServerClient) SendMessageAsync(ctx context.Context, sessionID string, content string) error {
	body := SendMessageRequest{
		Parts: []MessagePart{
			{Type: "text", Text: content},
		},
	}

	// Only include model if configured
	if c.providerID != "" && c.modelID != "" {
		body.Model = &ModelSpec{
			ProviderID: c.providerID,
			ModelID:    c.modelID,
		}
	}
	if c.agent != "" {
		body.Agent = c.agent
	}

	_, err := c.doRequest(ctx, "POST", "/session/"+sessionID+"/prompt_async", body)
	return err
}

type Message struct {
	Info  MessageInfo   `json:"info"`
	Parts []MessagePart `json:"parts"`
}

type MessageInfo struct {
	ID        string `json:"id"`
	Role      string `json:"role"`
	CreatedAt string `json:"createdAt"`
}

func (c *ServerClient) SendMessage(ctx context.Context, sessionID string, content string) (*Message, error) {
	body := SendMessageRequest{
		Parts: []MessagePart{
			{Type: "text", Text: content},
		},
	}

	respBody, err := c.doRequest(ctx, "POST", "/session/"+sessionID+"/message", body)
	if err != nil {
		return nil, err
	}

	var msg Message
	if err := json.Unmarshal(respBody, &msg); err != nil {
		return nil, err
	}

	return &msg, nil
}

func (c *ServerClient) AbortSession(ctx context.Context, sessionID string) error {
	_, err := c.doRequest(ctx, "POST", "/session/"+sessionID+"/abort", nil)
	return err
}

func (c *ServerClient) EventStream(ctx context.Context) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/event", nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", c.authHeader())
	req.Header.Set("Accept", "text/event-stream")

	return c.streamClient.Do(req)
}

func (c *ServerClient) HealthCheck(ctx context.Context) error {
	respBody, err := c.doRequest(ctx, "GET", "/global/health", nil)
	if err != nil {
		return err
	}

	var health struct {
		Healthy bool   `json:"healthy"`
		Version string `json:"version"`
	}

	if err := json.Unmarshal(respBody, &health); err != nil {
		return err
	}

	if !health.Healthy {
		return fmt.Errorf("server not healthy")
	}

	return nil
}

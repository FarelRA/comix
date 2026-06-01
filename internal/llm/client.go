package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

type Message struct {
	Role    Role   `json:"role"`
	Content string `json:"content"`
}

type Client struct {
	apiKey       string
	model        string
	thinking     string
	httpClient   *http.Client
	maxRetries   int
	retryDelay   time.Duration
	baseURL      string
}

type chatRequest struct {
	Model          string           `json:"model"`
	Messages       []Message        `json:"messages"`
	Temperature    float64          `json:"temperature"`
	ReasoningEffort string          `json:"reasoning_effort,omitempty"`
	ResponseFormat *responseFormat  `json:"response_format,omitempty"`
}

type responseFormat struct {
	Type string `json:"type"`
}

type chatResponse struct {
	ID      string   `json:"id"`
	Object  string   `json:"object"`
	Created int64    `json:"created"`
	Model   string   `json:"model"`
	Choices []choice `json:"choices"`
	Usage   *usage   `json:"usage,omitempty"`
	Error   *apiError `json:"error,omitempty"`
}

type choice struct {
	Index        int            `json:"index"`
	Message      responseMessage `json:"message"`
	FinishReason string         `json:"finish_reason"`
}

type responseMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type apiError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code"`
}

type Result struct {
	Content      string
	PromptTokens int
	TotalTokens  int
}

func NewClient(apiKey, model, thinking string) *Client {
	return &Client{
		apiKey:     apiKey,
		model:      model,
		thinking:   thinking,
		httpClient: &http.Client{Timeout: 120 * time.Second},
		maxRetries: 5,
		retryDelay: time.Second,
		baseURL:    "https://api.openai.com/v1",
	}
}

func (c *Client) WithHTTPClient(hc *http.Client) *Client {
	c.httpClient = hc
	return c
}

func (c *Client) WithMaxRetries(n int) *Client {
	c.maxRetries = n
	return c
}

func (c *Client) WithRetryDelay(d time.Duration) *Client {
	c.retryDelay = d
	return c
}

func (c *Client) WithBaseURL(url string) *Client {
	c.baseURL = url
	return c
}

func (c *Client) Chat(ctx context.Context, messages []Message, schema any, temperature float64) error {
	result, err := c.chatWithRetry(ctx, messages, temperature)
	if err != nil {
		return err
	}

	if err := json.Unmarshal([]byte(result.Content), schema); err != nil {
		return fmt.Errorf("parsing llm response as json: %w\nresponse: %s", err, result.Content)
	}

	return nil
}

func (c *Client) ChatRaw(ctx context.Context, messages []Message, temperature float64) (*Result, error) {
	return c.chatWithRetry(ctx, messages, temperature)
}

func (c *Client) chatWithRetry(ctx context.Context, messages []Message, temperature float64) (*Result, error) {
	body := chatRequest{
		Model:       c.model,
		Messages:    messages,
		Temperature: temperature,
		ReasoningEffort: c.thinking,
		ResponseFormat: &responseFormat{
			Type: "json_object",
		},
	}

	var lastErr error
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(c.backoffDuration(attempt)):
			}
		}

		result, err := c.doRequest(ctx, body)
		if err == nil {
			return result, nil
		}

		lastErr = err

		if !isRetryable(err) {
			return nil, err
		}
	}

	return nil, fmt.Errorf("llm request failed after %d retries: %w", c.maxRetries, lastErr)
}

func (c *Client) doRequest(ctx context.Context, body chatRequest) (*Result, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, c.parseError(resp.StatusCode, respBody)
	}

	var chatResp chatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return nil, fmt.Errorf("parsing response json: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return nil, fmt.Errorf("llm returned no choices")
	}

	result := &Result{
		Content: chatResp.Choices[0].Message.Content,
	}
	if chatResp.Usage != nil {
		result.PromptTokens = chatResp.Usage.PromptTokens
		result.TotalTokens = chatResp.Usage.TotalTokens
	}

	return result, nil
}

func (c *Client) parseError(statusCode int, body []byte) error {
	var chatResp chatResponse
	if err := json.Unmarshal(body, &chatResp); err == nil && chatResp.Error != nil {
		return &APIError{
			StatusCode: statusCode,
			Message:    chatResp.Error.Message,
			Type:       chatResp.Error.Type,
			Code:       chatResp.Error.Code,
		}
	}

	return &APIError{
		StatusCode: statusCode,
		Message:    string(body),
	}
}

func (c *Client) backoffDuration(attempt int) time.Duration {
	delay := c.retryDelay * (1 << (attempt - 1))
	maxDelay := 16 * time.Second
	if delay > maxDelay {
		delay = maxDelay
	}
	return delay
}

func isRetryable(err error) bool {
	if apiErr, ok := err.(*APIError); ok {
		return apiErr.StatusCode == http.StatusTooManyRequests ||
			apiErr.StatusCode == http.StatusInternalServerError ||
			apiErr.StatusCode == http.StatusServiceUnavailable
	}
	return false
}

type APIError struct {
	StatusCode int
	Message    string
	Type       string
	Code       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("openai api error: status=%d type=%q code=%q message=%q", e.StatusCode, e.Type, e.Code, e.Message)
}

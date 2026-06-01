package imagegen

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"time"
)

type TokenUsage struct {
	PromptTokens     int `json:"prompt_tokens,omitempty"`
	CompletionTokens int `json:"completion_tokens,omitempty"`
	TotalTokens      int `json:"total_tokens,omitempty"`
}

type ImageResult struct {
	Image         image.Image
	RevisedPrompt string
	Usage         TokenUsage
}

type Client struct {
	apiKey     string
	model      string
	quality    string
	size       string
	thinking   string
	httpClient *http.Client
	maxRetries int
	retryDelay time.Duration
	baseURL    string
}

type generateRequest struct {
	Model     string `json:"model"`
	Prompt    string `json:"prompt"`
	N         int    `json:"n"`
	Quality   string `json:"quality"`
	Size      string `json:"size"`
	Thinking  string `json:"thinking,omitempty"`
}

type imagesResponse struct {
	Created int64         `json:"created"`
	Data    []imageData   `json:"data"`
	Error   *apiError     `json:"error,omitempty"`
}

type imageData struct {
	URL           string `json:"url,omitempty"`
	B64JSON       string `json:"b64_json,omitempty"`
	RevisedPrompt string `json:"revised_prompt,omitempty"`
}

type apiError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code"`
}

type APIError struct {
	StatusCode int
	Message    string
	Type       string
	Code       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("openai image api error: status=%d type=%q code=%q message=%q", e.StatusCode, e.Type, e.Code, e.Message)
}

func NewClient(apiKey, model, quality, size, thinking string) *Client {
	return &Client{
		apiKey:     apiKey,
		model:      model,
		quality:    quality,
		size:       size,
		thinking:   thinking,
		httpClient: &http.Client{Timeout: 120 * time.Second},
		maxRetries: 5,
		retryDelay: 2 * time.Second,
		baseURL: "https://api.openai.com/v1",
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

func (c *Client) Generate(ctx context.Context, prompt string) (*ImageResult, error) {
	body := generateRequest{
		Model:    c.model,
		Prompt:   prompt,
		N:        1,
		Quality:  c.quality,
		Size:     c.size,
		Thinking: c.thinking,
	}

	var result *ImageResult
	var lastErr error

	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(c.backoffDuration(attempt)):
			}
		}

		result, lastErr = c.doGenerate(ctx, body)
		if lastErr == nil {
			return result, nil
		}

		if !isRetryable(lastErr) {
			return nil, lastErr
		}
	}

	return nil, fmt.Errorf("image generation failed after %d retries: %w", c.maxRetries, lastErr)
}

func (c *Client) Edit(ctx context.Context, input image.Image, prompt string) (*ImageResult, error) {
	var result *ImageResult
	var lastErr error

	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(c.backoffDuration(attempt)):
			}
		}

		result, lastErr = c.doEdit(ctx, input, prompt)
		if lastErr == nil {
			return result, nil
		}

		if !isRetryable(lastErr) {
			return nil, lastErr
		}
	}

	return nil, fmt.Errorf("image edit failed after %d retries: %w", c.maxRetries, lastErr)
}

func (c *Client) doGenerate(ctx context.Context, body generateRequest) (*ImageResult, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/images/generations", bytes.NewReader(payload))
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
		return nil, parseAPIError(resp.StatusCode, respBody)
	}

	return c.parseImageResponse(respBody)
}

func (c *Client) doEdit(ctx context.Context, input image.Image, prompt string) (*ImageResult, error) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	if err := writer.WriteField("model", c.model); err != nil {
		return nil, fmt.Errorf("writing model field: %w", err)
	}
	if err := writer.WriteField("prompt", prompt); err != nil {
		return nil, fmt.Errorf("writing prompt field: %w", err)
	}
	if err := writer.WriteField("n", "1"); err != nil {
		return nil, fmt.Errorf("writing n field: %w", err)
	}
	if err := writer.WriteField("size", c.size); err != nil {
		return nil, fmt.Errorf("writing size field: %w", err)
	}
	if err := writer.WriteField("quality", c.quality); err != nil {
		return nil, fmt.Errorf("writing quality field: %w", err)
	}
	if c.thinking != "" {
		if err := writer.WriteField("thinking", c.thinking); err != nil {
			return nil, fmt.Errorf("writing thinking field: %w", err)
		}
	}

	part, err := writer.CreateFormFile("image", "reference.png")
	if err != nil {
		return nil, fmt.Errorf("creating form file: %w", err)
	}

	if err := png.Encode(part, input); err != nil {
		return nil, fmt.Errorf("encoding image: %w", err)
	}

	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("closing multipart: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/images/edits", &buf)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())
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
		return nil, parseAPIError(resp.StatusCode, respBody)
	}

	return c.parseImageResponse(respBody)
}

func (c *Client) parseImageResponse(body []byte) (*ImageResult, error) {
	var resp imagesResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parsing response json: %w", err)
	}

	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("image response has no data")
	}

	d := resp.Data[0]
	result := &ImageResult{
		RevisedPrompt: d.RevisedPrompt,
	}

	if d.B64JSON != "" {
		decoded, err := base64.StdEncoding.DecodeString(d.B64JSON)
		if err != nil {
			return nil, fmt.Errorf("decoding base64 image: %w", err)
		}
		img, _, err := image.Decode(bytes.NewReader(decoded))
		if err != nil {
			return nil, fmt.Errorf("decoding image: %w", err)
		}
		result.Image = img
	} else if d.URL != "" {
		imgResp, err := c.httpClient.Get(d.URL)
		if err != nil {
			return nil, fmt.Errorf("downloading image from url: %w", err)
		}
		defer imgResp.Body.Close()
		img, _, err := image.Decode(imgResp.Body)
		if err != nil {
			return nil, fmt.Errorf("decoding downloaded image: %w", err)
		}
		result.Image = img
	} else {
		return nil, fmt.Errorf("image data has no b64_json or url")
	}

	return result, nil
}

func parseAPIError(statusCode int, body []byte) error {
	var resp imagesResponse
	if err := json.Unmarshal(body, &resp); err == nil && resp.Error != nil {
		return &APIError{
			StatusCode: statusCode,
			Message:    resp.Error.Message,
			Type:       resp.Error.Type,
			Code:       resp.Error.Code,
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



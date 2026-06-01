package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestChat_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Error("missing or wrong authorization header")
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Error("wrong content type")
		}

		var req chatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decoding request: %v", err)
		}

		if req.ResponseFormat == nil || req.ResponseFormat.Type != "json_object" {
			t.Error("missing response_format")
		}

		resp := chatResponse{
			Choices: []choice{
				{
					Message: responseMessage{
						Content: `{"name": "Alice", "age": 7}`,
					},
				},
			},
			Usage: &usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient("test-key", "gpt-5.4-mini", "medium").
		WithBaseURL(server.URL).
		WithHTTPClient(server.Client())

	messages := []Message{
		{Role: RoleSystem, Content: "You are a helpful assistant."},
		{Role: RoleUser, Content: "Extract character info."},
	}

	var result map[string]any
	if err := client.Chat(context.Background(), messages, &result, 0.1); err != nil {
		t.Fatalf("Chat failed: %v", err)
	}

	if result["name"] != "Alice" {
		t.Errorf("expected name=Alice, got %v", result["name"])
	}
}

func TestChat_RetryOn429(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			w.WriteHeader(http.StatusTooManyRequests)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(chatResponse{
				Error: &apiError{Message: "Rate limit", Type: "rate_limit_error", Code: "rate_limit"},
			})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(chatResponse{
			Choices: []choice{
				{Message: responseMessage{Content: `{"status": "ok"}`}},
			},
		})
	}))
	defer server.Close()

	client := NewClient("test-key", "gpt-5.4-mini", "medium").
		WithBaseURL(server.URL).
		WithHTTPClient(server.Client()).
		WithMaxRetries(5).
		WithRetryDelay(time.Millisecond)

	var result map[string]any
	if err := client.Chat(context.Background(), nil, &result, 0.1); err != nil {
		t.Fatalf("Chat failed after retries: %v", err)
	}

	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestChat_RetryOn500(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 2 {
			w.WriteHeader(http.StatusInternalServerError)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(chatResponse{
				Error: &apiError{Message: "Internal error", Type: "server_error", Code: "internal_error"},
			})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(chatResponse{
			Choices: []choice{
				{Message: responseMessage{Content: `{"status": "ok"}`}},
			},
		})
	}))
	defer server.Close()

	client := NewClient("test-key", "gpt-5.4-mini", "medium").
		WithBaseURL(server.URL).
		WithHTTPClient(server.Client()).
		WithMaxRetries(3).
		WithRetryDelay(time.Millisecond)

	var result map[string]any
	if err := client.Chat(context.Background(), nil, &result, 0.1); err != nil {
		t.Fatalf("Chat failed after retries: %v", err)
	}

	if attempts != 2 {
		t.Errorf("expected 2 attempts, got %d", attempts)
	}
}

func TestChat_ExhaustRetries(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(chatResponse{
			Error: &apiError{Message: "Service unavailable", Type: "server_error", Code: "service_unavailable"},
		})
	}))
	defer server.Close()

	client := NewClient("test-key", "gpt-5.4-mini", "medium").
		WithBaseURL(server.URL).
		WithHTTPClient(server.Client()).
		WithMaxRetries(2).
		WithRetryDelay(time.Millisecond)

	err := client.Chat(context.Background(), nil, nil, 0.1)
	if err == nil {
		t.Fatal("expected error after exhausting retries")
	}

	if attempts != 3 {
		t.Errorf("expected 3 attempts (initial + 2 retries), got %d", attempts)
	}
}

func TestChat_NonRetryable400(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(chatResponse{
			Error: &apiError{Message: "Invalid request", Type: "invalid_request_error", Code: "bad_request"},
		})
	}))
	defer server.Close()

	client := NewClient("test-key", "gpt-5.4-mini", "medium").
		WithBaseURL(server.URL).
		WithHTTPClient(server.Client()).
		WithMaxRetries(3).
		WithRetryDelay(time.Millisecond)

	err := client.Chat(context.Background(), nil, nil, 0.1)
	if err == nil {
		t.Fatal("expected error for 400")
	}

	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected APIError, got %T", err)
	}
	if apiErr.StatusCode != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", apiErr.StatusCode)
	}
}

func TestChat_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(chatResponse{
			Choices: []choice{
				{Message: responseMessage{Content: `{}`}},
			},
		})
	}))
	defer server.Close()

	client := NewClient("test-key", "gpt-5.4-mini", "medium").
		WithBaseURL(server.URL).
		WithHTTPClient(server.Client())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := client.Chat(ctx, nil, nil, 0.1)
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestChat_InvalidJSONResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(chatResponse{
			Choices: []choice{
				{Message: responseMessage{Content: "not valid json"}},
			},
		})
	}))
	defer server.Close()

	client := NewClient("test-key", "gpt-5.4-mini", "medium").
		WithBaseURL(server.URL).
		WithHTTPClient(server.Client())

	var result map[string]any
	err := client.Chat(context.Background(), nil, &result, 0.1)
	if err == nil {
		t.Fatal("expected error for invalid JSON response")
	}
}

func TestChat_EmptyChoices(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(chatResponse{Choices: []choice{}})
	}))
	defer server.Close()

	client := NewClient("test-key", "gpt-5.4-mini", "medium").
		WithBaseURL(server.URL).
		WithHTTPClient(server.Client())

	err := client.Chat(context.Background(), nil, nil, 0.1)
	if err == nil {
		t.Fatal("expected error for empty choices")
	}
}

func TestChatRaw_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(chatResponse{
			Choices: []choice{
				{Message: responseMessage{Content: "raw content"}},
			},
			Usage: &usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
		})
	}))
	defer server.Close()

	client := NewClient("test-key", "gpt-5.4-mini", "medium").
		WithBaseURL(server.URL).
		WithHTTPClient(server.Client())

	result, err := client.ChatRaw(context.Background(), nil, 0.1)
	if err != nil {
		t.Fatalf("ChatRaw failed: %v", err)
	}

	if result.Content != "raw content" {
		t.Errorf("expected 'raw content', got %q", result.Content)
	}
	if result.PromptTokens != 10 {
		t.Errorf("expected 10 prompt tokens, got %d", result.PromptTokens)
	}
	if result.TotalTokens != 15 {
		t.Errorf("expected 15 total tokens, got %d", result.TotalTokens)
	}
}

func TestChat_RetryOn503(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(chatResponse{
				Error: &apiError{Message: "Service unavailable", Type: "server_error", Code: "service_unavailable"},
			})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(chatResponse{
			Choices: []choice{
				{Message: responseMessage{Content: `{"status": "ok"}`}},
			},
		})
	}))
	defer server.Close()

	client := NewClient("test-key", "gpt-5.4-mini", "medium").
		WithBaseURL(server.URL).
		WithHTTPClient(server.Client()).
		WithMaxRetries(3).
		WithRetryDelay(time.Millisecond)

	var result map[string]any
	if err := client.Chat(context.Background(), nil, &result, 0.1); err != nil {
		t.Fatalf("Chat failed after retries: %v", err)
	}

	if attempts != 2 {
		t.Errorf("expected 2 attempts, got %d", attempts)
	}
}

func TestSystemPromptExtractCharacters(t *testing.T) {
	prompt := SystemPromptExtractCharacters()
	if prompt == "" {
		t.Error("expected non-empty prompt")
	}
	if !strings.Contains(prompt, "CharacterNote") {
		t.Error("expected CharacterNote in prompt")
	}
	if !strings.Contains(prompt, "physical_description") {
		t.Error("expected physical_description in prompt")
	}
}

func TestSystemPromptExtractScenes(t *testing.T) {
	prompt := SystemPromptExtractScenes()
	if prompt == "" {
		t.Error("expected non-empty prompt")
	}
	if !strings.Contains(prompt, "comic panel") {
		t.Error("expected 'comic panel' in prompt")
	}
	if !strings.Contains(prompt, "characters_present") {
		t.Error("expected characters_present in prompt")
	}
}

func TestBackoffDuration(t *testing.T) {
	client := NewClient("test-key", "gpt-5.4-mini", "medium").WithRetryDelay(time.Second)

	tests := []struct {
		attempt  int
		expected time.Duration
	}{
		{1, 1 * time.Second},
		{2, 2 * time.Second},
		{3, 4 * time.Second},
		{4, 8 * time.Second},
		{5, 16 * time.Second},
		{6, 16 * time.Second},
	}

	for _, tt := range tests {
		got := client.backoffDuration(tt.attempt)
		if got != tt.expected {
			t.Errorf("attempt %d: expected %v, got %v", tt.attempt, tt.expected, got)
		}
	}
}

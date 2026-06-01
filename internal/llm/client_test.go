package llm

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/openai/openai-go/v3"
)

func TestChatSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("expected /chat/completions, got %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Error("missing or wrong authorization header")
		}

		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decoding request: %v", err)
		}
		if req["model"] != "gpt-5.4-mini" {
			t.Errorf("expected model gpt-5.4-mini, got %v", req["model"])
		}
		format, ok := req["response_format"].(map[string]any)
		if !ok || format["type"] != "json_schema" {
			t.Errorf("expected json_schema response format, got %#v", req["response_format"])
		}
		jsonSchema, ok := format["json_schema"].(map[string]any)
		if !ok || jsonSchema["strict"] != true {
			t.Errorf("expected strict json schema response format, got %#v", req["response_format"])
		}

		writeChatResponse(t, w, `{"name":"Alice","age":7}`, 10, 15)
	}))
	defer server.Close()

	client := NewClient("test-key", "gpt-5.4-mini", "medium").
		WithBaseURL(server.URL).
		WithHTTPClient(server.Client())

	var result map[string]any
	if err := client.Chat(context.Background(), []Message{{Role: RoleUser, Content: "Extract."}}, &result, 0.1); err != nil {
		t.Fatalf("Chat failed: %v", err)
	}
	if result["name"] != "Alice" {
		t.Errorf("expected Alice, got %v", result["name"])
	}
}

func TestChatRawSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeChatResponse(t, w, "raw content", 10, 15)
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
		t.Errorf("expected raw content, got %q", result.Content)
	}
	if result.PromptTokens != 10 || result.TotalTokens != 15 {
		t.Errorf("unexpected token usage: %+v", result)
	}
}

func TestChatOpenAIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		if err := json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{"message": "Invalid request", "type": "invalid_request_error", "code": "bad_request"},
		}); err != nil {
			t.Fatalf("encoding error response: %v", err)
		}
	}))
	defer server.Close()

	client := NewClient("test-key", "gpt-5.4-mini", "medium").
		WithBaseURL(server.URL).
		WithHTTPClient(server.Client()).
		WithMaxRetries(0)

	var result map[string]any
	err := client.Chat(context.Background(), nil, &result, 0.1)
	if err == nil {
		t.Fatal("expected error")
	}
	var apiErr *openai.Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected openai.Error, got %T: %v", err, err)
	}
	if apiErr.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", apiErr.StatusCode)
	}
	if !strings.Contains(err.Error(), "llm chat") {
		t.Fatalf("expected wrapped OpenAI 400 error, got %v", err)
	}
}

func TestChatNilSchema(t *testing.T) {
	client := NewClient("test-key", "gpt-5.4-mini", "medium")
	err := client.Chat(context.Background(), nil, nil, 0.1)
	if err == nil || !strings.Contains(err.Error(), "schema is required") {
		t.Fatalf("expected schema required error, got %v", err)
	}
}

func TestChatInvalidJSONResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeChatResponse(t, w, "not valid json", 0, 0)
	}))
	defer server.Close()

	client := NewClient("test-key", "gpt-5.4-mini", "medium").WithBaseURL(server.URL).WithHTTPClient(server.Client())
	var result map[string]any
	if err := client.Chat(context.Background(), nil, &result, 0.1); err == nil {
		t.Fatal("expected invalid JSON error")
	}
}

func TestChatEmptyChoices(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(map[string]any{"choices": []any{}}); err != nil {
			t.Fatalf("encoding empty choices response: %v", err)
		}
	}))
	defer server.Close()

	client := NewClient("test-key", "gpt-5.4-mini", "medium").WithBaseURL(server.URL).WithHTTPClient(server.Client())
	if err := client.Chat(context.Background(), nil, nil, 0.1); err == nil {
		t.Fatal("expected empty choices error")
	}
}

func TestSystemPromptExtractCharacters(t *testing.T) {
	prompt := SystemPromptExtractCharacters()
	if prompt == "" || !strings.Contains(prompt, "CharacterNote") || !strings.Contains(prompt, "physical_description") {
		t.Fatalf("unexpected character prompt: %s", prompt)
	}
}

func TestSystemPromptExtractScenes(t *testing.T) {
	prompt := SystemPromptExtractScenes()
	if prompt == "" || !strings.Contains(prompt, "comic panel") || !strings.Contains(prompt, "characters_present") {
		t.Fatalf("unexpected scenes prompt: %s", prompt)
	}
}

func writeChatResponse(t *testing.T, w http.ResponseWriter, content string, promptTokens, totalTokens int) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{
		"id":      "chatcmpl-test",
		"object":  "chat.completion",
		"created": 1,
		"model":   "gpt-5.4-mini",
		"choices": []any{map[string]any{
			"index":         0,
			"finish_reason": "stop",
			"message": map[string]any{
				"role":    "assistant",
				"content": content,
			},
		}},
		"usage": map[string]any{
			"prompt_tokens":     promptTokens,
			"completion_tokens": totalTokens - promptTokens,
			"total_tokens":      totalTokens,
		},
	}); err != nil {
		t.Fatalf("encoding chat response: %v", err)
	}
}

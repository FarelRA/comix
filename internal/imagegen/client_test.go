package imagegen

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func createTestPNG() image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	img.Set(0, 0, color.RGBA{255, 0, 0, 255})
	return img
}

func pngBase64() string {
	img := createTestPNG()
	var buf bytes.Buffer
	png.Encode(&buf, img)
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

func TestGenerate_Success(t *testing.T) {
	expectedB64 := pngBase64()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Error("wrong auth header")
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Error("wrong content type")
		}

		var req generateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decoding request: %v", err)
		}

		if req.Model != "gpt-image-2" {
			t.Errorf("expected model gpt-image-2, got %s", req.Model)
		}
		if req.N != 1 {
			t.Errorf("expected n=1, got %d", req.N)
		}

		resp := imagesResponse{
			Data: []imageData{
				{
					B64JSON:       expectedB64,
					RevisedPrompt: "revised version",
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient("test-key", "gpt-image-2", "medium", "1024x1024", "medium").
		WithBaseURL(server.URL).
		WithHTTPClient(server.Client()).
		WithMaxRetries(0)


	result, err := client.Generate(context.Background(), "test prompt")
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	if result.RevisedPrompt != "revised version" {
		t.Errorf("expected revised prompt 'revised version', got %q", result.RevisedPrompt)
	}
	if result.Image == nil {
		t.Fatal("expected non-nil image")
	}
	if result.Image.Bounds().Dx() != 10 || result.Image.Bounds().Dy() != 10 {
		t.Errorf("expected 10x10 image, got %dx%d", result.Image.Bounds().Dx(), result.Image.Bounds().Dy())
	}
}

func TestGenerate_FromURL(t *testing.T) {
	imgServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		img := createTestPNG()
		w.Header().Set("Content-Type", "image/png")
		png.Encode(w, img)
	}))
	defer imgServer.Close()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := imagesResponse{
			Data: []imageData{
				{URL: imgServer.URL},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient("test-key", "gpt-image-2", "medium", "1024x1024", "medium").
		WithBaseURL(server.URL).
		WithHTTPClient(server.Client()).
		WithMaxRetries(0)


	result, err := client.Generate(context.Background(), "test prompt")
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if result.Image == nil {
		t.Fatal("expected non-nil image")
	}
}

func TestGenerate_RetryOn429(t *testing.T) {
	var attempts atomic.Int32
	expectedB64 := pngBase64()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := attempts.Add(1)
		if n < 3 {
			w.WriteHeader(http.StatusTooManyRequests)
			json.NewEncoder(w).Encode(imagesResponse{
				Error: &apiError{Message: "Rate limited", Type: "rate_limit_error", Code: "rate_limit"},
			})
			return
		}
		json.NewEncoder(w).Encode(imagesResponse{
			Data: []imageData{{B64JSON: expectedB64}},
		})
	}))
	defer server.Close()

	client := NewClient("test-key", "gpt-image-2", "medium", "1024x1024", "medium").
		WithBaseURL(server.URL).
		WithHTTPClient(server.Client()).
		WithMaxRetries(5).
		WithRetryDelay(time.Millisecond)


	_, err := client.Generate(context.Background(), "test")
	if err != nil {
		t.Fatalf("Generate failed after retries: %v", err)
	}

	if n := attempts.Load(); n != 3 {
		t.Errorf("expected 3 attempts, got %d", n)
	}
}

func TestGenerate_NonRetryable400(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(imagesResponse{
			Error: &apiError{Message: "Bad request", Type: "invalid_request_error", Code: "bad_request"},
		})
	}))
	defer server.Close()

	client := NewClient("test-key", "gpt-image-2", "medium", "1024x1024", "medium").
		WithBaseURL(server.URL).
		WithHTTPClient(server.Client()).
		WithMaxRetries(3).
		WithRetryDelay(time.Millisecond)


	_, err := client.Generate(context.Background(), "test")
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

func TestEdit_Success(t *testing.T) {
	expectedB64 := pngBase64()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Error("wrong auth header")
		}

		ct := r.Header.Get("Content-Type")
		if !strings.HasPrefix(ct, "multipart/form-data") {
			t.Errorf("expected multipart/form-data, got %s", ct)
		}

		if err := r.ParseMultipartForm(32 << 20); err != nil {
			t.Fatalf("parsing multipart form: %v", err)
		}

		if r.FormValue("model") != "gpt-image-2" {
			t.Errorf("expected model gpt-image-2, got %s", r.FormValue("model"))
		}
		if r.FormValue("prompt") != "edit prompt" {
			t.Errorf("expected 'edit prompt', got %s", r.FormValue("prompt"))
		}
		if r.FormValue("n") != "1" {
			t.Errorf("expected n=1, got %s", r.FormValue("n"))
		}
		if r.FormValue("size") != "1024x1024" {
			t.Errorf("expected 1024x1024, got %s", r.FormValue("size"))
		}

		file, _, err := r.FormFile("image")
		if err != nil {
			t.Fatalf("getting image file: %v", err)
		}
		file.Close()

		resp := imagesResponse{
			Data: []imageData{
				{B64JSON: expectedB64, RevisedPrompt: "edited version"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient("test-key", "gpt-image-2", "medium", "1024x1024", "medium").
		WithBaseURL(server.URL).
		WithHTTPClient(server.Client()).
		WithMaxRetries(0)


	input := createTestPNG()
	result, err := client.Edit(context.Background(), input, "edit prompt")
	if err != nil {
		t.Fatalf("Edit failed: %v", err)
	}

	if result.RevisedPrompt != "edited version" {
		t.Errorf("expected 'edited version', got %q", result.RevisedPrompt)
	}
	if result.Image == nil {
		t.Fatal("expected non-nil image")
	}
}

func TestParseImageResponse_FromURL(t *testing.T) {
	imgServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		png.Encode(w, createTestPNG())
	}))
	defer imgServer.Close()

	client := NewClient("test-key", "gpt-image-2", "medium", "1024x1024", "medium").
		WithHTTPClient(imgServer.Client()).
		WithMaxRetries(0)


	result, err := client.parseImageResponse([]byte(`{"data":[{"url":"` + imgServer.URL + `"}]}`))
	if err != nil {
		t.Fatalf("parseImageResponse failed: %v", err)
	}
	if result.Image == nil {
		t.Fatal("expected non-nil image")
	}
}

func TestParseImageResponse_NoB64OrURL(t *testing.T) {
	client := NewClient("test-key", "gpt-image-2", "medium", "1024x1024", "medium").
		WithMaxRetries(0)


	_, err := client.parseImageResponse([]byte(`{"data":[{}]}`))
	if err == nil {
		t.Fatal("expected error for data with no b64_json or url")
	}
}

func TestParseAPIError(t *testing.T) {
	err := parseAPIError(http.StatusBadRequest, []byte(`{"error":{"message":"Bad request","type":"invalid_request_error","code":"bad_request"}}`))
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected APIError, got %T", err)
	}
	if apiErr.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", apiErr.StatusCode)
	}
	if apiErr.Message != "Bad request" {
		t.Errorf("expected 'Bad request', got %q", apiErr.Message)
	}
}

func TestParseAPIError_NoErrorField(t *testing.T) {
	err := parseAPIError(http.StatusInternalServerError, []byte(`{"error":{}}`))
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected APIError, got %T", err)
	}
	if apiErr.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", apiErr.StatusCode)
	}
}

func TestParseAPIError_NonJSONBody(t *testing.T) {
	err := parseAPIError(http.StatusBadRequest, []byte(`not json`))
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected APIError, got %T", err)
	}
	if apiErr.Message == "" {
		t.Error("expected non-empty message from raw body")
	}
}

func TestParseImageResponse_InvalidJSON(t *testing.T) {
	client := NewClient("test-key", "gpt-image-2", "medium", "1024x1024", "medium").
		WithMaxRetries(0)


	_, err := client.parseImageResponse([]byte(`not json`))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestParseImageResponse_InvalidBase64(t *testing.T) {
	client := NewClient("test-key", "gpt-image-2", "medium", "1024x1024", "medium").
		WithMaxRetries(0)


	_, err := client.parseImageResponse([]byte(`{"data":[{"b64_json":"not-valid-base64!!!"}]}`))
	if err == nil {
		t.Fatal("expected error for invalid base64")
	}
}

func TestEdit_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(imagesResponse{
			Data: []imageData{{B64JSON: pngBase64()}},
		})
	}))
	defer server.Close()

	client := NewClient("test-key", "gpt-image-2", "medium", "1024x1024", "medium").
		WithBaseURL(server.URL).
		WithHTTPClient(server.Client()).
		WithMaxRetries(0)


	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := client.Edit(ctx, createTestPNG(), "test")
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestGenerate_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(imagesResponse{
			Data: []imageData{{B64JSON: pngBase64()}},
		})
	}))
	defer server.Close()

	client := NewClient("test-key", "gpt-image-2", "medium", "1024x1024", "medium").
		WithBaseURL(server.URL).
		WithHTTPClient(server.Client()).
		WithMaxRetries(0)


	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := client.Generate(ctx, "test")
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestGenerate_EmptyData(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(imagesResponse{Data: []imageData{}})
	}))
	defer server.Close()

	client := NewClient("test-key", "gpt-image-2", "medium", "1024x1024", "medium").
		WithBaseURL(server.URL).
		WithHTTPClient(server.Client()).
		WithMaxRetries(0)


	_, err := client.Generate(context.Background(), "test")
	if err == nil {
		t.Fatal("expected error for empty data")
	}
}

func TestGenerate_NoImageInData(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(imagesResponse{
			Data: []imageData{{}},
		})
	}))
	defer server.Close()

	client := NewClient("test-key", "gpt-image-2", "medium", "1024x1024", "medium").
		WithBaseURL(server.URL).
		WithHTTPClient(server.Client()).
		WithMaxRetries(0)


	_, err := client.Generate(context.Background(), "test")
	if err == nil {
		t.Fatal("expected error for data with no image content")
	}
}

func TestEdit_RetryOn503(t *testing.T) {
	var attempts atomic.Int32
	expectedB64 := pngBase64()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := attempts.Add(1)
		if n < 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(imagesResponse{
				Error: &apiError{Message: "Service unavailable", Type: "server_error", Code: "service_unavailable"},
			})
			return
		}
		json.NewEncoder(w).Encode(imagesResponse{
			Data: []imageData{{B64JSON: expectedB64}},
		})
	}))
	defer server.Close()

	client := NewClient("test-key", "gpt-image-2", "medium", "1024x1024", "medium").
		WithBaseURL(server.URL).
		WithHTTPClient(server.Client()).
		WithMaxRetries(3).
		WithRetryDelay(time.Millisecond)


	input := createTestPNG()
	_, err := client.Edit(context.Background(), input, "test")
	if err != nil {
		t.Fatalf("Edit failed after retries: %v", err)
	}

	if n := attempts.Load(); n != 2 {
		t.Errorf("expected 2 attempts, got %d", n)
	}
}

func TestBackoffDuration(t *testing.T) {
	client := NewClient("test-key", "gpt-image-2", "medium", "1024x1024", "medium").
		WithRetryDelay(time.Second)

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

func TestPromptBaseSheet(t *testing.T) {
	result := PromptBaseSheet("Alice", "Young girl, blonde hair")
	if !strings.Contains(result, "Alice") {
		t.Error("expected character name in prompt")
	}
	if !strings.Contains(result, "Young girl, blonde hair") {
		t.Error("expected physical description in prompt")
	}
	if !strings.Contains(result, "3x2") {
		t.Error("expected grid layout mention")
	}
}

func TestPromptPoseSheet(t *testing.T) {
	result := PromptPoseSheet("Alice")
	if !strings.Contains(result, "Alice") {
		t.Error("expected character name in prompt")
	}
	if !strings.Contains(result, "5x5") {
		t.Error("expected 5x5 mention")
	}
}

func TestPromptFirstScene(t *testing.T) {
	result := PromptFirstScene("Alice sits by the river.", "Alice: young girl")
	if !strings.Contains(result, "Alice sits by the river") {
		t.Error("expected scene description in prompt")
	}
	if !strings.Contains(result, "Alice: young girl") {
		t.Error("expected character refs in prompt")
	}
}

func TestPromptNextScene(t *testing.T) {
	result := PromptNextScene("Alice sits by the river.", "White rabbit runs past.", "Alice, White Rabbit")
	if !strings.Contains(result, "Alice sits") {
		t.Error("expected previous description")
	}
	if !strings.Contains(result, "White rabbit runs") {
		t.Error("expected scene description")
	}
	if !strings.Contains(result, "Alice, White Rabbit") {
		t.Error("expected character refs")
	}
	if !strings.Contains(result, "visual continuity") {
		t.Error("expected continuity instruction")
	}
}

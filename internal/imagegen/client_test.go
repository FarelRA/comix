package imagegen

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/openai/openai-go/v3"
)

func createTestPNG() image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	img.Set(0, 0, color.RGBA{255, 0, 0, 255})
	return img
}

func pngBase64(t *testing.T) string {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, createTestPNG()); err != nil {
		t.Fatalf("encoding test png: %v", err)
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

func TestGenerateSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/images/generations" {
			t.Errorf("expected /images/generations, got %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Error("wrong auth header")
		}

		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decoding request: %v", err)
		}
		if req["model"] != "gpt-image-2" || req["prompt"] != "test prompt" {
			t.Errorf("unexpected request: %#v", req)
		}

		writeImageResponse(t, w, pngBase64(t), "revised version")
	}))
	defer server.Close()

	client := NewClient("test-key", "gpt-image-2", "medium").
		WithBaseURL(server.URL).
		WithHTTPClient(server.Client())

	result, err := client.Generate(context.Background(), "test prompt", "2048x2048")
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if result.RevisedPrompt != "revised version" {
		t.Errorf("expected revised prompt, got %q", result.RevisedPrompt)
	}
	assertImage10x10(t, result.Image)
}

func TestGenerateWithReferencesUsesEditAPI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/images/edits" {
			t.Errorf("expected /images/edits for reference generation, got %s", r.URL.Path)
		}
		if !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
			t.Errorf("expected multipart/form-data, got %s", r.Header.Get("Content-Type"))
		}
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			t.Fatalf("parsing multipart form: %v", err)
		}
		if r.FormValue("prompt") != "with refs" || r.FormValue("model") != "gpt-image-2" {
			t.Errorf("unexpected form: %#v", r.Form)
		}
		if files := multipartFileCount(r.MultipartForm.File); files != 2 {
			t.Errorf("expected 2 image files, got %d", files)
		}
		writeImageResponse(t, w, pngBase64(t), "")
	}))
	defer server.Close()

	client := NewClient("test-key", "gpt-image-2", "medium").WithBaseURL(server.URL).WithHTTPClient(server.Client())
	_, err := client.GenerateWithReferences(context.Background(), "with refs", "2048x2048", createTestPNG(), createTestPNG())
	if err != nil {
		t.Fatalf("GenerateWithReferences failed: %v", err)
	}
}

func TestGenerateOpenAIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		if err := json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{"message": "Bad request", "type": "invalid_request_error", "code": "bad_request"},
		}); err != nil {
			t.Fatalf("encoding error response: %v", err)
		}
	}))
	defer server.Close()

	client := NewClient("test-key", "gpt-image-2", "medium").WithBaseURL(server.URL).WithHTTPClient(server.Client()).WithMaxRetries(0)
	_, err := client.Generate(context.Background(), "test", "2048x2048")
	if err == nil || !strings.Contains(err.Error(), "400") {
		t.Fatalf("expected OpenAI 400 error, got %v", err)
	}
}

func TestParseResponseFromURL(t *testing.T) {
	imgServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		if err := png.Encode(w, createTestPNG()); err != nil {
			t.Fatalf("encoding png response: %v", err)
		}
	}))
	defer imgServer.Close()

	result, err := parseResponse(imageResponseWithURL(imgServer.URL), imgServer.Client())
	if err != nil {
		t.Fatalf("parseResponse failed: %v", err)
	}
	assertImage10x10(t, result.Image)
}

func TestPromptHelpers(t *testing.T) {
	if got := PromptBaseSheet("Alice", "red hair"); !strings.Contains(got, "Alice") || !strings.Contains(got, "red hair") {
		t.Fatalf("unexpected base sheet prompt: %s", got)
	}
	if got := PromptPoseSheet("Alice"); !strings.Contains(got, "Alice") || !strings.Contains(got, "5x5") {
		t.Fatalf("unexpected pose prompt: %s", got)
	}
}

func writeImageResponse(t *testing.T, w http.ResponseWriter, b64, revised string) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{
		"created": 1,
		"data": []any{map[string]any{
			"b64_json":       b64,
			"revised_prompt": revised,
		}},
		"usage": map[string]any{
			"input_tokens":  2,
			"output_tokens": 3,
			"total_tokens":  5,
		},
	}); err != nil {
		t.Fatalf("encoding image response: %v", err)
	}
}

func imageResponseWithURL(url string) *openai.ImagesResponse {
	return &openai.ImagesResponse{Data: []openai.Image{{URL: url}}}
}

func assertImage10x10(t *testing.T, img image.Image) {
	t.Helper()
	if img == nil {
		t.Fatal("expected non-nil image")
	}
	if img.Bounds().Dx() != 10 || img.Bounds().Dy() != 10 {
		t.Fatalf("expected 10x10 image, got %dx%d", img.Bounds().Dx(), img.Bounds().Dy())
	}
}

func multipartFileCount(files map[string][]*multipart.FileHeader) int {
	total := 0
	for _, values := range files {
		total += len(values)
	}
	return total
}

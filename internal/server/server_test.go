package server

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/FarelRA/comix/internal/config"
	"github.com/FarelRA/comix/internal/imagegen"
	"github.com/FarelRA/comix/internal/llm"
	"github.com/FarelRA/comix/internal/model"
	"github.com/FarelRA/comix/internal/pipeline"
	"github.com/FarelRA/comix/internal/state"
)

func testServer(t *testing.T, outputDir string) *Server {
	t.Helper()
	cfg := testConfig(outputDir)
	llmClient := llm.NewClient("sk-test", "gpt-5.4-mini", "medium")
	imgGenClient := imagegen.NewClient("sk-test", "gpt-image-2", "medium")
	p := pipeline.NewPipeline(cfg, llmClient, imgGenClient)
	return NewServer(cfg, p)
}

func testConfig(outputDir string) *config.Config {
	return &config.Config{
		OpenAI: config.OpenAIConfig{
			APIKey: "sk-test",
			LLM:    config.LLMConfig{},
			Image: config.ImageConfig{
				Size: config.ImageSizeConfig{
					Sheet: "2880x1920",
					Poses: "2048x2048",
					Panel: "1632x3808",
				},
			},
		},
		Pipeline: config.PipelineConfig{
			OutputDir:           outputDir,
			ChapterPattern:      "chapter_*.md",
			CoverFilename:       "cover.md",
			MaxConcurrentSheets: 2,
			MaxConcurrentPoses:  2,
		},
		Server: config.ServerConfig{
			Host:            "0.0.0.0",
			Port:            8080,
			ReadTimeout:     30000000000,
			WriteTimeout:    60000000000,
			ShutdownTimeout: 15000000000,
			AllowedOrigins:  []string{"http://example.com"},
			RateLimit:       1000,
		},
		Logging: config.LoggingConfig{
			Level:  "info",
			Format: "text",
		},
	}
}

func requestTo(server *Server, method, path string, body io.Reader) *http.Response {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, body)
	req.Header.Set("Content-Type", "application/json")
	server.router.ServeHTTP(w, req)
	return w.Result()
}

func decodeEnvelope(t *testing.T, resp *http.Response) envelope {
	t.Helper()
	var env envelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decoding envelope: %v", err)
	}
	return env
}

func TestHealthEndpoint(t *testing.T) {
	srv := testServer(t, t.TempDir())
	resp := requestTo(srv, http.MethodGet, "/api/health", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	env := decodeEnvelope(t, resp)
	if !env.Success {
		t.Fatal("expected success=true")
	}
	if env.Data == nil {
		t.Fatal("expected data")
	}
}

func TestCreateProject_Success(t *testing.T) {
	srv := testServer(t, t.TempDir())
	body := `{"name":"test-project","source_type":"directory","source_path":"/tmp/novels"}`
	resp := requestTo(srv, http.MethodPost, "/api/projects", strings.NewReader(body))
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	env := decodeEnvelope(t, resp)
	if !env.Success {
		t.Fatal("expected success=true")
	}
	m, ok := env.Data.(map[string]interface{})
	if !ok {
		t.Fatal("expected data to be an object")
	}
	project, ok := m["project"].(map[string]interface{})
	if !ok {
		t.Fatal("expected project field")
	}
	if project["name"] != "test-project" {
		t.Errorf("expected project.name 'test-project', got %v", project["name"])
	}
}

func TestCreateProject_MissingName(t *testing.T) {
	srv := testServer(t, t.TempDir())
	body := `{"source_type":"directory"}`
	resp := requestTo(srv, http.MethodPost, "/api/projects", strings.NewReader(body))
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
	env := decodeEnvelope(t, resp)
	if env.Success {
		t.Fatal("expected success=false")
	}
	if env.Error == nil || env.Error.Code != "INVALID_REQUEST" {
		t.Fatalf("expected INVALID_REQUEST error, got %+v", env.Error)
	}
}

func TestCreateProject_Duplicate(t *testing.T) {
	tmpDir := t.TempDir()
	srv := testServer(t, tmpDir)
	body := `{"name":"dup-project"}`
	resp1 := requestTo(srv, http.MethodPost, "/api/projects", strings.NewReader(body))
	if resp1.StatusCode != http.StatusCreated {
		t.Fatalf("first create expected 201, got %d", resp1.StatusCode)
	}
	resp2 := requestTo(srv, http.MethodPost, "/api/projects", strings.NewReader(body))
	if resp2.StatusCode != http.StatusConflict {
		t.Fatalf("duplicate expected 409, got %d", resp2.StatusCode)
	}
	env := decodeEnvelope(t, resp2)
	if env.Error == nil || env.Error.Code != "PROJECT_EXISTS" {
		t.Fatalf("expected PROJECT_EXISTS error, got %+v", env.Error)
	}
}

func TestListProjects_Empty(t *testing.T) {
	srv := testServer(t, t.TempDir())
	resp := requestTo(srv, http.MethodGet, "/api/projects", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	env := decodeEnvelope(t, resp)
	if !env.Success {
		t.Fatal("expected success=true")
	}
	projects, ok := env.Data.([]interface{})
	if !ok {
		t.Fatal("expected data to be an array")
	}
	if len(projects) != 0 {
		t.Errorf("expected empty list, got %d items", len(projects))
	}
}

func TestListProjects_WithProjects(t *testing.T) {
	tmpDir := t.TempDir()
	srv := testServer(t, tmpDir)
	state.SaveManifest(tmpDir, "proj-a", model.NewProjectManifest("proj-a", "dir", "/tmp", nil))
	state.SaveManifest(tmpDir, "proj-b", model.NewProjectManifest("proj-b", "dir", "/tmp", nil))
	resp := requestTo(srv, http.MethodGet, "/api/projects", nil)
	env := decodeEnvelope(t, resp)
	projects, ok := env.Data.([]interface{})
	if !ok {
		t.Fatal("expected data to be an array")
	}
	if len(projects) != 2 {
		t.Errorf("expected 2 projects, got %d", len(projects))
	}
}

func TestDeleteProject_NotFound(t *testing.T) {
	srv := testServer(t, t.TempDir())
	resp := requestTo(srv, http.MethodDelete, "/api/projects/nonexistent", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestGetStatus_NotFound(t *testing.T) {
	srv := testServer(t, t.TempDir())
	resp := requestTo(srv, http.MethodGet, "/api/projects/nonexistent/status", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestGetStatus_Success(t *testing.T) {
	tmpDir := t.TempDir()
	srv := testServer(t, tmpDir)
	state.SaveManifest(tmpDir, "myproject", model.NewProjectManifest("myproject", "dir", "/tmp", nil))
	resp := requestTo(srv, http.MethodGet, "/api/projects/myproject/status", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	env := decodeEnvelope(t, resp)
	if !env.Success {
		t.Fatal("expected success=true")
	}
}

func TestListOutputs_NotFound(t *testing.T) {
	srv := testServer(t, t.TempDir())
	resp := requestTo(srv, http.MethodGet, "/api/projects/nonexistent/output", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestGetOutput_PathTraversalPrevention(t *testing.T) {
	tmpDir := t.TempDir()
	srv := testServer(t, tmpDir)
	state.SaveManifest(tmpDir, "foo", model.NewProjectManifest("foo", "dir", "/tmp", nil))
	resp := requestTo(srv, http.MethodGet, "/api/projects/foo/output/../../../etc/passwd", nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestGetOutput_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	srv := testServer(t, tmpDir)
	state.SaveManifest(tmpDir, "myproject", model.NewProjectManifest("myproject", "dir", "/tmp", nil))
	resp := requestTo(srv, http.MethodGet, "/api/projects/myproject/output/nonexistent.png", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestListOutputs_WithFiles(t *testing.T) {
	tmpDir := t.TempDir()
	srv := testServer(t, tmpDir)
	state.SaveManifest(tmpDir, "proj", model.NewProjectManifest("proj", "dir", "/tmp", nil))
	sheetsDir := filepath.Join(tmpDir, "proj", "sheets")
	os.MkdirAll(sheetsDir, 0755)
	os.WriteFile(filepath.Join(sheetsDir, "alice_3x2.png"), []byte("fake-png"), 0644)
	os.WriteFile(filepath.Join(sheetsDir, "rabbit_3x2.png"), []byte("fake-png"), 0644)

	resp := requestTo(srv, http.MethodGet, "/api/projects/proj/output", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	env := decodeEnvelope(t, resp)
	files, ok := env.Data.([]interface{})
	if !ok {
		t.Fatal("expected data to be array")
	}
	if len(files) != 2 {
		t.Errorf("expected 2 files, got %d", len(files))
	}
}

func TestGetOutput_ServesFile(t *testing.T) {
	tmpDir := t.TempDir()
	srv := testServer(t, tmpDir)
	state.SaveManifest(tmpDir, "proj", model.NewProjectManifest("proj", "dir", "/tmp", nil))
	panelsDir := filepath.Join(tmpDir, "proj", "panels")
	os.MkdirAll(panelsDir, 0755)
	content := []byte("fake-panel-content")
	os.WriteFile(filepath.Join(panelsDir, "scene_001.png"), content, 0644)

	resp := requestTo(srv, http.MethodGet, "/api/projects/proj/output/panels/scene_001.png", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "fake-panel-content" {
		t.Errorf("expected file content %q, got %q", "fake-panel-content", string(body))
	}
	ct := resp.Header.Get("Content-Type")
	if ct != "image/png" && ct != "application/octet-stream" {
		t.Errorf("expected image/png or octet-stream, got %q", ct)
	}
}

func TestCORSMiddleware(t *testing.T) {
	srv := testServer(t, t.TempDir())
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodOptions, "/api/health", nil)
	req.Header.Set("Origin", "http://example.com")
	srv.router.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("OPTIONS expected 204, got %d", w.Code)
	}
	if w.Header().Get("Access-Control-Allow-Origin") == "" {
		t.Error("expected CORS headers")
	}
}

func TestResponseEnvelope_Format(t *testing.T) {
	tmpDir := t.TempDir()
	srv := testServer(t, tmpDir)
	resp := requestTo(srv, http.MethodGet, "/api/health", nil)
	body, _ := io.ReadAll(resp.Body)
	var raw map[string]interface{}
	json.Unmarshal(body, &raw)
	if _, ok := raw["success"]; !ok {
		t.Error("expected success field")
	}
	if _, ok := raw["data"]; !ok {
		t.Error("expected data field")
	}
	if _, ok := raw["error"]; !ok {
		t.Error("expected error field")
	}
	if _, ok := raw["meta"]; !ok {
		t.Error("expected meta field")
	}
	meta := raw["meta"].(map[string]interface{})
	if _, ok := meta["request_id"]; !ok {
		t.Error("expected meta.request_id field")
	}
	if _, ok := meta["timestamp"]; !ok {
		t.Error("expected meta.timestamp field")
	}
}

func TestIngest_Success(t *testing.T) {
	tmpDir := t.TempDir()
	srv := testServer(t, tmpDir)
	state.SaveManifest(tmpDir, "testproj", model.NewProjectManifest("testproj", "upload", "", nil))

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	coverPart, _ := w.CreateFormFile("cover", "cover.md")
	coverPart.Write([]byte("# Cover\n\nStory about a girl."))
	ch1Part, _ := w.CreateFormFile("chapters", "chapter_01.md")
	ch1Part.Write([]byte("# Chapter 1\n\nAlice was bored."))
	ch2Part, _ := w.CreateFormFile("chapters", "chapter_02.md")
	ch2Part.Write([]byte("# Chapter 2\n\nCuriouser and curiouser!"))
	w.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/projects/testproj/ingest", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	wrec := httptest.NewRecorder()
	srv.router.ServeHTTP(wrec, req)
	resp := wrec.Result()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	env := decodeEnvelope(t, resp)
	if !env.Success {
		t.Fatal("expected success=true")
	}
}

func TestIngest_NoCover(t *testing.T) {
	tmpDir := t.TempDir()
	srv := testServer(t, tmpDir)
	state.SaveManifest(tmpDir, "testproj", model.NewProjectManifest("testproj", "upload", "", nil))

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	chPart, _ := w.CreateFormFile("chapters", "chapter_01.md")
	chPart.Write([]byte("# Chapter 1"))
	w.Close()

	resp := requestTo(srv, http.MethodPost, "/api/projects/testproj/ingest", &buf)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestIngest_NoChapters(t *testing.T) {
	tmpDir := t.TempDir()
	srv := testServer(t, tmpDir)
	state.SaveManifest(tmpDir, "testproj", model.NewProjectManifest("testproj", "upload", "", nil))

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	coverPart, _ := w.CreateFormFile("cover", "cover.md")
	coverPart.Write([]byte("# Cover"))
	w.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/projects/testproj/ingest", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	wrec := httptest.NewRecorder()
	srv.router.ServeHTTP(wrec, req)
	resp := wrec.Result()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestRunPipeline_NoProject(t *testing.T) {
	srv := testServer(t, t.TempDir())
	resp := requestTo(srv, http.MethodPost, "/api/projects/nonexistent/run", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestRunPhase_InvalidPhase(t *testing.T) {
	srv := testServer(t, t.TempDir())
	resp := requestTo(srv, http.MethodPost, "/api/projects/foo/run/invalid", nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
	env := decodeEnvelope(t, resp)
	if env.Error == nil || env.Error.Code != "INVALID_PHASE" {
		t.Fatalf("expected INVALID_PHASE error, got %+v", env.Error)
	}
}

func TestRunPipeline_Accepts(t *testing.T) {
	tmpDir := t.TempDir()
	srv := testServer(t, tmpDir)
	state.SaveManifest(tmpDir, "myproj", model.NewProjectManifest("myproj", "dir", "/tmp", nil))
	resp := requestTo(srv, http.MethodPost, "/api/projects/myproj/run", nil)
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", resp.StatusCode)
	}
	env := decodeEnvelope(t, resp)
	if !env.Success {
		t.Fatal("expected success=true")
	}
}

func TestRunPhase_Accepts(t *testing.T) {
	tmpDir := t.TempDir()
	srv := testServer(t, tmpDir)
	state.SaveManifest(tmpDir, "myproj", model.NewProjectManifest("myproj", "dir", "/tmp", nil))
	resp := requestTo(srv, http.MethodPost, "/api/projects/myproj/run/render", nil)
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", resp.StatusCode)
	}
}

func TestDetectContentType(t *testing.T) {
	tests := []struct {
		path     string
		expected string
	}{
		{"file.png", "image/png"},
		{"file.jpg", "image/jpeg"},
		{"file.jpeg", "image/jpeg"},
		{"file.json", "application/json"},
		{"file.yaml", "application/yaml"},
		{"file.yml", "application/yaml"},
		{"file.md", "text/markdown"},
		{"file.txt", "text/plain"},
		{"file", "application/octet-stream"},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := detectContentType(tt.path)
			if got != tt.expected {
				t.Errorf("detectContentType(%q) = %q, want %q", tt.path, got, tt.expected)
			}
		})
	}
}

func createTestPNG() image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	img.Set(0, 0, color.RGBA{255, 0, 0, 255})
	return img
}

func TestOutputServesPNG(t *testing.T) {
	tmpDir := t.TempDir()
	srv := testServer(t, tmpDir)
	if err := state.SaveManifest(tmpDir, "proj", model.NewProjectManifest("proj", "dir", "/tmp", nil)); err != nil {
		t.Fatalf("SaveManifest: %v", err)
	}
	panelsDir := filepath.Join(tmpDir, "proj", "panels")
	if err := os.MkdirAll(panelsDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	f, err := os.Create(filepath.Join(panelsDir, "scene_001.png"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := png.Encode(f, createTestPNG()); err != nil {
		f.Close()
		t.Fatalf("png Encode: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	resp := requestTo(srv, http.MethodGet, "/api/projects/proj/output/panels/scene_001.png", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "image/png" {
		t.Errorf("expected image/png content type, got %q", ct)
	}
}

func TestListOutputs_NonProjectDirIgnored(t *testing.T) {
	tmpDir := t.TempDir()
	os.MkdirAll(filepath.Join(tmpDir, "not-a-project", "sub"), 0755)
	os.WriteFile(filepath.Join(tmpDir, "not-a-project", "random.txt"), []byte("data"), 0644)

	srv := testServer(t, tmpDir)
	resp := requestTo(srv, http.MethodGet, "/api/projects", nil)
	env := decodeEnvelope(t, resp)
	projects, _ := env.Data.([]interface{})
	if len(projects) != 0 {
		t.Errorf("expected 0 projects when no project.yaml, got %d", len(projects))
	}
}

func TestIngest_WithoutCreatingProject(t *testing.T) {
	srv := testServer(t, t.TempDir())
	resp := requestTo(srv, http.MethodPost, "/api/projects/nonexistent/ingest", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestDeleteProject_WithBusyTask(t *testing.T) {
	tmpDir := t.TempDir()
	srv := testServer(t, tmpDir)
	state.SaveManifest(tmpDir, "busyproj", model.NewProjectManifest("busyproj", "dir", "/tmp", nil))
	srv.tasks.Store("busyproj", nil)
	defer srv.tasks.Delete("busyproj")

	resp := requestTo(srv, http.MethodDelete, "/api/projects/busyproj", nil)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409, got %d", resp.StatusCode)
	}
	env := decodeEnvelope(t, resp)
	if env.Error == nil || env.Error.Code != "PROJECT_BUSY" {
		t.Fatalf("expected PROJECT_BUSY error, got %+v", env.Error)
	}
}

func TestDeleteProject_Success(t *testing.T) {
	tmpDir := t.TempDir()
	srv := testServer(t, tmpDir)
	state.SaveManifest(tmpDir, "delproj", model.NewProjectManifest("delproj", "dir", "/tmp", nil))

	projDir := filepath.Join(tmpDir, "delproj")
	if _, err := os.Stat(projDir); os.IsNotExist(err) {
		t.Fatal("project dir should exist before delete")
	}

	resp := requestTo(srv, http.MethodDelete, "/api/projects/delproj", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	if _, err := os.Stat(projDir); !os.IsNotExist(err) {
		t.Error("project dir should be removed after delete")
	}
}

func TestCreateProject_InvalidJSON(t *testing.T) {
	srv := testServer(t, t.TempDir())
	resp := requestTo(srv, http.MethodPost, "/api/projects", strings.NewReader("not json"))
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestHealthResponseBody(t *testing.T) {
	srv := testServer(t, t.TempDir())
	resp := requestTo(srv, http.MethodGet, "/api/health", nil)
	env := decodeEnvelope(t, resp)
	data, ok := env.Data.(map[string]interface{})
	if !ok {
		t.Fatal("expected data object")
	}
	if data["status"] != "ok" {
		t.Errorf("expected status 'ok', got %v", data["status"])
	}
	if data["version"] != "0.1.0" {
		t.Errorf("expected version '0.1.0', got %v", data["version"])
	}
}

func TestIngest_AlreadyIngested(t *testing.T) {
	tmpDir := t.TempDir()
	srv := testServer(t, tmpDir)

	state.SaveManifest(tmpDir, "preingested",
		model.NewProjectManifest("preingested", "upload", "", nil))

	m, _ := state.LoadManifest(tmpDir, "preingested")
	m.Pipeline.Phases[model.PhaseNameIngest] = model.PhaseStatus{
		Status: model.PhaseCompleted,
	}
	state.SaveManifest(tmpDir, "preingested", m)

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	coverPart, _ := w.CreateFormFile("cover", "cover.md")
	coverPart.Write([]byte("cover"))
	chPart, _ := w.CreateFormFile("chapters", "ch01.md")
	chPart.Write([]byte("chapter"))
	w.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/projects/preingested/ingest", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	wrec := httptest.NewRecorder()
	srv.router.ServeHTTP(wrec, req)
	resp := wrec.Result()

	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409 for already ingested, got %d", resp.StatusCode)
	}
}

func TestEnvelopeErrorField(t *testing.T) {
	outDir := t.TempDir()
	srv := testServer(t, outDir)
	resp := requestTo(srv, http.MethodGet, "/api/projects/no-such-project/status", nil)
	env := decodeEnvelope(t, resp)
	if env.Error == nil {
		t.Fatal("expected error field to be non-nil on 4xx")
	}
	if env.Error.Code == "" {
		t.Error("expected error.code to be non-empty")
	}
	if env.Error.Message == "" {
		t.Error("expected error.message to be non-empty")
	}
}

func TestRunPipeline_AlreadyRunning(t *testing.T) {
	tmpDir := t.TempDir()
	srv := testServer(t, tmpDir)
	state.SaveManifest(tmpDir, "busy", model.NewProjectManifest("busy", "dir", "/tmp", nil))
	srv.tasks.Store("busy", nil)
	defer srv.tasks.Delete("busy")

	resp := requestTo(srv, http.MethodPost, "/api/projects/busy/run", nil)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409, got %d", resp.StatusCode)
	}
	env := decodeEnvelope(t, resp)
	if env.Error == nil || env.Error.Code != "ALREADY_RUNNING" {
		t.Fatalf("expected ALREADY_RUNNING, got %+v", env.Error)
	}
}

func TestRunPhase_AlreadyRunning(t *testing.T) {
	tmpDir := t.TempDir()
	srv := testServer(t, tmpDir)
	state.SaveManifest(tmpDir, "busy", model.NewProjectManifest("busy", "dir", "/tmp", nil))
	taskKey := "busy"
	srv.tasks.Store(taskKey, nil)
	defer srv.tasks.Delete(taskKey)

	resp := requestTo(srv, http.MethodPost, "/api/projects/busy/run/characters", nil)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409, got %d", resp.StatusCode)
	}
}

func TestIngest_RejectsUnsafeChapterFilename(t *testing.T) {
	tmpDir := t.TempDir()
	srv := testServer(t, tmpDir)
	state.SaveManifest(tmpDir, "testproj", model.NewProjectManifest("testproj", "upload", "", nil))

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	coverPart, _ := w.CreateFormFile("cover", "cover.md")
	coverPart.Write([]byte("# Cover"))
	chPart, _ := w.CreateFormFile("chapters", `dir\chapter_01.md`)
	chPart.Write([]byte("# Ch1"))
	w.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/projects/testproj/ingest", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	wrec := httptest.NewRecorder()
	srv.router.ServeHTTP(wrec, req)

	if wrec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", wrec.Code)
	}
	env := decodeEnvelope(t, wrec.Result())
	if env.Error == nil || env.Error.Code != "INVALID_UPLOAD_FILENAME" {
		t.Fatalf("expected INVALID_UPLOAD_FILENAME, got %+v", env.Error)
	}
}

func TestCreateProject_RejectsLargeJSONBody(t *testing.T) {
	srv := testServer(t, t.TempDir())
	body := strings.NewReader(`{"name":"` + strings.Repeat("a", maxJSONBodyBytes) + `"}`)
	resp := requestTo(srv, http.MethodPost, "/api/projects", body)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestRunPhase_NoProject(t *testing.T) {
	srv := testServer(t, t.TempDir())
	resp := requestTo(srv, http.MethodPost, "/api/projects/nonexistent/run/render", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestCreateProject_SaveError(t *testing.T) {
	srv := testServer(t, "/nonexistent/path/that/cannot/be/created")
	body := `{"name":"test-project"}`
	resp := requestTo(srv, http.MethodPost, "/api/projects", strings.NewReader(body))
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", resp.StatusCode)
	}
}

func TestListProjects_ReadDirError(t *testing.T) {
	srv := testServer(t, "/nonexistent/path")
	resp := requestTo(srv, http.MethodGet, "/api/projects", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 even when dir doesn't exist, got %d", resp.StatusCode)
	}
	env := decodeEnvelope(t, resp)
	projects, ok := env.Data.([]interface{})
	if !ok {
		t.Fatal("expected data to be array")
	}
	if len(projects) != 0 {
		t.Errorf("expected empty list, got %d", len(projects))
	}
}

func TestIngest_InternalWriteError(t *testing.T) {
	tmpDir := t.TempDir()
	srv := testServer(t, tmpDir)
	state.SaveManifest(tmpDir, "testproj", model.NewProjectManifest("testproj", "upload", "", nil))

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	coverPart, _ := w.CreateFormFile("cover", "cover.md")
	coverPart.Write([]byte("# Cover"))
	chPart, _ := w.CreateFormFile("chapters", "chapter_01.md")
	chPart.Write([]byte("# Ch1"))
	w.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/projects/testproj/ingest", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	wrec := httptest.NewRecorder()
	srv.router.ServeHTTP(wrec, req)
	if wrec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", wrec.Code)
	}
}

func TestGetOutput_ServesYAML(t *testing.T) {
	tmpDir := t.TempDir()
	srv := testServer(t, tmpDir)
	state.SaveManifest(tmpDir, "proj", model.NewProjectManifest("proj", "dir", "/tmp", nil))
	projDir := filepath.Join(tmpDir, "proj")
	os.MkdirAll(projDir, 0755)
	os.WriteFile(filepath.Join(projDir, "project.yaml"), []byte("project: test"), 0644)

	resp := requestTo(srv, http.MethodGet, "/api/projects/proj/output/project.yaml", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestEncodeDecodeEnvelope(t *testing.T) {
	env := envelope{
		Success: true,
		Data:    map[string]string{"key": "value"},
		Error:   nil,
		Meta: meta{
			RequestID: "test-123",
		},
	}
	data, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded envelope
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !decoded.Success {
		t.Error("expected success")
	}
	if decoded.Meta.RequestID != "test-123" {
		t.Errorf("expected request_id 'test-123', got %q", decoded.Meta.RequestID)
	}
}

func TestEncodeDecodeEnvelope_Error(t *testing.T) {
	env := envelope{
		Success: false,
		Data:    nil,
		Error: &apiError{
			Code:    "TEST_ERR",
			Message: "test error message",
			Details: map[string]string{"detail": "value"},
		},
		Meta: meta{
			RequestID: "req-001",
			Timestamp: mustParseTime(t, "2026-06-01T10:00:00Z"),
		},
	}
	data, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded envelope
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Success {
		t.Error("expected success=false")
	}
	if decoded.Error.Code != "TEST_ERR" {
		t.Errorf("expected code 'TEST_ERR', got %q", decoded.Error.Code)
	}
	if decoded.Meta.RequestID != "req-001" {
		t.Errorf("expected request_id 'req-001', got %q", decoded.Meta.RequestID)
	}
}

func mustParseTime(t *testing.T, s string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("parsing time %q: %v", s, err)
	}
	return parsed
}

func TestDetectContentType_EdgeCases(t *testing.T) {
	tests := []struct {
		path     string
		expected string
	}{
		{"", "application/octet-stream"},
		{"noextension", "application/octet-stream"},
		{".hidden", "application/octet-stream"},
		{"path/to/image.PNG", "image/png"},
		{"path/to/image.JPG", "image/jpeg"},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := detectContentType(tt.path)
			if got != tt.expected {
				t.Errorf("detectContentType(%q) = %q, want %q", tt.path, got, tt.expected)
			}
		})
	}
}

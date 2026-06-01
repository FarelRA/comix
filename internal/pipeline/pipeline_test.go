package pipeline

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
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/FarelRA/comix/internal/config"
	"github.com/FarelRA/comix/internal/imagegen"
	"github.com/FarelRA/comix/internal/llm"
	"github.com/FarelRA/comix/internal/model"
	"github.com/FarelRA/comix/internal/state"
	"github.com/FarelRA/comix/internal/storage"
)

func TestIngest_FromBookDir(t *testing.T) {
	tmpDir := t.TempDir()
	outputDir := filepath.Join(tmpDir, "output")

	createTestNovel(t, tmpDir)

	cfg := testConfig(outputDir)
	p := NewPipeline(cfg, nil, nil)

	ctx := context.Background()
	manifest, err := p.Ingest(ctx, IngestSource{BookDir: filepath.Join(tmpDir, "novel")})
	if err != nil {
		t.Fatalf("Ingest failed: %v", err)
	}

	if manifest.Project.Name != "novel" {
		t.Errorf("expected project name 'novel', got %q", manifest.Project.Name)
	}

	if len(manifest.Project.Chapters) != 2 {
		t.Fatalf("expected 2 chapters, got %d", len(manifest.Project.Chapters))
	}

	if manifest.Project.Chapters[0].ID != "chapter_01" {
		t.Errorf("expected first chapter ID 'chapter_01', got %q", manifest.Project.Chapters[0].ID)
	}

	if manifest.Project.Chapters[0].WordCount == 0 {
		t.Error("expected word count > 0 for first chapter")
	}

	rawDir := storage.RawDir(outputDir, "novel")
	coverPath := filepath.Join(rawDir, "cover.md")
	if _, err := os.Stat(coverPath); os.IsNotExist(err) {
		t.Error("cover.md was not copied to raw dir")
	}

	ch01Path := filepath.Join(rawDir, "chapter_01.md")
	if _, err := os.Stat(ch01Path); os.IsNotExist(err) {
		t.Error("chapter_01.md was not copied to raw dir")
	}

	_, err = state.LoadManifest(outputDir, "novel")
	if err != nil {
		t.Errorf("manifest should exist after ingest: %v", err)
	}
}

func TestIngest_FromExplicitPaths(t *testing.T) {
	tmpDir := t.TempDir()
	outputDir := filepath.Join(tmpDir, "output")

	createTestNovel(t, tmpDir)
	novelDir := filepath.Join(tmpDir, "novel")

	cfg := testConfig(outputDir)
	p := NewPipeline(cfg, nil, nil)

	ctx := context.Background()
	manifest, err := p.Ingest(ctx, IngestSource{
		Cover:    filepath.Join(novelDir, "cover.md"),
		Chapters: []string{filepath.Join(novelDir, "chapter_01.md"), filepath.Join(novelDir, "chapter_02.md")},
	})
	if err != nil {
		t.Fatalf("Ingest failed: %v", err)
	}

	if len(manifest.Project.Chapters) != 2 {
		t.Fatalf("expected 2 chapters, got %d", len(manifest.Project.Chapters))
	}
}

func TestIngest_NoChaptersError(t *testing.T) {
	tmpDir := t.TempDir()
	outputDir := filepath.Join(tmpDir, "output")

	cfg := testConfig(outputDir)
	p := NewPipeline(cfg, nil, nil)

	ctx := context.Background()
	_, err := p.Ingest(ctx, IngestSource{BookDir: tmpDir})
	if err == nil {
		t.Fatal("expected error for directory with no chapters")
	}
}

func TestManifestPersistence(t *testing.T) {
	tmpDir := t.TempDir()
	outputDir := filepath.Join(tmpDir, "output")

	createTestNovel(t, tmpDir)
	novelDir := filepath.Join(tmpDir, "novel")

	cfg := testConfig(outputDir)
	p := NewPipeline(cfg, nil, nil)

	ctx := context.Background()

	manifest, err := p.Ingest(ctx, IngestSource{BookDir: novelDir})
	if err != nil {
		t.Fatalf("Ingest failed: %v", err)
	}

	loaded, err := state.LoadManifest(outputDir, "novel")
	if err != nil {
		t.Fatalf("Loading manifest: %v", err)
	}

	if loaded.Project.Name != manifest.Project.Name {
		t.Errorf("persisted name %q != original %q", loaded.Project.Name, manifest.Project.Name)
	}

	if loaded.Pipeline.Status != model.PhaseIdle {
		t.Errorf("expected status 'idle', got %q", loaded.Pipeline.Status)
	}
}

func TestGenerateSheets_Success(t *testing.T) {
	tmpDir := t.TempDir()
	outputDir := filepath.Join(tmpDir, "output")

	cfg := testConfig(outputDir)
	mockSrv := newMockImageServer(t)
	imgClient := imagegen.NewClient("sk-test", "gpt-image-2", "medium").
		WithBaseURL(mockSrv.URL).
		WithHTTPClient(mockSrv.Client())

	p := NewPipeline(cfg, nil, imgClient)

	manifest := createTestManifest(t, outputDir)
	note := createTestCharacterNote()

	if err := p.GenerateSheets(context.Background(), manifest, note); err != nil {
		t.Fatalf("GenerateSheets failed: %v", err)
	}

	sheetPath := filepath.Join(storage.SheetsDir(outputDir, "test-proj"), "alice_3x2.png")
	if _, err := os.Stat(sheetPath); os.IsNotExist(err) {
		t.Error("sheet file was not created")
	}
}

func TestGenerateSheets_NoImgGen(t *testing.T) {
	cfg := testConfig(t.TempDir())
	p := NewPipeline(cfg, nil, nil)
	err := p.GenerateSheets(context.Background(), nil, &model.CharacterNote{})
	if err == nil {
		t.Fatal("expected error with nil imgGen")
	}
}

func TestGeneratePoses_Success(t *testing.T) {
	tmpDir := t.TempDir()
	outputDir := filepath.Join(tmpDir, "output")

	cfg := testConfig(outputDir)
	mockSrv := newMockImageServer(t)
	imgClient := imagegen.NewClient("sk-test", "gpt-image-2", "medium").
		WithBaseURL(mockSrv.URL).
		WithHTTPClient(mockSrv.Client())

	p := NewPipeline(cfg, nil, imgClient)

	manifest := createTestManifest(t, outputDir)
	note := createTestCharacterNote()

	sheetingDir := storage.SheetsDir(outputDir, "test-proj")
	if err := os.MkdirAll(sheetingDir, 0755); err != nil {
		t.Fatalf("mkdir sheets: %v", err)
	}
	sheetPath := filepath.Join(sheetingDir, "alice_3x2.png")
	if err := saveTestPNG(sheetPath, 100, 100); err != nil {
		t.Fatalf("creating test sheet: %v", err)
	}

	if err := p.GeneratePoses(context.Background(), manifest, note); err != nil {
		t.Fatalf("GeneratePoses failed: %v", err)
	}

	posePath := filepath.Join(storage.PosesDir(outputDir, "test-proj"), "alice_5x5.png")
	if _, err := os.Stat(posePath); os.IsNotExist(err) {
		t.Error("pose file was not created")
	}
}

func TestGeneratePoses_MissingSheet(t *testing.T) {
	tmpDir := t.TempDir()
	outputDir := filepath.Join(tmpDir, "output")

	cfg := testConfig(outputDir)
	mockSrv := newMockImageServer(t)
	imgClient := imagegen.NewClient("sk-test", "gpt-image-2", "medium").
		WithBaseURL(mockSrv.URL).
		WithHTTPClient(mockSrv.Client())

	p := NewPipeline(cfg, nil, imgClient)
	manifest := createTestManifest(t, outputDir)
	note := createTestCharacterNote()

	err := p.GeneratePoses(context.Background(), manifest, note)
	if err == nil {
		t.Fatal("expected error when sheet file is missing")
	}
}

func TestRenderScenes_FirstSceneGenerate(t *testing.T) {
	tmpDir := t.TempDir()
	outputDir := filepath.Join(tmpDir, "output")

	mockSrv := newMockImageServer(t)
	defer mockSrv.Close()
	imgClient := imagegen.NewClient("sk-test", "gpt-image-2", "medium").
		WithBaseURL(mockSrv.URL).
		WithHTTPClient(mockSrv.Client())

	p := NewPipeline(testConfig(outputDir), nil, imgClient)

	manifest := createTestManifest(t, outputDir)
	note := createTestCharacterNote()
	scenes := createTestSceneList()

	if err := p.RenderScenes(context.Background(), manifest, note, scenes); err != nil {
		t.Fatalf("RenderScenes failed: %v", err)
	}

	for _, s := range scenes.Scenes {
		panelPath := filepath.Join(storage.PanelsDir(outputDir, "test-proj"), s.Key()+".png")
		if _, err := os.Stat(panelPath); os.IsNotExist(err) {
			t.Errorf("panel file %s was not created", s.Key())
		}
	}
}

func TestRenderScenes_SequentialEdit(t *testing.T) {
	tmpDir := t.TempDir()
	outputDir := filepath.Join(tmpDir, "output")

	callCount := 0
	mockSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			if r.URL.Path != "/images/generations" {
				t.Errorf("expected first call to /images/generations, got %s", r.URL.Path)
			}
		} else {
			if r.URL.Path != "/images/edits" {
				t.Errorf("expected subsequent calls to /images/edits, got %s", r.URL.Path)
			}
		}
		writeMockImageResponse(w, t)
	}))
	defer mockSrv.Close()

	imgClient := imagegen.NewClient("sk-test", "gpt-image-2", "medium").
		WithBaseURL(mockSrv.URL).
		WithHTTPClient(mockSrv.Client())

	p := NewPipeline(testConfig(outputDir), nil, imgClient)

	manifest := createTestManifest(t, outputDir)
	note := createTestCharacterNote()
	scenes := createTestSceneList()

	if err := p.RenderScenes(context.Background(), manifest, note, scenes); err != nil {
		t.Fatalf("RenderScenes failed: %v", err)
	}

	if callCount != 3 {
		t.Errorf("expected 3 API calls (1 generate + 2 edits), got %d", callCount)
	}
}

func TestRenderScenes_ResumeSkipsExisting(t *testing.T) {
	tmpDir := t.TempDir()
	outputDir := filepath.Join(tmpDir, "output")

	callCount := 0
	mockSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		writeMockImageResponse(w, t)
	}))
	defer mockSrv.Close()

	imgClient := imagegen.NewClient("sk-test", "gpt-image-2", "medium").
		WithBaseURL(mockSrv.URL).
		WithHTTPClient(mockSrv.Client())

	p := NewPipeline(testConfig(outputDir), nil, imgClient)

	manifest := createTestManifest(t, outputDir)
	note := createTestCharacterNote()
	scenes := createTestSceneList()

	panelsDir := storage.PanelsDir(outputDir, "test-proj")
	if err := os.MkdirAll(panelsDir, 0755); err != nil {
		t.Fatalf("mkdir panels: %v", err)
	}
	if err := saveTestPNG(filepath.Join(panelsDir, scenes.Scenes[0].Key()+".png"), 50, 50); err != nil {
		t.Fatalf("creating fake panel: %v", err)
	}

	if err := p.RenderScenes(context.Background(), manifest, note, scenes); err != nil {
		t.Fatalf("RenderScenes failed: %v", err)
	}

	if callCount != 2 {
		t.Errorf("expected 2 API calls (skipping first scene), got %d", callCount)
	}
}

func TestPipelineRun_WithAllPhases(t *testing.T) {
	tmpDir := t.TempDir()
	outputDir := filepath.Join(tmpDir, "output")

	createTestNovel(t, tmpDir)
	novelDir := filepath.Join(tmpDir, "novel")

	callCount := 0
	mockSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		writeMockImageResponse(w, t)
	}))
	defer mockSrv.Close()

	cfg := &config.Config{
		OpenAI: config.OpenAIConfig{
			APIKey: "sk-test",
			Image:  config.ImageConfig{},
		},
		Pipeline: config.PipelineConfig{
			OutputDir:           outputDir,
			ChapterPattern:      "chapter_*.md",
			CoverFilename:       "cover.md",
			MaxConcurrentSheets: 2,
			MaxConcurrentPoses:  2,
		},
	}

	imgClient := imagegen.NewClient("sk-test", "gpt-image-2", "medium").
		WithBaseURL(mockSrv.URL).
		WithHTTPClient(mockSrv.Client())

	p := NewPipeline(cfg, nil, imgClient)

	ctx := context.Background()
	err := p.Run(ctx, "novel", IngestSource{BookDir: novelDir}, []string{model.PhaseNameIngest}, false)
	if err != nil {
		t.Fatalf("Pipeline Run failed at ingest: %v", err)
	}

	manifest, err := state.LoadManifest(outputDir, "novel")
	if err != nil {
		t.Fatalf("Loading manifest: %v", err)
	}

	if manifest.Pipeline.Phases["ingest"].Status != model.PhaseCompleted {
		t.Errorf("expected ingest completed, got %q", manifest.Pipeline.Phases["ingest"].Status)
	}
	if manifest.Pipeline.Status == model.PhaseCompleted {
		t.Errorf("single-phase run should not mark entire pipeline completed")
	}
}

func TestIngest_ExistingProjectRequiresAllowExisting(t *testing.T) {
	tmpDir := t.TempDir()
	outputDir := filepath.Join(tmpDir, "output")
	createTestNovel(t, tmpDir)
	novelDir := filepath.Join(tmpDir, "novel")

	p := NewPipeline(testConfig(outputDir), nil, nil)
	if _, err := p.Ingest(context.Background(), IngestSource{BookDir: novelDir}); err != nil {
		t.Fatalf("first ingest failed: %v", err)
	}
	if _, err := p.Ingest(context.Background(), IngestSource{BookDir: novelDir}); err == nil {
		t.Fatal("expected duplicate ingest to fail")
	}
	if _, err := p.Ingest(context.Background(), IngestSource{BookDir: novelDir, AllowExisting: true}); err != nil {
		t.Fatalf("expected duplicate ingest with AllowExisting to succeed: %v", err)
	}
}

func TestPipelineRun_Resume(t *testing.T) {
	tmpDir := t.TempDir()
	outputDir := filepath.Join(tmpDir, "output")

	createTestNovel(t, tmpDir)
	novelDir := filepath.Join(tmpDir, "novel")

	cfg := testConfig(outputDir)
	p := NewPipeline(cfg, nil, nil)

	ctx := context.Background()

	if err := p.Run(ctx, "novel", IngestSource{BookDir: novelDir}, []string{model.PhaseNameIngest}, false); err != nil {
		t.Fatalf("First run: %v", err)
	}

	if err := p.Run(ctx, "novel", IngestSource{}, []string{model.PhaseNameIngest}, true); err != nil {
		t.Fatalf("Resume should skip completed phase: %v", err)
	}

	manifest, err := state.LoadManifest(outputDir, "novel")
	if err != nil {
		t.Fatalf("Load manifest: %v", err)
	}

	if manifest.Pipeline.Phases["ingest"].Status != model.PhaseCompleted {
		t.Errorf("expected ingest still completed, got %q", manifest.Pipeline.Phases["ingest"].Status)
	}
}

// --- helpers ---

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
	}
}

func createTestNovel(t *testing.T, baseDir string) {
	t.Helper()
	novelDir := filepath.Join(baseDir, "novel")
	if err := os.MkdirAll(novelDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	coverContent := `# Alice in Wonderland

A story about a young girl who falls down a rabbit hole.
`
	if err := os.WriteFile(filepath.Join(novelDir, "cover.md"), []byte(coverContent), 0644); err != nil {
		t.Fatalf("writing cover: %v", err)
	}

	ch01 := `# Chapter 1: Down the Rabbit-Hole

Alice was beginning to get very tired of sitting by her sister on the bank.
`
	if err := os.WriteFile(filepath.Join(novelDir, "chapter_01.md"), []byte(ch01), 0644); err != nil {
		t.Fatalf("writing ch01: %v", err)
	}

	ch02 := `# Chapter 2: The Pool of Tears

Curiouser and curiouser! cried Alice.
`
	if err := os.WriteFile(filepath.Join(novelDir, "chapter_02.md"), []byte(ch02), 0644); err != nil {
		t.Fatalf("writing ch02: %v", err)
	}
}

func createTestManifest(t *testing.T, outputDir string) *model.ProjectManifest {
	t.Helper()
	manifest := model.NewProjectManifest("test-proj", "directory", "/tmp",
		[]model.ChapterMeta{
			{ID: "chapter_01", Filename: "chapter_01.md", Title: "Chapter 1", WordCount: 50},
			{ID: "chapter_02", Filename: "chapter_02.md", Title: "Chapter 2", WordCount: 50},
		},
	)
	if err := state.SaveManifest(outputDir, "test-proj", manifest); err != nil {
		t.Fatalf("saving test manifest: %v", err)
	}
	return manifest
}

func createTestCharacterNote() *model.CharacterNote {
	return &model.CharacterNote{
		Schema: "comix/character-note/v1",
		Characters: []model.Character{
			{
				Name:                "Alice",
				PhysicalDescription: "Young girl, blue eyes, blonde hair, blue dress",
				FirstChapter:        "chapter_01",
				ChaptersSeen:        []string{"chapter_01", "chapter_02"},
			},
		},
	}
}

func createTestSceneList() *model.SceneList {
	return &model.SceneList{
		Schema:    "comix/scene-list/v1",
		ProjectID: "test-proj",
		Scenes: []model.Scene{
			{
				Chapter:           "chapter_01",
				Sequence:          1,
				GlobalSequence:    1,
				Description:       "Alice sits on a riverbank.",
				CharactersPresent: []string{"Alice"},
				Location:          "riverside",
				Mood:              "peaceful",
				VisualCues:        []string{"sunny", "green grass"},
			},
			{
				Chapter:           "chapter_01",
				Sequence:          2,
				GlobalSequence:    2,
				Description:       "A white rabbit runs past.",
				CharactersPresent: []string{"Alice"},
				Location:          "riverside",
				Mood:              "sudden",
				VisualCues:        []string{"rabbit", "pocket watch"},
			},
			{
				Chapter:           "chapter_02",
				Sequence:          1,
				GlobalSequence:    3,
				Description:       "Alice grows very tall.",
				CharactersPresent: []string{"Alice"},
				Location:          "hallway",
				Mood:              "confused",
				VisualCues:        []string{"tall ceiling", "tiny door"},
			},
		},
	}
}

func newMockImageServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeMockImageResponse(w, t)
	}))
}

func writeMockImageResponse(w http.ResponseWriter, t *testing.T) {
	w.Header().Set("Content-Type", "application/json")
	img := createMockImage(100, 100)
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encoding mock png: %v", err)
	}
	b64 := base64.StdEncoding.EncodeToString(buf.Bytes())

	resp := map[string]any{
		"created": time.Now().Unix(),
		"data": []map[string]any{
			{"b64_json": b64, "revised_prompt": "A test image."},
		},
	}
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		t.Fatalf("encoding response: %v", err)
	}
}

func createMockImage(w, h int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for x := 0; x < w; x++ {
		for y := 0; y < h; y++ {
			img.Set(x, y, color.RGBA{100, 150, 200, 255})
		}
	}
	return img
}

func saveTestPNG(path string, w, h int) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, createMockImage(w, h))
}

// --- standalone function tests ---

func TestChapterIDFromFilename(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"chapter_01.md", "chapter_01"},
		{"Chapter 02.MD", "chapter_02"},
		{"cover.md", "cover"},
		{"my_chapter.txt", "my_chapter"},
		{"README", "readme"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := chapterIDFromFilename(tt.input)
			if got != tt.expected {
				t.Errorf("chapterIDFromFilename(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestChapterTitleFromContent(t *testing.T) {
	tests := []struct {
		name       string
		content    string
		fallbackID string
		expected   string
	}{
		{"h1 title", "# Chapter 1\n\nContent", "chapter_01", "Chapter 1"},
		{"frontmatter title", "title: \"My Title\"\n\nContent", "chapter_01", "My Title"},
		{"frontmatter single quotes", "title: 'My Title'\n\nContent", "chapter_01", "My Title"},
		{"no title found", "Just content", "chapter_01", "chapter_01"},
		{"empty content", "", "chapter_01", "chapter_01"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := chapterTitleFromContent(tt.content, tt.fallbackID)
			if got != tt.expected {
				t.Errorf("chapterTitleFromContent() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestWordCount(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"", 0},
		{"hello", 1},
		{"hello world", 2},
		{"  spaced  out  ", 2},
		{"hello\nworld\nfoo", 3},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := wordCount(tt.input)
			if got != tt.expected {
				t.Errorf("wordCount(%q) = %d, want %d", tt.input, got, tt.expected)
			}
		})
	}
}

func TestBuildCharacterIndex(t *testing.T) {
	note := &model.CharacterNote{
		Characters: []model.Character{
			{Name: "Alice", PhysicalDescription: "young girl"},
			{Name: "White Rabbit", PhysicalDescription: "white rabbit"},
		},
	}
	idx := buildCharacterIndex(note)
	if len(idx) != 2 {
		t.Fatalf("expected 2 characters, got %d", len(idx))
	}
	if idx["alice"].Name != "Alice" {
		t.Errorf("expected 'Alice', got %q", idx["alice"].Name)
	}
	if idx["white rabbit"].PhysicalDescription != "white rabbit" {
		t.Errorf("expected 'white rabbit', got %q", idx["rabbit"].PhysicalDescription)
	}
}

func TestBuildCharacterIndex_Empty(t *testing.T) {
	note := &model.CharacterNote{}
	idx := buildCharacterIndex(note)
	if len(idx) != 0 {
		t.Errorf("expected empty index, got %d", len(idx))
	}
}

func TestBuildSceneDescription(t *testing.T) {
	p := &Pipeline{}
	scene := model.Scene{
		Description: "Alice sits by the river.",
		Location:    "riverside",
		Mood:        "peaceful",
		VisualCues:  []string{"sunny", "green grass"},
		Dialogue:    []model.DialogueLine{{Speaker: "alice", Text: "Hello!"}},
	}
	desc := p.buildSceneDescription(scene)
	if !strings.Contains(desc, "Alice sits by the river") {
		t.Error("expected description in output")
	}
	if !strings.Contains(desc, "Mood: peaceful") {
		t.Error("expected mood in output")
	}
	if !strings.Contains(desc, "Visual cues: sunny, green grass") {
		t.Error("expected visual cues in output")
	}
	if !strings.Contains(desc, "alice: \"Hello!\"") {
		t.Error("expected dialogue in output")
	}
}

func TestBuildSceneDescription_NoMoodOrCues(t *testing.T) {
	p := &Pipeline{}
	scene := model.Scene{
		Description: "Just a description.",
	}
	desc := p.buildSceneDescription(scene)
	if desc != "Just a description." {
		t.Errorf("expected just description, got %q", desc)
	}
}

func TestResolveSpeaker_DirectName(t *testing.T) {
	charIndex := map[string]model.Character{
		"alice": {Name: "Alice"},
	}
	name := resolveSpeaker("alice", charIndex)
	if name != "Alice" {
		t.Errorf("expected 'Alice', got %q", name)
	}
}

func TestResolveSpeaker_ByName(t *testing.T) {
	charIndex := map[string]model.Character{
		"alice": {Name: "Alice"},
	}
	name := resolveSpeaker("Alice", charIndex)
	if name != "Alice" {
		t.Errorf("expected 'Alice', got %q", name)
	}
}

func TestResolveSpeaker_ByAlias(t *testing.T) {
	charIndex := map[string]model.Character{
		"white rabbit": {Name: "White Rabbit", Aliases: []string{"Whitey"}},
	}
	name := resolveSpeaker("Whitey", charIndex)
	if name != "White Rabbit" {
		t.Errorf("expected 'White Rabbit', got %q", name)
	}
}

func TestResolveSpeaker_NotFound(t *testing.T) {
	charIndex := map[string]model.Character{
		"alice": {Name: "Alice"},
	}
	name := resolveSpeaker("nonexistent", charIndex)
	if name != "" {
		t.Errorf("expected empty string, got %q", name)
	}
}

func TestResolveSpeaker_EmptyIndex(t *testing.T) {
	charIndex := map[string]model.Character{}
	name := resolveSpeaker("alice", charIndex)
	if name != "" {
		t.Errorf("expected empty string, got %q", name)
	}
}

func TestBuildDialogueMap(t *testing.T) {
	charIndex := map[string]model.Character{
		"alice": {Name: "Alice"},
	}
	dialogue := []model.DialogueLine{
		{Speaker: "alice", Text: "Hello"},
		{Speaker: "alice", Text: "World"},
		{Speaker: "rabbit", Text: "Late!"},
	}
	m := buildDialogueMap(dialogue, charIndex)
	if len(m["alice"]) != 2 {
		t.Errorf("expected 2 lines for alice, got %d", len(m["alice"]))
	}
	if _, ok := m["rabbit"]; ok {
		t.Error("rabbit should not be in map (not in charIndex)")
	}
}

func TestBuildDialogueMap_Empty(t *testing.T) {
	m := buildDialogueMap(nil, map[string]model.Character{})
	if len(m) != 0 {
		t.Errorf("expected empty map, got %d", len(m))
	}
}

func TestBuildSceneCharacterRefs(t *testing.T) {
	p := &Pipeline{}
	charIndex := map[string]model.Character{
		"alice": {Name: "Alice", PhysicalDescription: "young girl"},
	}
	scene := model.Scene{
		CharactersPresent: []string{"Alice"},
	}
	refs := p.buildSceneCharacterRefs(scene, charIndex, 0)
	if !strings.Contains(refs, "Alice") {
		t.Error("expected character name in refs")
	}
	if !strings.Contains(refs, "young girl") {
		t.Error("expected physical description in refs")
	}
}

func TestBuildSceneCharacterRefs_NoCharacters(t *testing.T) {
	p := &Pipeline{}
	scene := model.Scene{}
	refs := p.buildSceneCharacterRefs(scene, map[string]model.Character{}, 0)
	if refs != "No characters in this scene." {
		t.Errorf("expected 'No characters' message, got %q", refs)
	}
}

func TestBuildSceneCharacterRefs_MissingCharacter(t *testing.T) {
	p := &Pipeline{}
	charIndex := map[string]model.Character{}
	scene := model.Scene{
		CharactersPresent: []string{"Alice"},
	}
	refs := p.buildSceneCharacterRefs(scene, charIndex, 0)
	if !strings.Contains(refs, "character reference not found") {
		t.Error("expected missing character warning")
	}
}

func TestBuildSceneCharacterRefs_WithDialogue(t *testing.T) {
	p := &Pipeline{}
	charIndex := map[string]model.Character{
		"alice": {Name: "Alice", PhysicalDescription: "young girl"},
	}
	scene := model.Scene{
		CharactersPresent: []string{"Alice"},
		Dialogue:          []model.DialogueLine{{Speaker: "Alice", Text: "Hello!"}},
	}
	refs := p.buildSceneCharacterRefs(scene, charIndex, 0)
	if !strings.Contains(refs, "Hello!") {
		t.Error("expected dialogue in refs")
	}
}

func TestBuildSceneCharacterRefs_WithImageStartIdx(t *testing.T) {
	p := &Pipeline{}
	charIndex := map[string]model.Character{
		"alice":        {Name: "Alice", PhysicalDescription: "young girl"},
		"white rabbit": {Name: "White Rabbit", PhysicalDescription: "white rabbit with waistcoat"},
	}

	scene := model.Scene{
		CharactersPresent: []string{"Alice", "White Rabbit"},
		Dialogue:          []model.DialogueLine{{Speaker: "Alice", Text: "Oh dear!"}},
	}

	refs := p.buildSceneCharacterRefs(scene, charIndex, 1)

	if !strings.Contains(refs, "[image 1, image 2]") {
		t.Error("expected alice to have [image 1, image 2] (2 refs since <=7 chars)")
	}
	if !strings.Contains(refs, "[image 3, image 4]") {
		t.Error("expected White Rabbit to have [image 3, image 4]")
	}
	if !strings.Contains(refs, "\"Oh dear!\"") {
		t.Error("expected dialogue in refs")
	}
}

func TestBuildSceneCharacterRefs_WithImageStartIdxManyChars(t *testing.T) {
	p := &Pipeline{}
	charIndex := map[string]model.Character{
		"alice":           {Name: "Alice", PhysicalDescription: "young girl"},
		"mad hatter":      {Name: "Mad Hatter", PhysicalDescription: "eccentric man"},
		"white rabbit":    {Name: "White Rabbit", PhysicalDescription: "white rabbit"},
		"cheshire cat":    {Name: "Cheshire Cat", PhysicalDescription: "striped cat"},
		"queen of hearts": {Name: "Queen of Hearts", PhysicalDescription: "imperious woman"},
		"king of hearts":  {Name: "King of Hearts", PhysicalDescription: "nervous man"},
		"march hare":      {Name: "March Hare", PhysicalDescription: "frantic hare"},
		"dormouse":        {Name: "Dormouse", PhysicalDescription: "sleepy mouse"},
	}

	scene := model.Scene{
		CharactersPresent: []string{"Alice", "Mad Hatter", "White Rabbit", "Cheshire Cat", "Queen of Hearts", "King of Hearts", "March Hare", "Dormouse"},
	}

	refs := p.buildSceneCharacterRefs(scene, charIndex, 2)

	if !strings.Contains(refs, "[image 2]") {
		t.Error("expected alice (first of 8 chars) to have [image 2] since offset=2 (>7 chars = 1 image each)")
	}
	if !strings.Contains(refs, "[image 9]") {
		t.Error("expected dormouse (last of 8 chars) to have [image 9] (2+8-1)")
	}
	if strings.Contains(refs, "[image 2, image") {
		t.Error("expected single image tags since >7 chars")
	}
}

func TestChaptersNeedingCharacterExtraction_AllChapters(t *testing.T) {
	p := &Pipeline{}
	chapters := []model.ChapterMeta{
		{ID: "ch1", Filename: "ch1.md"},
		{ID: "ch2", Filename: "ch2.md"},
	}
	manifest := &model.ProjectManifest{
		Project: model.ProjectMeta{Chapters: chapters},
	}

	// No resume, no note
	got := p.chaptersNeedingCharacterExtraction(manifest, nil, false)
	if len(got) != 2 {
		t.Errorf("expected 2 chapters, got %d", len(got))
	}

	// Resume but nil note
	got = p.chaptersNeedingCharacterExtraction(manifest, nil, true)
	if len(got) != 2 {
		t.Errorf("expected 2 chapters with nil note, got %d", len(got))
	}

	// Resume but empty note
	got = p.chaptersNeedingCharacterExtraction(manifest, &model.CharacterNote{}, true)
	if len(got) != 2 {
		t.Errorf("expected 2 chapters with empty note, got %d", len(got))
	}
}

func TestChaptersNeedingCharacterExtraction_Resume(t *testing.T) {
	p := &Pipeline{}
	chapters := []model.ChapterMeta{
		{ID: "ch1", Filename: "ch1.md"},
		{ID: "ch2", Filename: "ch2.md"},
		{ID: "ch3", Filename: "ch3.md"},
	}
	manifest := &model.ProjectManifest{
		Project: model.ProjectMeta{Chapters: chapters},
	}
	note := &model.CharacterNote{
		Characters: []model.Character{{Name: "Alice", ChaptersSeen: []string{"ch1"}}},
	}

	got := p.chaptersNeedingCharacterExtraction(manifest, note, true)
	if len(got) != 2 {
		t.Errorf("expected 2 remaining chapters, got %d", len(got))
	}
	if got[0].ID != "ch2" {
		t.Errorf("expected first to be ch2, got %q", got[0].ID)
	}
}

func TestChaptersNeedingCharacterExtraction_AllDone(t *testing.T) {
	p := &Pipeline{}
	chapters := []model.ChapterMeta{
		{ID: "ch1", Filename: "ch1.md"},
		{ID: "ch2", Filename: "ch2.md"},
	}
	manifest := &model.ProjectManifest{
		Project: model.ProjectMeta{Chapters: chapters},
	}
	note := &model.CharacterNote{
		Characters: []model.Character{{Name: "Alice", ChaptersSeen: []string{"ch1", "ch2"}}},
	}

	got := p.chaptersNeedingCharacterExtraction(manifest, note, true)
	if len(got) != 0 {
		t.Errorf("expected 0 remaining chapters, got %d", len(got))
	}
}

func TestChaptersNeedingCharacterExtraction_LastChapterNotFound(t *testing.T) {
	p := &Pipeline{}
	chapters := []model.ChapterMeta{
		{ID: "ch1", Filename: "ch1.md"},
		{ID: "ch2", Filename: "ch2.md"},
	}
	manifest := &model.ProjectManifest{
		Project: model.ProjectMeta{Chapters: chapters},
	}
	note := &model.CharacterNote{
		Characters: []model.Character{{Name: "Alice", ChaptersSeen: []string{"nonexistent"}}},
	}

	got := p.chaptersNeedingCharacterExtraction(manifest, note, true)
	if len(got) != 2 {
		t.Errorf("expected all chapters when last not found, got %d", len(got))
	}
}

func TestExtractProcessedChapters(t *testing.T) {
	p := &Pipeline{}
	sceneList := &model.SceneList{
		Scenes: []model.Scene{
			{Chapter: "ch1"},
			{Chapter: "ch1"},
			{Chapter: "ch2"},
		},
	}
	processed := p.extractProcessedChapters(sceneList)
	if !processed["ch1"] {
		t.Error("expected ch1 to be processed")
	}
	if !processed["ch2"] {
		t.Error("expected ch2 to be processed")
	}
	if processed["ch3"] {
		t.Error("expected ch3 not to be processed")
	}
}

func TestMaxGlobalSequence(t *testing.T) {
	p := &Pipeline{}
	sceneList := &model.SceneList{
		Scenes: []model.Scene{
			{GlobalSequence: 5},
			{GlobalSequence: 2},
			{GlobalSequence: 10},
		},
	}
	max := p.maxGlobalSequence(sceneList)
	if max != 10 {
		t.Errorf("expected 10, got %d", max)
	}
}

func TestMaxGlobalSequence_Empty(t *testing.T) {
	p := &Pipeline{}
	max := p.maxGlobalSequence(&model.SceneList{})
	if max != 0 {
		t.Errorf("expected 0 for empty scene list, got %d", max)
	}
}

func TestMustMarshalJSON(t *testing.T) {
	data := map[string]string{"key": "value"}
	result := mustMarshalJSON(data)
	if !strings.Contains(result, "key") || !strings.Contains(result, "value") {
		t.Errorf("expected json with key and value, got %s", result)
	}
}

func TestMustMarshalJSON_Channel(t *testing.T) {
	result := mustMarshalJSON(make(chan int))
	if result != "{}" {
		t.Errorf("expected '{}' for un-marshalable value, got %s", result)
	}
}

func TestPanelExists(t *testing.T) {
	p := &Pipeline{}
	dir := t.TempDir()

	if p.panelExists(filepath.Join(dir, "nonexistent.png")) {
		t.Error("expected false for nonexistent file")
	}

	path := filepath.Join(dir, "exists.png")
	if err := os.WriteFile(path, []byte("data"), 0644); err != nil {
		t.Fatalf("writing test file: %v", err)
	}
	if !p.panelExists(path) {
		t.Error("expected true for existing file")
	}
}

func TestBuildCharacterMessages(t *testing.T) {
	p := &Pipeline{}
	note := &model.CharacterNote{
		Schema: "comix/character-note/v1",
		Characters: []model.Character{
			{Name: "Alice", PhysicalDescription: "young girl"},
		},
	}
	messages := p.buildCharacterMessages("cover content", "chapter content", note)
	if len(messages) < 2 {
		t.Fatalf("expected at least 2 messages, got %d", len(messages))
	}
	if messages[0].Role != llm.RoleSystem {
		t.Errorf("expected first message to be system")
	}
	if messages[1].Role != llm.RoleUser {
		t.Errorf("expected second message to be user")
	}
	if !strings.Contains(messages[1].Content, "cover content") {
		t.Errorf("expected cover content in user message")
	}
	if !strings.Contains(messages[1].Content, "chapter content") {
		t.Errorf("expected chapter content in user message")
	}
}

func TestBuildCharacterMessages_NoExistingNote(t *testing.T) {
	p := &Pipeline{}
	messages := p.buildCharacterMessages("cover", "chapter", &model.CharacterNote{})
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages with empty note, got %d", len(messages))
	}
}

func TestBuildSceneMessages(t *testing.T) {
	p := &Pipeline{}
	note := &model.CharacterNote{
		Characters: []model.Character{
			{Name: "Alice"},
		},
	}
	messages := p.buildSceneMessages("cover content", "chapter content", note)
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(messages))
	}
	if messages[0].Role != llm.RoleSystem {
		t.Errorf("expected first message system")
	}
	if !strings.Contains(messages[1].Content, "cover content") {
		t.Errorf("expected cover in content")
	}
	if !strings.Contains(messages[1].Content, "Character Reference") {
		t.Errorf("expected Character Reference in content")
	}
}

func TestValidateCharacterRefs_Found(t *testing.T) {
	p := &Pipeline{}
	note := &model.CharacterNote{
		Characters: []model.Character{
			{Name: "Alice"},
		},
	}
	scene := &model.Scene{
		Chapter:           "ch1",
		Sequence:          1,
		CharactersPresent: []string{"Alice"},
	}
	p.validateCharacterRefs(scene, note, "ch1")
}

func TestValidateCharacterRefs_NotFound(t *testing.T) {
	p := &Pipeline{}
	note := &model.CharacterNote{
		Characters: []model.Character{
			{Name: "Alice"},
		},
	}
	scene := &model.Scene{
		Chapter:           "ch1",
		Sequence:          1,
		CharactersPresent: []string{"nonexistent"},
	}
	p.validateCharacterRefs(scene, note, "ch1")
}

func TestLoadImage(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.png")
	if err := saveTestPNG(path, 10, 10); err != nil {
		t.Fatalf("saving test png: %v", err)
	}
	img, err := loadImage(path)
	if err != nil {
		t.Fatalf("loadImage failed: %v", err)
	}
	if img.Bounds().Dx() != 10 || img.Bounds().Dy() != 10 {
		t.Errorf("expected 10x10 image, got %dx%d", img.Bounds().Dx(), img.Bounds().Dy())
	}
}

func TestLoadImage_NotFound(t *testing.T) {
	_, err := loadImage("/nonexistent/path.png")
	if err == nil {
		t.Error("expected error for nonexistent image")
	}
}

func TestIngest_ExplicitPaths_NoCover(t *testing.T) {
	tmpDir := t.TempDir()
	outputDir := filepath.Join(tmpDir, "output")
	novelDir := filepath.Join(tmpDir, "novel")
	if err := os.MkdirAll(novelDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	chContent := "# Chapter 1\nContent."
	if err := os.WriteFile(filepath.Join(novelDir, "chapter_01.md"), []byte(chContent), 0644); err != nil {
		t.Fatalf("writing ch: %v", err)
	}

	cfg := testConfig(outputDir)
	p := NewPipeline(cfg, nil, nil)

	manifest, err := p.Ingest(context.Background(), IngestSource{
		Chapters: []string{filepath.Join(novelDir, "chapter_01.md")},
	})
	if err != nil {
		t.Fatalf("Ingest failed: %v", err)
	}

	if len(manifest.Project.Chapters) != 1 {
		t.Errorf("expected 1 chapter, got %d", len(manifest.Project.Chapters))
	}
}

func TestIngest_FromDir_NoCover(t *testing.T) {
	tmpDir := t.TempDir()
	outputDir := filepath.Join(tmpDir, "output")

	novelDir := filepath.Join(tmpDir, "novel")
	if err := os.MkdirAll(novelDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	ch01 := "# Chapter 1\nAlice was bored."
	if err := os.WriteFile(filepath.Join(novelDir, "chapter_01.md"), []byte(ch01), 0644); err != nil {
		t.Fatalf("writing ch01: %v", err)
	}

	cfg := testConfig(outputDir)
	p := NewPipeline(cfg, nil, nil)

	manifest, err := p.Ingest(context.Background(), IngestSource{BookDir: novelDir})
	if err != nil {
		t.Fatalf("Ingest failed: %v", err)
	}

	if len(manifest.Project.Chapters) != 1 {
		t.Errorf("expected 1 chapter, got %d", len(manifest.Project.Chapters))
	}
}

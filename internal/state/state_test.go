package state

import (
	"errors"
	"testing"
	"time"

	"github.com/FarelRA/comix/internal/model"
)

func TestSaveAndLoadCharacterNote(t *testing.T) {
	dir := t.TempDir()

	note := &model.CharacterNote{
		Schema: "comix/character-note/v1",
		Characters: []model.Character{
			{
				Name:                "Alice",
				PhysicalDescription: "Young girl",
				FirstChapter:        "chapter_01",
				ChaptersSeen:        []string{"chapter_01"},
			},
		},
	}

	if err := SaveCharacterNote(dir, "test-proj", note); err != nil {
		t.Fatalf("SaveCharacterNote: %v", err)
	}

	loaded, err := LoadCharacterNote(dir, "test-proj")
	if err != nil {
		t.Fatalf("LoadCharacterNote: %v", err)
	}

	if loaded.Schema != note.Schema {
		t.Errorf("schema: got %q, want %q", loaded.Schema, note.Schema)
	}
	if len(loaded.Characters) != 1 {
		t.Fatalf("expected 1 character, got %d", len(loaded.Characters))
	}
	if loaded.Characters[0].Name != "Alice" {
		t.Errorf("character name: got %q, want %q", loaded.Characters[0].Name, "Alice")
	}
}

func TestSaveAndLoadSceneList(t *testing.T) {
	dir := t.TempDir()

	scenes := &model.SceneList{
		Schema:    "comix/scene-list/v1",
		ProjectID: "test-proj",
		Scenes: []model.Scene{
			{
				Chapter:           "chapter_01",
				Sequence:          1,
				GlobalSequence:    1,
				Description:       "Test scene",
				CharactersPresent: []string{"alice"},
			},
		},
	}

	if err := SaveSceneList(dir, "test-proj", scenes); err != nil {
		t.Fatalf("SaveSceneList: %v", err)
	}

	loaded, err := LoadSceneList(dir, "test-proj")
	if err != nil {
		t.Fatalf("LoadSceneList: %v", err)
	}

	if len(loaded.Scenes) != 1 {
		t.Fatalf("expected 1 scene, got %d", len(loaded.Scenes))
	}
	if loaded.Scenes[0].Sequence != 1 {
		t.Errorf("scene sequence: got %d, want 1", loaded.Scenes[0].Sequence)
	}
}

func TestSaveAndLoadManifest(t *testing.T) {
	dir := t.TempDir()

	now := mustParseTime(t, "2026-06-01T10:00:00Z")
	manifest := &model.ProjectManifest{
		Project: model.ProjectMeta{
			Name:      "test-proj",
			CreatedAt: now,
			Source: model.SourceInfo{
				Type: "directory",
				Path: "/some/path",
			},
			Chapters: []model.ChapterMeta{
				{ID: "chapter_01", Filename: "chapter_01.md", Title: "Chapter 1", WordCount: 100},
			},
		},
		Pipeline: model.PipelineProgress{
			Status:       model.PhaseIdle,
			CurrentPhase: 0,
			Phases: map[string]model.PhaseStatus{
				"ingest": {Status: model.PhaseIdle},
			},
		},
	}

	if err := SaveManifest(dir, "test-proj", manifest); err != nil {
		t.Fatalf("SaveManifest: %v", err)
	}

	loaded, err := LoadManifest(dir, "test-proj")
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}

	if loaded.Project.Name != "test-proj" {
		t.Errorf("name: got %q, want %q", loaded.Project.Name, "test-proj")
	}
}

func TestSaveAndLoadCharacterNote_NotFound(t *testing.T) {
	dir := t.TempDir()

	_, err := LoadCharacterNote(dir, "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent character note")
	}
}

func TestUpdateManifestPhase(t *testing.T) {
	dir := t.TempDir()

	now := mustParseTime(t, "2026-06-01T10:00:00Z")
	manifest := &model.ProjectManifest{
		Project: model.ProjectMeta{
			Name:      "test-proj",
			CreatedAt: now,
			Chapters: []model.ChapterMeta{
				{ID: "chapter_01", Filename: "chapter_01.md"},
			},
		},
		Pipeline: model.PipelineProgress{
			Status:       model.PhaseIdle,
			CurrentPhase: 0,
			Phases:       make(map[string]model.PhaseStatus),
		},
	}

	for _, p := range model.AllPhases {
		status := model.PhasePending
		if p == "ingest" {
			status = model.PhaseIdle
		}
		manifest.Pipeline.Phases[p] = model.PhaseStatus{Status: status}
	}

	if err := SaveManifest(dir, "test-proj", manifest); err != nil {
		t.Fatalf("SaveManifest: %v", err)
	}

	completed := model.PhaseStatus{Status: model.PhaseCompleted}
	if err := UpdateManifestPhase(dir, "test-proj", "ingest", completed); err != nil {
		t.Fatalf("UpdateManifestPhase: %v", err)
	}

	loaded, err := LoadManifest(dir, "test-proj")
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}

	if loaded.Pipeline.Phases["ingest"].Status != model.PhaseCompleted {
		t.Errorf("ingest phase: got status %q, want %q", loaded.Pipeline.Phases["ingest"].Status, model.PhaseCompleted)
	}
}

func TestManifestExists(t *testing.T) {
	dir := t.TempDir()

	exists, err := ManifestExists(dir, "nonexistent")
	if err != nil {
		t.Fatalf("ManifestExists: %v", err)
	}
	if exists {
		t.Error("expected nonexistent project to not exist")
	}
}

func TestRecordPhaseError(t *testing.T) {
	dir := t.TempDir()

	manifest := model.NewProjectManifest("test-proj", "dir", "/tmp", []model.ChapterMeta{
		{ID: "ch1", Filename: "ch1.md"},
	})
	if err := SaveManifest(dir, "test-proj", manifest); err != nil {
		t.Fatalf("SaveManifest: %v", err)
	}

	if err := RecordPhaseError(dir, "test-proj", "scenes", errors.New("something went wrong")); err != nil {
		t.Fatalf("RecordPhaseError: %v", err)
	}

	loaded, err := LoadManifest(dir, "test-proj")
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}

	if len(loaded.Pipeline.Errors) != 1 {
		t.Errorf("expected 1 error, got %d", len(loaded.Pipeline.Errors))
	}
	if loaded.Pipeline.Phases["scenes"].Status != model.PhaseFailed {
		t.Errorf("expected scenes phase to be failed, got %q", loaded.Pipeline.Phases["scenes"].Status)
	}
	if loaded.Pipeline.Status != model.PhaseFailed {
		t.Errorf("expected pipeline status failed, got %q", loaded.Pipeline.Status)
	}
	if loaded.Pipeline.CurrentPhase != model.PhaseNumbers["scenes"] {
		t.Errorf("expected current phase scenes, got %d", loaded.Pipeline.CurrentPhase)
	}
	if loaded.Pipeline.Errors[0].Phase != "scenes" {
		t.Errorf("expected phase 'scenes', got %q", loaded.Pipeline.Errors[0].Phase)
	}
}

func TestRecordPhaseError_NoManifest(t *testing.T) {
	if err := RecordPhaseError(t.TempDir(), "nonexistent", "render", errors.New("test")); err == nil {
		t.Fatal("expected error for missing manifest")
	}
}

func TestSaveCharacterNote_InvalidPath(t *testing.T) {
	err := SaveCharacterNote("/nonexistent/path/that/cannot/be/created", "test", &model.CharacterNote{})
	if err == nil {
		t.Error("expected error for invalid path")
	}
}

func TestSaveSceneList_InvalidPath(t *testing.T) {
	err := SaveSceneList("/nonexistent/path/that/cannot/be/created", "test", &model.SceneList{})
	if err == nil {
		t.Error("expected error for invalid path")
	}
}

func TestSaveManifest_InvalidPath(t *testing.T) {
	err := SaveManifest("/nonexistent/path/that/cannot/be/created", "test", &model.ProjectManifest{})
	if err == nil {
		t.Error("expected error for invalid path")
	}
}

func TestLoadCharacterNote_NotFound(t *testing.T) {
	_, err := LoadCharacterNote(t.TempDir(), "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent")
	}
}

func TestLoadSceneList_NotFound(t *testing.T) {
	_, err := LoadSceneList(t.TempDir(), "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent")
	}
}

func TestLoadManifest_NotFound(t *testing.T) {
	_, err := LoadManifest(t.TempDir(), "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent")
	}
}

func TestManifestExists_StatError(t *testing.T) {
	_, err := ManifestExists("/nonexistent", "foo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUpdateManifestPhase_NotFound(t *testing.T) {
	err := UpdateManifestPhase(t.TempDir(), "nonexistent", "ingest", model.PhaseStatus{})
	if err == nil {
		t.Error("expected error for nonexistent project")
	}
}

func TestRecordPhaseError_UpdatesFailedPhaseAndErrors(t *testing.T) {
	dir := t.TempDir()

	manifest := model.NewProjectManifest("test-proj", "dir", "/tmp", []model.ChapterMeta{
		{ID: "ch1", Filename: "ch1.md"},
	})
	if err := SaveManifest(dir, "test-proj", manifest); err != nil {
		t.Fatalf("SaveManifest: %v", err)
	}

	if err := RecordPhaseError(dir, "test-proj", "render", errors.New("render failed")); err != nil {
		t.Fatalf("RecordPhaseError: %v", err)
	}

	loaded, err := LoadManifest(dir, "test-proj")
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	if loaded.Pipeline.Phases["render"].Status != model.PhaseFailed {
		t.Errorf("expected render failed, got %q", loaded.Pipeline.Phases["render"].Status)
	}
	if len(loaded.Pipeline.Errors) != 1 {
		t.Errorf("expected 1 error, got %d", len(loaded.Pipeline.Errors))
	}
	if loaded.Pipeline.Errors[0].Message != "render failed" {
		t.Errorf("expected 'render failed', got %q", loaded.Pipeline.Errors[0].Message)
	}
}

func TestRecordPhaseError_AccumulatesMultiple(t *testing.T) {
	dir := t.TempDir()

	manifest := model.NewProjectManifest("test-proj", "dir", "/tmp", []model.ChapterMeta{
		{ID: "ch1", Filename: "ch1.md"},
	})
	if err := SaveManifest(dir, "test-proj", manifest); err != nil {
		t.Fatalf("SaveManifest: %v", err)
	}

	if err := RecordPhaseError(dir, "test-proj", "sheets", errors.New("first error")); err != nil {
		t.Fatalf("RecordPhaseError sheets: %v", err)
	}
	if err := RecordPhaseError(dir, "test-proj", "poses", errors.New("second error")); err != nil {
		t.Fatalf("RecordPhaseError poses: %v", err)
	}

	loaded, err := LoadManifest(dir, "test-proj")
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}

	if len(loaded.Pipeline.Errors) != 2 {
		t.Errorf("expected 2 errors, got %d", len(loaded.Pipeline.Errors))
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

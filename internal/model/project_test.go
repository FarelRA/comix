package model

import (
	"testing"
	"time"
)

func TestNewProjectManifest(t *testing.T) {
	chapters := []ChapterMeta{
		{ID: "chapter_01", Filename: "chapter_01.md", Title: "Down the Rabbit-Hole", WordCount: 2145},
		{ID: "chapter_02", Filename: "chapter_02.md", Title: "The Pool of Tears", WordCount: 1897},
	}

	pm := NewProjectManifest("alice", "directory", "./novels/alice/", chapters)

	if pm.Project.Name != "alice" {
		t.Errorf("expected name 'alice', got %q", pm.Project.Name)
	}
	if pm.Project.CreatedAt.IsZero() {
		t.Error("expected created_at to be set")
	}
	if pm.Project.Source.Type != "directory" {
		t.Errorf("expected source type 'directory', got %q", pm.Project.Source.Type)
	}
	if len(pm.Project.Chapters) != 2 {
		t.Errorf("expected 2 chapters, got %d", len(pm.Project.Chapters))
	}
	if pm.Pipeline.Status != PhaseIdle {
		t.Errorf("expected pipeline status %q, got %q", PhaseIdle, pm.Pipeline.Status)
	}
	if pm.Pipeline.CurrentPhase != 0 {
		t.Errorf("expected current_phase 0, got %d", pm.Pipeline.CurrentPhase)
	}

	// Check all phases initialized
	for _, p := range AllPhases {
		ps, ok := pm.Pipeline.Phases[p]
		if !ok {
			t.Errorf("missing phase %q in pipeline phases", p)
			continue
		}
		expectedStatus := PhasePending
		if p == PhaseNameIngest {
			expectedStatus = PhaseIdle
		}
		if ps.Status != expectedStatus {
			t.Errorf("phase %q: expected status %q, got %q", p, expectedStatus, ps.Status)
		}
	}
}

func TestProjectManifest_Validate_Success(t *testing.T) {
	pm := &ProjectManifest{
		Project: ProjectMeta{
			Name:      "alice",
			CreatedAt: time.Now(),
			Chapters: []ChapterMeta{
				{ID: "ch1", Filename: "ch1.md"},
			},
		},
	}
	if err := pm.Validate(); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

func TestProjectManifest_Validate_NoName(t *testing.T) {
	pm := &ProjectManifest{
		Project: ProjectMeta{
			CreatedAt: time.Now(),
		},
	}
	if err := pm.Validate(); err == nil {
		t.Error("expected error for missing name, got nil")
	}
}

func TestProjectManifest_Validate_NoChapters(t *testing.T) {
	pm := &ProjectManifest{
		Project: ProjectMeta{
			Name:      "alice",
			CreatedAt: time.Now(),
		},
		Pipeline: PipelineProgress{Phases: map[string]PhaseStatus{
			PhaseNameIngest: {Status: PhaseCompleted},
		}},
	}
	if err := pm.Validate(); err == nil {
		t.Error("expected error for completed ingest with no chapters, got nil")
	}
}

func TestProjectManifest_Validate_PreIngestNoChapters(t *testing.T) {
	pm := NewProjectManifest("alice", "upload", "", nil)
	if err := pm.Validate(); err != nil {
		t.Errorf("expected no error for pre-ingest project, got: %v", err)
	}
}

func TestChapterMeta_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cm      ChapterMeta
		wantErr bool
	}{
		{"valid", ChapterMeta{ID: "ch1", Filename: "ch1.md"}, false},
		{"empty id", ChapterMeta{Filename: "ch1.md"}, true},
		{"empty filename", ChapterMeta{ID: "ch1"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cm.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

func TestPhaseNumbers(t *testing.T) {
	tests := []struct {
		name  string
		phase string
		num   int
	}{
		{"ingest", PhaseNameIngest, 1},
		{"characters", PhaseNameCharacters, 2},
		{"scenes", PhaseNameScenes, 3},
		{"sheets", PhaseNameSheets, 4},
		{"poses", PhaseNamePoses, 5},
		{"render", PhaseNameRender, 6},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if n := PhaseNumbers[tt.phase]; n != tt.num {
				t.Errorf("PhaseNumbers[%q] = %d, want %d", tt.phase, n, tt.num)
			}
		})
	}
}

func TestProjectManifest_Validate_ZeroCreatedAt(t *testing.T) {
	pm := &ProjectManifest{
		Project: ProjectMeta{
			Name: "alice",
			Chapters: []ChapterMeta{
				{ID: "ch1", Filename: "ch1.md"},
			},
		},
	}
	if err := pm.Validate(); err == nil {
		t.Error("expected error for zero created_at, got nil")
	}
}

func TestAllPhasesOrder(t *testing.T) {
	expected := []string{
		PhaseNameIngest,
		PhaseNameCharacters,
		PhaseNameScenes,
		PhaseNameSheets,
		PhaseNamePoses,
		PhaseNameRender,
	}
	for i, p := range AllPhases {
		if p != expected[i] {
			t.Errorf("AllPhases[%d] = %q, want %q", i, p, expected[i])
		}
	}
}

package model

import (
	"testing"
)

func TestSceneList_Validate_Success(t *testing.T) {
	sl := &SceneList{
		Schema:    "comix/scene-list/v1",
		ProjectID: "alice",
		Scenes: []Scene{
			{
				ID:               "scene_001",
				Chapter:          "chapter_01",
				ChapterSequence:  1,
				GlobalSequence:   1,
				Description:      "Alice sits by the riverbank.",
				CharactersPresent: []string{"alice"},
				Location:         "riverside",
				Mood:             "bored",
				VisualCues:       []string{"sunny", "green grass"},
				PanelCount:       1,
			},
		},
	}
	if err := sl.Validate(); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

func TestSceneList_Validate_MissingSchema(t *testing.T) {
	sl := &SceneList{
		ProjectID: "alice",
	}
	if err := sl.Validate(); err == nil {
		t.Error("expected error for missing schema, got nil")
	}
}

func TestScene_Validate_EmptyID(t *testing.T) {
	s := &Scene{
		Chapter:         "chapter_01",
		ChapterSequence: 1,
		GlobalSequence:  1,
		Description:     "desc",
	}
	if err := s.Validate(); err == nil {
		t.Error("expected error for empty id, got nil")
	}
}

func TestScene_Validate_EmptyDescription(t *testing.T) {
	s := &Scene{
		ID:               "scene_001",
		Chapter:          "chapter_01",
		ChapterSequence:  1,
		GlobalSequence:   1,
		CharactersPresent: []string{"alice"},
	}
	if err := s.Validate(); err == nil {
		t.Error("expected error for empty description, got nil")
	}
}

func TestScene_Validate_Dialogue(t *testing.T) {
	s := &Scene{
		ID:               "scene_001",
		Chapter:          "chapter_01",
		ChapterSequence:  1,
		GlobalSequence:   1,
		Description:      "Alice speaks.",
		CharactersPresent: []string{"alice"},
		Dialogue: []DialogueLine{
			{Speaker: "alice", Text: "Hello!"},
			{Speaker: "", Text: "missing speaker"},
		},
	}
	if err := s.Validate(); err == nil {
		t.Error("expected error for dialogue with empty speaker, got nil")
	}
}

func TestScene_Validate_PanelCountDefault(t *testing.T) {
	s := &Scene{
		ID:               "scene_001",
		Chapter:          "chapter_01",
		ChapterSequence:  1,
		GlobalSequence:   1,
		Description:      "desc",
		CharactersPresent: []string{"alice"},
		PanelCount:       0,
	}
	if err := s.Validate(); err != nil {
		t.Errorf("expected no error with panel_count=0 (defaults to 1), got: %v", err)
	}
	if s.PanelCount != 1 {
		t.Errorf("expected PanelCount to default to 1, got %d", s.PanelCount)
	}
}

func TestScene_HasCharacter(t *testing.T) {
	s := &Scene{
		CharactersPresent: []string{"alice", "white_rabbit"},
	}
	if !s.HasCharacter("alice") {
		t.Error("expected HasCharacter('alice') to be true")
	}
	if s.HasCharacter("cheshire_cat") {
		t.Error("expected HasCharacter('cheshire_cat') to be false")
	}
}

func TestSceneList_Validate_SceneError(t *testing.T) {
	sl := &SceneList{
		Schema:    "comix/scene-list/v1",
		ProjectID: "alice",
		Scenes: []Scene{
			{ID: "", Chapter: "ch1", ChapterSequence: 1, GlobalSequence: 1}, // missing description too
		},
	}
	if err := sl.Validate(); err == nil {
		t.Error("expected error for invalid scene, got nil")
	}
}

func TestSceneList_Validate_EmptyProjectID(t *testing.T) {
	sl := &SceneList{
		Schema: "comix/scene-list/v1",
	}
	if err := sl.Validate(); err == nil {
		t.Error("expected error for empty project_id")
	}
}

func TestScene_Validate_EmptyChapter(t *testing.T) {
	s := &Scene{
		ID:               "scene_001",
		ChapterSequence:  1,
		GlobalSequence:   1,
		Description:      "desc",
		CharactersPresent: []string{"alice"},
	}
	if err := s.Validate(); err == nil {
		t.Error("expected error for empty chapter")
	}
}

func TestScene_Validate_ChapterSequenceZero(t *testing.T) {
	s := &Scene{
		ID:               "scene_001",
		Chapter:          "ch1",
		ChapterSequence:  0,
		GlobalSequence:   1,
		Description:      "desc",
		CharactersPresent: []string{"alice"},
	}
	if err := s.Validate(); err == nil {
		t.Error("expected error for chapter_sequence < 1")
	}
}

func TestScene_Validate_GlobalSequenceZero(t *testing.T) {
	s := &Scene{
		ID:               "scene_001",
		Chapter:          "ch1",
		ChapterSequence:  1,
		GlobalSequence:   0,
		Description:      "desc",
		CharactersPresent: []string{"alice"},
	}
	if err := s.Validate(); err == nil {
		t.Error("expected error for global_sequence < 1")
	}
}

func TestScene_Validate_DialogueEmptyText(t *testing.T) {
	s := &Scene{
		ID:               "scene_001",
		Chapter:          "ch1",
		ChapterSequence:  1,
		GlobalSequence:   1,
		Description:      "desc",
		CharactersPresent: []string{"alice"},
		Dialogue: []DialogueLine{
			{Speaker: "alice", Text: ""},
		},
	}
	if err := s.Validate(); err == nil {
		t.Error("expected error for dialogue with empty text")
	}
}

func TestScene_HasCharacter_EmptyList(t *testing.T) {
	s := &Scene{}
	if s.HasCharacter("alice") {
		t.Error("expected HasCharacter to be false for empty list")
	}
}

func TestScene_PanelCountDefaultsToOne(t *testing.T) {
	s := &Scene{
		ID:               "scene_001",
		Chapter:          "ch1",
		ChapterSequence:  1,
		GlobalSequence:   1,
		Description:      "desc",
		CharactersPresent: []string{"alice"},
		PanelCount:       0,
	}
	s.Validate()
	if s.PanelCount != 1 {
		t.Errorf("expected PanelCount to default to 1 after Validate, got %d", s.PanelCount)
	}
}

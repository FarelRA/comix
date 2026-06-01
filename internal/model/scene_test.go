package model

import "testing"

func validScene() Scene {
	return Scene{
		Chapter:           "chapter_01",
		Sequence:          1,
		GlobalSequence:    1,
		Description:       "Alice sits by the riverbank.",
		CharactersPresent: []string{"alice"},
		Location:          "riverside",
		Mood:              "bored",
		VisualCues:        []string{"sunny", "green grass"},
	}
}

func TestSceneList_Validate_Success(t *testing.T) {
	sl := &SceneList{Schema: "comix/scene-list/v1", ProjectID: "alice", Scenes: []Scene{validScene()}}
	if err := sl.Validate(); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

func TestSceneList_Validate_MissingSchema(t *testing.T) {
	sl := &SceneList{ProjectID: "alice"}
	if err := sl.Validate(); err == nil {
		t.Error("expected error for missing schema, got nil")
	}
}

func TestSceneList_Validate_EmptyProjectID(t *testing.T) {
	sl := &SceneList{Schema: "comix/scene-list/v1"}
	if err := sl.Validate(); err == nil {
		t.Error("expected error for empty project_id")
	}
}

func TestSceneList_Validate_SceneError(t *testing.T) {
	sl := &SceneList{Schema: "comix/scene-list/v1", ProjectID: "alice", Scenes: []Scene{{Chapter: "ch1", Sequence: 1, GlobalSequence: 1}}}
	if err := sl.Validate(); err == nil {
		t.Error("expected error for invalid scene, got nil")
	}
}

func TestScene_Validate_EmptyDescription(t *testing.T) {
	s := validScene()
	s.Description = ""
	if err := s.Validate(); err == nil {
		t.Error("expected error for empty description, got nil")
	}
}

func TestScene_Validate_Dialogue(t *testing.T) {
	s := validScene()
	s.Dialogue = []DialogueLine{{Speaker: "alice", Text: "Hello!"}, {Speaker: "", Text: "missing speaker"}}
	if err := s.Validate(); err == nil {
		t.Error("expected error for dialogue with empty speaker, got nil")
	}
}

func TestScene_Validate_EmptyChapter(t *testing.T) {
	s := validScene()
	s.Chapter = ""
	if err := s.Validate(); err == nil {
		t.Error("expected error for empty chapter")
	}
}

func TestScene_Validate_SequenceZero(t *testing.T) {
	s := validScene()
	s.Sequence = 0
	if err := s.Validate(); err == nil {
		t.Error("expected error for sequence < 1")
	}
}

func TestScene_Validate_GlobalSequenceZero(t *testing.T) {
	s := validScene()
	s.GlobalSequence = 0
	if err := s.Validate(); err == nil {
		t.Error("expected error for global_sequence < 1")
	}
}

func TestScene_Validate_DialogueEmptyText(t *testing.T) {
	s := validScene()
	s.Dialogue = []DialogueLine{{Speaker: "alice", Text: ""}}
	if err := s.Validate(); err == nil {
		t.Error("expected error for dialogue with empty text")
	}
}

func TestScene_HasCharacter(t *testing.T) {
	s := &Scene{CharactersPresent: []string{"alice", "white_rabbit"}}
	if !s.HasCharacter("alice") {
		t.Error("expected HasCharacter('alice') to be true")
	}
	if s.HasCharacter("cheshire_cat") {
		t.Error("expected HasCharacter('cheshire_cat') to be false")
	}
}

func TestScene_HasCharacter_EmptyList(t *testing.T) {
	s := &Scene{}
	if s.HasCharacter("alice") {
		t.Error("expected HasCharacter to be false for empty list")
	}
}

func TestScene_Key(t *testing.T) {
	s := Scene{Chapter: "chapter_01", Sequence: 7}
	if got := s.Key(); got != "chapter_01_007" {
		t.Errorf("expected key chapter_01_007, got %q", got)
	}
}

package model

import (
	"testing"
)

func TestCharacterNote_Validate_Success(t *testing.T) {
	cn := &CharacterNote{
		Schema:             "comix/character-note/v1",
		Version:            1,
		LastUpdatedChapter: "chapter_01",
		Characters: []Character{
			{
				ID:                  "alice",
				Name:                "Alice",
				PhysicalDescription: "Young girl, blonde hair, blue eyes",
				FirstChapter:        "chapter_01",
				ChaptersSeen:        []string{"chapter_01"},
				PersonalityTraits:   []string{"curious"},
			},
		},
	}
	if err := cn.Validate(); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

func TestCharacterNote_Validate_MissingSchema(t *testing.T) {
	cn := &CharacterNote{
		Version:            1,
		LastUpdatedChapter: "chapter_01",
	}
	if err := cn.Validate(); err == nil {
		t.Error("expected error for missing schema, got nil")
	}
}

func TestCharacterNote_Validate_VersionZero(t *testing.T) {
	cn := &CharacterNote{
		Schema: "comix/character-note/v1",
		Characters: []Character{
			{
				ID:                  "alice",
				Name:                "Alice",
				PhysicalDescription: "desc",
				FirstChapter:        "ch1",
				ChaptersSeen:        []string{"ch1"},
			},
		},
	}
	if err := cn.Validate(); err == nil {
		t.Error("expected error for version < 1, got nil")
	}
}

func TestCharacter_Validate_Success(t *testing.T) {
	c := &Character{
		ID:                  "white_rabbit",
		Name:                "White Rabbit",
		PhysicalDescription: "White rabbit with pink eyes",
		FirstChapter:        "chapter_01",
		ChaptersSeen:        []string{"chapter_01"},
		PersonalityTraits:   []string{"anxious", "punctual"},
	}
	if err := c.Validate(); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

func TestCharacter_Validate_EmptyID(t *testing.T) {
	c := &Character{
		Name: "Alice",
	}
	if err := c.Validate(); err == nil {
		t.Error("expected error for empty id, got nil")
	}
}

func TestCharacter_AddChapter_New(t *testing.T) {
	c := &Character{ChaptersSeen: []string{"chapter_01"}}
	c.AddChapter("chapter_02")
	if len(c.ChaptersSeen) != 2 {
		t.Errorf("expected 2 chapters, got %d", len(c.ChaptersSeen))
	}
}

func TestCharacter_AddChapter_Duplicate(t *testing.T) {
	c := &Character{ChaptersSeen: []string{"chapter_01"}}
	c.AddChapter("chapter_01")
	if len(c.ChaptersSeen) != 1 {
		t.Errorf("expected 1 chapter, got %d", len(c.ChaptersSeen))
	}
}

func TestCharacterNote_Validate_CharacterError(t *testing.T) {
	cn := &CharacterNote{
		Schema:             "comix/character-note/v1",
		Version:            1,
		LastUpdatedChapter: "ch1",
		Characters: []Character{
			{ID: "", Name: "NoID"}, // missing required fields
		},
	}
	if err := cn.Validate(); err == nil {
		t.Error("expected error for invalid character, got nil")
	}
}

func TestCharacterNote_EmptyCharacters_Valid(t *testing.T) {
	cn := &CharacterNote{
		Schema:             "comix/character-note/v1",
		Version:            1,
		LastUpdatedChapter: "chapter_01",
		Characters:         []Character{},
	}
	if err := cn.Validate(); err != nil {
		t.Errorf("empty characters should be valid, got: %v", err)
	}
}

func TestCharacter_Validate_EmptyName(t *testing.T) {
	c := &Character{
		ID: "alice",
	}
	if err := c.Validate(); err == nil {
		t.Error("expected error for empty name")
	}
}

func TestCharacter_Validate_EmptyPhysicalDescription(t *testing.T) {
	c := &Character{
		ID:   "alice",
		Name: "Alice",
	}
	if err := c.Validate(); err == nil {
		t.Error("expected error for empty physical_description")
	}
}

func TestCharacter_Validate_EmptyFirstChapter(t *testing.T) {
	c := &Character{
		ID:                  "alice",
		Name:                "Alice",
		PhysicalDescription: "desc",
	}
	if err := c.Validate(); err == nil {
		t.Error("expected error for empty first_chapter")
	}
}

func TestCharacter_Validate_EmptyChaptersSeen(t *testing.T) {
	c := &Character{
		ID:                  "alice",
		Name:                "Alice",
		PhysicalDescription: "desc",
		FirstChapter:        "ch1",
	}
	if err := c.Validate(); err == nil {
		t.Error("expected error for empty chapters_seen")
	}
}

func TestCharacter_AddChapter_NoExistingChapters(t *testing.T) {
	c := &Character{}
	c.AddChapter("chapter_01")
	if len(c.ChaptersSeen) != 1 {
		t.Errorf("expected 1 chapter, got %d", len(c.ChaptersSeen))
	}
}

func TestCharacterNote_Validate_LastUpdatedChapterMissing(t *testing.T) {
	cn := &CharacterNote{
		Schema:  "comix/character-note/v1",
		Version: 1,
		Characters: []Character{
			{
				ID:                  "alice",
				Name:                "Alice",
				PhysicalDescription: "desc",
				FirstChapter:        "ch1",
				ChaptersSeen:        []string{"ch1"},
			},
		},
	}
	if err := cn.Validate(); err == nil {
		t.Error("expected error when last_updated_chapter is missing with characters present")
	}
}

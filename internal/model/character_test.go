package model

import "testing"

func validCharacter() Character {
	return Character{
		Name:                "Alice",
		PhysicalDescription: "Young girl, blonde hair, blue eyes",
		FirstChapter:        "chapter_01",
		ChaptersSeen:        []string{"chapter_01"},
		PersonalityTraits:   []string{"curious"},
	}
}

func TestCharacterNote_Validate_Success(t *testing.T) {
	cn := &CharacterNote{Schema: "comix/character-note/v1", Characters: []Character{validCharacter()}}
	if err := cn.Validate(); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

func TestCharacterNote_Validate_MissingSchema(t *testing.T) {
	cn := &CharacterNote{}
	if err := cn.Validate(); err == nil {
		t.Error("expected error for missing schema, got nil")
	}
}

func TestCharacter_Validate_Success(t *testing.T) {
	c := validCharacter()
	c.Name = "White Rabbit"
	c.PhysicalDescription = "White rabbit with pink eyes"
	c.PersonalityTraits = []string{"anxious", "punctual"}
	if err := c.Validate(); err != nil {
		t.Errorf("expected no error, got: %v", err)
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
	cn := &CharacterNote{Schema: "comix/character-note/v1", Characters: []Character{{Name: "NoDescription"}}}
	if err := cn.Validate(); err == nil {
		t.Error("expected error for invalid character, got nil")
	}
}

func TestCharacterNote_EmptyCharacters_Valid(t *testing.T) {
	cn := &CharacterNote{Schema: "comix/character-note/v1", Characters: []Character{}}
	if err := cn.Validate(); err != nil {
		t.Errorf("empty characters should be valid, got: %v", err)
	}
}

func TestCharacter_Validate_EmptyName(t *testing.T) {
	c := validCharacter()
	c.Name = ""
	if err := c.Validate(); err == nil {
		t.Error("expected error for empty name")
	}
}

func TestCharacter_Validate_EmptyPhysicalDescription(t *testing.T) {
	c := validCharacter()
	c.PhysicalDescription = ""
	if err := c.Validate(); err == nil {
		t.Error("expected error for empty physical_description")
	}
}

func TestCharacter_Validate_EmptyFirstChapter(t *testing.T) {
	c := validCharacter()
	c.FirstChapter = ""
	if err := c.Validate(); err == nil {
		t.Error("expected error for empty first_chapter")
	}
}

func TestCharacter_Validate_EmptyChaptersSeen(t *testing.T) {
	c := validCharacter()
	c.ChaptersSeen = nil
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

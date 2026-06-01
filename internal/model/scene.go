package model

import (
	"fmt"
	"strings"
)

type SceneList struct {
	Schema    string  `json:"$schema" yaml:"$schema"`
	ProjectID string  `json:"project_id" yaml:"project_id"`
	Scenes    []Scene `json:"scenes" yaml:"scenes"`
}

type Scene struct {
	ID               string         `json:"id" yaml:"id"`
	Chapter          string         `json:"chapter" yaml:"chapter"`
	ChapterSequence  int            `json:"chapter_sequence" yaml:"chapter_sequence"`
	GlobalSequence   int            `json:"global_sequence" yaml:"global_sequence"`
	Description      string         `json:"description" yaml:"description"`
	CharactersPresent []string      `json:"characters_present" yaml:"characters_present"`
	Location         string         `json:"location" yaml:"location"`
	Mood             string         `json:"mood" yaml:"mood"`
	VisualCues       []string       `json:"visual_cues" yaml:"visual_cues"`
	PanelCount       int            `json:"panel_count" yaml:"panel_count"`
	Dialogue         []DialogueLine `json:"dialogue,omitempty" yaml:"dialogue,omitempty"`
}

type DialogueLine struct {
	Speaker string `json:"speaker" yaml:"speaker"`
	Text    string `json:"text" yaml:"text"`
}

func (sl *SceneList) Validate() error {
	if sl.Schema == "" {
		return fmt.Errorf("scene list: $schema is required")
	}
	if sl.ProjectID == "" {
		return fmt.Errorf("scene list: project_id is required")
	}
	for i, s := range sl.Scenes {
		if err := s.Validate(); err != nil {
			return fmt.Errorf("scene list: scene[%d] (%s): %w", i, s.ID, err)
		}
	}
	return nil
}

func (s *Scene) Validate() error {
	var errs []string

	if s.ID == "" {
		errs = append(errs, "id is required")
	}
	if s.Chapter == "" {
		errs = append(errs, "chapter is required")
	}
	if s.ChapterSequence < 1 {
		errs = append(errs, "chapter_sequence must be >= 1")
	}
	if s.GlobalSequence < 1 {
		errs = append(errs, "global_sequence must be >= 1")
	}
	if s.Description == "" {
		errs = append(errs, "description is required")
	}
	if s.PanelCount < 1 {
		s.PanelCount = 1
	}
	for i, d := range s.Dialogue {
		if d.Speaker == "" {
			errs = append(errs, fmt.Sprintf("dialogue[%d]: speaker is required", i))
		}
		if d.Text == "" {
			errs = append(errs, fmt.Sprintf("dialogue[%d]: text is required", i))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("scene %q: %s", s.ID, strings.Join(errs, "; "))
	}
	return nil
}

func (s *Scene) HasCharacter(characterID string) bool {
	for _, c := range s.CharactersPresent {
		if c == characterID {
			return true
		}
	}
	return false
}

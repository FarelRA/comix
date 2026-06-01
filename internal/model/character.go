package model

import (
	"fmt"
	"strings"
)

type CharacterNote struct {
	Schema     string      `json:"$schema" yaml:"$schema"`
	Characters []Character `json:"characters" yaml:"characters"`
}

type Character struct {
	Name                string            `json:"name" yaml:"name"`
	PhysicalDescription string            `json:"physical_description" yaml:"physical_description"`
	PersonalityTraits   []string          `json:"personality_traits" yaml:"personality_traits"`
	FirstChapter        string            `json:"first_chapter" yaml:"first_chapter"`
	ChaptersSeen        []string          `json:"chapters_seen" yaml:"chapters_seen"`
	Aliases             []string          `json:"aliases,omitempty" yaml:"aliases,omitempty"`
	NotableActions      []string          `json:"notable_actions,omitempty" yaml:"notable_actions,omitempty"`
	Relationships       map[string]string `json:"relationships,omitempty" yaml:"relationships,omitempty"`
}

func (cn *CharacterNote) Validate() error {
	if cn.Schema == "" {
		return fmt.Errorf("character note: $schema is required")
	}
	for i, c := range cn.Characters {
		if err := c.Validate(); err != nil {
			return fmt.Errorf("character note: character[%d] (%s): %w", i, c.Name, err)
		}
	}
	return nil
}

func (c *Character) Validate() error {
	var errs []string

	if c.Name == "" {
		errs = append(errs, "name is required")
	}
	if c.PhysicalDescription == "" {
		errs = append(errs, "physical_description is required")
	}
	if c.FirstChapter == "" {
		errs = append(errs, "first_chapter is required")
	}
	if len(c.ChaptersSeen) == 0 {
		errs = append(errs, "chapters_seen must have at least one entry")
	}

	if len(errs) > 0 {
		return fmt.Errorf("character %q: %s", c.Name, strings.Join(errs, "; "))
	}
	return nil
}

func (c *Character) AddChapter(chapter string) {
	for _, ch := range c.ChaptersSeen {
		if ch == chapter {
			return
		}
	}
	c.ChaptersSeen = append(c.ChaptersSeen, chapter)
}

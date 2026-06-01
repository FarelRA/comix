package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"

	"github.com/FarelRA/comix/internal/llm"
	"github.com/FarelRA/comix/internal/model"
	"github.com/FarelRA/comix/internal/state"
	"github.com/FarelRA/comix/internal/storage"
)

func (p *Pipeline) ExtractCharacters(ctx context.Context, manifest *model.ProjectManifest, resume bool) (*model.CharacterNote, error) {
	outputDir := p.cfg.Pipeline.OutputDir
	projectName := manifest.Project.Name

	var note *model.CharacterNote

	if resume {
		existing, err := state.LoadCharacterNote(outputDir, projectName)
		if err == nil && existing != nil {
			note = existing
			slog.Info("loaded existing character note", "version", note.Version, "characters", len(note.Characters))
		}
	}

	if note == nil {
		note = &model.CharacterNote{
			Schema:  "comix/character-note/v1",
			Version: 1,
		}
	}

	coverContent, err := p.readRawFile(outputDir, projectName, p.cfg.Pipeline.CoverFilename)
	if err != nil {
		return nil, err
	}

	chaptersToProcess := p.chaptersNeedingCharacterExtraction(manifest, note, resume)

	for _, ch := range chaptersToProcess {
		if err := ctx.Err(); err != nil {
			return note, fmt.Errorf("character extraction cancelled: %w", err)
		}

		chapterContent, err := storage.ReadMarkdown(filepath.Join(storage.RawDir(outputDir, projectName), ch.Filename))
		if err != nil {
			return nil, fmt.Errorf("reading chapter %s: %w", ch.ID, err)
		}

		messages := p.buildCharacterMessages(coverContent, chapterContent, note)

		updated := &model.CharacterNote{}
		if err := p.llm.Chat(ctx, messages, updated, p.cfg.OpenAI.LLM.Temperature); err != nil {
			return nil, fmt.Errorf("llm character extraction for %s: %w", ch.ID, err)
		}

		if len(updated.Characters) == 0 && len(note.Characters) > 0 {
			updated.Characters = note.Characters
			updated.Version = note.Version
			updated.LastUpdatedChapter = ch.ID
			updated.Schema = note.Schema
		}

		if updated.Schema == "" {
			updated.Schema = "comix/character-note/v1"
		}
		if updated.Version < 1 {
			updated.Version = note.Version + 1
		}
		updated.LastUpdatedChapter = ch.ID

		note = updated

		if err := state.SaveCharacterNote(outputDir, projectName, note); err != nil {
			return nil, fmt.Errorf("saving character note after %s: %w", ch.ID, err)
		}

		slog.Info("characters extracted", "chapter", ch.ID, "total", len(note.Characters))
	}

	return note, nil
}

func (p *Pipeline) readRawFile(outputDir, projectName, filename string) (string, error) {
	content, err := storage.ReadMarkdown(filepath.Join(storage.RawDir(outputDir, projectName), filename))
	if err != nil {
		return "", fmt.Errorf("reading raw file %s: %w", filename, err)
	}
	return content, nil
}

func (p *Pipeline) chaptersNeedingCharacterExtraction(manifest *model.ProjectManifest, note *model.CharacterNote, resume bool) []model.ChapterMeta {
	if !resume || note == nil || note.LastUpdatedChapter == "" {
		return manifest.Project.Chapters
	}

	var remaining []model.ChapterMeta
	found := false
	for _, ch := range manifest.Project.Chapters {
		if ch.ID == note.LastUpdatedChapter {
			found = true
			continue
		}
		if found {
			remaining = append(remaining, ch)
		}
	}

	if !found {
		return manifest.Project.Chapters
	}

	if len(remaining) == 0 {
		slog.Debug("all chapters already processed for characters", "last", note.LastUpdatedChapter)
	}

	return remaining
}

func (p *Pipeline) buildCharacterMessages(coverContent, chapterContent string, note *model.CharacterNote) []llm.Message {
	messages := []llm.Message{
		{Role: llm.RoleSystem, Content: llm.SystemPromptExtractCharacters()},
	}

	userContent := ""
	if coverContent != "" {
		userContent += "Cover:\n" + coverContent + "\n\n"
	}
	userContent += "Chapter:\n" + chapterContent
	messages = append(messages, llm.Message{Role: llm.RoleUser, Content: userContent})

	if note != nil && len(note.Characters) > 0 {
		noteJSON := fmt.Sprintf(
			"Existing CharacterNote (return this COMPLETE with your updates appended):\n%s",
			mustMarshalJSON(note),
		)
		messages = append(messages, llm.Message{Role: llm.RoleUser, Content: noteJSON})
	}

	return messages
}

func mustMarshalJSON(v any) string {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "{}"
	}
	return string(data)
}

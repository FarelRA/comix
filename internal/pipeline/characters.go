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
			slog.Info("loaded existing character note", "characters", len(note.Characters))
		}
	}

	if note == nil {
		note = &model.CharacterNote{
			Schema: "comix/character-note/v1",
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

		chapterNote := &model.CharacterNote{}
		if err := p.llm.Chat(ctx, messages, chapterNote, p.cfg.OpenAI.LLM.Temperature); err != nil {
			return nil, fmt.Errorf("llm character extraction for %s: %w", ch.ID, err)
		}

		note = p.mergeChapterCharacters(note, chapterNote, ch.ID)

		if err := state.SaveCharacterNote(outputDir, projectName, note); err != nil {
			return nil, fmt.Errorf("saving character note after %s: %w", ch.ID, err)
		}

		slog.Info("characters extracted", "chapter", ch.ID, "total", len(note.Characters))
	}

	return note, nil
}

func (p *Pipeline) mergeChapterCharacters(master *model.CharacterNote, chapterNote *model.CharacterNote, chapterID string) *model.CharacterNote {
	charIndex := make(map[string]int)
	for i, c := range master.Characters {
		charIndex[characterLookupKey(c.Name)] = i
	}

	for _, chChar := range chapterNote.Characters {
		if idx, ok := charIndex[characterLookupKey(chChar.Name)]; ok {
			chChar.AddChapter(chapterID)
			master.Characters[idx] = chChar
		} else {
			if chChar.FirstChapter == "" {
				chChar.FirstChapter = chapterID
			}
			chChar.AddChapter(chapterID)
			master.Characters = append(master.Characters, chChar)
			charIndex[characterLookupKey(chChar.Name)] = len(master.Characters) - 1
		}
	}

	return master
}

func (p *Pipeline) readRawFile(outputDir, projectName, filename string) (string, error) {
	content, err := storage.ReadMarkdown(filepath.Join(storage.RawDir(outputDir, projectName), filename))
	if err != nil {
		return "", fmt.Errorf("reading raw file %s: %w", filename, err)
	}
	return content, nil
}

func (p *Pipeline) chaptersNeedingCharacterExtraction(manifest *model.ProjectManifest, note *model.CharacterNote, resume bool) []model.ChapterMeta {
	if !resume || note == nil {
		return manifest.Project.Chapters
	}
	lastChapter := lastCharacterChapter(note, manifest.Project.Chapters)
	if lastChapter == "" {
		return manifest.Project.Chapters
	}

	var remaining []model.ChapterMeta
	found := false
	for _, ch := range manifest.Project.Chapters {
		if ch.ID == lastChapter {
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
		slog.Debug("all chapters already processed for characters", "last", lastChapter)
	}

	return remaining
}

func lastCharacterChapter(note *model.CharacterNote, chapters []model.ChapterMeta) string {
	seen := make(map[string]bool)
	for _, c := range note.Characters {
		for _, ch := range c.ChaptersSeen {
			seen[ch] = true
		}
	}
	last := ""
	for _, ch := range chapters {
		if seen[ch.ID] {
			last = ch.ID
		}
	}
	return last
}

func (p *Pipeline) buildCharacterMessages(coverContent, chapterContent string, note *model.CharacterNote) []llm.Message {
	userContent := ""
	if coverContent != "" {
		userContent += "<cover>\n" + coverContent + "\n</cover>\n\n"
	}
	userContent += "<chapter_text>\n" + chapterContent + "\n</chapter_text>"

	if note != nil && len(note.Characters) > 0 {
		userContent += fmt.Sprintf(
			"\n\n<known_characters>\n%s\n</known_characters>\nReturn a CharacterNote with ONLY characters that appear in this chapter. Include existing characters with their full updated data.",
			mustMarshalJSON(note),
		)
	}

	return []llm.Message{
		{Role: llm.RoleSystem, Content: llm.SystemPromptExtractCharacters()},
		{Role: llm.RoleUser, Content: userContent},
	}
}

func mustMarshalJSON(v any) string {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "{}"
	}
	return string(data)
}

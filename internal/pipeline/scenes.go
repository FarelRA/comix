package pipeline

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"

	"github.com/FarelRA/comix/internal/llm"
	"github.com/FarelRA/comix/internal/model"
	"github.com/FarelRA/comix/internal/state"
	"github.com/FarelRA/comix/internal/storage"
)

func (p *Pipeline) ExtractScenes(ctx context.Context, manifest *model.ProjectManifest, note *model.CharacterNote, resume bool) (*model.SceneList, error) {
	outputDir := p.cfg.Pipeline.OutputDir
	projectName := manifest.Project.Name

	var sceneList *model.SceneList

	if resume {
		existing, err := state.LoadSceneList(outputDir, projectName)
		if err == nil && existing != nil {
			sceneList = existing
			slog.Info("loaded existing scene list", "scenes", len(sceneList.Scenes))
		}
	}

	if sceneList == nil {
		sceneList = &model.SceneList{
			Schema:    "comix/scene-list/v1",
			ProjectID: projectName,
		}
	}

	coverContent, err := p.readRawFile(outputDir, projectName, p.cfg.Pipeline.CoverFilename)
	if err != nil {
		return nil, err
	}

	processedChapters := p.extractProcessedChapters(sceneList)
	globalCounter := p.maxGlobalSequence(sceneList)

	for _, ch := range manifest.Project.Chapters {
		if err := ctx.Err(); err != nil {
			return sceneList, fmt.Errorf("scene extraction cancelled: %w", err)
		}

		if resume {
			if _, done := processedChapters[ch.ID]; done {
				slog.Debug("chapter scenes already extracted, skipping", "chapter", ch.ID)
				continue
			}
		}

		chapterContent, err := storage.ReadMarkdown(filepath.Join(storage.RawDir(outputDir, projectName), ch.Filename))
		if err != nil {
			return nil, fmt.Errorf("reading chapter %s: %w", ch.ID, err)
		}

		messages := p.buildSceneMessages(coverContent, chapterContent, note)

		chapterScenes := &model.SceneList{}
		if err := p.llm.Chat(ctx, messages, chapterScenes, p.cfg.OpenAI.LLM.Temperature); err != nil {
			return nil, fmt.Errorf("llm scene extraction for %s: %w", ch.ID, err)
		}

		for i := range chapterScenes.Scenes {
			s := &chapterScenes.Scenes[i]
			s.Chapter = ch.ID
			globalCounter++
			s.GlobalSequence = globalCounter
			if s.ChapterSequence == 0 {
				s.ChapterSequence = i + 1
			}
			if s.PanelCount < 1 {
				s.PanelCount = 1
			}

			p.validateCharacterRefs(s, note, ch.ID)
		}

		sceneList.Scenes = append(sceneList.Scenes, chapterScenes.Scenes...)

		if err := state.SaveSceneList(outputDir, projectName, sceneList); err != nil {
			return nil, fmt.Errorf("saving scene list after %s: %w", ch.ID, err)
		}

		slog.Info("scenes extracted", "chapter", ch.ID, "new", len(chapterScenes.Scenes), "total", len(sceneList.Scenes))
	}

	return sceneList, nil
}

func (p *Pipeline) validateCharacterRefs(scene *model.Scene, note *model.CharacterNote, chapterID string) {
	if note == nil {
		slog.Warn("scene validation skipped because CharacterNote is missing", "scene", scene.ID, "chapter", chapterID)
		return
	}
	charIndex := make(map[string]bool)
	for _, c := range note.Characters {
		charIndex[c.ID] = true
	}

	for _, charID := range scene.CharactersPresent {
		if !charIndex[charID] {
			slog.Warn("scene references character not found in CharacterNote",
				"scene", scene.ID, "chapter", chapterID, "character", charID)
		}
	}
}

func (p *Pipeline) extractProcessedChapters(sceneList *model.SceneList) map[string]bool {
	processed := make(map[string]bool)
	for _, s := range sceneList.Scenes {
		processed[s.Chapter] = true
	}
	return processed
}

func (p *Pipeline) maxGlobalSequence(sceneList *model.SceneList) int {
	max := 0
	for _, s := range sceneList.Scenes {
		if s.GlobalSequence > max {
			max = s.GlobalSequence
		}
	}
	return max
}

func (p *Pipeline) buildSceneMessages(coverContent, chapterContent string, note *model.CharacterNote) []llm.Message {
	messages := []llm.Message{
		{Role: llm.RoleSystem, Content: llm.SystemPromptExtractScenes()},
	}

	userContent := ""
	if coverContent != "" {
		userContent += "Cover: " + coverContent + "\n\n"
	}
	userContent += "Chapter: " + chapterContent + "\n\n"
	userContent += "Character Reference:\n" + mustMarshalJSON(note)

	messages = append(messages, llm.Message{Role: llm.RoleUser, Content: userContent})
	return messages
}

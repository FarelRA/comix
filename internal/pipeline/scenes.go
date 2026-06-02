package pipeline

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"sort"

	"github.com/FarelRA/comix/internal/llm"
	"github.com/FarelRA/comix/internal/model"
	"github.com/FarelRA/comix/internal/state"
	"github.com/FarelRA/comix/internal/storage"
)

type sceneExtractionResponse struct {
	Schema    string                 `json:"$schema"`
	ProjectID string                 `json:"project_id"`
	Scenes    []sceneExtractionScene `json:"scenes"`
}

type sceneExtractionScene struct {
	Sequence          int                  `json:"sequence"`
	Description       string               `json:"description"`
	CharactersPresent []string             `json:"characters_present"`
	Location          string               `json:"location"`
	Mood              string               `json:"mood"`
	VisualCues        []string             `json:"visual_cues"`
	Dialogue          []model.DialogueLine `json:"dialogue"`
}

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

		chapterScenes := &sceneExtractionResponse{}
		if err := p.llm.Chat(ctx, messages, chapterScenes, p.cfg.OpenAI.LLM.Temperature); err != nil {
			return nil, fmt.Errorf("llm scene extraction for %s: %w", ch.ID, err)
		}

		sort.SliceStable(chapterScenes.Scenes, func(i, j int) bool {
			return chapterScenes.Scenes[i].Sequence < chapterScenes.Scenes[j].Sequence
		})

		seenSequences := make(map[int]bool, len(chapterScenes.Scenes))
		for i := range chapterScenes.Scenes {
			draft := chapterScenes.Scenes[i]
			if draft.Sequence < 1 {
				return nil, fmt.Errorf("llm scene extraction for %s returned invalid sequence %d", ch.ID, draft.Sequence)
			}
			if seenSequences[draft.Sequence] {
				return nil, fmt.Errorf("llm scene extraction for %s returned duplicate sequence %d", ch.ID, draft.Sequence)
			}
			seenSequences[draft.Sequence] = true
			s := model.Scene{
				Description:       draft.Description,
				CharactersPresent: draft.CharactersPresent,
				Location:          draft.Location,
				Mood:              draft.Mood,
				VisualCues:        draft.VisualCues,
				Dialogue:          draft.Dialogue,
				Chapter:           ch.ID,
				Sequence:          draft.Sequence,
			}
			globalCounter++
			s.GlobalSequence = globalCounter

			p.validateCharacterRefs(&s, note, ch.ID)
			sceneList.Scenes = append(sceneList.Scenes, s)
		}

		if err := state.SaveSceneList(outputDir, projectName, sceneList); err != nil {
			return nil, fmt.Errorf("saving scene list after %s: %w", ch.ID, err)
		}

		slog.Info("scenes extracted", "chapter", ch.ID, "new", len(chapterScenes.Scenes), "total", len(sceneList.Scenes))
	}

	return sceneList, nil
}

func (p *Pipeline) validateCharacterRefs(scene *model.Scene, note *model.CharacterNote, chapterID string) {
	if note == nil {
		slog.Warn("scene validation skipped because CharacterNote is missing", "scene", scene.Key(), "chapter", chapterID)
		return
	}
	charIndex := make(map[string]bool)
	for _, c := range note.Characters {
		charIndex[characterLookupKey(c.Name)] = true
		for _, alias := range c.Aliases {
			charIndex[characterLookupKey(alias)] = true
		}
	}

	for _, charName := range scene.CharactersPresent {
		if !charIndex[characterLookupKey(charName)] {
			slog.Warn("scene references character not found in CharacterNote",
				"scene", scene.Key(), "chapter", chapterID, "character", charName)
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
	userContent := ""
	if coverContent != "" {
		userContent += "<cover>\n" + coverContent + "\n</cover>\n\n"
	}
	userContent += "<chapter_text>\n" + chapterContent + "\n</chapter_text>\n\n"
	userContent += "<character_reference>\n" + mustMarshalJSON(note) + "\n</character_reference>"

	return []llm.Message{
		{Role: llm.RoleSystem, Content: llm.SystemPromptExtractScenes()},
		{Role: llm.RoleUser, Content: userContent},
	}
}

package pipeline

import (
	"context"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/comix/comix/internal/imagegen"
	"github.com/comix/comix/internal/logger"
	"github.com/comix/comix/internal/model"
	"github.com/comix/comix/internal/storage"
)

func (p *Pipeline) RenderScenes(ctx context.Context, manifest *model.ProjectManifest, note *model.CharacterNote, scenes *model.SceneList) error {
	if p.imgGen == nil {
		return fmt.Errorf("image generation client not configured")
	}

	outputDir := p.cfg.Pipeline.OutputDir
	projectName := manifest.Project.Name

	sorted := make([]model.Scene, len(scenes.Scenes))
	copy(sorted, scenes.Scenes)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].GlobalSequence < sorted[j].GlobalSequence
	})

	charIndex := buildCharacterIndex(note)

	var prevPanel image.Image
	var prevSceneDesc string

	for _, scene := range sorted {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("rendering cancelled at scene %s: %w", scene.ID, err)
		}

		panelPath := filepath.Join(storage.PanelsDir(outputDir, projectName), fmt.Sprintf("%s.png", scene.ID))

		if p.panelExists(panelPath) {
			logger.Debug("scene already rendered, loading for continuity", "scene", scene.ID)
			img, err := loadImage(panelPath)
			if err != nil {
				logger.Warn("could not load existing panel", "scene", scene.ID, "error", err)
			} else {
				prevPanel = img
			}
			continue
		}

		charRefs := p.buildSceneCharacterRefs(scene, charIndex)
		sceneDesc := p.buildSceneDescription(scene)

		var result *imagegen.ImageResult
		var err error

		if prevPanel == nil {
			prompt := imagegen.PromptFirstScene(sceneDesc, charRefs)
			result, err = p.imgGen.Generate(ctx, prompt, p.cfg.OpenAI.Image.Size.Panel)
		} else {
			prompt := imagegen.PromptNextScene(prevSceneDesc, sceneDesc, charRefs)
			result, err = p.imgGen.Edit(ctx, prevPanel, prompt, p.cfg.OpenAI.Image.Size.Panel)
		}

		if err != nil {
			return fmt.Errorf("rendering scene %s: %w", scene.ID, err)
		}

		if err := storage.SavePNG(panelPath, result.Image); err != nil {
			return fmt.Errorf("saving panel for scene %s: %w", scene.ID, err)
		}

		prevPanel = result.Image
		prevSceneDesc = sceneDesc
		logger.Info("rendered scene", "scene", scene.ID, "global_seq", scene.GlobalSequence)
	}

	return nil
}

func (p *Pipeline) panelExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func (p *Pipeline) buildSceneCharacterRefs(scene model.Scene, charIndex map[string]model.Character) string {
	if len(scene.CharactersPresent) == 0 {
		return "No characters in this scene."
	}

	dialogueByChar := buildDialogueMap(scene.Dialogue, charIndex)

	var parts []string
	for _, charID := range scene.CharactersPresent {
		char, ok := charIndex[charID]
		if !ok {
			parts = append(parts, fmt.Sprintf("- %s: (character reference not found)", charID))
			continue
		}
		ref := fmt.Sprintf("- %s: %s", char.Name, char.PhysicalDescription)
		if lines, ok := dialogueByChar[charID]; ok && len(lines) > 0 {
			for _, line := range lines {
				ref += fmt.Sprintf("\n  \"%s\"", line.Text)
			}
		}
		parts = append(parts, ref)
	}

	return strings.Join(parts, "\n")
}

func buildDialogueMap(dialogue []model.DialogueLine, charIndex map[string]model.Character) map[string][]model.DialogueLine {
	m := make(map[string][]model.DialogueLine)
	for _, d := range dialogue {
		charID := resolveSpeaker(d.Speaker, charIndex)
		if charID != "" {
			m[charID] = append(m[charID], d)
		}
	}
	return m
}

func resolveSpeaker(speaker string, charIndex map[string]model.Character) string {
	if _, ok := charIndex[speaker]; ok {
		return speaker
	}
	for id, c := range charIndex {
		if c.Name == speaker {
			return id
		}
		for _, alias := range c.Aliases {
			if alias == speaker {
				return id
			}
		}
	}
	return ""
}

func (p *Pipeline) buildSceneDescription(scene model.Scene) string {
	var parts []string

	parts = append(parts, scene.Description)

	if scene.Mood != "" {
		parts = append(parts, fmt.Sprintf("Mood: %s", scene.Mood))
	}

	if len(scene.VisualCues) > 0 {
		parts = append(parts, fmt.Sprintf("Visual cues: %s", strings.Join(scene.VisualCues, ", ")))
	}

	if len(scene.Dialogue) > 0 {
		var dialogueParts []string
		for _, d := range scene.Dialogue {
			dialogueParts = append(dialogueParts, fmt.Sprintf("%s: \"%s\"", d.Speaker, d.Text))
		}
		parts = append(parts, fmt.Sprintf("Dialogue: %s", strings.Join(dialogueParts, ", ")))
	}

	return strings.Join(parts, "\n")
}

func buildCharacterIndex(note *model.CharacterNote) map[string]model.Character {
	idx := make(map[string]model.Character, len(note.Characters))
	for _, c := range note.Characters {
		idx[c.ID] = c
	}
	return idx
}

package pipeline

import (
	"context"
	"fmt"
	"image"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/FarelRA/comix/internal/imagegen"
	"github.com/FarelRA/comix/internal/model"
	"github.com/FarelRA/comix/internal/storage"
)

const charRefThreshold = 7

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
		sceneKey := scene.Key()
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("rendering cancelled at scene %s: %w", sceneKey, err)
		}

		panelPath := filepath.Join(storage.PanelsDir(outputDir, projectName), fmt.Sprintf("%s.png", sceneKey))

		if p.panelExists(panelPath) {
			slog.Debug("scene already rendered, loading for continuity", "scene", sceneKey)
			img, err := loadImage(panelPath)
			if err != nil {
				slog.Warn("could not load existing panel", "scene", sceneKey, "error", err)
			} else {
				prevPanel = img
				prevSceneDesc = p.buildSceneDescription(scene)
			}
			continue
		}

		charImages := p.loadCharacterRefImages(projectName, scene, charIndex)

		imageStartIdx := 1
		if prevPanel != nil {
			imageStartIdx = 2
		}
		charRefs := p.buildSceneCharacterRefs(scene, charIndex, imageStartIdx)
		sceneDesc := p.buildSceneDescription(scene)

		var result *imagegen.ImageResult
		var err error

		if prevPanel == nil {
			prompt := imagegen.PromptFirstScene(sceneDesc, charRefs)
			if len(charImages) > 0 {
				prompt += "\n\nReference images are attached for character appearance. See [image N] tags above."
			}
			if len(charImages) > 0 {
				result, err = p.imgGen.GenerateWithReferences(ctx, prompt, p.cfg.OpenAI.Image.Size.Panel, charImages...)
			} else {
				result, err = p.imgGen.Generate(ctx, prompt, p.cfg.OpenAI.Image.Size.Panel)
			}
		} else {
			prompt := imagegen.PromptNextScene(prevSceneDesc, sceneDesc, charRefs)
			if len(charImages) > 0 {
				prompt += "\n\nReference images: image 1 is the previous panel. Character references have [image N] tags above."
			} else {
				prompt += "\n\nReference image 1 is the previous panel. Maintain visual continuity."
			}
			refs := append([]image.Image{prevPanel}, charImages...)
			result, err = p.imgGen.GenerateWithReferences(ctx, prompt, p.cfg.OpenAI.Image.Size.Panel, refs...)
		}

		if err != nil {
			return fmt.Errorf("rendering scene %s: %w", sceneKey, err)
		}

		if err := storage.SavePNG(panelPath, result.Image); err != nil {
			return fmt.Errorf("saving panel for scene %s: %w", sceneKey, err)
		}

		prevPanel = result.Image
		prevSceneDesc = sceneDesc
		slog.Info("rendered scene", "scene", sceneKey, "global_seq", scene.GlobalSequence)
	}

	return nil
}

func (p *Pipeline) panelExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func (p *Pipeline) buildSceneCharacterRefs(scene model.Scene, charIndex map[string]model.Character, imageStartIdx int) string {
	if len(scene.CharactersPresent) == 0 {
		return "No characters in this scene."
	}

	dialogueByChar := buildDialogueMap(scene.Dialogue, charIndex)
	charCount := len(scene.CharactersPresent)

	var parts []string
	currentIdx := imageStartIdx
	for _, charName := range scene.CharactersPresent {
		char, ok := charIndex[characterLookupKey(charName)]
		if !ok {
			if imageStartIdx > 0 {
				parts = append(parts, fmt.Sprintf("- %s [image %d]: (character reference not found)", charName, currentIdx))
				currentIdx++
			} else {
				parts = append(parts, fmt.Sprintf("- %s: (character reference not found)", charName))
			}
			continue
		}

		ref := fmt.Sprintf("- %s: %s", char.Name, char.PhysicalDescription)

		if imageStartIdx > 0 {
			var imgParts []string
			imgCount := 1
			if charCount <= charRefThreshold {
				imgCount = 2
			}
			for i := 0; i < imgCount; i++ {
				imgParts = append(imgParts, fmt.Sprintf("image %d", currentIdx+i))
			}
			ref = fmt.Sprintf("- %s [%s]: %s", char.Name, strings.Join(imgParts, ", "), char.PhysicalDescription)
			currentIdx += imgCount
		}

		if lines, ok := dialogueByChar[characterLookupKey(char.Name)]; ok && len(lines) > 0 {
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
		charName := resolveSpeaker(d.Speaker, charIndex)
		if charName != "" {
			m[characterLookupKey(charName)] = append(m[characterLookupKey(charName)], d)
		}
	}
	return m
}

func resolveSpeaker(speaker string, charIndex map[string]model.Character) string {
	if c, ok := charIndex[characterLookupKey(speaker)]; ok {
		return c.Name
	}
	for _, c := range charIndex {
		if c.Name == speaker {
			return c.Name
		}
		for _, alias := range c.Aliases {
			if alias == speaker {
				return c.Name
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
		idx[characterLookupKey(c.Name)] = c
		for _, alias := range c.Aliases {
			idx[characterLookupKey(alias)] = c
		}
	}
	return idx
}

func characterLookupKey(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func (p *Pipeline) loadCharacterRefImages(projectName string, scene model.Scene, charIndex map[string]model.Character) []image.Image {
	outputDir := p.cfg.Pipeline.OutputDir
	charCount := len(scene.CharactersPresent)

	var refs []image.Image
	for _, charName := range scene.CharactersPresent {
		char, ok := charIndex[characterLookupKey(charName)]
		if !ok {
			continue
		}
		charKey := storage.SlugName(char.Name)

		if charCount <= charRefThreshold {
			sheetPath := filepath.Join(storage.SheetsDir(outputDir, projectName), fmt.Sprintf("%s_3x2.png", charKey))
			if img, err := loadImage(sheetPath); err == nil {
				refs = append(refs, img)
			} else {
				slog.Warn("missing character sheet reference", "character", char.Name, "path", sheetPath, "error", err)
			}
		}

		posePath := filepath.Join(storage.PosesDir(outputDir, projectName), fmt.Sprintf("%s_5x5.png", charKey))
		if img, err := loadImage(posePath); err == nil {
			refs = append(refs, img)
		} else {
			slog.Warn("missing character pose reference", "character", char.Name, "path", posePath, "error", err)
		}
	}

	return refs
}

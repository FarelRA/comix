package pipeline

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/comix/comix/internal/imagegen"
	"github.com/comix/comix/internal/logger"
	"github.com/comix/comix/internal/model"
	"github.com/comix/comix/internal/storage"
)

func (p *Pipeline) GenerateSheets(ctx context.Context, manifest *model.ProjectManifest, note *model.CharacterNote) error {
	if p.imgGen == nil {
		return fmt.Errorf("image generation client not configured")
	}

	outputDir := p.cfg.Pipeline.OutputDir
	projectName := manifest.Project.Name
	maxConcurrent := p.cfg.Pipeline.MaxConcurrentSheets
	if maxConcurrent < 1 {
		maxConcurrent = 1
	}

	sem := make(chan struct{}, maxConcurrent)
	errc := make(chan error, len(note.Characters))

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	for _, char := range note.Characters {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case sem <- struct{}{}:
		}

		go func(char model.Character) {
			defer func() { <-sem }()

			prompt := imagegen.PromptBaseSheet(char.Name, char.PhysicalDescription)
			result, err := p.imgGen.Generate(ctx, prompt, p.cfg.OpenAI.Image.Size.Sheet)
			if err != nil {
				errc <- fmt.Errorf("generating sheet for %q: %w", char.ID, err)
				return
			}

			sheetPath := filepath.Join(storage.SheetsDir(outputDir, projectName), fmt.Sprintf("%s_3x2.png", char.ID))
			if err := storage.SavePNG(sheetPath, result.Image); err != nil {
				errc <- fmt.Errorf("saving sheet for %q: %w", char.ID, err)
				return
			}

			logger.Info("generated 3x2 base sheet", "character", char.Name, "id", char.ID)
			errc <- nil
		}(char)
	}

	var errs []string
	for range note.Characters {
		if err := <-errc; err != nil {
			errs = append(errs, err.Error())
			cancel()
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("sheet generation completed with errors: %s", strings.Join(errs, "; "))
	}

	return nil
}

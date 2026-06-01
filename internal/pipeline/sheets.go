package pipeline

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/FarelRA/comix/internal/imagegen"
	"github.com/FarelRA/comix/internal/model"
	"github.com/FarelRA/comix/internal/storage"
	"golang.org/x/sync/errgroup"
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

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(maxConcurrent)
	errs := make(chan string, len(note.Characters))

	for _, char := range note.Characters {
		char := char
		g.Go(func() error {
			prompt := imagegen.PromptBaseSheet(char.Name, char.PhysicalDescription)
			result, err := p.imgGen.Generate(ctx, prompt, p.cfg.OpenAI.Image.Size.Sheet)
			if err != nil {
				errs <- fmt.Sprintf("generating sheet for %q: %v", char.Name, err)
				return err
			}

			charKey := storage.SlugName(char.Name)
			sheetPath := filepath.Join(storage.SheetsDir(outputDir, projectName), fmt.Sprintf("%s_3x2.png", charKey))
			if err := storage.SavePNG(sheetPath, result.Image); err != nil {
				errs <- fmt.Sprintf("saving sheet for %q: %v", char.Name, err)
				return err
			}

			slog.Info("generated 3x2 base sheet", "character", char.Name, "key", charKey)
			return nil
		})
	}

	_ = g.Wait()
	close(errs)

	var messages []string
	for msg := range errs {
		messages = append(messages, msg)
	}
	if len(messages) > 0 {
		return fmt.Errorf("sheet generation completed with errors: %s", strings.Join(messages, "; "))
	}

	return nil
}

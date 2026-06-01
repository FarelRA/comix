package pipeline

import (
	"context"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"strings"

	"github.com/comix/comix/internal/imagegen"
	"github.com/comix/comix/internal/logger"
	"github.com/comix/comix/internal/model"
	"github.com/comix/comix/internal/storage"
)

func (p *Pipeline) GeneratePoses(ctx context.Context, manifest *model.ProjectManifest, note *model.CharacterNote) error {
	if p.imgGen == nil {
		return fmt.Errorf("image generation client not configured")
	}

	outputDir := p.cfg.Pipeline.OutputDir
	projectName := manifest.Project.Name
	maxConcurrent := p.cfg.Pipeline.MaxConcurrentPoses
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

			sheetPath := filepath.Join(storage.SheetsDir(outputDir, projectName), fmt.Sprintf("%s_3x2.png", char.ID))
			sheetImage, err := loadImage(sheetPath)
			if err != nil {
				errc <- fmt.Errorf("loading sheet for %q: %w", char.ID, err)
				return
			}

			prompt := imagegen.PromptPoseSheet(char.Name)
			result, err := p.imgGen.Edit(ctx, sheetImage, prompt, p.cfg.OpenAI.Image.Size.Poses)
			if err != nil {
				errc <- fmt.Errorf("generating poses for %q: %w", char.ID, err)
				return
			}

			posePath := filepath.Join(storage.PosesDir(outputDir, projectName), fmt.Sprintf("%s_5x5.png", char.ID))
			if err := storage.SavePNG(posePath, result.Image); err != nil {
				errc <- fmt.Errorf("saving poses for %q: %w", char.ID, err)
				return
			}

			logger.Info("generated 5x5 pose sheet", "character", char.Name, "id", char.ID)
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
		return fmt.Errorf("pose generation completed with errors: %s", strings.Join(errs, "; "))
	}

	return nil
}

func loadImage(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening image %s: %w", path, err)
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("decoding image %s: %w", path, err)
	}
	return img, nil
}

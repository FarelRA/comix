package pipeline

import (
	"context"
	"fmt"
	"image"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/FarelRA/comix/internal/imagegen"
	"github.com/FarelRA/comix/internal/model"
	"github.com/FarelRA/comix/internal/storage"
	"golang.org/x/sync/errgroup"
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

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(maxConcurrent)
	errs := make(chan string, len(note.Characters))

	for _, char := range note.Characters {
		g.Go(func() error {
			charKey := storage.SlugName(char.Name)
			sheetPath := filepath.Join(storage.SheetsDir(outputDir, projectName), fmt.Sprintf("%s_3x2.png", charKey))
			sheetImage, err := loadImage(sheetPath)
			if err != nil {
				errs <- fmt.Sprintf("loading sheet for %q: %v", char.Name, err)
				return err
			}

			prompt := imagegen.PromptPoseSheet(char.Name)
			result, err := p.imgGen.GenerateWithReferences(ctx, prompt, p.cfg.OpenAI.Image.Size.Poses, sheetImage)
			if err != nil {
				errs <- fmt.Sprintf("generating poses for %q: %v", char.Name, err)
				return err
			}

			posePath := filepath.Join(storage.PosesDir(outputDir, projectName), fmt.Sprintf("%s_5x5.png", charKey))
			if err := storage.SavePNG(posePath, result.Image); err != nil {
				errs <- fmt.Sprintf("saving poses for %q: %v", char.Name, err)
				return err
			}

			slog.Info("generated 5x5 pose sheet", "character", char.Name, "key", charKey)
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
		return fmt.Errorf("pose generation completed with errors: %s", strings.Join(messages, "; "))
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

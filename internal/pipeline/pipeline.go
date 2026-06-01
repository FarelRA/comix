package pipeline

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/FarelRA/comix/internal/config"
	"github.com/FarelRA/comix/internal/imagegen"
	"github.com/FarelRA/comix/internal/llm"
	"github.com/FarelRA/comix/internal/model"
	"github.com/FarelRA/comix/internal/state"
)

type IngestSource struct {
	ProjectName   string
	BookDir       string
	Cover         string
	Chapters      []string
	AllowExisting bool
}

type Pipeline struct {
	cfg    *config.Config
	llm    *llm.Client
	imgGen *imagegen.Client
}

func NewPipeline(cfg *config.Config, llmClient *llm.Client, imgGenClient *imagegen.Client) *Pipeline {
	return &Pipeline{
		cfg:    cfg,
		llm:    llmClient,
		imgGen: imgGenClient,
	}
}

func (p *Pipeline) Run(ctx context.Context, projectName string, source IngestSource, phases []string, resume bool) error {
	if len(phases) == 0 {
		phases = model.AllPhases
	}

	outputDir := p.cfg.Pipeline.OutputDir

	var manifest *model.ProjectManifest

	if resume {
		exists, err := state.ManifestExists(outputDir, projectName)
		if err != nil {
			return fmt.Errorf("checking existing project: %w", err)
		}
		if exists {
			m, err := state.LoadManifest(outputDir, projectName)
			if err != nil {
				return fmt.Errorf("loading existing manifest for resume: %w", err)
			}
			manifest = m
			slog.Info("resuming project", "project", projectName, "phase", manifest.Pipeline.CurrentPhase)
		}
	}

	runPhase := func(name string, fn func(ctx context.Context) error) error {
		if manifest != nil {
			ps := manifest.Pipeline.Phases[name]
			if ps.Status == model.PhaseCompleted {
				slog.Debug("phase already completed, skipping", "phase", name)
				return nil
			}
		}

		slog.Info("starting phase", "phase", name)

		now := time.Now().UTC()
		phaseStatus := model.PhaseStatus{
			Status:    model.PhaseInProgress,
			StartedAt: &now,
		}

		if manifest != nil {
			manifest.Pipeline.Phases[name] = phaseStatus
			manifest.Pipeline.Status = model.PhaseInProgress
			if num, ok := model.PhaseNumbers[name]; ok {
				manifest.Pipeline.CurrentPhase = num
			}
			if err := state.SaveManifest(outputDir, projectName, manifest); err != nil {
				return fmt.Errorf("saving manifest before phase %q: %w", name, err)
			}
		}

		if err := fn(ctx); err != nil {
			if recordErr := state.RecordPhaseError(outputDir, projectName, name, err); recordErr != nil {
				return fmt.Errorf("phase %q failed: %w; recording phase error failed: %v", name, err, recordErr)
			}
			return fmt.Errorf("phase %q failed: %w", name, err)
		}

		completed := time.Now().UTC()
		phaseStatus = model.PhaseStatus{
			Status:      model.PhaseCompleted,
			StartedAt:   phaseStatus.StartedAt,
			CompletedAt: &completed,
		}

		if manifest != nil {
			manifest.Pipeline.Phases[name] = phaseStatus
			if err := state.UpdateManifestPhase(outputDir, projectName, name, phaseStatus); err != nil {
				return fmt.Errorf("updating manifest after phase %q: %w", name, err)
			}
		}

		slog.Info("phase completed", "phase", name)
		return nil
	}

	for _, phase := range phases {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("pipeline cancelled during phase %q: %w", phase, err)
		}
		if manifest == nil && phase != model.PhaseNameIngest {
			m, err := state.LoadManifest(outputDir, projectName)
			if err != nil {
				return fmt.Errorf("loading manifest for phase %q: %w", phase, err)
			}
			manifest = m
		}

		switch phase {
		case model.PhaseNameIngest:
			if err := runPhase(phase, func(ctx context.Context) error {
				if source.ProjectName == "" {
					source.ProjectName = projectName
				}
				m, err := p.Ingest(ctx, source)
				if err != nil {
					return err
				}
				manifest = m
				projectName = manifest.Project.Name
				return nil
			}); err != nil {
				return err
			}

		case model.PhaseNameCharacters:
			if err := runPhase(phase, func(ctx context.Context) error {
				_, err := p.ExtractCharacters(ctx, manifest, resume)
				if err != nil {
					return err
				}
				return nil
			}); err != nil {
				return err
			}

		case model.PhaseNameScenes:
			if err := runPhase(phase, func(ctx context.Context) error {
				note, err := state.LoadCharacterNote(outputDir, projectName)
				if err != nil {
					return fmt.Errorf("loading character note for scene extraction: %w", err)
				}
				_, err = p.ExtractScenes(ctx, manifest, note, resume)
				if err != nil {
					return err
				}
				return nil
			}); err != nil {
				return err
			}

		case model.PhaseNameSheets:
			if err := runPhase(phase, func(ctx context.Context) error {
				note, err := state.LoadCharacterNote(outputDir, projectName)
				if err != nil {
					return fmt.Errorf("loading character note for sheet generation: %w", err)
				}
				return p.GenerateSheets(ctx, manifest, note)
			}); err != nil {
				return err
			}

		case model.PhaseNamePoses:
			if err := runPhase(phase, func(ctx context.Context) error {
				note, err := state.LoadCharacterNote(outputDir, projectName)
				if err != nil {
					return fmt.Errorf("loading character note for pose generation: %w", err)
				}
				return p.GeneratePoses(ctx, manifest, note)
			}); err != nil {
				return err
			}

		case model.PhaseNameRender:
			if err := runPhase(phase, func(ctx context.Context) error {
				note, err := state.LoadCharacterNote(outputDir, projectName)
				if err != nil {
					return fmt.Errorf("loading character note for rendering: %w", err)
				}
				scenes, err := state.LoadSceneList(outputDir, projectName)
				if err != nil {
					return fmt.Errorf("loading scene list for rendering: %w", err)
				}
				return p.RenderScenes(ctx, manifest, note, scenes)
			}); err != nil {
				return err
			}

		default:
			return fmt.Errorf("unknown phase: %q", phase)
		}
	}

	if manifest != nil {
		for _, phase := range model.AllPhases {
			if manifest.Pipeline.Phases[phase].Status != model.PhaseCompleted {
				slog.Info("pipeline run completed with remaining phases", "project", projectName)
				return nil
			}
		}
		manifest.Pipeline.Status = model.PhaseCompleted
		if err := state.SaveManifest(outputDir, projectName, manifest); err != nil {
			return fmt.Errorf("saving final manifest: %w", err)
		}
	}

	slog.Info("pipeline completed", "project", projectName)
	return nil
}

package cli

import (
	"fmt"
	"log/slog"

	"github.com/FarelRA/comix/internal/imagegen"
	"github.com/FarelRA/comix/internal/llm"
	"github.com/FarelRA/comix/internal/model"
	"github.com/FarelRA/comix/internal/pipeline"

	"github.com/spf13/cobra"
)

var (
	extractProject string
)

func init() {
	extractCmd.PersistentFlags().StringVarP(&extractProject, "project", "p", "", "project name")
	extractCmd.MarkPersistentFlagRequired("project")
	extractCmd.AddCommand(extractCharactersCmd, extractScenesCmd)
	rootCmd.AddCommand(extractCmd)
}

var extractCmd = &cobra.Command{
	Use:   "extract",
	Short: "Extract characters or scenes from ingested chapters",
	Long: `Run LLM-powered extraction on an ingested project.

Available subcommands:
  characters  Extract and update character descriptions via LLM (Phase 2)
  scenes      Extract sequential scene descriptions via LLM (Phase 3)`,
}

var extractCharactersCmd = &cobra.Command{
	Use:   "characters",
	Short: "Extract characters via LLM (Phase 2)",
	Long: `Run Pass One of the pipeline: extract all characters from the ingested chapters
using the LLM. Results are saved to state/characters.json.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPhase(cmd, extractProject, model.PhaseNameCharacters)
	},
}

var extractScenesCmd = &cobra.Command{
	Use:   "scenes",
	Short: "Extract scenes via LLM (Phase 3)",
	Long: `Run Pass Two of the pipeline: extract sequential scene descriptions using
the complete CharacterNote. Results are saved to state/scenes.json.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPhase(cmd, extractProject, model.PhaseNameScenes)
	},
}

func runPhase(cmd *cobra.Command, project, phase string) error {
	cfg, err := loadConfig()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	if cfg.OpenAI.APIKey == "" {
		return fmt.Errorf("OPENAI_API_KEY is not set. Set it via export OPENAI_API_KEY=sk-... or in config.yaml")
	}

	llmClient := llm.NewClient(cfg.OpenAI.APIKey, cfg.OpenAI.LLM.Model, cfg.OpenAI.LLM.Thinking).
		WithBaseURL(cfg.OpenAI.BaseURL).
		WithMaxRetries(cfg.OpenAI.LLM.MaxRetries)

	imgClient := imagegen.NewClient(
		cfg.OpenAI.APIKey,
		cfg.OpenAI.Image.Model,
		cfg.OpenAI.Image.Quality,
	).WithBaseURL(cfg.OpenAI.BaseURL).
		WithMaxRetries(cfg.OpenAI.Image.MaxRetries)

	p := pipeline.NewPipeline(cfg, llmClient, imgClient)

	slog.Info("running phase", "phase", phase, "project", project)

	source := pipeline.IngestSource{}
	if err := p.Run(cmd.Context(), project, source, []string{phase}, true); err != nil {
		return fmt.Errorf("phase %q failed: %w", phase, err)
	}

	fmt.Printf("Phase %q completed for project %q\n", phase, project)
	return nil
}

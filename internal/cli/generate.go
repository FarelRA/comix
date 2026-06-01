package cli

import (
	"fmt"

	"github.com/comix/comix/internal/imagegen"
	"github.com/comix/comix/internal/llm"
	"github.com/comix/comix/internal/logger"
	"github.com/comix/comix/internal/model"
	"github.com/comix/comix/internal/pipeline"

	"github.com/spf13/cobra"
)

var (
	generateProject string
)

func init() {
	generateCmd.PersistentFlags().StringVarP(&generateProject, "project", "p", "", "project name")
	generateCmd.MarkPersistentFlagRequired("project")
	generateCmd.AddCommand(generateSheetsCmd, generatePosesCmd)
	rootCmd.AddCommand(generateCmd)
}

var generateCmd = &cobra.Command{
	Use:   "generate",
	Short: "Generate character sheets or pose grids",
	Long: `Generate reference images for characters using gpt-image-2.

Available subcommands:
  sheets  Generate 3x2 base model sheets for all characters (Phase 4)
  poses   Generate 5x5 dynamic pose grids from base sheets (Phase 5)`,
}

var generateSheetsCmd = &cobra.Command{
	Use:   "sheets",
	Short: "Generate 3x2 base model sheets (Phase 4)",
	Long: `Generate a 3x2 base reference model sheet for each character using gpt-image-2.
Each sheet shows the character from 6 angles: front, back, right profile,
left profile, top-down, and bottom-up.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runGeneratePhase(cmd, generateProject, model.PhaseNameSheets)
	},
}

var generatePosesCmd = &cobra.Command{
	Use:   "poses",
	Short: "Generate 5x5 dynamic pose grids (Phase 5)",
	Long: `Generate a 5x5 grid of 25 distinct dynamic poses for each character using
gpt-image-2 image-to-image editing, using the 3x2 base sheet as reference.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runGeneratePhase(cmd, generateProject, model.PhaseNamePoses)
	},
}

func runGeneratePhase(cmd *cobra.Command, project, phase string) error {
	cfg, err := loadConfig()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	if cfg.OpenAI.APIKey == "" {
		return fmt.Errorf("OPENAI_API_KEY is not set. Set it via export OPENAI_API_KEY=sk-... or in config.yaml")
	}

	llmClient := llm.NewClient(cfg.OpenAI.APIKey, cfg.OpenAI.LLM.Model).
		WithMaxRetries(cfg.OpenAI.LLM.MaxRetries).
		WithRetryDelay(cfg.OpenAI.LLM.RetryBaseDelay)

	imgClient := imagegen.NewClient(
		cfg.OpenAI.APIKey,
		cfg.OpenAI.Image.Model,
		cfg.OpenAI.Image.Quality,
		cfg.OpenAI.Image.Size,
		cfg.OpenAI.Image.Thinking,
	).WithMaxRetries(cfg.OpenAI.Image.MaxRetries).
		WithRetryDelay(cfg.OpenAI.Image.RetryBaseDelay).
		WithRateLimit(cfg.OpenAI.Image.RateLimitRPM)

	p := pipeline.NewPipeline(cfg, llmClient, imgClient)

		logger.Info("running phase", "phase", phase, "project", project)

	source := pipeline.IngestSource{}
	if err := p.Run(cmd.Context(), project, source, []string{phase}, true); err != nil {
		return fmt.Errorf("phase %q failed: %w", phase, err)
	}

	fmt.Printf("Phase %q completed for project %q\n", phase, project)
	return nil
}

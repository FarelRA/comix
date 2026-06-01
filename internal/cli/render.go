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
	renderProject string
)

func init() {
	renderCmd.Flags().StringVarP(&renderProject, "project", "p", "", "project name")
	renderCmd.MarkFlagRequired("project")
	rootCmd.AddCommand(renderCmd)
}

var renderCmd = &cobra.Command{
	Use:   "render",
	Short: "Render sequential comic panels (Phase 6)",
	Long: `Render all scenes as individual comic panel images. The first scene is generated
from text, and each subsequent scene uses the previous panel as reference to
maintain visual continuity. Results are saved to panels/*.png.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := loadConfig()
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}

		if cfg.OpenAI.APIKey == "" {
			return fmt.Errorf("OPENAI_API_KEY is not set. Set it via export OPENAI_API_KEY=sk-... or in config.yaml")
		}

		llmClient := llm.NewClient(cfg.OpenAI.APIKey, cfg.OpenAI.LLM.Model, cfg.OpenAI.LLM.Thinking).
			WithBaseURL(cfg.OpenAI.BaseURL).
			WithMaxRetries(cfg.OpenAI.LLM.MaxRetries).
			WithRetryDelay(cfg.OpenAI.LLM.RetryBaseDelay)

		imgClient := imagegen.NewClient(
			cfg.OpenAI.APIKey,
			cfg.OpenAI.Image.Model,
			cfg.OpenAI.Image.Quality,
			cfg.OpenAI.Image.Size,
			cfg.OpenAI.Image.Thinking,
		).WithBaseURL(cfg.OpenAI.BaseURL).
			WithMaxRetries(cfg.OpenAI.Image.MaxRetries).
			WithRetryDelay(cfg.OpenAI.Image.RetryBaseDelay)

		p := pipeline.NewPipeline(cfg, llmClient, imgClient)

		logger.Info("rendering scenes", "project", renderProject)

		source := pipeline.IngestSource{}
		if err := p.Run(cmd.Context(), renderProject, source, []string{model.PhaseNameRender}, true); err != nil {
			return fmt.Errorf("render failed: %w", err)
		}

		fmt.Printf("Render completed for project %q\n", renderProject)
		fmt.Printf("Panels: %s\n", cfg.Pipeline.OutputDir+"/"+renderProject+"/panels")
		return nil
	},
}

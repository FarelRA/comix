package cli

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/FarelRA/comix/internal/imagegen"
	"github.com/FarelRA/comix/internal/llm"
	"github.com/FarelRA/comix/internal/pipeline"

	"github.com/spf13/cobra"
)

var (
	bookDir     string
	coverFile   string
	chapters    string
	projectName string
	resume      bool
)

func init() {
	runCmd.Flags().StringVar(&bookDir, "book-dir", "", "directory containing cover.md and chapter_*.md files")
	runCmd.Flags().StringVar(&coverFile, "cover", "", "path to cover markdown file")
	runCmd.Flags().StringVar(&chapters, "chapters", "", "comma-separated list of chapter markdown files")
	runCmd.Flags().StringVarP(&projectName, "project", "p", "", "project name")
	runCmd.Flags().BoolVar(&resume, "resume", false, "resume from last checkpoint")
	rootCmd.AddCommand(runCmd)
}

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run the full pipeline",
	Long: `Execute the complete Comix pipeline end-to-end.

The pipeline consists of 6 phases:
1. Ingest cover and chapter markdown files
2. Extract characters via LLM
3. Extract scenes via LLM
4. Generate base model sheets (3x2 grids)
5. Generate dynamic poses (5x5 grids)
6. Render sequential comic panels`,
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
			WithMaxRetries(cfg.OpenAI.LLM.MaxRetries)

		imgClient := imagegen.NewClient(
			cfg.OpenAI.APIKey,
			cfg.OpenAI.Image.Model,
			cfg.OpenAI.Image.Quality,
		).WithBaseURL(cfg.OpenAI.BaseURL).
			WithMaxRetries(cfg.OpenAI.Image.MaxRetries)

		p := pipeline.NewPipeline(cfg, llmClient, imgClient)

		source := pipeline.IngestSource{
			ProjectName: projectName,
			BookDir:     bookDir,
			Cover:       coverFile,
			Chapters:    parseChapterList(chapters),
		}

		if source.BookDir == "" && source.Cover == "" && len(source.Chapters) == 0 {
			return fmt.Errorf("specify --book-dir or --cover/--chapters")
		}

		if projectName == "" && source.BookDir != "" {
			projectName = filepath.Base(source.BookDir)
		}
		if projectName == "" {
			return fmt.Errorf("--project is required when using --cover/--chapters")
		}

		slog.Info("starting Comix pipeline", "project", projectName)

		if err := p.Run(cmd.Context(), projectName, source, nil, resume); err != nil {
			return fmt.Errorf("pipeline failed: %w", err)
		}

		fmt.Printf("Pipeline completed successfully for project %q\n", projectName)
		fmt.Printf("Output directory: %s\n", filepath.Join(cfg.Pipeline.OutputDir, projectName))
		return nil
	},
}

func parseChapterList(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

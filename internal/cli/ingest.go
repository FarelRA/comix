package cli

import (
	"fmt"
	"log/slog"
	"path/filepath"

	"github.com/FarelRA/comix/internal/imagegen"
	"github.com/FarelRA/comix/internal/llm"
	"github.com/FarelRA/comix/internal/model"
	"github.com/FarelRA/comix/internal/pipeline"

	"github.com/spf13/cobra"
)

var (
	ingestBookDir   string
	ingestCoverFile string
	ingestChapters  string
	ingestProject   string
)

func init() {
	ingestCmd.Flags().StringVar(&ingestBookDir, "book-dir", "", "directory containing cover.md and chapter_*.md files")
	ingestCmd.Flags().StringVar(&ingestCoverFile, "cover", "", "path to cover markdown file")
	ingestCmd.Flags().StringVar(&ingestChapters, "chapters", "", "comma-separated list of chapter markdown files")
	ingestCmd.Flags().StringVarP(&ingestProject, "project", "p", "", "project name")
	rootCmd.AddCommand(ingestCmd)
}

var ingestCmd = &cobra.Command{
	Use:   "ingest",
	Short: "Ingest cover and chapter markdown files",
	Long: `Read and validate cover + chapter markdown files, then copy them into the project output directory.

This is Phase 1 of the Comix pipeline. After ingestion, the project is ready for character extraction.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := loadConfig()
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}

		source := pipeline.IngestSource{
			BookDir:  ingestBookDir,
			Cover:    ingestCoverFile,
			Chapters: parseChapterList(ingestChapters),
		}

		if source.BookDir == "" && source.Cover == "" && len(source.Chapters) == 0 {
			return fmt.Errorf("specify --book-dir or --cover/--chapters")
		}

		project := ingestProject
		if project == "" && source.BookDir != "" {
			project = filepath.Base(source.BookDir)
		}
		if project == "" {
			return fmt.Errorf("--project is required when using --cover/--chapters")
		}
		source.ProjectName = project

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

		slog.Info("ingesting files", "project", project)

		if err := p.Run(cmd.Context(), project, source, []string{model.PhaseNameIngest}, false); err != nil {
			return fmt.Errorf("ingest failed: %w", err)
		}

		fmt.Printf("Ingestion completed for project %q\n", project)
		fmt.Printf("Output directory: %s\n", filepath.Join(cfg.Pipeline.OutputDir, project))
		return nil
	},
}

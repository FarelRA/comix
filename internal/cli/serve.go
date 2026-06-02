package cli

import (
	"fmt"

	"github.com/FarelRA/comix/internal/config"
	"github.com/FarelRA/comix/internal/imagegen"
	"github.com/FarelRA/comix/internal/llm"
	"github.com/FarelRA/comix/internal/pipeline"
	"github.com/FarelRA/comix/internal/server"

	"github.com/spf13/cobra"
)

var (
	serverPort int
	serverHost string
	serveCmd   = &cobra.Command{
		Use:   "serve",
		Short: "Start the HTTP server",
		Long:  `Start the Comix HTTP server for web-based project management and pipeline execution.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.LoadConfigWithOverrides(cfgFile, rootCmd.PersistentFlags())
			if err != nil {
				return fmt.Errorf("loading config: %w", err)
			}

			if serverPort > 0 {
				cfg.Server.Port = serverPort
			}
			if serverHost != "" {
				cfg.Server.Host = serverHost
			}
			if rootCmd.PersistentFlags().Changed("output") {
				cfg.Pipeline.OutputDir = outputDir
			}
			if rootCmd.PersistentFlags().Changed("log-format") {
				cfg.Logging.Format = logFormat
			}
			if err := cfg.ValidateForOpenAI(); err != nil {
				return err
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
			srv := server.NewServer(cfg, p)

			return srv.Start()
		},
	}
)

func init() {
	serveCmd.Flags().IntVarP(&serverPort, "port", "p", 0, "server port (overrides config)")
	serveCmd.Flags().StringVar(&serverHost, "host", "", "server host (overrides config)")
	rootCmd.AddCommand(serveCmd)
}

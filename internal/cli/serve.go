package cli

import (
	"fmt"

	"github.com/comix/comix/internal/imagegen"
	"github.com/comix/comix/internal/llm"
	"github.com/comix/comix/internal/pipeline"
	"github.com/comix/comix/internal/server"

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
		cfg, err := loadConfig()
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}

		if cfg.OpenAI.APIKey == "" {
			return fmt.Errorf("OPENAI_API_KEY is not set. Set it via export OPENAI_API_KEY=sk-... or in config.yaml")
		}

			if serverPort > 0 {
				cfg.Server.Port = serverPort
			}
			if serverHost != "" {
				cfg.Server.Host = serverHost
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

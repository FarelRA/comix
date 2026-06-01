package cli

import (
	"context"
	"log/slog"
	"os"

	"github.com/FarelRA/comix/internal/config"
	"github.com/FarelRA/comix/internal/logger"

	"github.com/spf13/cobra"
)

var (
	cfgFile   string
	outputDir string
	verbose   bool
	logFormat string
	rootCtx   context.Context
	rootCmd   = &cobra.Command{
		Use:   "comix",
		Short: "Comix - Comic Creator Studio",
		Long: `Convert text-based novels into sequential comic panels using generative AI.

Comix orchestrates a 6-phase pipeline: ingestion, character extraction,
scene extraction, base model sheet generation, dynamic pose generation,
and sequential scene rendering.`,
	}
)

func SetRootContext(ctx context.Context) {
	rootCtx = ctx
	rootCmd.SetContext(ctx)
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		slog.Error("command failed", "error", err)
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file path (default ./config.yaml)")
	rootCmd.PersistentFlags().StringVarP(&outputDir, "output", "o", "./comix-output", "output directory")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "enable verbose logging")
	rootCmd.PersistentFlags().StringVar(&logFormat, "log-format", "text", "log format (text|json)")
}

func loadConfig() (*config.Config, error) {
	return loadConfigWithValidation(false)
}

func loadConfigForOpenAI() (*config.Config, error) {
	return loadConfigWithValidation(true)
}

func loadConfigWithValidation(requireOpenAI bool) (*config.Config, error) {
	cfg, err := config.LoadConfigWithOverrides(cfgFile, rootCmd.PersistentFlags())
	if err != nil {
		return nil, err
	}
	if rootCmd.PersistentFlags().Changed("output") {
		cfg.Pipeline.OutputDir = outputDir
	}
	if rootCmd.PersistentFlags().Changed("log-format") {
		cfg.Logging.Format = logFormat
	}
	var validateErr error
	if requireOpenAI {
		validateErr = cfg.ValidateForOpenAI()
	} else {
		validateErr = cfg.ValidateLocal()
	}
	if validateErr != nil {
		return nil, validateErr
	}
	return cfg, nil
}

func initConfig() {
	logger.Configure("info", logFormat, verbose)
}

package cli

import (
	"context"
	"os"

	"github.com/comix/comix/internal/config"
	"github.com/comix/comix/internal/logger"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
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
		logger.Error("command failed", "error", err)
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
	cfg, err := config.LoadConfig(cfgFile)
	if err != nil {
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func initConfig() {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		viper.AddConfigPath(".")
		viper.SetConfigName("config")
	}

	viper.SetEnvPrefix("COMIX")
	viper.AutomaticEnv()

	viper.BindPFlag("pipeline.output_dir", rootCmd.PersistentFlags().Lookup("output"))
	viper.BindPFlag("logging.format", rootCmd.PersistentFlags().Lookup("log-format"))

	if err := viper.ReadInConfig(); err == nil {
		logger.Info("using config file", "path", viper.ConfigFileUsed())
	}

	logger.Configure(viper.GetString("logging.level"), viper.GetString("logging.format"), verbose)
}

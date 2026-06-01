package config

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	OpenAI   OpenAIConfig   `mapstructure:"openai"`
	Pipeline PipelineConfig `mapstructure:"pipeline"`
	Server   ServerConfig   `mapstructure:"server"`
	Logging  LoggingConfig  `mapstructure:"logging"`
}

type OpenAIConfig struct {
	APIKey string     `mapstructure:"api_key"`
	LLM    LLMConfig  `mapstructure:"llm"`
	Image  ImageConfig `mapstructure:"image"`
}

type LLMConfig struct {
	Model          string        `mapstructure:"model"`
	Temperature    float64       `mapstructure:"temperature"`
	MaxRetries     int           `mapstructure:"max_retries"`
	RetryBaseDelay time.Duration `mapstructure:"retry_base_delay"`
}

type ImageConfig struct {
	Model          string        `mapstructure:"model"`
	Quality        string        `mapstructure:"quality"`
	Size           string        `mapstructure:"size"`
	Thinking       string        `mapstructure:"thinking"`
	MaxRetries     int           `mapstructure:"max_retries"`
	RetryBaseDelay time.Duration `mapstructure:"retry_base_delay"`
	RateLimitRPM   int           `mapstructure:"rate_limit_rpm"`
}

type PipelineConfig struct {
	OutputDir           string `mapstructure:"output_dir"`
	ChapterPattern      string `mapstructure:"chapter_pattern"`
	CoverFilename       string `mapstructure:"cover_filename"`
	MaxConcurrentSheets int    `mapstructure:"max_concurrent_sheets"`
	MaxConcurrentPoses  int    `mapstructure:"max_concurrent_poses"`
}

type ServerConfig struct {
	Host            string        `mapstructure:"host"`
	Port            int           `mapstructure:"port"`
	ReadTimeout     time.Duration `mapstructure:"read_timeout"`
	WriteTimeout    time.Duration `mapstructure:"write_timeout"`
	ShutdownTimeout time.Duration `mapstructure:"shutdown_timeout"`
}

type LoggingConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
}

func LoadConfig(path string) (*Config, error) {
	v := viper.New()

	if path != "" {
		v.SetConfigFile(path)
	} else {
		v.AddConfigPath(".")
		v.SetConfigName("config")
	}

	v.SetEnvPrefix("COMIX")
	v.AutomaticEnv()

	setDefaults(v)

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("reading config: %w", err)
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshaling config: %w", err)
	}

	return &cfg, nil
}

func (c *Config) Validate() error {
	var errs []string

	if c.OpenAI.APIKey == "" {
		errs = append(errs, "openai.api_key is required. Set it via COMIX_OPENAI_API_KEY env var, OPENAI_API_KEY env var, or in config.yaml")
	} else if !regexp.MustCompile(`^sk-`).MatchString(c.OpenAI.APIKey) {
		errs = append(errs, "openai.api_key should start with 'sk-'. Check your OpenAI API key is correct")
	}

	if c.OpenAI.LLM.Temperature < 0 || c.OpenAI.LLM.Temperature > 2 {
		errs = append(errs, "openai.llm.temperature must be between 0 and 2")
	}
	if c.OpenAI.LLM.MaxRetries < 0 {
		errs = append(errs, "openai.llm.max_retries must be >= 0")
	}
	if c.OpenAI.LLM.RetryBaseDelay <= 0 {
		errs = append(errs, "openai.llm.retry_base_delay must be > 0")
	}

	validQualities := map[string]bool{"low": true, "medium": true, "high": true}
	if !validQualities[c.OpenAI.Image.Quality] {
		errs = append(errs, fmt.Sprintf("openai.image.quality must be one of: low, medium, high (got %q)", c.OpenAI.Image.Quality))
	}
	validThinking := map[string]bool{"off": true, "low": true, "medium": true, "high": true}
	if !validThinking[c.OpenAI.Image.Thinking] {
		errs = append(errs, fmt.Sprintf("openai.image.thinking must be one of: off, low, medium, high (got %q)", c.OpenAI.Image.Thinking))
	}
	if c.OpenAI.Image.MaxRetries < 0 {
		errs = append(errs, "openai.image.max_retries must be >= 0")
	}
	if c.OpenAI.Image.RetryBaseDelay <= 0 {
		errs = append(errs, "openai.image.retry_base_delay must be > 0")
	}
	if c.OpenAI.Image.RateLimitRPM <= 0 {
		errs = append(errs, "openai.image.rate_limit_rpm must be > 0")
	}

	if c.Pipeline.MaxConcurrentSheets < 1 {
		errs = append(errs, "pipeline.max_concurrent_sheets must be >= 1")
	}
	if c.Pipeline.MaxConcurrentPoses < 1 {
		errs = append(errs, "pipeline.max_concurrent_poses must be >= 1")
	}

	if c.Server.Port < 1 || c.Server.Port > 65535 {
		errs = append(errs, fmt.Sprintf("server.port must be between 1 and 65535 (got %d)", c.Server.Port))
	}
	if c.Server.ReadTimeout <= 0 {
		errs = append(errs, "server.read_timeout must be > 0")
	}
	if c.Server.WriteTimeout <= 0 {
		errs = append(errs, "server.write_timeout must be > 0")
	}
	if c.Server.ShutdownTimeout <= 0 {
		errs = append(errs, "server.shutdown_timeout must be > 0")
	}

	validLevels := map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
	if !validLevels[strings.ToLower(c.Logging.Level)] {
		errs = append(errs, fmt.Sprintf("logging.level must be one of: debug, info, warn, error (got %q)", c.Logging.Level))
	}
	validFormats := map[string]bool{"text": true, "json": true}
	if !validFormats[strings.ToLower(c.Logging.Format)] {
		errs = append(errs, fmt.Sprintf("logging.format must be one of: text, json (got %q)", c.Logging.Format))
	}

	if len(errs) > 0 {
		return errors.New("configuration validation failed:\n  - " + strings.Join(errs, "\n  - "))
	}
	return nil
}

func (c *Config) RemediateAPIKey() string {
	return "Set OPENAI_API_KEY environment variable or configure openai.api_key in config.yaml"
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("openai.api_key", "")
	v.SetDefault("openai.llm.model", "gpt-4o")
	v.SetDefault("openai.llm.temperature", 0.1)
	v.SetDefault("openai.llm.max_retries", 5)
	v.SetDefault("openai.llm.retry_base_delay", "1s")
	v.SetDefault("openai.image.model", "gpt-image-2")
	v.SetDefault("openai.image.quality", "medium")
	v.SetDefault("openai.image.size", "1024x1024")
	v.SetDefault("openai.image.thinking", "medium")
	v.SetDefault("openai.image.max_retries", 5)
	v.SetDefault("openai.image.retry_base_delay", "2s")
	v.SetDefault("openai.image.rate_limit_rpm", 5)
	v.SetDefault("pipeline.output_dir", "./comix-output")
	v.SetDefault("pipeline.chapter_pattern", "chapter_*.md")
	v.SetDefault("pipeline.cover_filename", "cover.md")
	v.SetDefault("pipeline.max_concurrent_sheets", 3)
	v.SetDefault("pipeline.max_concurrent_poses", 2)
	v.SetDefault("server.host", "0.0.0.0")
	v.SetDefault("server.port", 8080)
	v.SetDefault("server.read_timeout", "30s")
	v.SetDefault("server.write_timeout", "60s")
	v.SetDefault("server.shutdown_timeout", "15s")
	v.SetDefault("logging.level", "info")
	v.SetDefault("logging.format", "text")
}

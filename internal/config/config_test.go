package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func validConfig() *Config {
	return &Config{
		OpenAI: OpenAIConfig{
			APIKey: "sk-test-key-12345",
			LLM: LLMConfig{
				Model:       "gpt-5.4-mini",
				Temperature: 0.1,
				Thinking:    "medium",
				MaxRetries:  5,
			},
			Image: ImageConfig{
				Model:   "gpt-image-2",
				Quality: "medium",
				Size: ImageSizeConfig{
					Sheet: "2880x1920",
					Poses: "2048x2048",
					Panel: "1632x3808",
				},
				MaxRetries: 5,
			},
		},
		Pipeline: PipelineConfig{
			OutputDir:           "./comix-output",
			ChapterPattern:      "chapter_*.md",
			CoverFilename:       "cover.md",
			MaxConcurrentSheets: 3,
			MaxConcurrentPoses:  2,
		},
		Server: ServerConfig{
			Host:            "0.0.0.0",
			Port:            8080,
			ReadTimeout:     30 * time.Second,
			WriteTimeout:    60 * time.Second,
			ShutdownTimeout: 15 * time.Second,
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "text",
		},
	}
}

func TestValidate_Success(t *testing.T) {
	cfg := validConfig()
	if err := cfg.Validate(); err != nil {
		t.Errorf("expected no error, got: %v", err)
	}
}

func TestValidate_EmptyAPIKey(t *testing.T) {
	cfg := validConfig()
	cfg.OpenAI.APIKey = ""
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for empty api_key")
	}
}

func TestValidate_APIKeyPrefix(t *testing.T) {
	cfg := validConfig()
	cfg.OpenAI.BaseURL = "https://api.openai.com/v1"
	cfg.OpenAI.APIKey = "invalid-key"
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for api_key not starting with sk-")
	}
}

func TestValidate_CustomBaseURLAllowsOpaqueAPIKey(t *testing.T) {
	cfg := validConfig()
	cfg.OpenAI.BaseURL = "http://localhost:11434/v1"
	cfg.OpenAI.APIKey = "local-token"
	if err := cfg.Validate(); err != nil {
		t.Errorf("expected custom base URL to allow opaque API key, got %v", err)
	}
}

func TestValidate_TemperatureOutOfRange(t *testing.T) {
	cfg := validConfig()
	cfg.OpenAI.LLM.Temperature = -0.1
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for temperature < 0")
	}

	cfg.OpenAI.LLM.Temperature = 2.1
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for temperature > 2")
	}
}

func TestValidate_LLMMaxRetriesNegative(t *testing.T) {
	cfg := validConfig()
	cfg.OpenAI.LLM.MaxRetries = -1
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for negative max_retries")
	}
}

func TestValidate_LLMThinkingInvalid(t *testing.T) {
	cfg := validConfig()
	cfg.OpenAI.LLM.Thinking = "extreme"
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for invalid llm thinking")
	}
}

func TestValidate_ImageQualityInvalid(t *testing.T) {
	cfg := validConfig()
	cfg.OpenAI.Image.Quality = "ultra"
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for invalid quality")
	}
}

func TestValidate_ImageMaxRetriesNegative(t *testing.T) {
	cfg := validConfig()
	cfg.OpenAI.Image.MaxRetries = -1
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for negative image max_retries")
	}
}

func TestValidate_MaxConcurrentSheetsZero(t *testing.T) {
	cfg := validConfig()
	cfg.Pipeline.MaxConcurrentSheets = 0
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for max_concurrent_sheets < 1")
	}
}

func TestValidate_MaxConcurrentPosesZero(t *testing.T) {
	cfg := validConfig()
	cfg.Pipeline.MaxConcurrentPoses = 0
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for max_concurrent_poses < 1")
	}
}

func TestValidate_PortOutOfRange(t *testing.T) {
	cfg := validConfig()
	cfg.Server.Port = 0
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for port 0")
	}

	cfg.Server.Port = 65536
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for port > 65535")
	}
}

func TestValidate_TimeoutZero(t *testing.T) {
	cfg := validConfig()
	cfg.Server.ReadTimeout = 0
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for read_timeout <= 0")
	}

	cfg.Server.ReadTimeout = 30 * time.Second
	cfg.Server.WriteTimeout = 0
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for write_timeout <= 0")
	}

	cfg.Server.WriteTimeout = 60 * time.Second
	cfg.Server.ShutdownTimeout = 0
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for shutdown_timeout <= 0")
	}
}

func TestValidate_LoggingLevelInvalid(t *testing.T) {
	cfg := validConfig()
	cfg.Logging.Level = "trace"
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for invalid logging level")
	}
}

func TestValidate_LoggingFormatInvalid(t *testing.T) {
	cfg := validConfig()
	cfg.Logging.Format = "xml"
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for invalid logging format")
	}
}

func TestValidate_CaseInsensitiveLevel(t *testing.T) {
	cfg := validConfig()
	cfg.Logging.Level = "DEBUG"
	if err := cfg.Validate(); err != nil {
		t.Errorf("expected no error for case-insensitive level, got: %v", err)
	}
}

func TestRemediateAPIKey(t *testing.T) {
	cfg := validConfig()
	msg := cfg.RemediateAPIKey()
	if msg == "" {
		t.Error("expected non-empty remediation message")
	}
}

func TestLoadConfig_NoPath(t *testing.T) {
	cfg, err := LoadConfig("")
	if err != nil {
		t.Fatalf("LoadConfig without path should succeed: %v", err)
	}
	if cfg.OpenAI.APIKey != "" {
		t.Error("expected empty api key when no config file")
	}
}

func TestLoadConfig_WithFile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	content := `
openai:
  api_key: "sk-from-file"
  llm:
    model: "gpt-5.4-mini"
pipeline:
  output_dir: "/tmp/test-output"
`
	if err := os.WriteFile(cfgPath, []byte(content), 0644); err != nil {
		t.Fatalf("writing config: %v", err)
	}

	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if cfg.OpenAI.APIKey != "sk-from-file" {
		t.Errorf("expected 'sk-from-file', got %q", cfg.OpenAI.APIKey)
	}
	if cfg.OpenAI.LLM.Model != "gpt-5.4-mini" {
		t.Errorf("expected 'gpt-5.4-mini', got %q", cfg.OpenAI.LLM.Model)
	}
	if cfg.Pipeline.OutputDir != "/tmp/test-output" {
		t.Errorf("expected '/tmp/test-output', got %q", cfg.Pipeline.OutputDir)
	}
}

func TestLoadConfig_Defaults(t *testing.T) {
	cfg, err := LoadConfig("")
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if cfg.OpenAI.LLM.Model != "gpt-5.4-mini" {
		t.Errorf("expected default model 'gpt-5.4-mini', got %q", cfg.OpenAI.LLM.Model)
	}
	if cfg.OpenAI.LLM.Temperature != 0.1 {
		t.Errorf("expected default temperature 0.1, got %f", cfg.OpenAI.LLM.Temperature)
	}
	if cfg.OpenAI.Image.Model != "gpt-image-2" {
		t.Errorf("expected default image model 'gpt-image-2', got %q", cfg.OpenAI.Image.Model)
	}
	if cfg.OpenAI.Image.Quality != "medium" {
		t.Errorf("expected default quality 'medium', got %q", cfg.OpenAI.Image.Quality)
	}
	if cfg.Pipeline.OutputDir != "./comix-output" {
		t.Errorf("expected default output dir './comix-output', got %q", cfg.Pipeline.OutputDir)
	}
	if cfg.Server.Port != 8080 {
		t.Errorf("expected default port 8080, got %d", cfg.Server.Port)
	}
}

func TestLoadConfig_NonExistentFile(t *testing.T) {
	_, err := LoadConfig("/nonexistent/path/config.yaml")
	if err == nil {
		t.Error("expected error for explicit nonexistent config file path")
	}
}

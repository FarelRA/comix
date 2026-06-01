package storage

import (
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

func ReadMarkdown(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("reading markdown %s: %w", path, err)
	}
	return string(data), nil
}

func SaveJSON(path string, data any) error {
	encoded, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding json: %w", err)
	}

	if err := EnsureDir(filepath.Dir(path)); err != nil {
		return err
	}

	if err := os.WriteFile(path, encoded, 0644); err != nil {
		return fmt.Errorf("writing json %s: %w", path, err)
	}
	return nil
}

func LoadJSON(path string, dest any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading json %s: %w", path, err)
	}

	if err := json.Unmarshal(data, dest); err != nil {
		return fmt.Errorf("decoding json %s: %w", path, err)
	}
	return nil
}

func SaveYAML(path string, data any) error {
	encoded, err := yaml.Marshal(data)
	if err != nil {
		return fmt.Errorf("encoding yaml: %w", err)
	}

	if err := EnsureDir(filepath.Dir(path)); err != nil {
		return err
	}

	if err := os.WriteFile(path, encoded, 0644); err != nil {
		return fmt.Errorf("writing yaml %s: %w", path, err)
	}
	return nil
}

func LoadYAML(path string, dest any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading yaml %s: %w", path, err)
	}

	if err := yaml.Unmarshal(data, dest); err != nil {
		return fmt.Errorf("decoding yaml %s: %w", path, err)
	}
	return nil
}

func SavePNG(path string, img image.Image) error {
	if err := EnsureDir(filepath.Dir(path)); err != nil {
		return err
	}

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("creating png %s: %w", path, err)
	}
	defer f.Close()

	if err := png.Encode(f, img); err != nil {
		return fmt.Errorf("encoding png %s: %w", path, err)
	}
	return nil
}

func EnsureDir(path string) error {
	if path == "" {
		return nil
	}
	return os.MkdirAll(path, 0755)
}

func DirectoryExists(path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("checking directory %s: %w", path, err)
	}
	return info.IsDir(), nil
}

package storage

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"

	"github.com/google/renameio/v2/maybe"
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

	if err := WriteFileAtomic(path, encoded, 0644); err != nil {
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

	if err := WriteFileAtomic(path, encoded, 0644); err != nil {
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
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return fmt.Errorf("encoding png %s: %w", path, err)
	}
	if err := maybe.WriteFile(path, buf.Bytes(), 0644); err != nil {
		return fmt.Errorf("writing png %s: %w", path, err)
	}
	return nil
}

func WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	if err := EnsureDir(filepath.Dir(path)); err != nil {
		return err
	}
	return maybe.WriteFile(path, data, perm)
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

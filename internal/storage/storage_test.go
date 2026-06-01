package storage

import (
	"image"
	"image/color"
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureDir(t *testing.T) {
	dir := t.TempDir()
	testDir := filepath.Join(dir, "a", "b", "c")

	if err := EnsureDir(testDir); err != nil {
		t.Fatalf("EnsureDir failed: %v", err)
	}

	info, err := os.Stat(testDir)
	if err != nil {
		t.Fatalf("stat failed: %v", err)
	}
	if !info.IsDir() {
		t.Error("expected directory")
	}
}

func TestDirectoryExists(t *testing.T) {
	dir := t.TempDir()

	exists, err := DirectoryExists(dir)
	if err != nil {
		t.Fatalf("DirectoryExists failed: %v", err)
	}
	if !exists {
		t.Error("expected directory to exist")
	}

	nonexistent := filepath.Join(dir, "nonexistent")
	exists, err = DirectoryExists(nonexistent)
	if err != nil {
		t.Fatalf("DirectoryExists failed: %v", err)
	}
	if exists {
		t.Error("expected directory to not exist")
	}
}

func TestSaveAndLoadJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.json")

	type TestData struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}

	original := TestData{Name: "alice", Value: 42}
	if err := SaveJSON(path, original); err != nil {
		t.Fatalf("SaveJSON failed: %v", err)
	}

	var loaded TestData
	if err := LoadJSON(path, &loaded); err != nil {
		t.Fatalf("LoadJSON failed: %v", err)
	}

	if loaded.Name != original.Name || loaded.Value != original.Value {
		t.Errorf("got %+v, want %+v", loaded, original)
	}
}

func TestSaveAndLoadYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.yaml")

	type TestData struct {
		Name  string `yaml:"name"`
		Value int    `yaml:"value"`
	}

	original := TestData{Name: "alice", Value: 42}
	if err := SaveYAML(path, original); err != nil {
		t.Fatalf("SaveYAML failed: %v", err)
	}

	var loaded TestData
	if err := LoadYAML(path, &loaded); err != nil {
		t.Fatalf("LoadYAML failed: %v", err)
	}

	if loaded.Name != original.Name || loaded.Value != original.Value {
		t.Errorf("got %+v, want %+v", loaded, original)
	}
}

func TestReadMarkdown(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.md")
	content := "# Chapter 1\n\nOnce upon a time..."

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	read, err := ReadMarkdown(path)
	if err != nil {
		t.Fatalf("ReadMarkdown failed: %v", err)
	}
	if read != content {
		t.Errorf("got %q, want %q", read, content)
	}
}

func TestReadMarkdown_NotFound(t *testing.T) {
	_, err := ReadMarkdown("/nonexistent/file.md")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestSavePNG(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.png")

	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	img.Set(0, 0, color.RGBA{255, 0, 0, 255})

	if err := SavePNG(path, img); err != nil {
		t.Fatalf("SavePNG failed: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat failed: %v", err)
	}
	if info.Size() == 0 {
		t.Error("expected non-empty PNG file")
	}
}

func TestSaveJSON_CreatesDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "newdir", "data.json")

	if err := SaveJSON(path, map[string]string{"key": "val"}); err != nil {
		t.Fatalf("SaveJSON failed: %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Errorf("expected file to exist: %v", err)
	}
}

func TestLoadJSON_NotFound(t *testing.T) {
	var dest any
	err := LoadJSON("/nonexistent/file.json", &dest)
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

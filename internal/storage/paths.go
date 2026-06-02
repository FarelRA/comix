package storage

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/gosimple/slug"
)

func ValidateName(name string) error {
	if name == "" || strings.Contains(name, "..") || filepath.Base(name) != name || slug.Make(name) != name {
		return fmt.Errorf("invalid name %q: use a URL-safe slug", name)
	}
	return nil
}

func SlugName(name string) string { return slug.Make(name) }

func SafeProjectDir(root, project string) (string, error) {
	if err := ValidateName(project); err != nil {
		return "", err
	}
	return filepath.Join(root, project), nil
}

func SafeJoin(root string, parts ...string) (string, error) {
	base, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolving root: %w", err)
	}
	full, err := filepath.Abs(filepath.Join(append([]string{root}, parts...)...))
	if err != nil {
		return "", fmt.Errorf("resolving path: %w", err)
	}
	rel, err := filepath.Rel(base, full)
	if err != nil {
		return "", fmt.Errorf("checking relative path: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("path escapes root")
	}
	return full, nil
}

func ProjectDir(root, project string) string {
	return filepath.Join(root, project)
}

func RawDir(root, project string) string {
	return filepath.Join(root, project, "raw")
}

func StateDir(root, project string) string {
	return filepath.Join(root, project, "state")
}

func SheetsDir(root, project string) string {
	return filepath.Join(root, project, "sheets")
}

func PosesDir(root, project string) string {
	return filepath.Join(root, project, "poses")
}

func PanelsDir(root, project string) string {
	return filepath.Join(root, project, "panels")
}

func CharactersPath(root, project string) string {
	return filepath.Join(StateDir(root, project), "characters.json")
}

func ScenesPath(root, project string) string {
	return filepath.Join(StateDir(root, project), "scenes.json")
}

func ManifestPath(root, project string) string {
	return filepath.Join(ProjectDir(root, project), "project.yaml")
}

func ManifestLockPath(root, project string) string {
	return filepath.Join(ProjectDir(root, project), ".project.lock")
}

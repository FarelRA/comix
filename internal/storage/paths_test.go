package storage

import (
	"path/filepath"
	"testing"
)

func TestPathFunctions(t *testing.T) {
	root := "/tmp/comix-output"
	project := "alice"

	tests := []struct {
		name string
		got  string
		want string
	}{
		{"ProjectDir", ProjectDir(root, project), filepath.Join(root, project)},
		{"RawDir", RawDir(root, project), filepath.Join(root, project, "raw")},
		{"StateDir", StateDir(root, project), filepath.Join(root, project, "state")},
		{"SheetsDir", SheetsDir(root, project), filepath.Join(root, project, "sheets")},
		{"PosesDir", PosesDir(root, project), filepath.Join(root, project, "poses")},
		{"PanelsDir", PanelsDir(root, project), filepath.Join(root, project, "panels")},
		{"CharactersPath", CharactersPath(root, project), filepath.Join(root, project, "state", "characters.json")},
		{"ScenesPath", ScenesPath(root, project), filepath.Join(root, project, "state", "scenes.json")},
		{"ManifestPath", ManifestPath(root, project), filepath.Join(root, project, "project.yaml")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("got %q, want %q", tt.got, tt.want)
			}
		})
	}
}

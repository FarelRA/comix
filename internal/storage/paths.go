package storage

import "path/filepath"

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

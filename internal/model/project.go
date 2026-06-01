package model

import (
	"fmt"
	"time"
)

type ProjectManifest struct {
	Project  ProjectMeta      `json:"project" yaml:"project"`
	Pipeline PipelineProgress `json:"pipeline" yaml:"pipeline"`
}

type ProjectMeta struct {
	Name      string        `json:"name" yaml:"name"`
	CreatedAt time.Time     `json:"created_at" yaml:"created_at"`
	Source    SourceInfo    `json:"source" yaml:"source"`
	Chapters  []ChapterMeta `json:"chapters" yaml:"chapters"`
}

type SourceInfo struct {
	Type string `json:"type" yaml:"type"`
	Path string `json:"path" yaml:"path"`
}

type ChapterMeta struct {
	ID        string `json:"id" yaml:"id"`
	Filename  string `json:"filename" yaml:"filename"`
	Title     string `json:"title" yaml:"title"`
	WordCount int    `json:"word_count" yaml:"word_count"`
}

type PipelineProgress struct {
	Status       string                 `json:"status" yaml:"status"`
	CurrentPhase int                    `json:"current_phase" yaml:"current_phase"`
	Phases       map[string]PhaseStatus `json:"phases" yaml:"phases"`
	Errors       []PhaseError           `json:"errors,omitempty" yaml:"errors,omitempty"`
}

type PhaseStatus struct {
	Status      string     `json:"status" yaml:"status"`
	StartedAt   *time.Time `json:"started_at,omitempty" yaml:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty" yaml:"completed_at,omitempty"`
}

type PhaseError struct {
	Phase       string    `json:"phase" yaml:"phase"`
	Timestamp   time.Time `json:"timestamp" yaml:"timestamp"`
	Message     string    `json:"message" yaml:"message"`
	Recoverable bool      `json:"recoverable" yaml:"recoverable"`
}

const (
	PhaseIdle       = "idle"
	PhaseInProgress = "in_progress"
	PhaseCompleted  = "completed"
	PhaseFailed     = "failed"
	PhasePending    = "pending"

	PhaseNameIngest     = "ingest"
	PhaseNameCharacters = "characters"
	PhaseNameScenes     = "scenes"
	PhaseNameSheets     = "sheets"
	PhaseNamePoses      = "poses"
	PhaseNameRender     = "render"
)

var AllPhases = []string{
	PhaseNameIngest,
	PhaseNameCharacters,
	PhaseNameScenes,
	PhaseNameSheets,
	PhaseNamePoses,
	PhaseNameRender,
}

var PhaseNumbers = map[string]int{
	PhaseNameIngest:     1,
	PhaseNameCharacters: 2,
	PhaseNameScenes:     3,
	PhaseNameSheets:     4,
	PhaseNamePoses:      5,
	PhaseNameRender:     6,
}

func (pm *ProjectManifest) Validate() error {
	if pm.Project.Name == "" {
		return fmt.Errorf("project manifest: project name is required")
	}
	if pm.Project.CreatedAt.IsZero() {
		return fmt.Errorf("project manifest: created_at is required")
	}
	if len(pm.Project.Chapters) == 0 {
		return fmt.Errorf("project manifest: at least one chapter is required")
	}
	for i, ch := range pm.Project.Chapters {
		if err := ch.Validate(); err != nil {
			return fmt.Errorf("project manifest: chapter[%d]: %w", i, err)
		}
	}
	return nil
}

func (cm *ChapterMeta) Validate() error {
	if cm.ID == "" {
		return fmt.Errorf("chapter id is required")
	}
	if cm.Filename == "" {
		return fmt.Errorf("chapter filename is required")
	}
	return nil
}

func NewProjectManifest(name, sourceType, sourcePath string, chapters []ChapterMeta) *ProjectManifest {
	phases := make(map[string]PhaseStatus)
	for _, p := range AllPhases {
		status := PhasePending
		if p == PhaseNameIngest {
			status = PhaseIdle
		}
		phases[p] = PhaseStatus{Status: status}
	}

	return &ProjectManifest{
		Project: ProjectMeta{
			Name:      name,
			CreatedAt: time.Now().UTC(),
			Source: SourceInfo{
				Type: sourceType,
				Path: sourcePath,
			},
			Chapters: chapters,
		},
		Pipeline: PipelineProgress{
			Status:       PhaseIdle,
			CurrentPhase: 0,
			Phases:       phases,
			Errors:       []PhaseError{},
		},
	}
}

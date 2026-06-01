package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"

	"github.com/comix/comix/internal/logger"
	"github.com/comix/comix/internal/model"
	"github.com/comix/comix/internal/pipeline"
	"github.com/comix/comix/internal/state"
	"github.com/comix/comix/internal/storage"
)

type envelope struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data"`
	Error   *apiError   `json:"error"`
	Meta   meta        `json:"meta"`
}

type apiError struct {
	Code    string      `json:"code"`
	Message string      `json:"message"`
	Details interface{} `json:"details,omitempty"`
}

type meta struct {
	RequestID string    `json:"request_id"`
	Timestamp time.Time `json:"timestamp"`
}

func writeJSON(w http.ResponseWriter, r *http.Request, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	reqID := chiMiddleware.GetReqID(r.Context())
	resp := envelope{
		Success: status >= 200 && status < 400,
		Data:    data,
		Meta: meta{
			RequestID: reqID,
			Timestamp: time.Now().UTC(),
		},
	}

	json.NewEncoder(w).Encode(resp)
}

func writeError(w http.ResponseWriter, r *http.Request, status int, code, message string, details interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	reqID := chiMiddleware.GetReqID(r.Context())
	resp := envelope{
		Success: false,
		Data:    nil,
		Error: &apiError{
			Code:    code,
			Message: message,
			Details: details,
		},
		Meta: meta{
			RequestID: reqID,
			Timestamp: time.Now().UTC(),
		},
	}

	json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, r, http.StatusOK, map[string]string{
		"status": "ok",
		"version": "0.1.0",
	})
}

func (s *Server) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name       string `json:"name"`
		SourceType string `json:"source_type"`
		SourcePath string `json:"source_path"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "Invalid JSON body", nil)
		return
	}

	if req.Name == "" {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "Project name is required", nil)
		return
	}

	exists, err := state.ManifestExists(s.cfg.Pipeline.OutputDir, req.Name)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "CHECK_FAILED", err.Error(), nil)
		return
	}
	if exists {
		writeError(w, r, http.StatusConflict, "PROJECT_EXISTS", fmt.Sprintf("Project %q already exists", req.Name), nil)
		return
	}

	manifest := model.NewProjectManifest(req.Name, req.SourceType, req.SourcePath, nil)
	if err := state.SaveManifest(s.cfg.Pipeline.OutputDir, req.Name, manifest); err != nil {
		writeError(w, r, http.StatusInternalServerError, "SAVE_FAILED", err.Error(), nil)
		return
	}

	writeJSON(w, r, http.StatusCreated, manifest)
}

func (s *Server) handleListProjects(w http.ResponseWriter, r *http.Request) {
	root := s.cfg.Pipeline.OutputDir
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			writeJSON(w, r, http.StatusOK, []interface{}{})
			return
		}
		writeError(w, r, http.StatusInternalServerError, "READ_FAILED", err.Error(), nil)
		return
	}

	var projects []*model.ProjectManifest
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		manifestPath := filepath.Join(root, entry.Name(), "project.yaml")
		if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
			continue
		}
		m, err := state.LoadManifest(s.cfg.Pipeline.OutputDir, entry.Name())
		if err != nil {
			continue
		}
		projects = append(projects, m)
	}

	if projects == nil {
		projects = []*model.ProjectManifest{}
	}

	writeJSON(w, r, http.StatusOK, projects)
}

func (s *Server) handleDeleteProject(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if _, ok := s.tasks.Load(id); ok {
		writeError(w, r, http.StatusConflict, "PROJECT_BUSY", fmt.Sprintf("Project %q has a running task", id), nil)
		return
	}

	projectDir := storage.ProjectDir(s.cfg.Pipeline.OutputDir, id)
	exists, err := storage.DirectoryExists(projectDir)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "CHECK_FAILED", err.Error(), nil)
		return
	}
	if !exists {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", fmt.Sprintf("Project %q not found", id), nil)
		return
	}

	if err := os.RemoveAll(projectDir); err != nil {
		writeError(w, r, http.StatusInternalServerError, "DELETE_FAILED", err.Error(), nil)
		return
	}

	writeJSON(w, r, http.StatusOK, map[string]string{
		"deleted": id,
	})
}

func (s *Server) handleIngest(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	exists, err := state.ManifestExists(s.cfg.Pipeline.OutputDir, id)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "CHECK_FAILED", err.Error(), nil)
		return
	}
	if !exists {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", fmt.Sprintf("Project %q not found. Create it first via POST /api/projects", id), nil)
		return
	}

	manifest, err := state.LoadManifest(s.cfg.Pipeline.OutputDir, id)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "LOAD_FAILED", err.Error(), nil)
		return
	}

	if manifest.Pipeline.Phases[model.PhaseNameIngest].Status == model.PhaseCompleted {
		writeError(w, r, http.StatusConflict, "ALREADY_INGESTED", fmt.Sprintf("Project %q has already been ingested", id), nil)
		return
	}

	if err := r.ParseMultipartForm(128 << 20); err != nil {
		writeError(w, r, http.StatusBadRequest, "INVALID_UPLOAD", "Failed to parse multipart form: "+err.Error(), nil)
		return
	}

	tmpDir, err := os.MkdirTemp("", "comix-ingest-*")
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "TMP_DIR_FAILED", err.Error(), nil)
		return
	}
	defer os.RemoveAll(tmpDir)

	coverFile, coverHeader, err := r.FormFile("cover")
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "MISSING_COVER", "Cover file is required (field name: 'cover')", nil)
		return
	}
	defer coverFile.Close()

	coverData := make([]byte, coverHeader.Size)
	if _, err := coverFile.Read(coverData); err != nil {
		writeError(w, r, http.StatusInternalServerError, "READ_FAILED", "Failed to read cover file: "+err.Error(), nil)
		return
	}

	coverPath := filepath.Join(tmpDir, "cover.md")
	if err := os.WriteFile(coverPath, coverData, 0644); err != nil {
		writeError(w, r, http.StatusInternalServerError, "WRITE_FAILED", err.Error(), nil)
		return
	}

	chapters := r.MultipartForm.File["chapters"]
	if len(chapters) == 0 {
		writeError(w, r, http.StatusBadRequest, "MISSING_CHAPTERS", "At least one chapter file is required (field name: 'chapters')", nil)
		return
	}

	var chapterPaths []string
	for i, fh := range chapters {
		f, err := fh.Open()
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "READ_FAILED", fmt.Sprintf("Failed to read chapter[%d]: %s", i, err.Error()), nil)
			return
		}
		defer f.Close()

		data := make([]byte, fh.Size)
		if _, err := f.Read(data); err != nil {
			writeError(w, r, http.StatusInternalServerError, "READ_FAILED", fmt.Sprintf("Failed to read chapter[%d]: %s", i, err.Error()), nil)
			return
		}

		chPath := filepath.Join(tmpDir, fh.Filename)
		if err := os.WriteFile(chPath, data, 0644); err != nil {
			writeError(w, r, http.StatusInternalServerError, "WRITE_FAILED", err.Error(), nil)
			return
		}
		chapterPaths = append(chapterPaths, chPath)
	}

	source := pipeline.IngestSource{
		BookDir:  tmpDir,
		Cover:    coverPath,
		Chapters: chapterPaths,
	}

	ctx := r.Context()
	if err := s.pipeline.Run(ctx, id, source, []string{model.PhaseNameIngest}, false); err != nil {
		writeError(w, r, http.StatusInternalServerError, "INGEST_FAILED", err.Error(), nil)
		return
	}

	updated, err := state.LoadManifest(s.cfg.Pipeline.OutputDir, id)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "LOAD_FAILED", err.Error(), nil)
		return
	}

	writeJSON(w, r, http.StatusOK, updated)
}

func (s *Server) handleGetStatus(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	exists, err := state.ManifestExists(s.cfg.Pipeline.OutputDir, id)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "CHECK_FAILED", err.Error(), nil)
		return
	}
	if !exists {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", fmt.Sprintf("Project %q not found", id), nil)
		return
	}

	manifest, err := state.LoadManifest(s.cfg.Pipeline.OutputDir, id)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "LOAD_FAILED", err.Error(), nil)
		return
	}

	_, running := s.tasks.Load(id)

	writeJSON(w, r, http.StatusOK, map[string]interface{}{
		"project":   manifest.Project,
		"pipeline":  manifest.Pipeline,
		"running":   running,
	})
}

func (s *Server) handleRunPipeline(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	exists, err := state.ManifestExists(s.cfg.Pipeline.OutputDir, id)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "CHECK_FAILED", err.Error(), nil)
		return
	}
	if !exists {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", fmt.Sprintf("Project %q not found", id), nil)
		return
	}

	if _, loaded := s.tasks.LoadOrStore(id, nil); loaded {
		writeError(w, r, http.StatusConflict, "ALREADY_RUNNING", fmt.Sprintf("A pipeline is already running for project %q", id), nil)
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	s.tasks.Store(id, cancel)

	writeJSON(w, r, http.StatusAccepted, map[string]string{
		"status":     "started",
		"project_id": id,
		"message":    "Pipeline execution started. Poll GET /api/projects/" + id + "/status for progress.",
	})

	go func() {
		defer func() {
			s.tasks.Delete(id)
			cancel()
		}()

		source := pipeline.IngestSource{}
		if err := s.pipeline.Run(ctx, id, source, nil, true); err != nil {
			logger.Error("pipeline run failed", "project", id, "error", err)
		}
	}()
}

func (s *Server) handleRunPhase(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	phase := chi.URLParam(r, "phase")

	validPhases := map[string]bool{
		model.PhaseNameCharacters: true,
		model.PhaseNameScenes:     true,
		model.PhaseNameSheets:     true,
		model.PhaseNamePoses:      true,
		model.PhaseNameRender:     true,
	}

	if !validPhases[phase] {
		writeError(w, r, http.StatusBadRequest, "INVALID_PHASE", fmt.Sprintf("Invalid phase %q. Valid: characters, scenes, sheets, poses, render", phase), nil)
		return
	}

	exists, err := state.ManifestExists(s.cfg.Pipeline.OutputDir, id)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "CHECK_FAILED", err.Error(), nil)
		return
	}
	if !exists {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", fmt.Sprintf("Project %q not found", id), nil)
		return
	}

	taskKey := id + ":" + phase
	if _, loaded := s.tasks.LoadOrStore(taskKey, nil); loaded {
		writeError(w, r, http.StatusConflict, "ALREADY_RUNNING", fmt.Sprintf("Phase %q is already running for project %q", phase, id), nil)
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	s.tasks.Store(taskKey, cancel)

	writeJSON(w, r, http.StatusAccepted, map[string]string{
		"status":     "started",
		"project_id": id,
		"phase":      phase,
		"message":    fmt.Sprintf("Phase %q execution started.", phase),
	})

	go func() {
		defer func() {
			s.tasks.Delete(taskKey)
			cancel()
		}()

		source := pipeline.IngestSource{}
		if err := s.pipeline.Run(ctx, id, source, []string{phase}, true); err != nil {
			logger.Error("phase run failed", "phase", phase, "project", id, "error", err)
		}
	}()
}

func (s *Server) handleListOutputs(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	projectDir := storage.ProjectDir(s.cfg.Pipeline.OutputDir, id)
	exists, err := storage.DirectoryExists(projectDir)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "CHECK_FAILED", err.Error(), nil)
		return
	}
	if !exists {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", fmt.Sprintf("Project %q not found", id), nil)
		return
	}

	var files []map[string]interface{}
	err = filepath.Walk(projectDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		relPath, _ := filepath.Rel(projectDir, path)
		if relPath == "project.yaml" {
			return nil
		}

		files = append(files, map[string]interface{}{
			"path":         relPath,
			"size":         info.Size(),
			"modified_at":  info.ModTime().UTC(),
		})
		return nil
	})
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "WALK_FAILED", err.Error(), nil)
		return
	}

	if files == nil {
		files = []map[string]interface{}{}
	}

	writeJSON(w, r, http.StatusOK, files)
}

func (s *Server) handleGetOutput(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	filePath := chi.URLParam(r, "*")

	projectDir := storage.ProjectDir(s.cfg.Pipeline.OutputDir, id)
	fullPath := filepath.Join(projectDir, filePath)

	absProject, _ := filepath.Abs(projectDir)
	absFull, _ := filepath.Abs(fullPath)

	if !strings.HasPrefix(absFull, absProject) {
		writeError(w, r, http.StatusForbidden, "PATH_TRAVERSAL", "Access denied", nil)
		return
	}

	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", fmt.Sprintf("File %q not found", filePath), nil)
		return
	}

	w.Header().Set("Content-Type", detectContentType(fullPath))
	http.ServeFile(w, r, fullPath)
}

func detectContentType(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".json":
		return "application/json"
	case ".yaml", ".yml":
		return "application/x-yaml"
	case ".md":
		return "text/markdown"
	default:
		return "application/octet-stream"
	}
}

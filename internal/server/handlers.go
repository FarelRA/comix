package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-playground/validator/v10"

	"github.com/FarelRA/comix/internal/model"
	"github.com/FarelRA/comix/internal/pipeline"
	"github.com/FarelRA/comix/internal/state"
	"github.com/FarelRA/comix/internal/storage"
)

var validate = validator.New()

const maxUploadBytes = 128 << 20
const maxJSONBodyBytes = 1 << 20

func projectID(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := chi.URLParam(r, "id")
	if err := storage.ValidateName(id); err != nil {
		writeError(w, r, http.StatusBadRequest, "INVALID_PROJECT_ID", err.Error(), nil)
		return "", false
	}
	return id, true
}

type envelope struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data"`
	Error   *apiError   `json:"error"`
	Meta    meta        `json:"meta"`
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

	if err := json.NewEncoder(w).Encode(resp); err != nil {
		slog.Error("writing json response failed", "error", err)
	}
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

	if err := json.NewEncoder(w).Encode(resp); err != nil {
		slog.Error("writing error response failed", "error", err)
	}
}

func validationDetails(err error) map[string]string {
	details := make(map[string]string)
	var validationErrs validator.ValidationErrors
	if !errors.As(err, &validationErrs) {
		details["error"] = err.Error()
		return details
	}
	for _, fieldErr := range validationErrs {
		details[fieldErr.Field()] = fieldErr.Tag()
	}
	return details
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, r, http.StatusOK, map[string]string{
		"status":  "ok",
		"version": "0.1.0",
	})
}

func (s *Server) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBodyBytes)
	var req struct {
		Name       string `json:"name" validate:"required"`
		SourceType string `json:"source_type"`
		SourcePath string `json:"source_path"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "Invalid JSON body", nil)
		return
	}

	if err := validate.Struct(req); err != nil {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body", validationDetails(err))
		return
	}
	if err := storage.ValidateName(req.Name); err != nil {
		writeError(w, r, http.StatusBadRequest, "INVALID_PROJECT_ID", err.Error(), nil)
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
	id, ok := projectID(w, r)
	if !ok {
		return
	}

	if _, ok := s.tasks.Load(id); ok {
		writeError(w, r, http.StatusConflict, "PROJECT_BUSY", fmt.Sprintf("Project %q has a running task", id), nil)
		return
	}

	projectDir, err := storage.SafeProjectDir(s.cfg.Pipeline.OutputDir, id)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "INVALID_PROJECT_ID", err.Error(), nil)
		return
	}
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
	id, ok := projectID(w, r)
	if !ok {
		return
	}

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

	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)
	if err := r.ParseMultipartForm(maxUploadBytes); err != nil {
		writeError(w, r, http.StatusBadRequest, "INVALID_UPLOAD", "Failed to parse multipart form: "+err.Error(), nil)
		return
	}

	tmpDir, err := os.MkdirTemp("", "comix-ingest-*")
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "TMP_DIR_FAILED", err.Error(), nil)
		return
	}
	defer os.RemoveAll(tmpDir)

	coverFile, _, err := r.FormFile("cover")
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "MISSING_COVER", "Cover file is required (field name: 'cover')", nil)
		return
	}
	defer coverFile.Close()

	coverData, err := io.ReadAll(io.LimitReader(coverFile, maxUploadBytes+1))
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "READ_FAILED", "Failed to read cover file: "+err.Error(), nil)
		return
	}
	if int64(len(coverData)) > maxUploadBytes {
		writeError(w, r, http.StatusRequestEntityTooLarge, "UPLOAD_TOO_LARGE", "Cover file is too large", nil)
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
		filename, err := safeUploadFilename(fh.Filename)
		if err != nil {
			writeError(w, r, http.StatusBadRequest, "INVALID_UPLOAD_FILENAME", err.Error(), nil)
			return
		}

		err = func() error {
			f, err := fh.Open()
			if err != nil {
				return fmt.Errorf("opening chapter: %w", err)
			}
			defer f.Close()

			data, err := io.ReadAll(io.LimitReader(f, maxUploadBytes+1))
			if err != nil {
				return fmt.Errorf("reading chapter: %w", err)
			}
			if int64(len(data)) > maxUploadBytes {
				return fmt.Errorf("chapter too large")
			}

			chPath := filepath.Join(tmpDir, filename)
			if err := os.WriteFile(chPath, data, 0644); err != nil {
				return fmt.Errorf("writing chapter: %w", err)
			}
			chapterPaths = append(chapterPaths, chPath)
			return nil
		}()
		if err != nil {
			writeError(w, r, http.StatusInternalServerError, "READ_FAILED", fmt.Sprintf("Failed to process chapter[%d]: %s", i, err.Error()), nil)
			return
		}
	}

	source := pipeline.IngestSource{
		ProjectName:   id,
		BookDir:       tmpDir,
		Cover:         coverPath,
		Chapters:      chapterPaths,
		AllowExisting: true,
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
	id, ok := projectID(w, r)
	if !ok {
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

	manifest, err := state.LoadManifest(s.cfg.Pipeline.OutputDir, id)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "LOAD_FAILED", err.Error(), nil)
		return
	}

	_, running := s.tasks.Load(id)
	var task *taskSnapshot
	if value, ok := s.tasks.Load(id); ok {
		if current, ok := value.(*taskState); ok {
			task = current.snapshot()
		}
	}

	writeJSON(w, r, http.StatusOK, map[string]interface{}{
		"project":  manifest.Project,
		"pipeline": manifest.Pipeline,
		"running":  running,
		"task":     task,
	})
}

func (s *Server) handleRunPipeline(w http.ResponseWriter, r *http.Request) {
	id, ok := projectID(w, r)
	if !ok {
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

	ctx, cancel := context.WithCancel(context.Background())
	task := &taskState{Cancel: cancel, ProjectID: id, Phase: "all", Status: "running", StartedAt: time.Now().UTC()}
	if _, loaded := s.tasks.LoadOrStore(id, task); loaded {
		cancel()
		writeError(w, r, http.StatusConflict, "ALREADY_RUNNING", fmt.Sprintf("A pipeline is already running for project %q", id), nil)
		return
	}

	writeJSON(w, r, http.StatusAccepted, map[string]string{
		"status":     "started",
		"project_id": id,
		"message":    "Pipeline execution started. Poll GET /api/projects/" + id + "/status for progress.",
	})

	go func() {
		defer func() {
			task.finish()
			s.tasks.Delete(id)
			cancel()
		}()

		source := pipeline.IngestSource{}
		if err := s.pipeline.Run(ctx, id, source, nil, true); err != nil {
			task.fail(err)
			slog.Error("pipeline run failed", "project", id, "error", err)
		}
	}()
}

func (s *Server) handleRunPhase(w http.ResponseWriter, r *http.Request) {
	id, ok := projectID(w, r)
	if !ok {
		return
	}
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

	taskKey := id
	ctx, cancel := context.WithCancel(context.Background())
	task := &taskState{Cancel: cancel, ProjectID: id, Phase: phase, Status: "running", StartedAt: time.Now().UTC()}
	if _, loaded := s.tasks.LoadOrStore(taskKey, task); loaded {
		cancel()
		writeError(w, r, http.StatusConflict, "ALREADY_RUNNING", fmt.Sprintf("Phase %q is already running for project %q", phase, id), nil)
		return
	}

	writeJSON(w, r, http.StatusAccepted, map[string]string{
		"status":     "started",
		"project_id": id,
		"phase":      phase,
		"message":    fmt.Sprintf("Phase %q execution started.", phase),
	})

	go func() {
		defer func() {
			task.finish()
			s.tasks.Delete(taskKey)
			cancel()
		}()

		source := pipeline.IngestSource{}
		if err := s.pipeline.Run(ctx, id, source, []string{phase}, true); err != nil {
			task.fail(err)
			slog.Error("phase run failed", "phase", phase, "project", id, "error", err)
		}
	}()
}

func (s *Server) handleListOutputs(w http.ResponseWriter, r *http.Request) {
	id, ok := projectID(w, r)
	if !ok {
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

	var files []map[string]interface{}
	err = filepath.WalkDir(projectDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}

		relPath, _ := filepath.Rel(projectDir, path)
		if relPath == "project.yaml" {
			return nil
		}

		files = append(files, map[string]interface{}{
			"path":        relPath,
			"size":        info.Size(),
			"modified_at": info.ModTime().UTC(),
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
	id, ok := projectID(w, r)
	if !ok {
		return
	}
	filePath := chi.URLParam(r, "*")

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

	projectFS := os.DirFS(projectDir)
	w.Header().Set("Content-Type", detectContentType(filePath))
	http.ServeFileFS(w, r, projectFS, filePath)
}

func safeUploadFilename(name string) (string, error) {
	base := filepath.Base(name)
	if base == "." || base == string(filepath.Separator) || base == "" || base != name || strings.ContainsAny(name, `/\\`) {
		return "", fmt.Errorf("invalid upload filename %q", name)
	}
	return base, nil
}

func detectContentType(path string) string {
	if ct := mime.TypeByExtension(strings.ToLower(filepath.Ext(path))); ct != "" {
		return strings.Split(ct, ";")[0]
	}
	return "application/octet-stream"
}

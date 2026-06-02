package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/go-chi/httprate"

	"github.com/FarelRA/comix/internal/config"
	"github.com/FarelRA/comix/internal/pipeline"
)

type Server struct {
	cfg      *config.Config
	pipeline *pipeline.Pipeline
	router   chi.Router
	srv      *http.Server
	tasks    sync.Map
}

type taskState struct {
	mu        sync.Mutex
	Cancel    context.CancelFunc
	ProjectID string
	Phase     string
	Status    string
	StartedAt time.Time
	EndedAt   *time.Time
	Error     string
}

type taskSnapshot struct {
	ProjectID string     `json:"project_id"`
	Phase     string     `json:"phase"`
	Status    string     `json:"status"`
	StartedAt time.Time  `json:"started_at"`
	EndedAt   *time.Time `json:"ended_at,omitempty"`
	Error     string     `json:"error,omitempty"`
}

func (t *taskState) snapshot() *taskSnapshot {
	t.mu.Lock()
	defer t.mu.Unlock()
	return &taskSnapshot{ProjectID: t.ProjectID, Phase: t.Phase, Status: t.Status, StartedAt: t.StartedAt, EndedAt: t.EndedAt, Error: t.Error}
}

func (t *taskState) fail(err error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.Status = "failed"
	t.Error = err.Error()
}

func (t *taskState) finish() {
	t.mu.Lock()
	defer t.mu.Unlock()
	ended := time.Now().UTC()
	t.EndedAt = &ended
	if t.Error == "" {
		t.Status = "completed"
	}
}

func NewServer(cfg *config.Config, p *pipeline.Pipeline) *Server {
	s := &Server{
		cfg:      cfg,
		pipeline: p,
		router:   chi.NewRouter(),
	}

	s.setupMiddleware()
	s.setupRoutes()

	return s
}

func (s *Server) setupMiddleware() {
	s.router.Use(chiMiddleware.RequestID)
	s.router.Use(chiMiddleware.RealIP)
	s.router.Use(chiMiddleware.Recoverer)
	s.router.Use(chiMiddleware.Logger)
	s.router.Use(httprate.LimitByIP(s.cfg.Server.RateLimit, time.Minute))
	s.router.Use(cors.Handler(cors.Options{
		AllowedOrigins:   s.cfg.Server.AllowedOrigins,
		AllowedMethods:   []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodOptions},
		AllowedHeaders:   []string{"Content-Type", "Authorization", "X-Request-ID"},
		ExposedHeaders:   []string{"X-Request-ID"},
		AllowCredentials: false,
		MaxAge:           86400,
	}))
	s.router.Use(s.auth)
}

func (s *Server) setupRoutes() {
	s.router.Route("/api", func(r chi.Router) {
		r.Options("/*", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		})
		r.Get("/health", s.handleHealth)

		r.Route("/projects", func(r chi.Router) {
			r.Get("/", s.handleListProjects)
			r.Post("/", s.handleCreateProject)

			r.Route("/{id}", func(r chi.Router) {
				r.Get("/status", s.handleGetStatus)
				r.Post("/ingest", s.handleIngest)
				r.Post("/run", s.handleRunPipeline)
				r.Post("/run/{phase}", s.handleRunPhase)
				r.Get("/output", s.handleListOutputs)
				r.Get("/output/*", s.handleGetOutput)
				r.Delete("/", s.handleDeleteProject)
			})
		})
	})
}

func (s *Server) Start() error {

	if s.cfg.Server.AuthToken == "" && s.cfg.Server.Host != "localhost" && s.cfg.Server.Host != "127.0.0.1" && s.cfg.Server.Host != "::1" {
		return fmt.Errorf("server.auth_token is required when binding to non-local host %q", s.cfg.Server.Host)
	}
	if s.cfg.Server.AuthToken == "" {
		slog.Warn("server authentication disabled; use only behind trusted local access", "host", s.cfg.Server.Host)
	}

	addr := fmt.Sprintf("%s:%d", s.cfg.Server.Host, s.cfg.Server.Port)

	s.srv = &http.Server{
		Addr:         addr,
		Handler:      s.router,
		ReadTimeout:  s.cfg.Server.ReadTimeout,
		WriteTimeout: s.cfg.Server.WriteTimeout,
	}

	shutdownTimeout := s.cfg.Server.ShutdownTimeout
	if shutdownTimeout == 0 {
		shutdownTimeout = 15 * time.Second
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-quit
		slog.Info("shutting down server...")

		s.tasks.Range(func(key, value interface{}) bool {
			if task, ok := value.(*taskState); ok && task.Cancel != nil {
				task.Cancel()
			}
			return true
		})

		ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()

		if err := s.srv.Shutdown(ctx); err != nil {
			slog.Error("server forced to shutdown", "error", err)
		}
	}()

	slog.Info("comix server starting", "addr", addr)
	if err := s.srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("server error: %w", err)
	}

	slog.Info("server stopped")
	return nil
}

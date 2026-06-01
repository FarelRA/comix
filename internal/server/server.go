package server

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"

	"github.com/comix/comix/internal/config"
	"github.com/comix/comix/internal/logger"
	"github.com/comix/comix/internal/pipeline"
)

type Server struct {
	cfg      *config.Config
	pipeline *pipeline.Pipeline
	router   chi.Router
	srv      *http.Server
	tasks    sync.Map
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
	s.router.Use(cors)
	s.router.Use(requestLogger)
}

func (s *Server) setupRoutes() {
	s.router.Route("/api", func(r chi.Router) {
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
		logger.Info("shutting down server...")

		s.tasks.Range(func(key, value interface{}) bool {
			if cancel, ok := value.(context.CancelFunc); ok && cancel != nil {
				cancel()
			}
			return true
		})

		ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()

		if err := s.srv.Shutdown(ctx); err != nil {
			logger.Error("server forced to shutdown", "error", err)
		}
	}()

	logger.Info("comix server starting", "addr", addr)
	if err := s.srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("server error: %w", err)
	}

	logger.Info("server stopped")
	return nil
}

package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/danilostorm/stormflix/internal/config"
	"github.com/danilostorm/stormflix/internal/database"
	"github.com/danilostorm/stormflix/internal/httpapi"
	"github.com/danilostorm/stormflix/internal/library"
)

func main() {
	cfg := config.Load()
	lifecycle, stopSignals := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stopSignals()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	db, err := database.Open(cfg.DatabasePath())
	if err != nil {
		logger.Error("failed to open database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	libraryService := library.NewService(db)
	if err := libraryService.Bootstrap(cfg.BootstrapLibraryName, cfg.BootstrapLibraryPath); err != nil {
		logger.Error("failed to bootstrap library", "error", err)
		os.Exit(1)
	}
	if len(cfg.ManagedMoviePaths) > 0 {
		managed, err := libraryService.EnsureManagedSources(context.Background(), cfg.ManagedMovieLibraryName, "movies", cfg.ManagedMoviePaths)
		if err != nil {
			// A temporary remote/mount conflict must not stop the media server from
			// starting. The administrator can still fix the source in the library UI.
			logger.Warn("could not reconcile managed movie sources", "library", cfg.ManagedMovieLibraryName, "error", err)
		} else if managed.ID > 0 {
			logger.Info("managed movie sources ready", "library", managed.Name, "sources", managed.SourceCount)
		}
	}

	handler := httpapi.NewWithContext(lifecycle, db, libraryService, cfg)
	server := &http.Server{
		Addr:              cfg.Address,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}

	shutdownDone := make(chan struct{})
	go func() {
		<-lifecycle.Done()

		logger.Info("shutting down StormFlix")
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			logger.Error("graceful shutdown failed", "error", err)
		}
		close(shutdownDone)
	}()

	logger.Info("StormFlix started", "address", cfg.Address, "data_dir", cfg.DataDir, "transcoding", "disabled")
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("http server failed", "error", err)
		os.Exit(1)
	}
	<-shutdownDone
}

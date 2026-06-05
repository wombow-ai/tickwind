// Command server runs the Tickwind API and the ingest scheduler.
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

	"github.com/wombow-ai/tickwind/internal/api"
	"github.com/wombow-ai/tickwind/internal/config"
	"github.com/wombow-ai/tickwind/internal/edgar"
	"github.com/wombow-ai/tickwind/internal/ingest"
	"github.com/wombow-ai/tickwind/internal/store"
	"github.com/wombow-ai/tickwind/internal/store/memory"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	cfg := config.Load()

	st := newStore(cfg, log)
	edgarClient := edgar.New(cfg.EDGARUserAgent)
	scheduler := ingest.NewScheduler(st, edgarClient, cfg.Watchlist, cfg.IngestEvery, log)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go scheduler.Run(ctx)

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           api.New(st, log),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Info("tickwind listening", "addr", srv.Addr, "store", cfg.StoreBackend, "watchlist", cfg.Watchlist)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("server error", "err", err)
			stop()
		}
	}()

	<-ctx.Done()
	log.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
}

func newStore(cfg config.Config, log *slog.Logger) store.Store {
	switch cfg.StoreBackend {
	case "memory":
		return memory.New()
	default:
		// Postgres backend lands when we deploy to the server; fall back for now.
		log.Warn("store backend not yet implemented, using memory", "backend", cfg.StoreBackend)
		return memory.New()
	}
}

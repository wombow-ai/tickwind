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

	"github.com/wombow-ai/tickwind/internal/alpaca"
	"github.com/wombow-ai/tickwind/internal/api"
	"github.com/wombow-ai/tickwind/internal/config"
	"github.com/wombow-ai/tickwind/internal/edgar"
	"github.com/wombow-ai/tickwind/internal/finnhub"
	"github.com/wombow-ai/tickwind/internal/ingest"
	"github.com/wombow-ai/tickwind/internal/reddit"
	"github.com/wombow-ai/tickwind/internal/stocktwits"
	"github.com/wombow-ai/tickwind/internal/store"
	"github.com/wombow-ai/tickwind/internal/store/memory"
	"github.com/wombow-ai/tickwind/internal/store/postgres"
	"github.com/wombow-ai/tickwind/internal/stream"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	cfg := config.Load()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	st, closeStore, err := newStore(ctx, cfg, log)
	if err != nil {
		log.Error("store init", "err", err)
		os.Exit(1)
	}
	defer closeStore()

	// Seed the watchlist from WATCHLIST on first run; thereafter it's edited via
	// the API and persisted in the store.
	if err := seedWatchlist(ctx, st, cfg.Watchlist, log); err != nil {
		log.Warn("watchlist seed failed", "err", err)
	}

	hub := stream.NewHub()

	// News ingestion runs only when a Finnhub token is configured.
	var newsClient *finnhub.Client
	if cfg.FinnhubToken != "" {
		newsClient = finnhub.New(cfg.FinnhubToken)
		log.Info("finnhub news enabled")
	} else {
		log.Warn("FINNHUB_TOKEN not set — news ingestion disabled")
	}

	// Social sources are all public (no key required).
	social := []ingest.SocialSource{stocktwits.New(), reddit.New()}

	edgarClient := edgar.New(cfg.EDGARUserAgent)
	scheduler := ingest.NewScheduler(st, edgarClient, newsClient, social, cfg.IngestEvery, log)
	go scheduler.Run(ctx)

	// Price polling runs only when Alpaca credentials are present.
	if cfg.AlpacaKeyID != "" && cfg.AlpacaSecret != "" {
		priceClient := alpaca.New(cfg.AlpacaKeyID, cfg.AlpacaSecret, cfg.AlpacaDataURL, cfg.AlpacaFeed)
		poller := ingest.NewPricePoller(st, priceClient, cfg.PricePollEvery, hub.Publish, log)
		go poller.Run(ctx)
		log.Info("price polling enabled", "every", cfg.PricePollEvery.String(), "feed", cfg.AlpacaFeed)
	} else {
		log.Warn("ALPACA_API_KEY/SECRET not set — price polling disabled")
	}

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           api.New(st, hub, log),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Info("tickwind listening", "addr", srv.Addr, "store", cfg.StoreBackend)
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

// seedWatchlist populates the watchlist with defaults only when it is empty.
func seedWatchlist(ctx context.Context, st store.Store, defaults []string, log *slog.Logger) error {
	existing, err := st.Watchlist(ctx)
	if err != nil {
		return err
	}
	if len(existing) > 0 {
		return nil
	}
	for _, t := range defaults {
		if err := st.AddToWatchlist(ctx, t); err != nil {
			return err
		}
	}
	log.Info("seeded watchlist", "tickers", defaults)
	return nil
}

// newStore builds the configured store and a cleanup func. A "postgres" backend
// that fails to initialize is fatal (returns an error) rather than silently
// falling back, so a misconfigured deployment fails loudly instead of dropping
// data into memory.
func newStore(ctx context.Context, cfg config.Config, log *slog.Logger) (store.Store, func(), error) {
	switch cfg.StoreBackend {
	case "postgres":
		pg, err := postgres.New(ctx, cfg.DatabaseURL)
		if err != nil {
			return nil, nil, err
		}
		log.Info("using postgres store")
		return pg, pg.Close, nil
	case "memory":
		return memory.New(), func() {}, nil
	default:
		log.Warn("unknown STORE_BACKEND, using memory", "backend", cfg.StoreBackend)
		return memory.New(), func() {}, nil
	}
}

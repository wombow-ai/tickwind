// Command server runs the Tickwind API and the ingest scheduler.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/wombow-ai/tickwind/internal/alpaca"
	"github.com/wombow-ai/tickwind/internal/alphavantage"
	"github.com/wombow-ai/tickwind/internal/apewisdom"
	"github.com/wombow-ai/tickwind/internal/api"
	"github.com/wombow-ai/tickwind/internal/auth"
	"github.com/wombow-ai/tickwind/internal/bluesky"
	"github.com/wombow-ai/tickwind/internal/config"
	"github.com/wombow-ai/tickwind/internal/dart"
	"github.com/wombow-ai/tickwind/internal/edgar"
	"github.com/wombow-ai/tickwind/internal/enrich"
	"github.com/wombow-ai/tickwind/internal/events"
	"github.com/wombow-ai/tickwind/internal/finnhub"
	"github.com/wombow-ai/tickwind/internal/guru"
	"github.com/wombow-ai/tickwind/internal/ingest"
	"github.com/wombow-ai/tickwind/internal/krx"
	"github.com/wombow-ai/tickwind/internal/market"
	"github.com/wombow-ai/tickwind/internal/opportunity"
	"github.com/wombow-ai/tickwind/internal/reddit"
	"github.com/wombow-ai/tickwind/internal/sec"
	"github.com/wombow-ai/tickwind/internal/stocktwits"
	"github.com/wombow-ai/tickwind/internal/store"
	"github.com/wombow-ai/tickwind/internal/store/memory"
	"github.com/wombow-ai/tickwind/internal/store/postgres"
	"github.com/wombow-ai/tickwind/internal/stream"
	"github.com/wombow-ai/tickwind/internal/substack"
	"github.com/wombow-ai/tickwind/internal/symbols"
	"github.com/wombow-ai/tickwind/internal/tickertick"
	"github.com/wombow-ai/tickwind/internal/topics"
	"github.com/wombow-ai/tickwind/internal/tpex"
	"github.com/wombow-ai/tickwind/internal/twse"
	"github.com/wombow-ai/tickwind/internal/xueqiu"
	"github.com/wombow-ai/tickwind/internal/yahoo"
)

// maxIngestTickers caps how many distinct tickers we ingest, to control cost as
// the user base (and thus the union of watchlists) grows.
const maxIngestTickers = 200

// taiwanSeed is a small set of Taiwan large-caps (TWSE .TW codes) always
// ingested, so TW stock pages have data out of the box — TSMC, Hon Hai,
// MediaTek, Delta, Chunghwa Telecom, UMC.
var taiwanSeed = []string{"2330.TW", "2317.TW", "2454.TW", "2308.TW", "2412.TW", "2303.TW"}

// hongKongSeed is the HK names the owner follows — Tencent, Zhipu / Z.ai (listed
// as "Knowledge Atlas") and MiniMax — always ingested via the owner-authorized
// (gray, delayed) Yahoo quote adapter. Values are Yahoo 4-digit .HK codes.
var hongKongSeed = []string{"0700.HK", "2513.HK", "0100.HK"}

// koreaSeed is the two KR large-caps the owner follows — Samsung Electronics and
// SK Hynix — ingested only when Korea is enabled, so their pages have data the
// moment the (free) KRX key is set.
var koreaSeed = []string{"005930.KS", "000660.KS"}

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

	hub := stream.NewHub()

	var jwksURL string
	if cfg.SupabaseURL != "" {
		jwksURL = cfg.SupabaseURL + "/auth/v1/.well-known/jwks.json"
	}
	verifier := auth.NewVerifier(cfg.SupabaseJWTSecret, jwksURL)
	if verifier.Enabled() {
		log.Info("auth enabled (supabase jwt)", "es256", jwksURL != "", "hs256", cfg.SupabaseJWTSecret != "")
	} else {
		log.Warn("auth disabled — set SUPABASE_URL and/or SUPABASE_JWT_SECRET; per-user endpoints return 401")
	}

	// Optional LLM enrichment (disabled without LLM_API_KEY).
	enricher := enrich.New(enrich.Config{APIKey: cfg.LLMAPIKey, BaseURL: cfg.LLMBaseURL, Model: cfg.LLMModel})
	if enricher.Enabled() {
		log.Info("llm enrichment enabled")
	}

	// News ingestion runs only when a Finnhub token is configured.
	var newsClient *finnhub.Client
	if cfg.FinnhubToken != "" {
		newsClient = finnhub.New(cfg.FinnhubToken)
		log.Info("finnhub news enabled")
	} else {
		log.Warn("FINNHUB_TOKEN not set — news ingestion disabled")
	}

	social := []ingest.SocialSource{
		stocktwits.New(),
		reddit.New(cfg.RedditClientID, cfg.RedditSecret, cfg.RedditUsername, cfg.RedditPassword),
		bluesky.New(cfg.BlueskyHandle, cfg.BlueskyAppPassword),
		xueqiu.New(),
		tickertick.New(),
	}

	// Tickers to ingest = the default set (always available for public pages)
	// ∪ every user's watchlist, deduped and capped.
	var koreaSeedActive []string // populated below when Korea is enabled
	ingestTickers := func(ctx context.Context) []string {
		seen := make(map[string]struct{})
		var out []string
		add := func(t string) {
			t = strings.ToUpper(strings.TrimSpace(t))
			if t == "" || len(out) >= maxIngestTickers {
				return
			}
			if _, ok := seen[t]; !ok {
				seen[t] = struct{}{}
				out = append(out, t)
			}
		}
		for _, t := range cfg.Watchlist {
			add(t)
		}
		for _, t := range taiwanSeed { // always-on TW large-caps
			add(t)
		}
		for _, t := range hongKongSeed { // always-on HK names (Yahoo delayed quotes)
			add(t)
		}
		for _, t := range koreaSeedActive { // KR large-caps when Korea is enabled
			add(t)
		}
		if all, err := st.AllWatchlistTickers(ctx); err != nil {
			log.Warn("all-watchlist read failed", "err", err)
		} else {
			for _, t := range all {
				add(t)
			}
		}
		return out
	}

	edgarClient := edgar.New(cfg.EDGARUserAgent)
	fundCache := ingest.NewFundamentalsCache(edgarClient)
	// Bulk numeric signals (buzz/sentiment): one call per source per cycle.
	// ApeWisdom is keyless; Alpha Vantage self-disables without a key. The same
	// ApeWisdom client also drives the market-wide trending hot list.
	apewisdomClient := apewisdom.New()
	signals := []ingest.SignalSource{
		apewisdomClient,
		alphavantage.New(cfg.AlphaVantageKey),
	}
	// Trending topics are recomputed each cycle from ingested news; the cache is
	// shared with the API (lock-free reads).
	topicCache := topics.NewCache()
	scheduler := ingest.NewScheduler(st, edgarClient, newsClient, social, signals, apewisdomClient, topicCache, ingestTickers, cfg.IngestEvery, log)

	// Taiwan market: keyless TWSE + TPEx EOD prices/names (Taiwan OGDL). The
	// adapter routes only .TW/.TWO tickers; bare US tickers keep the EDGAR/Alpaca
	// path untouched. Registered on the scheduler always, and on the price poller
	// below when Alpaca is enabled.
	marketAdapters := map[market.Market]ingest.MarketAdapter{
		market.TW: ingest.NewTWAdapter(twse.New(), tpex.New()),
		market.HK: ingest.NewHKAdapter(yahoo.New()), // gray, owner-authorized Yahoo delayed quotes
	}
	// Korea is opt-in via a free KRX key (DART key adds filings); when set, the
	// KR adapter + seed activate and KOSPI/KOSDAQ go live with no further change.
	if krxClient := krx.New(cfg.KRXAPIKey); krxClient.Enabled() {
		marketAdapters[market.KR] = ingest.NewKRAdapter(krxClient, dart.New(cfg.OpenDARTKey))
		koreaSeedActive = koreaSeed
		log.Info("korea market enabled (KRX + OpenDART)", "dart_filings", cfg.OpenDARTKey != "")
	}
	scheduler.SetAdapters(marketAdapters)
	go scheduler.Run(ctx)
	log.Info("taiwan market enabled (TWSE + TPEx EOD)", "seed", len(taiwanSeed))
	log.Info("hong kong market enabled (Yahoo delayed quotes — gray source)", "seed", len(hongKongSeed))

	// Guru-watch rail: curated finance-KOL newsletters (public RSS) → the tickers
	// they mention. Needs no API key, so it always runs (independent of prices).
	guruCache := guru.NewCache()
	guruIngestor := ingest.NewGuruIngestor(substack.New(), substack.Feeds, guruCache, st, 60, 2*time.Hour, log)
	go guruIngestor.Run(ctx)
	log.Info("guru-watch rail enabled", "feeds", len(substack.Feeds))

	// Symbol search directory: SEC public-domain US tickers for autocomplete,
	// refreshed daily (key-free; needs SEC's required User-Agent).
	symbolCache := symbols.NewCache()
	go ingest.NewSymbolIngestor(symbolCache, cfg.EDGARUserAgent, 24*time.Hour, log).Run(ctx)

	// Major-events timeline: BLS economic calendar + curated FOMC/world events,
	// refreshed twice a day (key-free, public-domain sources).
	eventsCache := events.NewCache()
	go ingest.NewEventsIngestor(eventsCache, 12*time.Hour, log).Run(ctx)

	// Retention pruner: bounds the durable market tables off the request path —
	// evicts old non-key data, but keeps hot-list tickers and the 大V / Serenity
	// "substack" rail on longer/indefinite windows. Disabled only if the store
	// doesn't implement store.Pruner (memory, postgres and Split all do).
	if pr, ok := st.(store.Pruner); ok {
		go ingest.NewPruner(pr, cfg.Retention, log).Run(ctx)
		log.Info("retention pruner enabled", "every", cfg.Retention.Every.String())
	}

	// Opportunity board (small-cap insider buys); shared cache, populated below
	// when Alpaca prices are available (needed for market cap).
	oppCache := opportunity.NewCache()

	// bars feeds the sparkline endpoint; nil (disabled) without Alpaca creds.
	var bars api.BarSource
	if cfg.AlpacaKeyID != "" && cfg.AlpacaSecret != "" {
		priceClient := alpaca.New(cfg.AlpacaKeyID, cfg.AlpacaSecret, cfg.AlpacaDataURL, cfg.AlpacaFeed)
		poller := ingest.NewPricePoller(st, priceClient, ingestTickers, cfg.PricePollEvery, hub.Publish, log)
		poller.SetAdapters(marketAdapters) // route .TW/.TWO to the TWSE/TPEx adapter
		go poller.Run(ctx)
		bars = ingest.NewBarCache(priceClient, 30, time.Hour)
		log.Info("price polling enabled", "every", cfg.PricePollEvery.String(), "feed", cfg.AlpacaFeed)

		// Opportunity board: SEC Form-4 insider buys + market cap (needs prices).
		secClient := sec.New(cfg.EDGARUserAgent)
		oppIngestor := ingest.NewOpportunityIngestor(st, secClient, priceClient, oppCache, 2*time.Hour, cfg.OpportunityBackfillDays, log)
		go oppIngestor.Run(ctx)
		log.Info("opportunity board enabled (SEC insider buys)", "backfill_days", cfg.OpportunityBackfillDays)

		// Alert evaluator: checks active user alerts against the latest price.
		go ingest.NewAlertEvaluator(st, bars, 2*time.Minute, log).Run(ctx)
		log.Info("alert evaluator enabled", "every", "2m")
	} else {
		log.Warn("ALPACA_API_KEY/SECRET not set — price polling + opportunity board disabled")
	}

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           api.New(st, hub, enricher, verifier, bars, topicCache, oppCache, guruCache, scheduler, symbolCache, eventsCache, fundCache, cfg.AdminUserIDs, log),
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

// newStore builds the configured store and a cleanup func. A "postgres" backend
// that fails to initialize is fatal (returns an error) rather than silently
// falling back, so a misconfigured deployment fails loudly instead of dropping
// data into memory.
func newStore(ctx context.Context, cfg config.Config, log *slog.Logger) (store.Store, func(), error) {
	switch cfg.StoreBackend {
	case "postgres":
		// Split storage when both URLs are set: collected/market data goes to the
		// durable MarketDatabaseURL, per-user data to the local UserDatabaseURL.
		if cfg.MarketDatabaseURL != "" && cfg.UserDatabaseURL != "" {
			market, err := postgres.New(ctx, cfg.MarketDatabaseURL)
			if err != nil {
				return nil, nil, err
			}
			user, err := postgres.New(ctx, cfg.UserDatabaseURL)
			if err != nil {
				market.Close()
				return nil, nil, err
			}
			log.Info("using split postgres store (market=durable, user=local)")
			cleanup := func() {
				user.Close()
				market.Close()
			}
			return store.Split{Market: market, User: user}, cleanup, nil
		}
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

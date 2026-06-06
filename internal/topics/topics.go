// Package topics derives a "hot topics" leaderboard from recent news headlines:
// a small curated keyword→theme dictionary matched over already-ingested
// articles, ranked by recency-weighted volume × momentum (vs the prior 24h).
// Stdlib-only; no NLP deps. Generic macro buckets are demoted so specific
// themes (e.g. "AI capex") surface over perennial ones (e.g. "Earnings").
package topics

import (
	"math"
	"regexp"
	"sort"
	"strings"
	"sync/atomic"
	"time"
)

// Article is the minimal input: a news item to scan for themes.
type Article struct {
	Headline    string
	Summary     string
	Tickers     []string
	PublishedAt time.Time
}

// HotTopic is one ranked trending theme.
type HotTopic struct {
	Key            string   `json:"key"`
	Label          string   `json:"label"`
	Count          int      `json:"count"`    // matching articles in the last 24h
	Momentum       float64  `json:"momentum"` // >1 = heating up vs prior 24h
	RelatedTickers []string `json:"related_tickers"`
	hotness        float64  // internal ranking score (not marshaled)
}

// Snapshot is the ranked trending-topics list at a moment.
type Snapshot struct {
	GeneratedAt time.Time  `json:"generated_at"`
	Window      string     `json:"window"`
	Topics      []HotTopic `json:"topics"`
}

// theme is a curated trending theme + its trigger keywords. generic marks broad
// macro buckets that get demoted so they don't pin the top every day.
type theme struct {
	key, label string
	keywords   []string
	generic    bool
}

// themes is the curated dictionary (hand-tunable). Keep labels clean and stable.
var themes = []theme{
	{"ai_capex", "AI capex", []string{"ai capex", "data center", "datacenter", "data centre", "hyperscaler", "gpu", "gpus", "accelerator", "inference", "compute cluster"}, false},
	{"ai", "AI", []string{"artificial intelligence", "generative ai", "genai", "chatbot", "llm", "copilot", "openai"}, false},
	{"semis", "Semiconductors", []string{"chip", "chips", "chipmaker", "semiconductor", "semiconductors", "foundry", "wafer", "tsmc", "hbm", "euv"}, false},
	{"fed", "Fed", []string{"fed", "fomc", "powell", "rate cut", "rate hike", "interest rate", "interest rates", "basis points", "dot plot"}, false},
	{"inflation", "Inflation", []string{"inflation", "cpi", "pce", "ppi", "disinflation"}, false},
	{"jobs", "Jobs report", []string{"nonfarm", "payrolls", "jobless claims", "unemployment", "jobs report"}, false},
	{"earnings", "Earnings", []string{"earnings", "eps", "guidance", "beat estimates", "missed estimates", "quarterly results"}, true},
	{"tariffs", "Tariffs", []string{"tariff", "tariffs", "trade war", "export control", "export controls", "sanction", "sanctions"}, false},
	{"crypto", "Crypto", []string{"bitcoin", "ethereum", "crypto", "cryptocurrency", "stablecoin"}, false},
	{"layoffs", "Layoffs", []string{"layoff", "layoffs", "job cuts", "restructuring", "workforce reduction"}, false},
	{"ma", "M&A", []string{"acquisition", "merger", "takeover", "buyout", "to acquire", "acquires"}, false},
	{"energy", "Oil & Energy", []string{"opec", "crude oil", "wti crude", "brent crude", "natural gas", "lng"}, false},
	{"ev", "EV", []string{"electric vehicle", "electric vehicles", "ev maker", "ev sales", "charging network"}, false},
	{"ipo", "IPO", []string{"ipo", "goes public", "public offering", "files to go public"}, false},
	{"china", "China", []string{"china", "beijing", "yuan", "shenzhen", "hong kong"}, true},
	{"dividend", "Dividends & Buybacks", []string{"dividend", "buyback", "share repurchase", "special dividend"}, false},
}

// matchers[i] is the precompiled word-boundary regex for themes[i].
var matchers []*regexp.Regexp

// themeIndex maps a topic key to its themes/matchers index (-1 if unknown).
var themeIndex = map[string]int{}

func init() {
	matchers = make([]*regexp.Regexp, len(themes))
	for i, th := range themes {
		quoted := make([]string, len(th.keywords))
		for j, kw := range th.keywords {
			quoted[j] = regexp.QuoteMeta(kw)
		}
		// Word-boundary both sides keeps "chip" from matching "Chipotle".
		matchers[i] = regexp.MustCompile(`(?i)\b(` + strings.Join(quoted, "|") + `)\b`)
		themeIndex[th.key] = i
	}
}

// Tuning constants.
const (
	tau          = 10.0 // recency decay (hours)
	momentumBeta = 0.6  // momentum exponent
	momentumK    = 3.0  // Laplace smoothing for momentum
	minCount     = 3    // anti-noise floor (matching articles in window)
	maxTopics    = 8    // chips served
	genericW     = 0.35 // demotion factor for generic macro buckets
	windowHours  = 24.0
)

// Recompute ranks the curated themes over articles (which should span ~48h:
// the last 24h is the live window, 24–48h is the prior window for momentum).
func Recompute(now time.Time, articles []Article) Snapshot {
	type acc struct {
		cur, prior int
		weighted   float64
		tickers    map[string]int
	}
	accs := make([]acc, len(themes))
	for i := range accs {
		accs[i].tickers = map[string]int{}
	}

	for _, art := range articles {
		ageH := now.Sub(art.PublishedAt).Hours()
		if ageH < 0 {
			ageH = 0
		}
		if ageH > 2*windowHours {
			continue
		}
		text := strings.ToLower(art.Headline + ". " + art.Summary)
		for i := range themes {
			if !matchers[i].MatchString(text) {
				continue
			}
			if ageH <= windowHours {
				accs[i].cur++
				accs[i].weighted += math.Exp(-ageH / tau)
				for _, tk := range art.Tickers {
					accs[i].tickers[tk]++
				}
			} else {
				accs[i].prior++
			}
		}
	}

	out := make([]HotTopic, 0, len(themes))
	for i := range themes {
		a := accs[i]
		if a.cur < minCount {
			continue
		}
		mom := (float64(a.cur) + momentumK) / (float64(a.prior) + momentumK)
		genre := 1.0
		if themes[i].generic {
			genre = genericW
		}
		out = append(out, HotTopic{
			Key:            themes[i].key,
			Label:          themes[i].label,
			Count:          a.cur,
			Momentum:       mom,
			RelatedTickers: topTickers(a.tickers, 6),
			hotness:        a.weighted * math.Pow(mom, momentumBeta) * genre,
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].hotness > out[j].hotness })
	if len(out) > maxTopics {
		out = out[:maxTopics]
	}
	return Snapshot{GeneratedAt: now, Window: "24h", Topics: out}
}

// Match reports whether text matches the given topic key's keywords. Used to
// filter a news list to a clicked chip.
func Match(key, text string) bool {
	i, ok := themeIndex[key]
	if !ok {
		return false
	}
	return matchers[i].MatchString(strings.ToLower(text))
}

// topTickers returns the n most-frequent tickers, highest first.
func topTickers(counts map[string]int, n int) []string {
	type kv struct {
		t string
		c int
	}
	arr := make([]kv, 0, len(counts))
	for t, c := range counts {
		arr = append(arr, kv{t, c})
	}
	sort.Slice(arr, func(i, j int) bool {
		if arr[i].c != arr[j].c {
			return arr[i].c > arr[j].c
		}
		return arr[i].t < arr[j].t
	})
	out := make([]string, 0, n)
	for _, e := range arr {
		if len(out) >= n {
			break
		}
		out = append(out, e.t)
	}
	return out
}

// Cache holds the latest snapshot for lock-free reads (atomic pointer swap).
type Cache struct {
	v atomic.Value
}

// NewCache returns a Cache seeded with an empty snapshot.
func NewCache() *Cache {
	c := &Cache{}
	c.v.Store(Snapshot{Window: "24h", Topics: []HotTopic{}})
	return c
}

// Set replaces the current snapshot.
func (c *Cache) Set(s Snapshot) { c.v.Store(s) }

// Get returns the current snapshot.
func (c *Cache) Get() Snapshot { return c.v.Load().(Snapshot) }

package indicators

import "testing"

// stockApplicableCount is the expected number of stock-applicable indicators
// (applies_to ∈ {stock, both}) in the embedded dataset: 184 stock + 98 both.
const stockApplicableCount = 282

func TestLoad(t *testing.T) {
	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := c.Len(); got != stockApplicableCount {
		t.Errorf("stock-applicable count = %d, want %d", got, stockApplicableCount)
	}
	// No crypto-only indicator should survive the filter.
	for _, ind := range c.All() {
		if !appliesToStock(ind.AppliesTo) {
			t.Errorf("catalog contains non-stock indicator %q (applies_to=%q)", ind.ID, ind.AppliesTo)
		}
		if ind.Domain == "onchain" {
			t.Errorf("catalog contains onchain indicator %q (should be crypto-only)", ind.ID)
		}
	}
}

func TestFilter(t *testing.T) {
	c := MustLoad()

	tests := []struct {
		name  string
		query Query
		check func(t *testing.T, got []Indicator)
	}{
		{
			name:  "empty returns all",
			query: Query{},
			check: func(t *testing.T, got []Indicator) {
				if len(got) != stockApplicableCount {
					t.Errorf("len = %d, want %d", len(got), stockApplicableCount)
				}
			},
		},
		{
			name:  "domain technical",
			query: Query{Domain: "technical"},
			check: func(t *testing.T, got []Indicator) {
				if len(got) == 0 {
					t.Fatal("technical domain returned nothing")
				}
				for _, ind := range got {
					if ind.Domain != "technical" {
						t.Errorf("got domain %q, want technical", ind.Domain)
					}
				}
			},
		},
		{
			name:  "priority P0",
			query: Query{Priority: "P0"},
			check: func(t *testing.T, got []Indicator) {
				if len(got) == 0 {
					t.Fatal("P0 returned nothing")
				}
				for _, ind := range got {
					if ind.Priority != "P0" {
						t.Errorf("got priority %q, want P0", ind.Priority)
					}
				}
			},
		},
		{
			name:  "domain + priority compose",
			query: Query{Domain: "technical", Priority: "P0"},
			check: func(t *testing.T, got []Indicator) {
				for _, ind := range got {
					if ind.Domain != "technical" || ind.Priority != "P0" {
						t.Errorf("got %s/%s, want technical/P0", ind.Domain, ind.Priority)
					}
				}
			},
		},
		{
			name:  "text search RSI (case-insensitive)",
			query: Query{Text: "rsi"},
			check: func(t *testing.T, got []Indicator) {
				found := false
				for _, ind := range got {
					if ind.ID == "technical.rsi" {
						found = true
					}
				}
				if !found {
					t.Error("text search 'rsi' did not surface technical.rsi")
				}
			},
		},
		{
			name:  "no match",
			query: Query{Domain: "technical", Subcategory: "DefinitelyNotASubcategory"},
			check: func(t *testing.T, got []Indicator) {
				if got == nil {
					t.Error("filter returned nil; want non-nil empty slice")
				}
				if len(got) != 0 {
					t.Errorf("len = %d, want 0", len(got))
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.check(t, c.Filter(tc.query))
		})
	}
}

func TestFacets(t *testing.T) {
	c := MustLoad()
	f := c.Facets()

	if len(f.Domains) == 0 || len(f.Priorities) == 0 || len(f.Subcategories) == 0 {
		t.Fatalf("facets are empty: %+v", f)
	}

	// Domain facet counts must sum to the whole catalog.
	sum := 0
	for _, d := range f.Domains {
		sum += d.Count
	}
	if sum != c.Len() {
		t.Errorf("domain facet counts sum to %d, want %d", sum, c.Len())
	}

	// No crypto-only "onchain" domain should appear.
	for _, d := range f.Domains {
		if d.Value == "onchain" {
			t.Error("onchain domain leaked into facets")
		}
	}

	// Priorities must be ordered P0, P1, P2.
	wantOrder := []string{"P0", "P1", "P2"}
	for i, p := range f.Priorities {
		if i < len(wantOrder) && p.Value != wantOrder[i] {
			t.Errorf("priority facet[%d] = %q, want %q", i, p.Value, wantOrder[i])
		}
	}
}

func TestAllReturnsCopy(t *testing.T) {
	c := MustLoad()
	a := c.All()
	if len(a) == 0 {
		t.Fatal("All returned nothing")
	}
	orig := a[0].NameEN
	a[0].NameEN = "MUTATED"
	if c.All()[0].NameEN != orig {
		t.Error("All() does not return an independent copy; mutation leaked into the catalog")
	}
}

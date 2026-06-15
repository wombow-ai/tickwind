package config

import "testing"

// TestDeepResearchMonthlyLimitEnv covers the per-user deep-research quota knob and
// its day→month env rename with back-compat:
//   - unset → default 1 (free 1 report/user/month);
//   - the legacy DEEP_RESEARCH_DAILY_LIMIT still tunes it (back-compat);
//   - the new DEEP_RESEARCH_MONTHLY_LIMIT takes precedence over the legacy name.
func TestDeepResearchMonthlyLimitEnv(t *testing.T) {
	tests := []struct {
		name      string
		monthly   string // DEEP_RESEARCH_MONTHLY_LIMIT ("" → unset)
		legacyDay string // DEEP_RESEARCH_DAILY_LIMIT ("" → unset)
		want      int
	}{
		{name: "default", want: 1},
		{name: "legacy daily name still works (back-compat)", legacyDay: "3", want: 3},
		{name: "new monthly name", monthly: "5", want: 5},
		{name: "new name wins over legacy", monthly: "5", legacyDay: "3", want: 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.monthly != "" {
				t.Setenv("DEEP_RESEARCH_MONTHLY_LIMIT", tt.monthly)
			}
			if tt.legacyDay != "" {
				t.Setenv("DEEP_RESEARCH_DAILY_LIMIT", tt.legacyDay)
			}
			if got := Load().DeepResearchMonthlyLimit; got != tt.want {
				t.Errorf("DeepResearchMonthlyLimit = %d; want %d", got, tt.want)
			}
		})
	}
}

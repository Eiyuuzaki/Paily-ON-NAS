package handler

import "testing"

// validateNumericConfigs guards the two silent failure modes: a negative
// window pushes the cleanup cutoff into the future (deleting every row) and a
// zero interval panics time.NewTicker in a goroutine with no recover.
func TestValidateNumericConfigs(t *testing.T) {
	cases := []struct {
		name    string
		key     string
		val     string
		wantErr bool
	}{
		{"zero interval rejected", "cleanup_interval_minutes", "0", true},
		{"negative interval rejected", "cleanup_interval_minutes", "-5", true},
		{"non-numeric rejected", "cleanup_interval_minutes", "abc", true},
		{"positive interval ok", "cleanup_interval_minutes", "30", false},
		{"whitespace tolerated", "cleanup_interval_minutes", " 45 ", false},
		{"negative window rejected", "fetch_history_window_days", "-1", true},
		{"zero window allowed (disables)", "node_never_alive_retention_days", "0", false},
		{"negative hours rejected", "node_exhausted_archive_hours", "-2", true},
		{"untracked key untouched", "max_distribute", "-1", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validateNumericConfigs(map[string]string{c.key: c.val})
			if c.wantErr && err == nil {
				t.Fatalf("%s=%q must be rejected, got nil", c.key, c.val)
			}
			if !c.wantErr && err != nil {
				t.Fatalf("%s=%q must be accepted, got %v", c.key, c.val, err)
			}
		})
	}
}

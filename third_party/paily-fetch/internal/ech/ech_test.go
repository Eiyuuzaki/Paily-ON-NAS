package ech

import (
	"fmt"
	"testing"
)

func reset() {
	mu.Lock()
	cache = map[string]cacheEntry{}
	mu.Unlock()
}

// withLookup swaps the network probe for the duration of one test.
func withLookup(t *testing.T, fn func(string) (bool, error)) {
	t.Helper()
	orig := lookup
	lookup = fn
	t.Cleanup(func() { lookup = orig; reset() })
	reset()
}

func TestEnabledSkipsHostWithoutECH(t *testing.T) {
	withLookup(t, func(string) (bool, error) { return false, nil })
	entry := map[string]any{"tls": true, "server": "app2301.hidenplay.net", "servername": "app2301.hidenplay.net"}
	if Enabled(entry) {
		t.Fatal("host without ech= must not get ech-opts; clients refuse the connection")
	}
}

func TestEnabledAddsWhenRecordHasECH(t *testing.T) {
	withLookup(t, func(string) (bool, error) { return true, nil })
	entry := map[string]any{"tls": true, "server": "1.2.3.4", "servername": "lencf.dpdns.org"}
	if !Enabled(entry) {
		t.Fatal("host publishing ech= must get ech-opts")
	}
}

func TestHostForPrefersServernameAndSkipsIPs(t *testing.T) {
	cases := []struct {
		entry map[string]any
		want  string
	}{
		{map[string]any{"server": "1.2.3.4", "servername": "a.example"}, "a.example"},
		{map[string]any{"server": "a.example"}, "a.example"},
		{map[string]any{"server": "104.16.1.1"}, ""},
		{map[string]any{"server": "2606:4700::1"}, ""},
		{map[string]any{}, ""},
	}
	for _, c := range cases {
		if got := HostFor(c.entry); got != c.want {
			t.Fatalf("HostFor(%v) = %q, want %q", c.entry, got, c.want)
		}
	}
}

func TestIPLiteralDoesNotQuery(t *testing.T) {
	called := false
	withLookup(t, func(string) (bool, error) { called = true; return true, nil })
	Enabled(map[string]any{"tls": true, "server": "104.16.1.1"})
	if called {
		t.Fatal("IP literal should not trigger a DNS query")
	}
}

func TestLookupFailureFallsBackToNoECH(t *testing.T) {
	withLookup(t, func(string) (bool, error) { return false, fmt.Errorf("resolver down") })
	if Enabled(map[string]any{"tls": true, "server": "example.org"}) {
		t.Fatal("resolver failure must fall back to no ech-opts")
	}
}

func TestCacheAvoidsRepeatedLookups(t *testing.T) {
	n := 0
	withLookup(t, func(string) (bool, error) { n++; return true, nil })
	for i := 0; i < 5; i++ {
		HasRecord("cache.example")
	}
	if n != 1 {
		t.Fatalf("expected 1 lookup, got %d", n)
	}
}

func TestNoAnswerMeansNoECH(t *testing.T) {
	withLookup(t, func(string) (bool, error) { return false, nil })
	if HasRecord("norecord.example") {
		t.Fatal("empty answer must be treated as no ECH")
	}
}

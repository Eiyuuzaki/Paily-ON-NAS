package distribution

import (
	"testing"

	"github.com/Eiyuuzaki/paily-on-nas/internal/cache"
	node "github.com/Eiyuuzaki/paily-on-nas/third_party/paily-fetch/node"
)

func workerEntry(server, sni, region string, score float64) *cache.NodeCacheEntry {
	m := map[string]any{"name": "CF-" + region, "type": "vless", "server": server, "port": 443}
	if sni != "" {
		m["servername"] = sni
	}
	n, err := node.New(m)
	if err != nil {
		panic(err)
	}
	return &cache.NodeCacheEntry{Server: server, Region: region, Score: score, ProxyNode: n}
}

// An open Worker must lose only its own entries. The bug this guards against
// dropped every CF- node as soon as any Worker opened.
func TestWorkerBreakerOpenIsPerWorker(t *testing.T) {
	a1 := workerEntry("104.16.1.1", "a.example.org", "HK", 0.9)
	a2 := workerEntry("104.16.1.2", "a.example.org", "US", 0.9)
	b1 := workerEntry("104.16.2.1", "b.example.org", "HK", 0.9)
	plain := workerEntry("141.94.16.4", "", "US", 0.5)
	cands := []*cache.NodeCacheEntry{a1, a2, b1, plain}
	got := applyWorkerBreaker(cands, map[string]struct{}{"a.example.org": {}}, nil)
	if len(got) != 2 {
		t.Fatalf("want 2 survivors, got %d", len(got))
	}
	for _, e := range got {
		if entrySNI(e) == "a.example.org" {
			t.Fatalf("entry behind the open Worker survived: %s", e.Server)
		}
	}
	if got[0] != b1 || got[1] != plain {
		t.Fatalf("wrong survivors: %s, %s", got[0].Server, got[1].Server)
	}
}

// A degraded Worker keeps one representative per region, highest score first.
func TestWorkerBreakerThrottleKeepsBestPerRegion(t *testing.T) {
	weak := workerEntry("104.16.3.1", "c.example.org", "HK", 0.2)
	strong := workerEntry("104.16.3.2", "c.example.org", "HK", 0.8)
	us := workerEntry("104.16.3.3", "c.example.org", "US", 0.1)
	other := workerEntry("104.16.4.1", "d.example.org", "HK", 0.9)
	cands := []*cache.NodeCacheEntry{weak, strong, us, other}
	got := applyWorkerBreaker(cands, nil, map[string]struct{}{"c.example.org": {}})
	if len(got) != 3 {
		t.Fatalf("want 3 survivors (best HK, one US, untouched other), got %d", len(got))
	}
	seen := map[string]bool{}
	for _, e := range got {
		seen[e.Server] = true
	}
	if seen["104.16.3.1"] {
		t.Fatal("weaker same-region entry must be dropped")
	}
	if !seen["104.16.3.2"] || !seen["104.16.3.3"] || !seen["104.16.4.1"] {
		t.Fatalf("unexpected survivors: %v", seen)
	}
}

// No breaker state means no filtering at all.
func TestWorkerBreakerNoStateIsNoop(t *testing.T) {
	cands := []*cache.NodeCacheEntry{workerEntry("104.16.5.1", "e.example.org", "HK", 0.5)}
	got := applyWorkerBreaker(cands, nil, nil)
	if len(got) != 1 || got[0] != cands[0] {
		t.Fatal("empty breaker state must return the input unchanged")
	}
}

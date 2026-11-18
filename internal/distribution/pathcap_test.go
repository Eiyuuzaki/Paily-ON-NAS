package distribution

import (
	"fmt"
	"testing"

	"github.com/Eiyuuzaki/paily-on-nas/internal/cache"
	"github.com/Eiyuuzaki/paily-on-nas/third_party/paily-fetch/node"
)

func pathEntry(t *testing.T, region, server string, port int) *cache.NodeCacheEntry {
	t.Helper()
	raw := map[string]any{
		"name":   fmt.Sprintf("host-%s-%d", server, port),
		"type":   "vmess",
		"server": server,
		"port":   port,
		"uuid":   fmt.Sprintf("00000000-0000-0000-0000-%012d", port),
	}
	n, err := node.New(raw)
	if err != nil {
		t.Fatalf("node.New: %v", err)
	}
	return &cache.NodeCacheEntry{ProxyNode: n, Region: region, Server: server, Protocol: "vmess", Score: 0.5}
}

// One host publishes many ports. Without a per-path cap it can fill a whole
// region's quota alone, which is how the live pool shipped 208 entries over
// only 43 distinct paths (2026-11-18). Cap must keep at most `pathMax` per host.
func TestPathCapKeepsRegionDiverse(t *testing.T) {
	var cands []*cache.NodeCacheEntry
	for port := 10000; port < 10010; port++ {
		cands = append(cands, pathEntry(t, "IT", "10.0.0.1", port))
		cands = append(cands, pathEntry(t, "IT", "10.0.0.2", port))
	}
	got := SelectRegionQuota(cands, 4, 16, 2, true, false, "short")
	if len(got) != 4 {
		t.Fatalf("want 4 entries (2 hosts x cap 2), got %d", len(got))
	}
	perHost := map[string]int{}
	for _, s := range got {
		perHost[s.Entry.Server]++
	}
	for host, n := range perHost {
		if n > 2 {
			t.Errorf("host %s contributed %d entries, cap is 2", host, n)
		}
	}
}

// pathMax <= 0 keeps the old behaviour, so the knob is safe to disable.
func TestPathCapDisabled(t *testing.T) {
	var cands []*cache.NodeCacheEntry
	for port := 10000; port < 10005; port++ {
		cands = append(cands, pathEntry(t, "IT", "10.0.0.1", port))
	}
	got := SelectRegionQuota(cands, 4, 16, 0, true, false, "short")
	if len(got) != 5 {
		t.Fatalf("cap disabled should keep all 5 entries, got %d", len(got))
	}
}
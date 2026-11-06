package distribution

import (
	"context"
	"testing"

	"github.com/Eiyuuzaki/paily-on-nas/internal/cache"
	"github.com/Eiyuuzaki/paily-on-nas/third_party/paily-fetch/node"
)

func entry(region, name string) *cache.NodeCacheEntry {
	raw := map[string]any{"name": name, "type": "vmess", "server": "public-ip-test", "port": 10086}
	n, err := node.New(raw)
	if err != nil {
		panic(err)
	}
	return &cache.NodeCacheEntry{ProxyNode: n, Region: region}
}

func renderStub(_ *cache.NodeCacheEntry, seq int, flags string) string {
	return flags + "-" + string(rune('0'+seq))
}

// No CN entry survived: the plan must be empty so the caller emits a flat list
// (this is the pre-chain behaviour and must not regress).
func TestBuildChainPlan_NoEntriesIsFlat(t *testing.T) {
	sel := []SelectedNode{{Entry: entry("DE", "de1"), SeqNo: 1}}
	plan, names := buildChainPlan(context.Background(), sel, nil, renderStub)
	if plan != nil || names != nil {
		t.Fatalf("expected nil plan, got plan=%v names=%v", plan, names)
	}
}

// Every non-CN exit gets a relay; a CN exit never does (it is the first hop).
func TestBuildChainPlan_AssignsExitsOnly(t *testing.T) {
	sel := []SelectedNode{
		{Entry: entry("CN", "cn1"), SeqNo: 1},
		{Entry: entry("DE", "de1"), SeqNo: 1},
		{Entry: entry("SG", "sg1"), SeqNo: 1},
	}
	entries := []*cache.NodeCacheEntry{entry("CN", "cn1")}
	plan, names := buildChainPlan(context.Background(), sel, entries, renderStub)

	if len(names) != 1 {
		t.Fatalf("want 1 entry name, got %v", names)
	}
	if _, ok := plan[0]; ok {
		t.Errorf("CN exit must not be chained, got %q", plan[0])
	}
	if plan[1] == "" || plan[2] == "" {
		t.Errorf("non-CN exits must be chained: %v", plan)
	}
}

// Multiple entries are reused round-robin so one relay does not carry all exits.
func TestBuildChainPlan_RoundRobin(t *testing.T) {
	entries := []*cache.NodeCacheEntry{entry("CN", "a"), entry("CN", "b")}
	sel := []SelectedNode{
		{Entry: entry("DE", "d1"), SeqNo: 1},
		{Entry: entry("DE", "d2"), SeqNo: 2},
		{Entry: entry("DE", "d3"), SeqNo: 3},
	}
	plan, _ := buildChainPlan(context.Background(), sel, entries, renderStub)
	if plan[0] == plan[1] {
		t.Errorf("round-robin should alternate, got %v", plan)
	}
	if plan[0] != plan[2] {
		t.Errorf("third exit should wrap to first relay, got %v", plan)
	}
}

// applyChain must not mutate the input map: cache.rebuild shares ProxyNode
// pointers across requests, so in-place edits leak into later responses.
func TestApplyChain_DoesNotMutateInput(t *testing.T) {
	orig := map[string]any{"name": "n", "type": "vmess", "server": "s", "port": 1}
	out := applyChain(orig, "relay")
	if _, leaked := orig["dialer-proxy"]; leaked {
		t.Fatalf("input map was mutated: %v", orig)
	}
	if out["dialer-proxy"] != "relay" {
		t.Fatalf("copy missing chain field: %v", out)
	}
	if out["name"] != "n" {
		t.Fatalf("copy lost original fields: %v", out)
	}
}

func TestApplyChain_EmptyEntryReturnsOriginal(t *testing.T) {
	orig := map[string]any{"name": "n", "type": "vmess"}
	if got := applyChain(orig, ""); got["dialer-proxy"] != nil {
		t.Fatalf("empty entry must not add a field: %v", got)
	}
}

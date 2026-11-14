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

// Chain plan is currently DISABLED: buildChainPlan returns nil early because a
// chain through an unreliable CN relay made every node time out (see AGENTS.md
// §5 "分发系统 chain plan"). This test pins that disabled state so the change
// cannot be undone by accident. Restore the relay-assignment assertions when
// the feature is re-enabled with a dependable entry node.
func TestBuildChainPlan_Disabled(t *testing.T) {
	sel := []SelectedNode{
		{Entry: entry("CN", "cn1"), SeqNo: 1},
		{Entry: entry("DE", "de1"), SeqNo: 1},
		{Entry: entry("SG", "sg1"), SeqNo: 1},
	}
	entries := []*cache.NodeCacheEntry{entry("CN", "cn1")}
	plan, names := buildChainPlan(context.Background(), sel, entries, renderStub)
	if plan != nil || names != nil {
		t.Fatalf("chain plan is disabled, want nil/nil, got plan=%v names=%v", plan, names)
	}
}

// Round-robin relay reuse is part of the same disabled feature; pinned here so
// re-enabling chain plan is a deliberate change that must revisit both tests.
func TestBuildChainPlan_RoundRobinDisabled(t *testing.T) {
	entries := []*cache.NodeCacheEntry{entry("CN", "a"), entry("CN", "b")}
	sel := []SelectedNode{
		{Entry: entry("DE", "d1"), SeqNo: 1},
		{Entry: entry("DE", "d2"), SeqNo: 2},
		{Entry: entry("DE", "d3"), SeqNo: 3},
	}
	plan, names := buildChainPlan(context.Background(), sel, entries, renderStub)
	if plan != nil || names != nil {
		t.Fatalf("chain plan is disabled, want nil/nil, got plan=%v names=%v", plan, names)
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

package distribution

import (
	"context"

	"github.com/Eiyuuzaki/paily-on-nas/internal/cache"
	"github.com/Eiyuuzaki/paily-on-nas/internal/naming"
	"github.com/Eiyuuzaki/paily-on-nas/third_party/paily-fetch/node"
	"gorm.io/gorm"
)

// buildChainPlan picks domestic entry nodes and assigns each selected
// non-CN exit node to one of them, round-robin.
//
// Why this exists: free nodes reachable from a mainland mobile line are often
// only usable *through* a domestic relay — direct egress to the overseas
// endpoint times out (see AGENTS.md §6: overseas entries ~0.5% alive,
// CN-segment entries several times higher). Without a chain the client dials
// the exit directly and the node looks dead; with a chain the entry carries
// the connection and the exit does the egress.
//
// Selection rules:
//   - only region == "CN" entries are used as relays (measured 224-267ms vs
//     2.2-4.3s for direct overseas, i.e. an order of magnitude faster)
//   - if no CN node survived this round, no plan is produced and the caller
//     emits a flat list exactly as before (feature degrades to a no-op)
//   - entries are reused round-robin so a single relay does not carry all
func buildChainPlan(
	ctx context.Context,
	selected []SelectedNode,
	entries []*cache.NodeCacheEntry,
	render func(*cache.NodeCacheEntry, int, string) string,
) (map[int]string, []string) {
	// chainPlanEnabled is a kill switch, not a bug: the CN relay hop relies on
	// free domestic nodes, and when those go down every chained exit times out
	// at once (2026-11-08: all 50 alive nodes -> timeout). Direct dialing is
	// verified working, so the plan builder is bypassed until we have a
	// reliable CN relay. Keeping the body intact documents the intended
	// architecture and makes re-enabling a one-line change.
	const chainPlanEnabled = false
	if !chainPlanEnabled {
		return nil, nil
	}
	if len(entries) == 0 || len(selected) == 0 {
		return nil, nil
	}

	entryNames := make([]string, 0, len(entries))
	for i, e := range entries {
		entryNames = append(entryNames, render(e, i+1, "relay"))
	}

	assignment := make(map[int]string, len(selected))
	for i, sn := range selected {
		if sn.Entry.Region == "CN" {
			// A CN node is itself the first hop — dialling it through another
			// relay would be a pointless loop.
			continue
		}
		assignment[i] = entryNames[i%len(entryNames)]
	}
	return assignment, entryNames
}

// applyChain returns a copy of the node's clash map with the chain field added.
// Field name follows mihomo's "dialer-proxy" convention.
//
// A copy is mandatory: cache.rebuild shares one *node.ProxyNode pointer across
// every request (rebuild.go sharedProxy), so mutating ClashMap in place would
// leak the chain setting into later requests and permanently alter the cache.
// Returns the original map untouched when no entry is assigned.
func applyChain(clashMap map[string]any, entryName string) map[string]any {
	if entryName == "" {
		return clashMap
	}
	out := make(map[string]any, len(clashMap)+1)
	for k, v := range clashMap {
		out[k] = v
	}
	out["dialer-proxy"] = entryName
	return out
}

// proxyWithChain builds a throwaway ProxyNode whose raw map carries the chain
// field, leaving the cached node untouched.
func proxyWithChain(orig *node.ProxyNode, entryName string) *node.ProxyNode {
	if entryName == "" {
		return orig
	}
	clone, err := node.New(applyChain(orig.ClashMap(), entryName))
	if err != nil {
		// Unparseable map: fall back to the original so one bad node cannot
		// break the whole subscription.
		return orig
	}
	// Push() overrides the output name, so the clone only needs the raw map.
	return clone
}

// chainRenderFunc builds the naming.Render callback used for entry names.
func chainRenderFunc(ctx context.Context, db *gorm.DB, preset, filterMode string) func(*cache.NodeCacheEntry, int, string) string {
	return func(e *cache.NodeCacheEntry, seq int, flags string) string {
		return naming.Render(ctx, e, seq, flags, filterMode, preset, db)
	}
}

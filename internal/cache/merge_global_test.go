package cache

import (
	"testing"

	"github.com/Eiyuuzaki/paily-on-nas/internal/db/model"
	"github.com/google/uuid"
)

func deepEntry(id uuid.UUID, region string, score float64, streaming map[string]bool, speed int) *NodeCacheEntry {
	return &NodeCacheEntry{ID: id, Region: region, Score: score, StreamingResult: streaming, SpeedKbps: speed}
}

// The global view merges every tag's copy of a node, and tagSet always carries
// "default", which normally has no deep rows. Picking a copy by map iteration
// order made a node's unlock flags flip between rebuilds, so nodes appeared and
// vanished from filter_unlock subscriptions. Repeat the merge many times: a
// single run cannot catch an order dependency.
func TestMergeGlobalPrefersDeepEvidence(t *testing.T) {
	id := uuid.New()
	empty := deepEntry(id, "HK", 0.10, nil, 0)
	withDeep := deepEntry(id, "HK", 0.90, map[string]bool{"netflix": true}, 1200)
	tags := map[string]struct{}{"default": {}, "local": {}}
	nodeByID := map[uuid.UUID]*model.Node{id: {ID: id, Score: 0.55}}

	for i := 0; i < 50; i++ {
		newTags := map[string]*tagSubCache{
			"default": {entries: map[uuid.UUID]*NodeCacheEntry{id: empty}},
			"local":   {entries: map[uuid.UUID]*NodeCacheEntry{id: withDeep}},
		}
		got := mergeGlobal(newTags, tags, nodeByID).entries[id]
		if got == nil {
			t.Fatal("node missing from the global view")
		}
		if !got.StreamingResult["netflix"] {
			t.Fatalf("run %d: unlock flags lost, merge is order-dependent", i)
		}
		if got.Score != 0.55 {
			t.Fatalf("run %d: score must come from nodes.score, got %v", i, got.Score)
		}
	}
}

// Two copies with deep evidence resolve by tag name, so the result is stable.
func TestMergeGlobalTieBreaksByTagName(t *testing.T) {
	id := uuid.New()
	a := deepEntry(id, "HK", 0.1, map[string]bool{"a": true}, 100)
	b := deepEntry(id, "HK", 0.2, map[string]bool{"b": true}, 200)
	tags := map[string]struct{}{"alpha": {}, "zeta": {}}
	for i := 0; i < 30; i++ {
		newTags := map[string]*tagSubCache{
			"alpha": {entries: map[uuid.UUID]*NodeCacheEntry{id: a}},
			"zeta":  {entries: map[uuid.UUID]*NodeCacheEntry{id: b}},
		}
		got := mergeGlobal(newTags, tags, nil).entries[id]
		if !got.StreamingResult["a"] || got.StreamingResult["b"] {
			t.Fatalf("run %d: expected the alphabetically first tag to win", i)
		}
	}
}

// An empty region keeps landing in its own bucket, as before.
func TestMergeGlobalBucketsUnknownRegion(t *testing.T) {
	id := uuid.New()
	e := deepEntry(id, "", 0.5, nil, 0)
	tags := map[string]struct{}{"local": {}}
	newTags := map[string]*tagSubCache{"local": {entries: map[uuid.UUID]*NodeCacheEntry{id: e}}}
	got := mergeGlobal(newTags, tags, nil)
	if len(got.byRegion["unknown"]) != 1 {
		t.Fatalf("empty region must bucket as unknown, got buckets %v", len(got.byRegion))
	}
}

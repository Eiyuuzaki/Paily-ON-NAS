package distribution

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Eiyuuzaki/paily-on-nas/internal/cache"
)

func isCFNode(raw string) bool { return strings.Contains(raw, `"name":"CF-`) }

func isPlaintext(e *cache.NodeCacheEntry) bool {
	p := e.Protocol
	if p != "http" && p != "socks" && p != "socks5" {
		return false
	}
	return !strings.Contains(e.Raw, `"tls":true`)
}

func isSkipCert(e *cache.NodeCacheEntry) bool {
	return e.Raw != "" && (strings.Contains(e.Raw, `"skip-cert-verify":true`) || strings.Contains(e.Raw, `"skip-cert-verify": true`))
}

// rankPick orders candidates for choosing which to keep: source priority first
// (paid/highest before free), then score.
func rankPick(xs []*cache.NodeCacheEntry) {
	sort.SliceStable(xs, func(i, j int) bool {
		if xs[i].SourcePriority != xs[j].SourcePriority {
			return xs[i].SourcePriority > xs[j].SourcePriority
		}
		if xs[i].Score != xs[j].Score {
			return xs[i].Score > xs[j].Score
		}
		return xs[i].Server < xs[j].Server
	})
}

// rankDisplay orders nodes inside a region for output: score first (good
// nodes first), source priority as tiebreak.
func rankDisplay(xs []*cache.NodeCacheEntry) {
	sort.SliceStable(xs, func(i, j int) bool {
		if xs[i].Score != xs[j].Score {
			return xs[i].Score > xs[j].Score
		}
		if xs[i].SourcePriority != xs[j].SourcePriority {
			return xs[i].SourcePriority > xs[j].SourcePriority
		}
		return xs[i].Server < xs[j].Server
	})
}

func capPerEgressLimit(entries []*cache.NodeCacheEntry, limit int) []*cache.NodeCacheEntry {
	if limit <= 0 || len(entries) == 0 {
		return entries
	}
	groups := make(map[string][]*cache.NodeCacheEntry, 8)
	order := make([]string, 0, 8)
	for _, e := range entries {
		k := egressIdentity(e)
		if _, ok := groups[k]; !ok {
			order = append(order, k)
		}
		groups[k] = append(groups[k], e)
	}
	out := make([]*cache.NodeCacheEntry, 0, len(entries))
	for _, k := range order {
		g := groups[k]
		rankPick(g)
		if len(g) > limit {
			g = g[:limit]
		}
		out = append(out, g...)
	}
	return out
}

type regionBlock struct {
	region string
	picked []*cache.NodeCacheEntry
	sum    float64 // sum of scores of picked nodes ≈ count × quality
	avg    float64
}

// SelectRegionQuota keeps each region contiguous (so the list reads region by
// region) but orders the regions themselves by aggregate quality: regions whose
// picked nodes have the highest total score (many AND high-scoring) come first.
// Inside a region nodes are ordered by score (best first).
//
// Per region: CF capped per Worker (cfPerWorker), plus up to otherMax secure
// non-CF; when secure non-CF is short, self-signed tops up to otherMax. Plaintext
// is never used; blank/UN regions are dropped so every entry has a flag.
func SelectRegionQuota(candidates []*cache.NodeCacheEntry, cfPerWorker, otherMax int) []SelectedNode {
	byRegion := make(map[string][]*cache.NodeCacheEntry, 32)
	for _, e := range candidates {
		if isPlaintext(e) {
			continue
		}
		if e.Region == "" || e.Region == "UN" {
			continue
		}
		byRegion[e.Region] = append(byRegion[e.Region], e)
	}

	blocks := make([]regionBlock, 0, len(byRegion))
	for r, list := range byRegion {
		var cf, secureOther, skipOther []*cache.NodeCacheEntry
		for _, e := range list {
			switch {
			case isCFNode(e.Raw):
				cf = append(cf, e)
			case isSkipCert(e) && e.SourcePriority < 1:
				skipOther = append(skipOther, e)
			default:
				secureOther = append(secureOther, e)
			}
		}
		cf = capPerEgressLimit(cf, cfPerWorker)
		rankPick(secureOther)
		if otherMax > 0 && len(secureOther) > otherMax {
			secureOther = secureOther[:otherMax]
		}
		picked := append(append([]*cache.NodeCacheEntry{}, cf...), secureOther...)
		if otherMax > 0 {
			short := otherMax - len(secureOther)
			if short > 0 && len(skipOther) > 0 {
				rankPick(skipOther)
				n := short
				if len(skipOther) < n {
					n = len(skipOther)
				}
				picked = append(picked, skipOther[:n]...)
			}
		}
		if len(picked) == 0 {
			continue
		}
		rankDisplay(picked)
		var sum float64
		for _, e := range picked {
			sum += e.Score
		}
		blocks = append(blocks, regionBlock{region: r, picked: picked, sum: sum, avg: sum / float64(len(picked))})
	}

	sort.SliceStable(blocks, func(i, j int) bool {
		if blocks[i].sum != blocks[j].sum {
			return blocks[i].sum > blocks[j].sum
		}
		if blocks[i].avg != blocks[j].avg {
			return blocks[i].avg > blocks[j].avg
		}
		return blocks[i].region < blocks[j].region
	})

	out := make([]SelectedNode, 0, 256)
	for _, b := range blocks {
		lower := strings.ToLower(b.region)
		for i, e := range b.picked {
			out = append(out, SelectedNode{Entry: e, SeqNo: i + 1, SeqID: fmt.Sprintf("%s%02d", lower, i+1)})
		}
	}
	return out
}

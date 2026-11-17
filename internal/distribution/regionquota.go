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

func scoreDesc(xs []*cache.NodeCacheEntry) {
	sort.SliceStable(xs, func(i, j int) bool {
		// Source priority first: paid/highest (1/2) rank above free (0) so a
		// trusted paid airport is preferred over a same-score free node.
		if xs[i].SourcePriority != xs[j].SourcePriority {
			return xs[i].SourcePriority > xs[j].SourcePriority
		}
		if xs[i].Score != xs[j].Score {
			return xs[i].Score > xs[j].Score
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
		scoreDesc(g)
		if len(g) > limit {
			g = g[:limit]
		}
		out = append(out, g...)
	}
	scoreDesc(out)
	return out
}

// SelectRegionQuota: per region, take CF (capped per Worker) + up to otherMax
// secure non-CF nodes. Plaintext http/socks are always dropped. If a region has
// no secure node at all, fall back to its highest-scoring self-signed (skip-cert)
// non-CF nodes so the region is not emptied (user 2026-11-17: "有些地区实在没
// 节点，自签勉强放进来"). Plaintext is never used, even as fallback.
func SelectRegionQuota(candidates []*cache.NodeCacheEntry, cfPerWorker, otherMax int) []SelectedNode {
	byRegion := make(map[string][]*cache.NodeCacheEntry, 32)
	for _, e := range candidates {
		if isPlaintext(e) {
			continue
		}
		// Only distribute nodes with a known region so every entry has a flag;
		// blank / UN nodes have no country and would render without a flag.
		if e.Region == "" || e.Region == "UN" {
			continue
		}
		byRegion[e.Region] = append(byRegion[e.Region], e)
	}
	regions := make([]string, 0, len(byRegion))
	for r := range byRegion {
		regions = append(regions, r)
	}
	sort.Strings(regions)

	out := make([]SelectedNode, 0, 256)
	for _, r := range regions {
		var cf, secureOther, skipOther []*cache.NodeCacheEntry
		for _, e := range byRegion[r] {
			switch {
			case isCFNode(e.Raw):
				cf = append(cf, e)
			case isSkipCert(e) && e.SourcePriority < 1:
				// Self-signed from free/aggregator sources: last-resort only.
				skipOther = append(skipOther, e)
			default:
				// Secure, OR a trusted paid/highest source (priority>=1) even if it
				// uses a self-signed cert — the user trusts the paid provider.
				secureOther = append(secureOther, e)
			}
		}
		cf = capPerEgressLimit(cf, cfPerWorker)
		scoreDesc(secureOther)
		if otherMax > 0 && len(secureOther) > otherMax {
			secureOther = secureOther[:otherMax]
		}
		picked := append(append([]*cache.NodeCacheEntry{}, cf...), secureOther...)
		// Top up with self-signed (skip-cert) nodes when the secure non-CF supply
		// cannot fill this region's otherMax slots (user: 不够就放自签). Plaintext
		// is never used; paid/highest self-signed already counts as secure above.
		if otherMax > 0 {
			short := otherMax - len(secureOther)
			if short > 0 && len(skipOther) > 0 {
				scoreDesc(skipOther)
				n := short
				if len(skipOther) < n {
					n = len(skipOther)
				}
				picked = append(picked, skipOther[:n]...)
			}
		}
		lower := strings.ToLower(r)
		for i, e := range picked {
			out = append(out, SelectedNode{Entry: e, SeqNo: i + 1, SeqID: fmt.Sprintf("%s%02d", lower, i+1)})
		}
	}
	// Best first: order by source priority then score so the highest-quality
	// nodes appear at the top regardless of region. SeqNo (naming) is per-region
	// and unaffected by this display ordering.
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i].Entry, out[j].Entry
		if a.Score != b.Score {
			return a.Score > b.Score
		}
		if a.SourcePriority != b.SourcePriority {
			return a.SourcePriority > b.SourcePriority
		}
		return a.Server < b.Server
	})
	return out
}

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
			case isSkipCert(e):
				skipOther = append(skipOther, e)
			default:
				secureOther = append(secureOther, e)
			}
		}
		cf = capPerEgressLimit(cf, cfPerWorker)
		scoreDesc(secureOther)
		if otherMax > 0 && len(secureOther) > otherMax {
			secureOther = secureOther[:otherMax]
		}
		picked := append(append([]*cache.NodeCacheEntry{}, cf...), secureOther...)
		if len(picked) == 0 && len(skipOther) > 0 {
			// No secure node in this region: allow self-signed as last resort.
			scoreDesc(skipOther)
			n := otherMax
			if n <= 0 || len(skipOther) < n {
				n = len(skipOther)
			}
			picked = skipOther[:n]
		}
		lower := strings.ToLower(r)
		for i, e := range picked {
			out = append(out, SelectedNode{Entry: e, SeqNo: i + 1, SeqID: fmt.Sprintf("%s%02d", lower, i+1)})
		}
	}
	return out
}

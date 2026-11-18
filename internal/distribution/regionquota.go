package distribution

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/Eiyuuzaki/paily-on-nas/internal/cache"
)

// rawProxy is the subset of a Clash proxy config these helpers care about.
// Matching is done on the decoded values, never on the raw text: rewriting the
// JSON through jsonb (which the entry swap does) re-serialises it with spaces,
// and a substring test for `"name":"CF-` silently stopped recognising every
// self-built node, which let them bypass the per-Worker cap and starve other
// sources out of the region quotas (2026-11-17).
type rawProxy struct {
	Name          string `json:"name"`
	TLS           bool   `json:"tls"`
	SkipCertVerify bool   `json:"skip-cert-verify"`
}

func parseRaw(raw string) rawProxy {
	var p rawProxy
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &p)
	}
	return p
}

func isCFNode(raw string) bool {
	return strings.HasPrefix(parseRaw(raw).Name, "CF-")
}

func isPlaintext(e *cache.NodeCacheEntry) bool {
	p := e.Protocol
	if p != "http" && p != "socks" && p != "socks5" {
		return false
	}
	return !parseRaw(e.Raw).TLS
}

func isSkipCert(e *cache.NodeCacheEntry) bool {
	return e.Raw != "" && parseRaw(e.Raw).SkipCertVerify
}

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
	sum    float64
	avg    float64
}

// SelectRegionQuota: region-contiguous output, regions ordered by aggregate
// quality (sum of picked scores). Per region: CF capped per Worker
// (cfPerWorker), plus up to otherMax secure non-CF; self-signed tops up per
// selfSignedFallback ("short"=fill to otherMax, "empty"=only when region empty,
// "none"=never). Plaintext is dropped unless allowPlaintext. Paid/highest
// self-signed counts as secure when allowPaidSelfSigned.
func SelectRegionQuota(candidates []*cache.NodeCacheEntry, cfPerWorker, otherMax, pathMax int, allowPaidSelfSigned, allowPlaintext bool, selfSignedFallback string) []SelectedNode {
	byRegion := make(map[string][]*cache.NodeCacheEntry, 32)
	for _, e := range candidates {
		if isPlaintext(e) && !allowPlaintext {
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
			case isSkipCert(e) && !(e.SourcePriority >= 1 && allowPaidSelfSigned):
				skipOther = append(skipOther, e)
			default:
				secureOther = append(secureOther, e)
			}
		}
		cf = capPerEgressLimit(cf, cfPerWorker)
		rankPick(secureOther)
		// Per-path cap for everything else too (2026-11-18). Measured on the live
		// pool: 208 shipped entries covered only 43 distinct paths, 94% of them
		// duplicated, because one host could fill a whole region's quota with
		// many ports or uuids of the same machine. Capping per path to 2 yields
		// 105 entries over 57 paths — fewer entries, MORE real redundancy, same
		// median latency.
		secureOther = capPerEgressLimit(secureOther, pathMax)
		if otherMax > 0 && len(secureOther) > otherMax {
			secureOther = secureOther[:otherMax]
		}
		picked := append(append([]*cache.NodeCacheEntry{}, cf...), secureOther...)

		topup := 0
		switch selfSignedFallback {
		case "short":
			topup = otherMax - len(secureOther)
		case "empty":
			if len(picked) == 0 {
				topup = otherMax
			}
		default: // "none" / unknown
			topup = 0
		}
		if topup > 0 && len(skipOther) > 0 {
			rankPick(skipOther)
			skipOther = capPerEgressLimit(skipOther, pathMax)
			n := topup
			if len(skipOther) < n {
				n = len(skipOther)
			}
			picked = append(picked, skipOther[:n]...)
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

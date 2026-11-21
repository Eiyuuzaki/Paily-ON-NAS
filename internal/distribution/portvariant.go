package distribution

import (
	"maps"
	"net"
	"strconv"
	"strings"

	"github.com/Eiyuuzaki/paily-on-nas/internal/cache"
	node "github.com/Eiyuuzaki/paily-on-nas/third_party/paily-fetch/node"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

// Port redundancy for CF-fronted entries.
//
// A Cloudflare Worker answers on 13 ports for the same backend (HTTPS
// 443/8443/2053/2083/2087/2096, HTTP 80/8080/8880/2052/2082/2086/2095). Every
// self-built node in the pool dials 443 only, so a port-level block on 443
// (precedent: 2022) would take the entire self-built pool down even though
// every Worker is still healthy. Emitting the same entry on a second TLS port
// puts a reachable twin in the subscription at zero probe cost: the port is a
// property of the platform, not of the node, so there is nothing to measure.
//
// Scope is deliberately the "bare IP server + distinct SNI" shape only — the
// same shared-Worker identity egressIdentity() uses. An ordinary proxy
// endpoint does not answer on a second port, and inventing one would only add
// an entry that can never connect.
//
// Exit condition for this feature: drop it if CF stops serving these ports, or
// if the client's url-test proves it always picks the 443 member (meaning the
// redundant twins are dead weight rather than insurance).

// cfPortVariants returns the extra TLS ports to render for each CF-shaped
// entry, from configs.dist_cf_port_variants (comma separated list).
//
// Default "8443": measured 2026-11-22 as 200 on both a custom domain entry
// (lencf.dpdns.org) and a workers.dev entry. 2053 answers on the custom domain
// too but not on workers.dev, so it is not enabled by default. An empty value
// disables the feature entirely.
func cfPortVariants(db *gorm.DB) []int {
	raw := configString(db, "dist_cf_port_variants", "8443")
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	seen := make(map[int]bool, 4)
	out := make([]int, 0, 4)
	for _, field := range strings.Split(raw, ",") {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		port, err := strconv.Atoi(field)
		if err != nil || port <= 0 || port > 65535 {
			// Bad input must be visible: silently dropping it would make a
			// typo look like "redundancy is on" while nothing is emitted.
			log.Warn().Str("value", field).Msg("distribution: dist_cf_port_variants entry is not a valid port")
			continue
		}
		if seen[port] {
			continue
		}
		seen[port] = true
		out = append(out, port)
	}
	return out
}

// cfEntryPort reports the port a CF entry dials, and whether this entry may be
// duplicated onto another port at all.
//
// Two conditions must both hold:
//
//  1. the backend is one the project operates (origin "self-built" or the CF
//     apex subscription) — measured 2026-11-22, and only true for Workers;
//  2. the entry dials a bare IP behind a distinct SNI, i.e. the shared-Worker
//     shape from egressIdentity().
//
// Condition 1 is what keeps this honest. The IP+SNI test alone also matches
// free-list nodes that point an IP at a decoy SNI: measured on the live pool,
// that produced 8443 twins for ordinary VPS entrypoints which do not listen
// there — dead nodes in every subscription. Port duplication is a Cloudflare
// property, so the scope has to be "our Worker", not "any TLS endpoint".
func cfEntryPort(e *cache.NodeCacheEntry, selfBuilt bool) (int, bool) {
	if !selfBuilt || e == nil || e.ProxyNode == nil {
		return 0, false
	}
	m := e.ProxyNode.ClashMap()
	sni, _ := m["servername"].(string)
	if sni == "" {
		sni, _ = m["sni"].(string)
	}
	if sni == "" {
		return 0, false
	}
	server, _ := m["server"].(string)
	if net.ParseIP(server) == nil {
		return 0, false
	}
	port := clashIntField(m, "port")
	if port <= 0 {
		port = 443
	}
	return port, true
}

// withPort returns a copy of n dialling a different port. Every other field is
// preserved — notably ech-opts and dialer-proxy, which are already baked into
// the map by the time variants are built. Returns nil when the map cannot be
// turned back into a ProxyNode; the caller then skips the variant rather than
// emitting a node the client cannot parse.
func withPort(n *node.ProxyNode, port int) *node.ProxyNode {
	m := maps.Clone(n.ClashMap())
	m["port"] = port
	clone, err := node.New(m)
	if err != nil {
		return nil
	}
	return clone
}

// clashIntField reads an integer field that may have arrived as int, float64,
// int64 or string depending on which source produced the node.
func clashIntField(m map[string]any, key string) int {
	switch v := m[key].(type) {
	case int:
		return v
	case float64:
		return int(v)
	case int64:
		return int(v)
	case string:
		n, _ := strconv.Atoi(v)
		return n
	}
	return 0
}

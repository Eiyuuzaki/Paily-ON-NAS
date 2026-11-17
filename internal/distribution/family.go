package distribution

import (
	"context"
	"net"
	"sync"
	"time"
)

// IPv4-less endpoints are useless to almost every client on this line: the
// airport nodes behind ptxlv6-*.78778m.cn only publish AAAA records, and home
// IPv6 (let alone international IPv6) is unreliable to broken. Shipping them
// produces a list where a tap on the node times out immediately, which is what
// happened on 2026-11-17 for the Singapore trojan group.
//
// Resolution is cached and only ever used to DROP a node when a lookup
// positively succeeds with zero A records; a DNS error keeps the node, so a
// transient resolver hiccup cannot empty the pool.
type familyEntry struct {
	hasV4   bool
	checked time.Time
}

var (
	familyMu    sync.Mutex
	familyCache = map[string]familyEntry{}
)

const (
	familyTTL     = time.Hour
	familyTimeout = 3 * time.Second
)

// hasIPv4 reports whether a domain server publishes an A record.
func hasIPv4(parent context.Context, host string) bool {
	familyMu.Lock()
	ent, ok := familyCache[host]
	familyMu.Unlock()
	if ok && time.Since(ent.checked) < familyTTL {
		return ent.hasV4
	}

	ctx, cancel := context.WithTimeout(parent, familyTimeout)
	defer cancel()
	// Look up every address and filter here: an "ip4"-only query returns an
	// error for an AAAA-only host on Go's resolver, which would be mistaken for a
	// transient failure and keep the node.
	ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
	has := true
	if err == nil {
		has = false
		for _, ip := range ips {
			if ip.To4() != nil {
				has = true
				break
			}
		}
	}

	familyMu.Lock()
	familyCache[host] = familyEntry{hasV4: has, checked: time.Now()}
	familyMu.Unlock()
	return has
}

// reachableByFamily reports whether the node's endpoint can be dialled over IPv4.
func reachableByFamily(ctx context.Context, server string) bool {
	if server == "" || net.ParseIP(server) != nil {
		return true
	}
	return hasIPv4(ctx, server)
}

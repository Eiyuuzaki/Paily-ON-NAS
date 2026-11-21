package distribution

import (
	"encoding/json"
	"strings"

	"github.com/Eiyuuzaki/paily-on-nas/third_party/paily-fetch/node"
	"gorm.io/gorm"
)

// warpConfig is the persisted shape of an optional WARP (WireGuard) exit.
//
// WARP is a standalone fallback exit, not a pool node: it never passes the
// checker, carries no score, and is emitted verbatim when enabled. Measured
// 2026-11-22 from this line: colo stays LAX/SJC no matter which endpoint or
// account is used, and the exit IP is a Cloudflare segment that consumer
// services already flag as VPN. It is therefore offered as a last-resort exit
// for reachability, not as a replacement for a clean VPS or a CF Worker.
type warpConfig struct {
	Server     string `json:"server"`
	Port       int    `json:"port"`
	PrivateKey string `json:"private-key"`
	PublicKey  string `json:"public-key"`
	IP         string `json:"ip"`
	IPv6       string `json:"ipv6"`
	Reserved   []int  `json:"reserved"`
	MTU        int    `json:"mtu"`
	Name       string `json:"name"`
	Region     string `json:"region"`
}

// validWARPEndpoint reports whether the stored config can produce a dialable
// WireGuard outbound. Every field checked here is required by the protocol;
// a half-written config would render a node the client cannot dial, which is
// worse than emitting nothing.
func validWARPEndpoint(c warpConfig) bool {
	return strings.TrimSpace(c.Server) != "" &&
		c.Port > 0 && c.Port <= 65535 &&
		strings.TrimSpace(c.PrivateKey) != "" &&
		strings.TrimSpace(c.PublicKey) != "" &&
		strings.TrimSpace(c.IP) != ""
}

// clashMap renders the config as a mihomo/clash proxy entry.
//
// "ip" must be a bare address: mihomo parses it with netip.ParsePrefix and
// rejects a CIDR suffix, which cost one debugging round on 2026-11-22.
func (c warpConfig) clashMap() map[string]any {
	m := map[string]any{
		"name":        c.Name,
		"type":        "wireguard",
		"server":      c.Server,
		"port":        c.Port,
		"private-key": c.PrivateKey,
		"public-key":  c.PublicKey,
		"ip":          c.IP,
		"udp":         true,
		"mtu":         c.MTU,
	}
	if c.IPv6 != "" {
		m["ipv6"] = c.IPv6
	}
	// Cloudflare's WARP servers require the reserved triple that wgcf writes
	// into the profile; omitting it leaves the handshake unanswered.
	if len(c.Reserved) == 3 {
		m["reserved"] = []any{c.Reserved[0], c.Reserved[1], c.Reserved[2]}
	}
	return m
}

// loadWARPNode builds the optional WARP exit from the configs table.
//
// Returns nil when the feature is off, the JSON is unreadable, or the stored
// endpoint is incomplete. Serving silence is deliberate: a malformed WARP block
// must never break an otherwise healthy subscription.
func loadWARPNode(db *gorm.DB) *node.ProxyNode {
	if configInt(db, "warp_enabled", 0) == 0 {
		return nil
	}
	raw := configString(db, "warp_config", "")
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var c warpConfig
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		return nil
	}
	if c.Name == "" {
		c.Name = "WARP"
	}
	if c.MTU <= 0 {
		c.MTU = 1280
	}
	if !validWARPEndpoint(c) {
		return nil
	}
	n, err := node.New(c.clashMap())
	if err != nil {
		return nil
	}
	return n
}

// warpRegion returns the region label used for subscription grouping. The
// measured landing zone of this line is the US west coast (LAX/SJC), so that is
// the fallback when the config does not state one.
func warpRegion(db *gorm.DB) string {
	raw := configString(db, "warp_config", "")
	var c warpConfig
	if raw != "" && json.Unmarshal([]byte(raw), &c) == nil && c.Region != "" {
		return c.Region
	}
	return "US"
}

// pushWARP appends the WARP exit to the generator when the feature is enabled.
//
// sing-box drops wireguard unconditionally (its generator lists the type as
// unsupported), so a subscription in that format simply will not carry the
// exit. That is the intended degradation: no format is broken by enabling it.
func pushWARP(db *gorm.DB, gen interface {
	Push(*node.ProxyNode, string)
}) {
	n := loadWARPNode(db)
	if n == nil {
		return
	}
	name := configString(db, "warp_name", "WARP")
	if region := warpRegion(db); region != "" {
		if rg, ok := gen.(interface {
			PushWithRegion(*node.ProxyNode, string, string)
		}); ok {
			rg.PushWithRegion(n, name, region)
			return
		}
	}
	gen.Push(n, name)
}

package clash

import (
	"fmt"
	"maps"
	"sort"
	"strings"

	"github.com/Eiyuuzaki/paily-on-nas/third_party/paily-fetch/internal/format"
	"github.com/Eiyuuzaki/paily-on-nas/third_party/paily-fetch/node"
	"gopkg.in/yaml.v3"
)

// Generator implements format.FormatGenerator for the clash output format.
// It collects proxy entries as clash map[string]any items and serialises them
// as a YAML document with a top-level "proxies:" list.
//
// All clash-supported proxy types are accepted; no skip conditions apply.
type Generator struct {
	items       []map[string]any
	regionNames map[string][]string
}

var _ format.FormatGenerator = (*Generator)(nil)

// Push appends a proxy entry to the internal list.
// If name is non-empty it overrides the "name" field in the output entry;
// otherwise the original n.Name() value is kept.
// FormatConfig is not used by the clash format and may be nil.
func (g *Generator) Push(n *node.ProxyNode, name string) {
	entry := maps.Clone(n.ClashMap())
	if name != "" {
		entry["name"] = name
	}
	g.items = append(g.items, entry)
}

// PushWithRegion appends a proxy entry and tracks it under region for auto groups.
func (g *Generator) PushWithRegion(n *node.ProxyNode, name, region string) {
	g.Push(n, name)
	if region == "" || region == "CN" {
		return
	}
	if g.regionNames == nil {
		g.regionNames = make(map[string][]string)
	}
	if name != "" {
		g.regionNames[region] = append(g.regionNames[region], name)
	}
}

// regionFlag converts a two-letter ISO country code to its flag emoji.
//
// The length check must happen on runes, not bytes: a two-byte single-rune
// string (e.g. "é") passes a byte-length check but would index past the end
// of the rune slice and panic. Non A-Z input also has no flag representation.
func regionFlag(code string) string {
	rs := []rune(strings.ToUpper(code))
	if len(rs) != 2 {
		return ""
	}
	for _, r := range rs {
		if r < 'A' || r > 'Z' {
			return ""
		}
	}
	return string(rune(0x1F1E6+(rs[0]-'A'))) + string(rune(0x1F1E6+(rs[1]-'A')))
}

// Generate serialises all pushed entries as YAML.
//
// When config contains a non-empty Template string, the template is parsed as
// a full Clash YAML config and the generator:
//   - replaces the top-level "proxies" list with all pushed nodes, and
//   - appends every pushed node's name to the "proxies" list of every proxy-group.
//
// When config is nil or Template is empty, a plain document is emitted:
//
//	proxies:
//	  - {name: ..., type: ..., ...}
func (g *Generator) Generate(cfg format.FormatConfig) ([]byte, error) {
	c := parseConfig(cfg)
	if c.Template == "" {
		out := map[string][]map[string]any{"proxies": g.items}
		data, err := yaml.Marshal(out)
		if err != nil {
			return nil, fmt.Errorf("clash generator: yaml.Marshal: %w", err)
		}
		return data, nil
	}

	// Template mode: parse the full Clash config and inject nodes.
	var doc map[string]any
	if err := yaml.Unmarshal([]byte(c.Template), &doc); err != nil {
		return nil, fmt.Errorf("clash generator: parse template YAML: %w", err)
	}

	// Replace the proxies list.
	doc["proxies"] = g.items

	// Append every pushed node's name to each proxy-group's proxies list.
	names := g.proxyNames()
	if groups, ok := doc["proxy-groups"].([]any); ok {
		for _, grp := range groups {
			gm, ok := grp.(map[string]any)
			if !ok {
				continue
			}
			existing, _ := gm["proxies"].([]any)
			for _, n := range names {
				existing = append(existing, n)
			}
			gm["proxies"] = existing
		}
	}

	// Auto-generate region groups (url-test) from tracked nodes.
	if groups, ok := doc["proxy-groups"].([]any); ok {
		var regionGroups []any
		codes := make([]string, 0, len(g.regionNames))
		for code := range g.regionNames {
			codes = append(codes, code)
		}
		sort.Strings(codes)
		for _, code := range codes {
			names := g.regionNames[code]
			if len(names) == 0 {
				continue
			}
			sorted := append([]string(nil), names...)
			sort.Strings(sorted)
			gm := map[string]any{
				"name":      regionFlag(code) + " " + code + " 节点",
				"type":      "url-test",
				"url":       "https://www.gstatic.com/generate_204",
				"interval":  300,
				"tolerance": 50,
				"proxies":   sorted,
			}
			regionGroups = append(regionGroups, gm)
		}
		// template groups first (Paily Connect / 自动选择 等核心组), then region groups
		doc["proxy-groups"] = append(groups, regionGroups...)
	}

	data, err := yaml.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("clash generator: yaml.Marshal template: %w", err)
	}
	return data, nil
}

// proxyNames returns the "name" field of every pushed proxy item, in order.
func (g *Generator) proxyNames() []string {
	names := make([]string, 0, len(g.items))
	for _, item := range g.items {
		if n, ok := item["name"].(string); ok && n != "" {
			names = append(names, n)
		}
	}
	return names
}

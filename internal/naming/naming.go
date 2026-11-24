// Package naming generates human-readable node names from an expr-lang template.
package naming

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"net"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/Eiyuuzaki/paily-on-nas/internal/cache"
	"github.com/Eiyuuzaki/paily-on-nas/internal/db/model"
	"github.com/Eiyuuzaki/paily-on-nas/internal/filter"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

//go:embed flags.json
var flagsJSON []byte

//go:embed cfcolo.txt
var cfColoData []byte

// cfColo maps a measured CF /24 to the edge colo it lands on from our line.
// CF anycast entry IPs do not sit in the country their address is registered
// to, so a name prefix like "CF-US" records where the IP was sourced, not
// where traffic enters. Measured colo overrides the prefix.
var cfColo []struct {
	Net  *net.IPNet
	Colo string
}

type flagEntry struct {
	Emoji string `json:"emoji"`
	Name  string `json:"name"`
}

// flagsEmoji maps ISO region code → emoji flag.
// flagsName  maps ISO region code → full country/territory name.
var (
	flagsEmoji map[string]string
	flagsName  map[string]string
)

func init() {
	for _, line := range strings.Split(string(cfColoData), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		if _, n, err := net.ParseCIDR(f[0]); err == nil {
			cfColo = append(cfColo, struct {
				Net  *net.IPNet
				Colo string
			}{n, strings.ToUpper(f[1])})
		}
	}
	var raw map[string]flagEntry
	if err := json.Unmarshal(flagsJSON, &raw); err != nil {
		panic("naming: failed to parse flags.json: " + err.Error())
	}
	flagsEmoji = make(map[string]string, len(raw))
	flagsName = make(map[string]string, len(raw))
	for code, fe := range raw {
		flagsEmoji[code] = fe.Emoji
		flagsName[code] = fe.Name
	}
}

// regionOrder defines the preferred display/selection order for known regions.
// Keep this list in sync with product expectations.
var regionOrder = []string{
	"HK", "TW", "JP", "SG", "US", "UK", "GB", "KR", "DE", "FR",
	"AU", "CA", "NL", "RU", "IN", "BR", "TR", "TH", "MY", "PH",
	"ID", "VN", "AR", "MX", "IT", "ES", "PL", "SE", "NO", "FI",
	"CH", "AT", "BE", "DK", "PT", "CZ", "HU", "RO", "ZA", "AE",
	"SA", "IL", "EG", "NG", "UA",
}

// RegionOrder returns the configured preferred region order.
func RegionOrder() []string {
	out := make([]string, len(regionOrder))
	copy(out, regionOrder)
	return out
}

// nodeFields holds the values passed to the naming expression as node.*.
type nodeFields struct {
	Region         string          `expr:"region"`
	RegionFlag     string          `expr:"region_flag"`
	Grade          string          `expr:"grade"`
	Confidence     string          `expr:"confidence"`
	Score          float64         `expr:"score"`
	Seq            string          `expr:"seq"`
	Server         string          `expr:"server"`
	Protocol       string          `expr:"protocol"`
	Alive          bool            `expr:"alive"`
	Streaming      map[string]bool `expr:"streaming"`
	Sponsor        string          `expr:"sponsor"`
	SourcePriority int             `expr:"source_priority"` // 0=公益, 1=机场/付费, 2=最高
	EntryRegion    string          `expr:"entry_region"`    // 入口地区（CF 名字前缀推断，非CF=出口）
	EntryFlag      string          `expr:"entry_flag"`
	// Speed is the measured single-flow throughput of the entry ("18M"),
	// empty when unmeasured. Latency cannot stand in for it: see speedLabel.
	Speed string `expr:"speed"`
}

// namingEnv is the top-level context for the node naming expression.
type namingEnv struct {
	Node        nodeFields `expr:"node"`
	UnlockFlags string     `expr:"unlock_flags"`
	FilterMode  string     `expr:"filter_mode"`
}

// nodeGrade prefers the band the checker actually measured (stored as
// nodes.grade from effective latency). Nodes that have not been re-scored since
// the switch fall back to the older score-based band so nothing goes blank.
func nodeGrade(entry *cache.NodeCacheEntry, gradeS, gradeA, gradeB, gradeC float64) string {
	if entry.Grade != "" {
		return entry.Grade
	}
	return gradeFromScore(entry.Score, gradeS, gradeA, gradeB, gradeC)
}

// defaultTemplate is used when no node_name_template config entry exists.
const defaultTemplate = `(unlock_flags != "" ? "[" + unlock_flags + "] " : "") + node.grade + (node.speed != "" ? " " + node.speed : "") + " · " + (node.entry_region == "" || node.entry_region == node.region ? node.region_flag + node.region : node.entry_region + "→" + node.region) + " " + node.seq + (node.sponsor != "" ? " [" + node.sponsor + "]" : "")`

// cfNameRe extracts the region from a self-built CF node name like CF-HKG-245
// (optional single-letter account segment, e.g. CF-C-SG-01).
var cfNameRe = regexp.MustCompile(`"name":"CF-(?:[A-Z]-)?([A-Z]{2,3})-`)

// coloRegion maps a CF edge colo code to an ISO region code.
var coloRegion = map[string]string{
	"HKG": "HK", "NRT": "JP", "KIX": "JP", "ICN": "KR", "SIN": "SG",
	"LAX": "US", "SJC": "US", "SEA": "US", "ORD": "US", "DFW": "US",
	"IAD": "US", "EWR": "US", "ATL": "US", "MIA": "US",
	"LHR": "GB", "FRA": "DE", "AMS": "NL", "CDG": "FR", "SYD": "AU",
	"TPE": "TW", "KHH": "TW", "BKK": "TH", "KUL": "MY", "SGN": "VN", "MNL": "PH",
}

// Measured entry colo lives in the entry_ips table (filled by core's passive
// refresh and by the periodic discovery scan). Read it through a small cache so
// naming one subscription does not hit the database per node.
var (
	coloMu   sync.RWMutex
	coloMap  = map[string]string{}
	speedMap = map[string]int{}
	coloExp  time.Time
)

const coloTTL = 5 * time.Minute

func refreshEntryColo(db *gorm.DB) {
	coloMu.RLock()
	fresh := time.Now().Before(coloExp) && db != nil
	coloMu.RUnlock()
	if fresh {
		return
	}
	type row struct {
		IP             string
		Colo           string
		ThroughputKbps int
	}
	// Throughput rides along with colo on purpose: both describe the entry the
	// node dials, and one query serves both. Rows with an empty colo are kept
	// when they carry a throughput reading, otherwise a measured-but-unlabelled
	// entry would lose its speed tag.
	var rows []row
	if err := db.Raw("SELECT ip, colo, throughput_kbps FROM entry_ips WHERE colo <> '' OR throughput_kbps > 0").Scan(&rows).Error; err != nil {
		return
	}
	m := make(map[string]string, len(rows))
	sp := make(map[string]int, len(rows))
	for _, r := range rows {
		if r.Colo != "" {
			m[r.IP] = strings.ToUpper(r.Colo)
		}
		if r.ThroughputKbps > 0 {
			sp[r.IP] = r.ThroughputKbps
		}
	}
	coloMu.Lock()
	coloMap = m
	speedMap = sp
	coloExp = time.Now().Add(coloTTL)
	coloMu.Unlock()
	// One line per refresh: the entry cache feeds both the colo and the speed
	// tag, and a silent failure here shows up only as missing labels.
	log.Info().Int("rows", len(rows)).Int("colo", len(m)).Int("speed", len(sp)).
		Msg("naming: entry cache refreshed")
}

// entrySpeed returns the measured single-flow throughput for an entry IP, in
// kbit/s, or 0 when it was never measured.
func entrySpeed(ip string) int {
	coloMu.RLock()
	defer coloMu.RUnlock()
	return speedMap[ip]
}

// speedLabel renders a throughput reading as the short tag carried in node
// names: "18M" for 18 MB/s, "0.8M" below one, "" when unmeasured.
//
// It exists because latency ranks the wrong path on a China Mobile line: the
// Hong Kong edge this line reaches in 47 ms throttles each flow to under
// 1 MB/s while a US West edge at 188 ms carried 18.7 MB/s over a 50 MB pull.
// Without a visible number the two are indistinguishable in a client that
// sorts by latency, which is exactly how the slow one keeps winning.
func speedLabel(ip string) string {
	kbps := entrySpeed(ip)
	if kbps <= 0 {
		return ""
	}
	mbps := float64(kbps) / 8000
	if mbps >= 10 {
		return fmt.Sprintf("%.0fM", mbps)
	}
	return fmt.Sprintf("%.1fM", mbps)
}

// entryColo returns the measured colo for an entry IP, if known.
func entryColo(ip string) (string, bool) {
	coloMu.RLock()
	defer coloMu.RUnlock()
	c, ok := coloMap[ip]
	return c, ok
}

// ColoCandidates returns the /24 CIDRs known to have landed on a fast edge from
// this line, for the discovery scan to re-verify.
func ColoCandidates() []string {
	out := make([]string, 0, len(cfColo))
	for _, e := range cfColo {
		out = append(out, e.Net.String())
	}
	return out
}

// coloForIP returns the measured edge colo for an entry IP (matched by /24).
func coloForIP(ip string) string {
	a := net.ParseIP(strings.TrimSpace(ip))
	if a == nil {
		return ""
	}
	for _, e := range cfColo {
		if e.Net.Contains(a) {
			return e.Colo
		}
	}
	return ""
}

// EntryRegionOf returns the region a CF node is actually entered from, or "".
// server is the node's dialable server (IPv4); raw may carry an IPv6 entry, so
// both are tried against the measured colo map.
func EntryRegionOf(raw, server string) string { return entryRegionOf(raw, server) }

func entryRegionOf(raw, server string) string {
	// Only self-built CF nodes have a meaningful entry region; for everything
	// else traffic enters exactly where it exits.
	m := cfNameRe.FindStringSubmatch(raw)
	if len(m) < 2 {
		return ""
	}
	var obj map[string]any
	if json.Unmarshal([]byte(raw), &obj) == nil {
		if s, ok := obj["server"].(string); ok {
			if c, ok := entryColo(s); ok {
				if r, ok := coloRegion[c]; ok {
					return r
				}
			}
			// Fall back to the embedded scan snapshot; it goes stale (measured
			// 2026-11-17: 4 of 12 entries had drifted), hence live data first.
			if r, ok := coloRegion[coloForIP(s)]; ok {
				return r
			}
		}
	}
	if c, ok := entryColo(server); ok {
		if r, ok := coloRegion[c]; ok {
			return r
		}
	}
	if r, ok := coloRegion[coloForIP(server)]; ok {
		return r
	}
	if rr, ok := coloRegion[m[1]]; ok {
		return rr
	}
	return m[1]
}

// gradeFromScore maps a numeric score to a letter grade using thresholds from
// the configs table.
//
// S exists because the A band had become meaningless: after the probing fix the
// CF pool clustered around 0.71-0.78, so the old A≥0.75 boundary ran straight
// through the middle of the mass and a 20ms wobble flipped a node between A and
// B. Defaults (calibrated on 844 live CF nodes): S≥0.85 (top ~0.5%), A≥0.78
// (~2.7%), B≥0.60 (~32%), C≥0.35 (~56%), else D.
func gradeFromScore(score float64, gradeS, gradeA, gradeB, gradeC float64) string {
	switch {
	case score >= gradeS:
		return "S"
	case score >= gradeA:
		return "A"
	case score >= gradeB:
		return "B"
	case score >= gradeC:
		return "C"
	default:
		return "D"
	}
}

// flagFor returns the emoji flag for a region code, or an empty string if unknown.
func flagFor(region string) string {
	return flagsEmoji[strings.ToUpper(region)]
}

// RegionName returns the full country/territory name for an ISO region code.
// Falls back to the code itself if not found in flags.json.
func RegionName(code string) string {
	if name, ok := flagsName[strings.ToUpper(code)]; ok {
		return name
	}
	return code
}

// seqID formats a region-scoped sequence number, e.g. region="HK", seq=1 → "hk01".
func seqID(region string, seq int) string {
	return fmt.Sprintf("%02d", seq)
}

// configFloat reads a float64 config value; returns def on any error.
func configFloat(db *gorm.DB, key string, def float64) float64 {
	var cfg model.Config
	if err := db.First(&cfg, "key = ?", key).Error; err != nil {
		return def
	}
	var f float64
	if _, err := fmt.Sscanf(cfg.Value, "%f", &f); err != nil {
		return def
	}
	return f
}

// Render generates a display name for a node using the configured expr template.
//
//   - entry: the node cache entry
//   - seqNo: 1-based sequence number within the region
//   - unlockFlags: pre-assembled unlock flag string (e.g. "Netflix|Disney+")
//   - filterMode: "select" or "mark"
//   - preset: distribution preset ("common"/"all"/"other"); non-common uses full region name
//   - db: used to read grade thresholds and the name template from the config table
func Render(ctx context.Context, entry *cache.NodeCacheEntry, seqNo int, unlockFlags, filterMode, preset string, db *gorm.DB) string {
	refreshEntryColo(db)
	gradeS := configFloat(db, "grade_s_min", 0.85)
	gradeA := configFloat(db, "grade_a_min", 0.78)
	gradeB := configFloat(db, "grade_b_min", 0.60)
	gradeC := configFloat(db, "grade_c_min", 0.35)

	region := strings.ToUpper(entry.Region)
	if region == "" {
		region = "UN"
	}
	entryRegion := entryRegionOf(entry.Raw, entry.Server)
	if entryRegion == "" {
		entryRegion = region
	}

	// For non-common presets use the full country name instead of the ISO code.
	regionDisplay := region
	if preset != "common" && preset != "" {
		regionDisplay = RegionName(region)
	}

	env := namingEnv{
		Node: nodeFields{
			Region:         regionDisplay,
			RegionFlag:     flagFor(region),
			Grade:          nodeGrade(entry, gradeS, gradeA, gradeB, gradeC),
			Confidence:     entry.Confidence,
			Score:          entry.Score,
			Seq:            seqID(region, seqNo),
			Server:         entry.Server,
			Speed:          speedLabel(entry.Server),
			Protocol:       entry.Protocol,
			Alive:          entry.Alive,
			Streaming:      entry.StreamingResult,
			Sponsor:        entry.SponsorText,
			SourcePriority: entry.SourcePriority,
			EntryRegion:    entryRegion,
			EntryFlag:      flagFor(entryRegion),
		},
		UnlockFlags: unlockFlags,
		FilterMode:  filterMode,
	}

	tmpl := template(db)
	name, err := filter.EvalString(tmpl, env)
	if err != nil {
		// Fallback: use a minimal safe name to avoid returning empty string.
		return fmt.Sprintf("%s %s", region, seqID(region, seqNo))
	}
	return name
}

// template loads the node_name_template config value, falling back to the default.
func template(db *gorm.DB) string {
	var cfg model.Config
	if err := db.First(&cfg, "key = ?", "node_name_template").Error; err != nil || cfg.Value == "" {
		return defaultTemplate
	}
	return cfg.Value
}

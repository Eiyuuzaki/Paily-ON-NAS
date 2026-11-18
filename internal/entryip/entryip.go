// Package entryip keeps a measured view of the Cloudflare anycast entry
// addresses this deployment uses.
//
// Two sources feed it:
//
//   - Passive: every deep check already measures the node through its entry, so
//     aggregating those results per entry IP gives a free, always-current ranking
//     of the IPs we are actually using. No extra probing.
//   - Scan: a weekly, low-concurrency sweep over candidate /24s finds entries
//     that currently land on a fast colo (HKG from this line).
//
// Rationale: of 12 previously-mapped entries re-checked on 2026-11-17, four that
// used to land HKG had drifted to LAX. Any static table goes stale, so colo is a
// measurement, not a label.
package entryip

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

// coloRegion maps a CF edge colo to an ISO region code.
var coloRegion = map[string]string{
	"HKG": "HK", "NRT": "JP", "KIX": "JP", "ICN": "KR", "SIN": "SG",
	"LAX": "US", "SJC": "US", "SEA": "US", "ORD": "US", "DFW": "US",
	"IAD": "US", "EWR": "US", "ATL": "US", "MIA": "US",
	"LHR": "GB", "FRA": "DE", "AMS": "NL", "CDG": "FR", "SYD": "AU",
	"TPE": "TW", "BKK": "TH", "KUL": "MY", "SGN": "VN", "MNL": "PH",
}

// RegionForColo converts a colo code to a region code ("" when unknown).
func RegionForColo(colo string) string { return coloRegion[strings.ToUpper(colo)] }

// RefreshPassive recomputes per-entry statistics from the most recent deep check
// of every live node. It costs one query and never probes anything.
func RefreshPassive(ctx context.Context, db *gorm.DB, window time.Duration) (int, error) {
	if window <= 0 {
		window = 6 * time.Hour
	}
	type row struct {
		Server  string
		RTTMs   float64
		Samples int
		Success float64
	}
	var rows []row
	err := db.WithContext(ctx).Raw(`
WITH latest AS (
    SELECT DISTINCT ON (n.id) n.server AS server, d.avg_latency_ms AS lat
    FROM nodes n
    JOIN node_deep_checks d ON d.node_id = n.id
    WHERE n.archived_at IS NULL AND d.checked_at > now() - (? * interval '1 second')
    ORDER BY n.id, d.checked_at DESC
)
SELECT server,
       percentile_disc(0.5) WITHIN GROUP (ORDER BY lat) AS rtt_ms,
       count(*) AS samples,
       avg(CASE WHEN lat > 0 THEN 1.0 ELSE 0.0 END) AS success
FROM latest
GROUP BY server`, int(window.Seconds())).Scan(&rows).Error
	if err != nil {
		return 0, fmt.Errorf("entryip: aggregate: %w", err)
	}

	now := time.Now()
	n := 0
	for _, r := range rows {
		ip := strings.TrimSpace(r.Server)
		if ip == "" {
			continue
		}
		if err := db.WithContext(ctx).Exec(`
INSERT INTO entry_ips (ip, colo, rtt_ms, success_rate, samples, source, measured_at)
VALUES (?, '', ?, ?, ?, 'passive', ?)
ON CONFLICT (ip) DO UPDATE
SET rtt_ms = EXCLUDED.rtt_ms,
    success_rate = EXCLUDED.success_rate,
    samples = EXCLUDED.samples,
    measured_at = EXCLUDED.measured_at,
    source = CASE WHEN entry_ips.source = 'scan' THEN 'scan' ELSE 'passive' END`,
			ip, int(r.RTTMs), r.Success, r.Samples, now).Error; err != nil {
			return n, fmt.Errorf("entryip: upsert %s: %w", ip, err)
		}
		n++
	}
	return n, nil
}

// ScanOptions configures a discovery sweep.
type ScanOptions struct {
	// Candidates are /24 CIDRs or plain IPs to probe.
	Candidates []string
	// Pinned entries are always probed regardless of Limit. In-use entry IPs go
	// here: they must keep a fresh colo even when the candidate sample rotates.
	Pinned      []string
	Limit       int
	Concurrency int
	MaxRTTMs    int
}

// Discover probes candidate /24s, keeps the ones that answer quickly and land on
// a Cloudflare colo, and stores them. Returns how many HKG landers were seen.
func Discover(ctx context.Context, db *gorm.DB, opt ScanOptions) (found int, probed int, err error) {
	if opt.Concurrency <= 0 {
		opt.Concurrency = 16
	}
	if opt.MaxRTTMs <= 0 {
		opt.MaxRTTMs = 250
	}
	cands := opt.Candidates
	if opt.Limit > 0 && len(cands) > opt.Limit {
		day := time.Now().YearDay()
		sort.Slice(cands, func(i, j int) bool {
			return hashPair(cands[i], day) < hashPair(cands[j], day)
		})
		cands = cands[:opt.Limit]
	}

	sem := make(chan struct{}, opt.Concurrency)
	var wg sync.WaitGroup
	var mu sync.Mutex
	now := time.Now()
	stats := map[string]int{}

	all := make([]string, 0, len(cands)+len(opt.Pinned))
	all = append(all, opt.Pinned...)
	all = append(all, cands...)
	seen := map[string]bool{}
	for _, cidr := range all {
		ip := representativeIP(cidr)
		if ip == "" || seen[ip] {
			continue
		}
		seen[ip] = true
		wg.Add(1)
		sem <- struct{}{}
		go func(ip string) {
			defer wg.Done()
			defer func() { <-sem }()
			mu.Lock()
			probed++
			mu.Unlock()

			rtt, ok := tcpRTT(ctx, ip, opt.MaxRTTMs)
			if !ok {
				return
			}
			colo, ok := traceColo(ctx, ip)
			if !ok || colo == "" {
				return
			}
			mu.Lock()
			stats[colo]++
			if colo == "HKG" {
				found++
			}
			mu.Unlock()

			_ = db.WithContext(ctx).Exec(`
INSERT INTO entry_ips (ip, colo, scan_rtt_ms, success_rate, samples, source, measured_at)
VALUES (?, ?, ?, 1, 0, 'scan', ?)
ON CONFLICT (ip) DO UPDATE
SET colo = EXCLUDED.colo,
    scan_rtt_ms = EXCLUDED.scan_rtt_ms,
    source = 'scan',
    measured_at = EXCLUDED.measured_at`,
				ip, colo, int(rtt), now).Error
		}(ip)
	}
	wg.Wait()
	log.Info().Interface("colos", stats).Int("found_hkg", found).Int("probed", probed).Msg("entryip: discovery done")
	return found, probed, nil
}

func hashPair(s string, day int) uint32 {
	var h uint32 = 2166136261
	for _, c := range []byte(s) {
		h ^= uint32(c)
		h *= 16777619
	}
	h ^= uint32(day) * 2654435761
	return h
}

func representativeIP(cidr string) string {
	cidr = strings.TrimSpace(cidr)
	// A plain IP is used as-is (in-use entry addresses).
	if !strings.Contains(cidr, "/") {
		if a := net.ParseIP(cidr); a != nil {
			return a.String()
		}
		return ""
	}
	_, n, err := net.ParseCIDR(cidr)
	if err != nil {
		return ""
	}
	if base := n.IP.To4(); base != nil {
		ip := make(net.IP, 4)
		copy(ip, base)
		ip[3] = 1
		return ip.String()
	}
	// IPv6 has no /24 to expand: probe the first address the same way the IPv4
	// branch picks .1. Without this, v6 entry IPs never get a measured colo and
	// naming falls back to the name prefix (the misleading "US→HK" display).
	ip := make(net.IP, len(n.IP))
	copy(ip, n.IP)
	ip[len(ip)-1] = 1
	return ip.String()
}

func tcpRTT(ctx context.Context, ip string, maxMs int) (float64, bool) {
	d := net.Dialer{Timeout: time.Duration(maxMs) * time.Millisecond}
	t0 := time.Now()
	c, err := d.DialContext(ctx, "tcp", net.JoinHostPort(ip, "443"))
	if err != nil {
		return 0, false
	}
	rtt := float64(time.Since(t0).Microseconds()) / 1000.0
	_ = c.Close()
	if int(rtt) > maxMs {
		return 0, false
	}
	return rtt, true
}

func traceColo(ctx context.Context, ip string) (string, bool) {
	tr := &http.Transport{
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, network, net.JoinHostPort(ip, "443"))
		},
		TLSClientConfig:     &tls.Config{ServerName: "cp.cloudflare.com", MinVersion: tls.VersionTLS12},
		DisableKeepAlives:   true,
		TLSHandshakeTimeout: 5 * time.Second,
	}
	cl := &http.Client{Transport: tr, Timeout: 8 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "GET", "https://cp.cloudflare.com/cdn-cgi/trace", nil)
	if err != nil {
		return "", false
	}
	resp, err := cl.Do(req)
	if err != nil {
		return "", false
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return "", false
	}
	for _, line := range strings.Split(string(body), "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(line), "colo="); ok {
			return strings.ToUpper(strings.TrimSpace(v)), true
		}
	}
	return "", false
}

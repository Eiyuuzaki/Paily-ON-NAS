package entryip

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
)

// defaultBenchLimit is how many entries one run measures when the caller sets
// no limit. Kept in one place so Candidates and Bench cannot disagree about it.
const defaultBenchLimit = 40

// BenchOptions configures a per-entry throughput measurement.
type BenchOptions struct {
	// URL must be served by an edge that accepts any Cloudflare anycast address.
	URL string
	// Runs is how many samples per entry; the median is stored.
	Runs int
	// Concurrency bounds simultaneous downloads. Keep it small: the point is to
	// measure per-flow throughput, so the flows must not compete for the home
	// uplink.
	Concurrency int
	// Limit caps how many entries are measured per run.
	Limit int
	// Timeout bounds a single download.
	Timeout time.Duration
}

// Bench measures single-flow download throughput for the entry addresses in
// rotation and stores the median per entry.
//
// Single-flow, on purpose: the throttle that motivated this column is per TCP
// flow (~1 MB/s on CF's HKG edge from a China Mobile line, unlimited on US
// West). Measuring parallel connections would average that away and report a
// healthy number for an entry that feels unusable to one client.
//
// The candidate set is "entries nodes are actually dialling", then the fastest
// scanned entries as filler, so the cost is bounded by Limit rather than by the
// 2k rows in entry_ips.
func Bench(ctx context.Context, db *gorm.DB, opt BenchOptions) (measured int, failed int, err error) {
	if opt.URL == "" {
		return 0, 0, fmt.Errorf("entryip: bench needs a URL")
	}
	if opt.Runs <= 0 {
		opt.Runs = 2
	}
	if opt.Concurrency <= 0 {
		opt.Concurrency = 2
	}
	if opt.Limit <= 0 {
		opt.Limit = defaultBenchLimit
	}
	if opt.Timeout <= 0 {
		opt.Timeout = 20 * time.Second
	}
	u, err := url.Parse(opt.URL)
	if err != nil {
		return 0, 0, fmt.Errorf("entryip: bad bench URL: %w", err)
	}
	host := u.Hostname()

	ips, err := benchCandidates(db, opt.Limit)
	if err != nil {
		return 0, 0, err
	}

	type result struct {
		ip   string
		kbps int
		ok   bool
	}
	out := make(chan result, len(ips))
	sem := make(chan struct{}, opt.Concurrency)
	var wg sync.WaitGroup
	for _, ip := range ips {
		wg.Add(1)
		go func(ip string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			kbps, ok := benchOne(ctx, ip, host, opt)
			out <- result{ip: ip, kbps: kbps, ok: ok}
		}(ip)
	}
	wg.Wait()
	close(out)

	now := time.Now()
	for r := range out {
		if !r.ok {
			failed++
			// Advance the rotation even on failure. Without this an entry that
			// never answers keeps sorting first and is retried on every run,
			// spending the budget the rest of the fleet needs: on 2026-11-24 the
			// head of the candidate list was 32 IPv6 entries never measured.
			if e := markAttempted(db, r.ip, now); e != nil {
				return measured, failed, e
			}
			continue
		}
		if e := storeThroughput(db, r.ip, r.kbps, now); e != nil {
			return measured, failed, e
		}
		measured++
	}
	return measured, failed, nil
}

// Candidates returns the entry addresses the next bench would measure, in the
// order it would try them. Exposed so a dry run can show the rotation without
// downloading anything. A non-positive limit takes Bench's own default, so a
// dry run never reports an empty list for a setting Bench would have accepted.
func Candidates(db *gorm.DB, limit int) ([]string, error) {
	if limit <= 0 {
		limit = defaultBenchLimit
	}
	return benchCandidates(db, limit)
}

// isEntryAddress reports whether s is a literal IPv4 or IPv6 address.
// Hostnames are rejected: benchOne dials the address itself, so a hostname only
// measures whatever DNS happens to resolve to, which is not an entry reading.
// Those attempts always burned a full payload before failing.
func isEntryAddress(s string) bool {
	if s == "" || strings.ContainsAny(s, "/ ") {
		return false
	}
	return net.ParseIP(s) != nil
}

// benchCandidates returns in-use entries first, then the lowest-RTT entries,
// de-duplicated, capped at limit.
func benchCandidates(db *gorm.DB, limit int) ([]string, error) {
	seen := make(map[string]bool, limit)
	ips := make([]string, 0, limit)

	// Ordered by least-recently-measured, then by the best score behind each
	// entry. Score alone re-measured the same top slice every run: the fleet has
	// ~750 distinct servers in rotation while one run measures at most Limit of
	// them, so entries outside that slice never received a reading at all
	// (observed 2026-11-24: 32 of 2287 entry rows carried a throughput figure).
	// Ordering by throughput_at first rotates coverage across the whole fleet on
	// successive runs, while still preferring the entries distribution ships.
	var inUse []string
	if err := db.Raw(`SELECT n.server FROM nodes n
		LEFT JOIN entry_ips e ON e.ip = n.server
		WHERE n.archived_at IS NULL AND n.alive = true AND n.server <> ''
		  AND (n.server ~ '^[0-9.]+$' OR n.server ~ ':')
		GROUP BY n.server, e.throughput_at, e.attempted_at
		ORDER BY greatest(coalesce(e.throughput_at, to_timestamp(0)),
			coalesce(e.attempted_at, to_timestamp(0))) ASC,
			max(n.score) DESC, count(*) DESC`).Scan(&inUse).Error; err != nil {
		return nil, err
	}
	for _, ip := range inUse {
		if !isEntryAddress(ip) || seen[ip] {
			continue
		}
		seen[ip] = true
		ips = append(ips, ip)
		if len(ips) >= limit {
			return ips, nil
		}
	}

	// Filler accepts any entry with a latency reading, not only scanned entries
	// that also carry a colo. Entries created by the passive refresh have rtt_ms
	// but neither colo nor scan_rtt_ms, and the old predicate made them
	// permanently ineligible for a measurement — observed 2026-11-24 on the
	// self-built KHH/SIN edges, whose throughput had to be written by hand.
	var extra []string
	if err := db.Raw(`SELECT ip FROM entry_ips
		WHERE (rtt_ms > 0 OR scan_rtt_ms > 0)
		  AND (ip ~ '^[0-9.]+$' OR ip ~ ':')
		ORDER BY coalesce(nullif(scan_rtt_ms, 0), rtt_ms) ASC`).Scan(&extra).Error; err != nil {
		return nil, err
	}
	for _, ip := range extra {
		if !isEntryAddress(ip) || seen[ip] {
			continue
		}
		seen[ip] = true
		ips = append(ips, ip)
		if len(ips) >= limit {
			break
		}
	}
	return ips, nil
}

// markAttempted records a failed attempt so the candidate ordering moves past
// the entry without claiming a throughput reading.
func markAttempted(db *gorm.DB, ip string, at time.Time) error {
	return db.Exec(`UPDATE entry_ips SET attempted_at = ? WHERE ip = ?`, at, ip).Error
}

// storeThroughput writes the measurement, inserting a row when the entry was
// never scanned (in-use addresses can predate entry_ips).
func storeThroughput(db *gorm.DB, ip string, kbps int, at time.Time) error {
	res := db.Exec(`UPDATE entry_ips SET throughput_kbps = ?, throughput_at = ?
		WHERE ip = ?`, kbps, at, ip)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected > 0 {
		return nil
	}
	return db.Exec(`INSERT INTO entry_ips (ip, colo, rtt_ms, scan_rtt_ms, success_rate, samples, source, measured_at, throughput_kbps, throughput_at)
		VALUES (?, '', 0, 0, 0, 0, 'bench', ?, ?, ?)
		ON CONFLICT (ip) DO UPDATE SET throughput_kbps = EXCLUDED.throughput_kbps,
			throughput_at = EXCLUDED.throughput_at`, ip, at, kbps, at).Error
}

// benchOne downloads the URL once per run pinned to the given entry address and
// returns the median throughput in kbit/s.
func benchOne(ctx context.Context, ip, host string, opt BenchOptions) (int, bool) {
	samples := make([]int, 0, opt.Runs)
	for i := 0; i < opt.Runs; i++ {
		kbps, err := downloadThrough(ctx, ip, host, opt)
		if err != nil {
			continue
		}
		samples = append(samples, kbps)
	}
	if len(samples) == 0 {
		return 0, false
	}
	sort.Ints(samples)
	return samples[len(samples)/2], true
}

func downloadThrough(ctx context.Context, ip, host string, opt BenchOptions) (int, error) {
	dialer := &net.Dialer{Timeout: opt.Timeout}
	tr := &http.Transport{
		DisableKeepAlives: true,
		DialTLSContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(ip, "443"))
			if err != nil {
				return nil, err
			}
			tlsConn := tls.Client(conn, &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12})
			if err := tlsConn.HandshakeContext(ctx); err != nil {
				conn.Close()
				return nil, err
			}
			return tlsConn, nil
		},
	}
	defer tr.CloseIdleConnections()

	client := &http.Client{Transport: tr, Timeout: opt.Timeout}
	reqCtx, cancel := context.WithTimeout(ctx, opt.Timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, opt.URL, nil)
	if err != nil {
		return 0, err
	}
	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("status %d", resp.StatusCode)
	}
	n, err := io.Copy(io.Discard, resp.Body)
	elapsed := time.Since(start).Seconds()
	if n == 0 {
		return 0, fmt.Errorf("no bytes")
	}
	if err != nil && n < 64*1024 {
		return 0, err
	}
	if elapsed <= 0 {
		return 0, fmt.Errorf("zero elapsed")
	}
	return int(float64(n) * 8 / 1000 / elapsed), nil
}

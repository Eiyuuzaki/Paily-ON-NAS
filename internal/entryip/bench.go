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
	"sync"
	"time"

	"gorm.io/gorm"
)

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
		opt.Limit = 40
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
			continue
		}
		if e := storeThroughput(db, r.ip, r.kbps, now); e != nil {
			return measured, failed, e
		}
		measured++
	}
	return measured, failed, nil
}

// benchCandidates returns in-use entries first, then the lowest-RTT scanned
// entries, de-duplicated, capped at limit.
func benchCandidates(db *gorm.DB, limit int) ([]string, error) {
	seen := make(map[string]bool, limit)
	ips := make([]string, 0, limit)

	// Ordered by the best score behind each entry, not alphabetically: with a
	// limit well below the number of entries in rotation, the reading only helps
	// if it covers the entries distribution is most likely to ship. Ranking by
	// server name would sample an arbitrary slice and leave the shipped nodes
	// unmeasured — observed on 2026-11-23, where 12 measured entries overlapped
	// the shipped set by zero.
	var inUse []string
	if err := db.Raw(`SELECT server FROM nodes
		WHERE archived_at IS NULL AND alive = true AND server <> ''
		GROUP BY server ORDER BY max(score) DESC, count(*) DESC`).Scan(&inUse).Error; err != nil {
		return nil, err
	}
	for _, ip := range inUse {
		if ip == "" || seen[ip] {
			continue
		}
		seen[ip] = true
		ips = append(ips, ip)
		if len(ips) >= limit {
			return ips, nil
		}
	}

	var extra []string
	if err := db.Raw(`SELECT ip FROM entry_ips
		WHERE colo <> '' AND scan_rtt_ms > 0 ORDER BY scan_rtt_ms ASC`).Scan(&extra).Error; err != nil {
		return nil, err
	}
	for _, ip := range extra {
		if ip == "" || seen[ip] {
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

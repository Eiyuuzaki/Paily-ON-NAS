// Package ech decides whether a TLS proxy may safely advertise ECH.
//
// mihomo and sing-box both fetch the ECHConfigList from the target's DNS HTTPS
// record at connect time and refuse the connection when that record is absent
// ("resolve ECH config error: no ECH config found in DNS records"). Injecting
// ECH blindly therefore converts a working node into a dead one, which is worse
// than a plaintext SNI. The opposite case matters just as much: when the record
// does carry a config, ECH hides the SNI the censoring middlebox matches on, so
// injection is the difference between a blocked and a working node.
//
// Measured on the live subscription (2026-11-20): of 171 nodes, injecting ECH
// for every TLS proxy left 126 reachable, stripping it everywhere left 53, and
// the union is 137 - 84 nodes need ECH, 11 are killed by it. This package is
// what recovers that union: gate on the record, inject only where it exists.
package ech

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// dohTarget pins a resolver endpoint to a literal address. Resolving the DoH
// host through the system resolver first is not acceptable here: inside the
// deployment container that lookup intermittently fails, which would silently
// downgrade every node to no-ECH and invert the measurement above.
type dohTarget struct {
	IP   string
	Host string
	Path string
}

var dohTargets = []dohTarget{
	{"223.5.5.5", "dns.alidns.com", "/resolve?name=%s&type=HTTPS"},
	{"223.6.6.6", "dns.alidns.com", "/resolve?name=%s&type=HTTPS"},
	{"1.1.1.1", "cloudflare-dns.com", "/dns-query?name=%s&type=HTTPS"},
}

const (
	// A published config rarely disappears, while a negative answer may only
	// reflect a resolver hiccup, so negatives are re-checked far sooner.
	positiveTTL = 30 * time.Minute
	negativeTTL = 5 * time.Minute
	queryTO     = 4 * time.Second
)

type cacheEntry struct {
	has     bool
	expires time.Time
}

var (
	mu    sync.Mutex
	cache = map[string]cacheEntry{}
)

// lookup is the network probe; tests replace it to stay hermetic.
var lookup = queryDoH

// Enabled reports whether entry may receive an ECH directive: it must be TLS
// and its ECH-covered name must publish a config.
func Enabled(entry map[string]any) bool {
	host := HostFor(entry)
	if host == "" {
		return false
	}
	return HasRecord(host)
}

// HostFor returns the name ECH would cover. ECH encrypts the SNI, so the
// servername wins over the server address; an IP literal yields "" because a
// literal has no HTTPS record to consult.
func HostFor(entry map[string]any) string {
	for _, key := range []string{"servername", "sni", "server"} {
		v, ok := entry[key].(string)
		if !ok {
			continue
		}
		v = strings.TrimSpace(v)
		if v == "" || net.ParseIP(v) != nil {
			continue
		}
		return v
	}
	return ""
}

// HasRecord reports whether host publishes an ECH config. A failed lookup is
// reported as false: refusing to connect costs more than a plaintext SNI.
func HasRecord(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
	if host == "" {
		return false
	}
	now := time.Now()

	mu.Lock()
	if e, ok := cache[host]; ok && now.Before(e.expires) {
		mu.Unlock()
		return e.has
	}
	mu.Unlock()

	has, err := lookup(host)
	ttl := negativeTTL
	if has {
		ttl = positiveTTL
	}
	mu.Lock()
	cache[host] = cacheEntry{has: has, expires: now.Add(ttl)}
	mu.Unlock()
	return has && err == nil
}

// queryDoH asks each endpoint in turn for the host's HTTPS record.
func queryDoH(host string) (bool, error) {
	var lastErr error
	for _, t := range dohTargets {
		has, err := queryOne(t, host)
		if err == nil {
			return has, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no DoH endpoint answered")
	}
	return false, lastErr
}

// queryOne performs one DoH GET. The URL carries the literal address and the
// resolver name travels in the Host header and SNI, so no system resolution is
// needed for the endpoint itself.
func queryOne(t dohTarget, host string) (bool, error) {
	url := "https://" + net.JoinHostPort(t.IP, "443") + fmt.Sprintf(t.Path, host)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return false, err
	}
	req.Host = t.Host
	req.Header.Set("accept", "application/dns-json")

	client := &http.Client{
		Timeout: queryTO,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				ServerName: t.Host,
				MinVersion: tls.VersionTLS12,
			},
			ForceAttemptHTTP2: false,
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return false, err
	}
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("%s: status %d", t.Host, resp.StatusCode)
	}
	var doc struct {
		Status int
		Answer []struct {
			Type int
			Data string
		}
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return false, err
	}
	// Status 0 (NOERROR) and 3 (NXDOMAIN) are definitive; anything else means
	// the resolver could not answer and must not be read as "no ECH here".
	if doc.Status != 0 && doc.Status != 3 {
		return false, fmt.Errorf("%s: dns status %d", t.Host, doc.Status)
	}
	for _, a := range doc.Answer {
		if a.Type == 65 && strings.Contains(strings.ToLower(a.Data), "ech=") {
			return true, nil
		}
	}
	return false, nil
}

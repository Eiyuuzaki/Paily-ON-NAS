package checker

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"time"

	api "github.com/Eiyuuzaki/paily-on-nas/internal/api/generated"
	"github.com/google/uuid"
)

// collectEgressIPs maps node id -> measured egress IP from deep metadata.
// egress_ip is recorded by the checker's layer-egress probe; it is the real
// exit address, unlike node.Server which is the entry.
func collectEgressIPs(results []api.CheckResult) map[uuid.UUID]string {
	m := make(map[uuid.UUID]string)
	for _, r := range results {
		if r.Deep == nil || r.Deep.Metadata == nil {
			continue
		}
		v, ok := (*r.Deep.Metadata)["egress_ip"].(string)
		if !ok || v == "" || net.ParseIP(v) == nil {
			continue
		}
		m[uuid.UUID(r.NodeId)] = v
	}
	return m
}

// enrichRegionFromEgress fills region for nodes that have a measured egress IP
// but no region yet, by geo-resolving the EGRESS IP (accurate exit country).
// Runs before the entry-IP fallback so the exit country wins when available.
// Region_source is set to native_egress. Best-effort.
func enrichRegionFromEgress(ctx context.Context, egressIPs map[uuid.UUID]string, regionUpdates, regionSources map[uuid.UUID]string) {
	if len(egressIPs) == 0 {
		return
	}
	ipToNodes := make(map[string][]uuid.UUID, len(egressIPs))
	order := make([]string, 0, len(egressIPs))
	for id, ip := range egressIPs {
		if _, ok := regionUpdates[id]; ok {
			continue
		}
		if _, seen := ipToNodes[ip]; !seen {
			order = append(order, ip)
		}
		ipToNodes[ip] = append(ipToNodes[ip], id)
	}
	if len(order) == 0 {
		return
	}

	client := &http.Client{Timeout: 10 * time.Second}
	const batchSize = 100
	for start := 0; start < len(order); start += batchSize {
		end := start + batchSize
		if end > len(order) {
			end = len(order)
		}
		batch := order[start:end]
		body, _ := json.Marshal(batch)
		req, err := http.NewRequestWithContext(ctx, "POST",
			"http://ip-api.com/batch?fields=query,countryCode", bytes.NewReader(body))
		if err != nil {
			return
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			return
		}
		var results []geoipEntry
		err = json.NewDecoder(resp.Body).Decode(&results)
		resp.Body.Close()
		if err != nil {
			return
		}
		for _, r := range results {
			if r.CountryCode == "" {
				continue
			}
			for _, id := range ipToNodes[r.Query] {
				if _, exists := regionUpdates[id]; !exists {
					regionUpdates[id] = r.CountryCode
					regionSources[id] = "native_egress"
				}
			}
		}
		if start+batchSize < len(order) {
			select {
			case <-ctx.Done():
				return
			case <-time.After(1500 * time.Millisecond):
			}
		}
	}
}

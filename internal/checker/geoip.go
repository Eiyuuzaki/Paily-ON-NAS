package checker

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/Eiyuuzaki/paily-on-nas/internal/db/model"
	"gorm.io/gorm"
)

type geoipEntry struct {
	Query       string `json:"query"`
	CountryCode string `json:"countryCode"`
}

// enrichRegionGeoIP fills region for alive nodes that mihomo didn't provide a
// region for, by resolving the server address and querying ip-api.com batch.
// Mutates regionUpdates in place. Silently skips on any error (best-effort).
func enrichRegionGeoIP(ctx context.Context, db *gorm.DB, nodeIDs []uuid.UUID, regionUpdates map[uuid.UUID]string) {
	if len(nodeIDs) == 0 {
		return
	}

	// Collect nodes that lack a region
	need := make([]uuid.UUID, 0, len(nodeIDs))
	for _, id := range nodeIDs {
		if _, ok := regionUpdates[id]; !ok {
			need = append(need, id)
		}
	}
	if len(need) == 0 {
		return
	}

	// Fetch server addresses
	var nodes []model.Node
	if err := db.WithContext(ctx).Where("id IN ?", need).Find(&nodes).Error; err != nil {
		return
	}

	// Resolve hostnames → IPs
	ipToNode := make(map[string]uuid.UUID, len(nodes))
	var ips []string
	for _, n := range nodes {
		if !n.Alive {
			continue
		}
		if net.ParseIP(n.Server) != nil {
			ipToNode[n.Server] = n.ID
			ips = append(ips, n.Server)
			continue
		}
		addrs, err := net.LookupHost(n.Server)
		if err != nil || len(addrs) == 0 {
			continue
		}
		ipToNode[addrs[0]] = n.ID
		ips = append(ips, addrs[0])
	}
	if len(ips) == 0 {
		return
	}

	// Batch query ip-api.com (100 per request, free tier)
	client := &http.Client{Timeout: 10 * time.Second}
	batchSize := 100
	for start := 0; start < len(ips); start += batchSize {
		end := start + batchSize
		if end > len(ips) {
			end = len(ips)
		}
		batch := ips[start:end]
		body, _ := json.Marshal(batch)
		req, err := http.NewRequestWithContext(ctx, "POST",
			"http://ip-api.com/batch?fields=query,countryCode",
			bytes.NewReader(body))
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
			if nodeID, ok := ipToNode[r.Query]; ok {
				if _, exists := regionUpdates[nodeID]; !exists {
					regionUpdates[nodeID] = r.CountryCode
				}
			}
		}
		// Rate limit: ip-api free = 45 req/min
		if start+batchSize < len(ips) {
			select {
			case <-ctx.Done():
				return
			case <-time.After(1500 * time.Millisecond):
			}
		}
	}
}

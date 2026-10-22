// Package traffic polls the mihomo external-controller API to collect
// per-proxy traffic stats, then aggregates them by source.
//
// The reporter runs as a background goroutine started from main.go.
// It requires:
//   - MIHOMO_API_URL:   e.g. "http://172.17.0.1:9090"
//   - MIHOMO_API_SECRET: e.g. "admin"
package traffic

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"github.com/Eiyuuzaki/paily-on-nas/internal/db/model"
	"gorm.io/gorm"
)

// individualProxyTypes are the proxy types that represent individual nodes
// (not proxy groups). Proxy groups have types like "select", "url-test", etc.
var individualProxyTypes = map[string]bool{
	"vless":    true,
	"vmess":    true,
	"ss":       true,
	"trojan":   true,
	"http":     true,
	"socks5":   true,
	"socks":    true,
	"hysteria2": true,
	"tuic":     true,
	"anytls":   true,
	"snell":    true,
	"hysteria": true,
	"ssr":      true,
}

// mihomoProxy is the subset of mihomo's proxy object we care about.
type mihomoProxy struct {
	Server   string `json:"server"`
	Port     int    `json:"port"`
	Type     string `json:"type"`
	Upload   int64  `json:"upload"`
	Download int64  `json:"download"`
}

// mihomoProxiesResponse is the response from GET /proxies.
type mihomoProxiesResponse struct {
	Proxies map[string]mihomoProxy `json:"proxies"`
}

// Reporter polls mihomo and stores aggregated traffic per source.
type Reporter struct {
	mihomoURL  string
	secret     string
	db         *gorm.DB
	interval   time.Duration
	httpClient *http.Client
}

// New creates a traffic Reporter.
func New(mihomoURL, secret string, db *gorm.DB) *Reporter {
	return &Reporter{
		mihomoURL:  strings.TrimRight(mihomoURL, "/"),
		secret:     secret,
		db:         db,
		interval:   10 * time.Minute,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// Run starts the polling loop. It blocks until the context is cancelled.
func (r *Reporter) Run(ctx context.Context) {
	if r.mihomoURL == "" {
		log.Info().Msg("traffic reporter: MIHOMO_API_URL not set, skipping")
		return
	}
	log.Info().Str("url", r.mihomoURL).Msg("traffic reporter: starting")

	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	// Run once immediately, then on each tick.
	r.fetchAndStore(ctx)

	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("traffic reporter: stopping")
			return
		case <-ticker.C:
			r.fetchAndStore(ctx)
		}
	}
}

// fetchAndStore queries mihomo for per-proxy traffic, maps proxies to
// sources via the nodes/node_sources tables, and stores aggregated stats.
func (r *Reporter) fetchAndStore(ctx context.Context) {
	proxies, err := r.fetchProxies(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("traffic reporter: failed to fetch proxies")
		return
	}

	// Build a map of server:port:protocol → node ID for alive nodes.
	nodeMap, err := r.buildNodeMap(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("traffic reporter: failed to build node map")
		return
	}

	// Aggregate traffic by source.
	trafficBySource := make(map[uuid.UUID]*model.SourceTrafficStat)

	for _, proxy := range proxies {
		if !individualProxyTypes[proxy.Type] {
			continue // skip proxy groups
		}
		if proxy.Upload + proxy.Download == 0 {
			continue
	}

		key := fmt.Sprintf("%s:%d:%s", proxy.Server, proxy.Port, proxy.Type)
		nodeID, ok := nodeMap[key]
		if !ok {
			continue
		}

		// Get source IDs for this node, prefer highest priority.
		var sourceIDs []uuid.UUID
		var sources []model.Source
		if err := r.db.WithContext(ctx).
			Table("node_sources ns").
			Select("ns.source_id").
			Joins("JOIN sources s ON s.id = ns.source_id").
			Where("ns.node_id = ?", nodeID).
			Pluck("ns.source_id", &sourceIDs).Error; err != nil {
			continue
		}

		if len(sourceIDs) == 0 {
			continue
		}

		// If multiple sources, attribute to the highest priority one.
		if len(sourceIDs) > 1 {
			if err := r.db.WithContext(ctx).
				Where("id IN ?", sourceIDs).
				Order("priority DESC").
				Find(&sources).Error; err != nil {
				continue
			}
			if len(sources) > 0 {
				sourceIDs = sourceIDs[:0]
				sourceIDs = append(sourceIDs, sources[0].ID)
			}
		}

		for _, sid := range sourceIDs {
			if trafficBySource[sid] == nil {
				trafficBySource[sid] = &model.SourceTrafficStat{
					SourceID: sid,
					Upload:   0,
					Download: 0,
				}
			}
			trafficBySource[sid].Upload += proxy.Upload
			trafficBySource[sid].Download += proxy.Download
		}
	}

	if len(trafficBySource) == 0 {
		log.Debug().Msg("traffic reporter: no traffic to store")
		return
	}

	// Store stats (replace old records for this cycle).
	now := time.Now()
	for sid, stats := range trafficBySource {
		stats.RecordedAt = now
		// Delete old record and insert new one.
		if err := r.db.WithContext(ctx).
			Where("source_id = ?", sid).
			Delete(&model.SourceTrafficStat{}).Error; err != nil {
			log.Warn().Err(err).Str("source_id", sid.String()).Msg("traffic reporter: failed to delete old stat")
			continue
		}
		if err := r.db.WithContext(ctx).Create(stats).Error; err != nil {
			log.Warn().Err(err).Str("source_id", sid.String()).Msg("traffic reporter: failed to store stat")
		}
	}

	log.Info().
		Int("sources", len(trafficBySource)).
		Msg("traffic reporter: stats stored")
}

// fetchProxies queries the mihomo /proxies endpoint.
func (r *Reporter) fetchProxies(ctx context.Context) (map[string]mihomoProxy, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.mihomoURL+"/proxies", nil)
	if err != nil {
		return nil, err
	}
	if r.secret != "" {
		req.Header.Set("Authorization", "Bearer "+r.secret)
	}

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("mihomo API returned %d", resp.StatusCode)
	}

	var result mihomoProxiesResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return result.Proxies, nil
}

// buildNodeMap queries alive nodes and returns a map of
// "server:port:protocol" → node ID.
func (r *Reporter) buildNodeMap(ctx context.Context) (map[string]uuid.UUID, error) {
	var nodes []model.Node
	if err := r.db.WithContext(ctx).
		Where("alive = ?", true).
		Find(&nodes).Error; err != nil {
		return nil, err
	}

	nodeMap := make(map[string]uuid.UUID, len(nodes))
	for _, n := range nodes {
		key := fmt.Sprintf("%s:%d:%s", n.Server, n.Port, n.Protocol)
		nodeMap[key] = n.ID
	}
	return nodeMap, nil
}

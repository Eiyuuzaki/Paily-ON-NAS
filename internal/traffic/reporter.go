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

		// 先读出上一轮快照，用于计算增量。必须在覆写之前读。
		var prev model.SourceTrafficStat
		hasPrev := r.db.WithContext(ctx).
			Where("source_id = ?", sid).
			Order("recorded_at DESC").
			Limit(1).
			Find(&prev).Error == nil && prev.ID != 0

		// 覆写快照。
		if err := r.db.WithContext(ctx).
			Where("source_id = ?", sid).
			Delete(&model.SourceTrafficStat{}).Error; err != nil {
			log.Warn().Err(err).Str("source_id", sid.String()).Msg("traffic reporter: failed to delete old stat")
			continue
		}
		if err := r.db.WithContext(ctx).Create(stats).Error; err != nil {
			log.Warn().Err(err).Str("source_id", sid.String()).Msg("traffic reporter: failed to store stat")
			continue
		}

		// 累加当日增量。
		if err := r.addDailyDelta(ctx, sid, now, prev, hasPrev, stats); err != nil {
			log.Warn().Err(err).Str("source_id", sid.String()).Msg("traffic reporter: failed to store daily delta")
		}
	}

	log.Info().
		Int("sources", len(trafficBySource)).
		Msg("traffic reporter: stats stored")
}

// addDailyDelta 把「本轮累计值 - 上轮累计值」累加到当天的聚合行上。
//
// 为什么要增量而不是直接存累计值：mihomo 的计数器会随重启归零，
// 如果每天存累计值，重启那天会突然变小，按范围求和就失去意义。
// 存增量则重启只会丢掉「重启前最后一段未采集的部分」，历史不受影响。
//
// 负数增量的两种情况都按「计数器归零后重新累加」处理，即直接用当前值：
//   - mihomo 重启，计数器从 0 开始
//   - 上一轮之后没有流量，但读者预期不应出现负数
func (r *Reporter) addDailyDelta(
	ctx context.Context,
	sid uuid.UUID,
	now time.Time,
	prev model.SourceTrafficStat,
	hasPrev bool,
	cur *model.SourceTrafficStat,
) error {
	var dUp, dDown int64
	if hasPrev {
		dUp = cur.Upload - prev.Upload
		dDown = cur.Download - prev.Download
		if dUp < 0 {
			dUp = cur.Upload
		}
		if dDown < 0 {
			dDown = cur.Download
		}
	} else {
		// 首次采集：把当前累计值整体记为今天的量。
		dUp = cur.Upload
		dDown = cur.Download
	}

	if dUp == 0 && dDown == 0 {
		return nil
	}

	// 用本地时区的日期做分组键，避免跨零点时按 UTC 分到前一天。
	y, m, d := now.In(time.Local).Date()
	day := time.Date(y, m, d, 0, 0, 0, 0, time.Local)

	return r.db.WithContext(ctx).Exec(`
		INSERT INTO source_traffic_daily (source_id, day, upload, download, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (source_id, day) DO UPDATE SET
			upload     = source_traffic_daily.upload   + EXCLUDED.upload,
			download   = source_traffic_daily.download + EXCLUDED.download,
			updated_at = EXCLUDED.updated_at
	`, sid, day, dUp, dDown, now).Error
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

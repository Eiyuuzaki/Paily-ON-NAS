package handler

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

var pathCapabilityNames = []string{
	"access-chatgpt", "access-netflix", "access-youtube", "layer-egress", "layer-tls443",
}

type pathQualityObservation struct {
	PathKey         string    `gorm:"column:path_key"`
	AvgLatencyMs    int       `gorm:"column:avg_latency_ms"`
	JitterMs        int       `gorm:"column:jitter_ms"`
	CheckedAt       time.Time `gorm:"column:checked_at"`
	StreamingResult string    `gorm:"column:streaming_result"`
	Protocol        string    `gorm:"column:protocol"`
	Region          string    `gorm:"column:region"`
	RegionSource    string    `gorm:"column:region_source"`
	Alive           bool      `gorm:"column:alive"`
}

type pathCapabilityCount struct {
	Passed  int `json:"passed"`
	Failed  int `json:"failed"`
	Unknown int `json:"unknown"`
}

type pathQualityItem struct {
	PathKey       string                         `json:"path_key"`
	Nodes         int                            `json:"nodes"`
	Alive         int                            `json:"alive"`
	AvgLatencyMs  *int                           `json:"avg_latency_ms"`
	P50LatencyMs  *int                           `json:"p50_latency_ms"`
	AvgJitterMs   *int                           `json:"avg_jitter_ms"`
	Protocols     []string                       `json:"protocols"`
	Regions       []string                       `json:"regions"`
	RegionSources []string                       `json:"region_sources"`
	Capabilities  map[string]pathCapabilityCount `json:"capabilities"`
	LastObserved  time.Time                      `json:"last_observed"`
	latencies     []int                          `json:"-"`
	latencyTotal  int64                          `json:"-"`
	jitterTotal   int64                          `json:"-"`
}

// PathQuality returns latest-per-node path observations from the selected window.
// It is read-only and deliberately does not produce routing decisions.
type PathQualityHandler struct{ db *gorm.DB }

func NewPathQualityHandler(db *gorm.DB) *PathQualityHandler { return &PathQualityHandler{db: db} }

func (h *PathQualityHandler) Get(c *gin.Context) {
	hours := 168
	if raw := c.Query("hours"); raw != "" {
		if value, err := strconv.Atoi(raw); err == nil && value >= 24 && value <= 720 {
			hours = value
		}
	}
	cutoff := time.Now().Add(-time.Duration(hours) * time.Hour)
	const query = `
WITH latest AS (
    SELECT DISTINCT ON (node_id) node_id, avg_latency_ms, jitter_ms, checked_at,
           streaming_result, metadata::jsonb->>'path_key' AS path_key
    FROM node_deep_checks
    WHERE checked_at >= ? AND metadata::jsonb ? 'path_key'
    ORDER BY node_id, checked_at DESC, id DESC
)
SELECT latest.path_key, latest.avg_latency_ms, latest.jitter_ms, latest.checked_at,
       latest.streaming_result, n.protocol, n.region, n.region_source, n.alive
FROM latest JOIN nodes n ON n.id = latest.node_id
WHERE n.archived_at IS NULL AND latest.path_key <> ''
ORDER BY latest.checked_at DESC
`
	var observations []pathQualityObservation
	if err := h.db.WithContext(c.Request.Context()).Raw(query, cutoff).Scan(&observations).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "path quality query failed"})
		return
	}
	groups := aggregatePathQuality(observations)
	c.JSON(http.StatusOK, gin.H{
		"window_hours":    hours,
		"total_paths":     len(groups),
		"total_nodes":     len(observations),
		"routing_changed": false,
		"paths":           groups,
	})
}

func aggregatePathQuality(observations []pathQualityObservation) []pathQualityItem {
	byKey := make(map[string]*pathQualityItem)
	for _, observation := range observations {
		item := byKey[observation.PathKey]
		if item == nil {
			item = &pathQualityItem{PathKey: observation.PathKey, Capabilities: make(map[string]pathCapabilityCount)}
			byKey[observation.PathKey] = item
		}
		item.Nodes++
		if observation.Alive {
			item.Alive++
		}
		if observation.AvgLatencyMs > 0 {
			item.latencies = append(item.latencies, observation.AvgLatencyMs)
			item.latencyTotal += int64(observation.AvgLatencyMs)
		}
		if observation.JitterMs >= 0 {
			item.jitterTotal += int64(observation.JitterMs)
		}
		if observation.CheckedAt.After(item.LastObserved) {
			item.LastObserved = observation.CheckedAt
		}
		item.Protocols = appendUnique(item.Protocols, observation.Protocol)
		item.Regions = appendUnique(item.Regions, observation.Region)
		item.RegionSources = appendUnique(item.RegionSources, observation.RegionSource)
		var streaming map[string]bool
		if json.Unmarshal([]byte(observation.StreamingResult), &streaming) == nil {
			for name, passed := range streaming {
				count := item.Capabilities[name]
				if passed {
					count.Passed++
				} else {
					count.Failed++
				}
				item.Capabilities[name] = count
			}
		}
	}
	out := make([]pathQualityItem, 0, len(byKey))
	for _, item := range byKey {
		if len(item.latencies) > 0 {
			sort.Ints(item.latencies)
			avg := int(item.latencyTotal / int64(len(item.latencies)))
			p50 := item.latencies[(len(item.latencies)-1)/2]
			item.AvgLatencyMs, item.P50LatencyMs = &avg, &p50
		}
		if item.Nodes > 0 {
			avg := int(item.jitterTotal / int64(item.Nodes))
			item.AvgJitterMs = &avg
		}
		for _, name := range pathCapabilityNames {
			count := item.Capabilities[name]
			tested := count.Passed + count.Failed
			count.Unknown = item.Nodes - tested
			item.Capabilities[name] = count
		}
		sort.Strings(item.Protocols)
		sort.Strings(item.Regions)
		sort.Strings(item.RegionSources)
		out = append(out, *item)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Nodes != out[j].Nodes {
			return out[i].Nodes > out[j].Nodes
		}
		if out[i].Alive != out[j].Alive {
			return out[i].Alive > out[j].Alive
		}
		return out[i].PathKey < out[j].PathKey
	})
	return out
}

func appendUnique(values []string, value string) []string {
	if value == "" {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

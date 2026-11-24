package handler

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

var pathCapabilityNames = []string{
	"access-chatgpt", "access-netflix", "access-youtube", "layer-egress", "layer-tls443", "quic",
}

type pathQualityObservation struct {
	ObservedRuns       int       `gorm:"column:observed_runs"`
	PathKey            string    `gorm:"column:path_key"`
	AvgLatencyMs       int       `gorm:"column:avg_latency_ms"`
	JitterMs           int       `gorm:"column:jitter_ms"`
	CheckedAt          time.Time `gorm:"column:checked_at"`
	StreamingResult    string    `gorm:"column:streaming_result"`
	Metadata           string    `gorm:"column:metadata"`
	Protocol           string    `gorm:"column:protocol"`
	Region             string    `gorm:"column:region"`
	RegionSource       string    `gorm:"column:region_source"`
	EgressIP           string    `gorm:"column:egress_ip"`
	EgressProvider     string    `gorm:"column:egress_provider"`
	EgressASN          int       `gorm:"column:egress_asn"`
	EgressASNOrg       string    `gorm:"column:egress_asn_org"`
	EgressObservedRuns int       `gorm:"column:egress_observed_runs"`
	FailReasonsJSON    string    `gorm:"column:fail_reasons_json"`
	Alive              bool      `gorm:"column:alive"`
}

type pathCapabilityCount struct {
	Passed  int `json:"passed"`
	Failed  int `json:"failed"`
	Unknown int `json:"unknown"`
}

type pathQualityItem struct {
	PathKey                string                         `json:"path_key"`
	Nodes                  int                            `json:"nodes"`
	Alive                  int                            `json:"alive"`
	AvgLatencyMs           *int                           `json:"avg_latency_ms"`
	P50LatencyMs           *int                           `json:"p50_latency_ms"`
	AvgJitterMs            *int                           `json:"avg_jitter_ms"`
	Protocols              []string                       `json:"protocols"`
	Regions                []string                       `json:"regions"`
	RegionSources          []string                       `json:"region_sources"`
	EgressIPs              []string                       `json:"egress_ips"`
	EgressProviders        []string                       `json:"egress_providers"`
	EgressASNs             []int                          `json:"egress_asns"`
	EgressASNOrgs          []string                       `json:"egress_asn_orgs"`
	EgressObservedRuns     int                            `json:"egress_observed_runs"`
	EgressObservationNodes int                            `json:"egress_observation_nodes"`
	Capabilities           map[string]pathCapabilityCount `json:"capabilities"`
	FailReasons            map[string]int                 `json:"fail_reasons"`
	ShadowClass            string                         `json:"shadow_class"`
	ShadowReasons          []string                       `json:"shadow_reasons"`
	ObservedRuns           int                            `json:"observed_runs"`
	LastObserved           time.Time                      `json:"last_observed"`
	latencies              []int                          `json:"-"`
	latencyTotal           int64                          `json:"-"`
	jitterTotal            int64                          `json:"-"`
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
WITH run_counts AS (
    SELECT metadata::jsonb->>'path_key' AS path_key, COUNT(DISTINCT check_run_id) AS observed_runs
    FROM node_deep_checks WHERE checked_at >= ? AND jsonb_exists(metadata::jsonb, 'path_key')
    GROUP BY metadata::jsonb->>'path_key'
), egress_counts AS (
    SELECT metadata::jsonb->>'path_key' AS path_key, COUNT(DISTINCT check_run_id) AS egress_observed_runs
    FROM node_deep_checks
    WHERE checked_at >= ?
      AND metadata::jsonb->>'region_source' = 'native_egress'
      AND jsonb_exists(metadata::jsonb, 'egress_ip')
      AND NULLIF(metadata::jsonb->>'egress_ip', '') IS NOT NULL
    GROUP BY metadata::jsonb->>'path_key'
), latest AS (
    SELECT DISTINCT ON (node_id) node_id, avg_latency_ms, jitter_ms, checked_at,
           streaming_result, metadata::jsonb AS metadata, metadata::jsonb->>'path_key' AS path_key
    FROM node_deep_checks
    WHERE checked_at >= ? AND jsonb_exists(metadata::jsonb, 'path_key')
    ORDER BY node_id, checked_at DESC, id DESC
), failure_counts AS (
    SELECT nic.node_id, jsonb_object_agg(nic.reason, nic.reason_count) AS fail_reasons_json
    FROM (
        SELECT node_id, COALESCE(NULLIF(fail_reason, ''), 'unknown') AS reason, COUNT(*) AS reason_count
        FROM node_initial_checks WHERE checked_at >= ? AND latency_ms = -1
        GROUP BY node_id, COALESCE(NULLIF(fail_reason, ''), 'unknown')
    ) nic GROUP BY nic.node_id
)
SELECT latest.path_key, COALESCE(run_counts.observed_runs, 0) AS observed_runs, COALESCE(egress_counts.egress_observed_runs, 0) AS egress_observed_runs, latest.avg_latency_ms, latest.jitter_ms, latest.checked_at,
       latest.streaming_result, COALESCE(failure_counts.fail_reasons_json, '{}'::jsonb)::text AS fail_reasons_json, n.protocol,
       COALESCE(NULLIF(latest.metadata->>'region', ''), '') AS region,
       COALESCE(NULLIF(latest.metadata->>'region_source', ''), 'unknown') AS region_source,
       COALESCE(latest.metadata->>'egress_ip', '') AS egress_ip,
       COALESCE(latest.metadata->>'egress_probe_provider', '') AS egress_provider,
       CASE WHEN latest.metadata->>'egress_asn' ~ '^[0-9]+$' THEN (latest.metadata->>'egress_asn')::integer ELSE 0 END AS egress_asn,
       COALESCE(latest.metadata->>'egress_asn_org', '') AS egress_asn_org, n.alive
FROM latest JOIN nodes n ON n.id = latest.node_id
LEFT JOIN run_counts ON run_counts.path_key = latest.path_key
LEFT JOIN egress_counts ON egress_counts.path_key = latest.path_key
LEFT JOIN failure_counts ON failure_counts.node_id = latest.node_id
WHERE n.archived_at IS NULL AND latest.path_key <> ''
ORDER BY latest.checked_at DESC
`
	var observations []pathQualityObservation
	if err := h.db.WithContext(c.Request.Context()).Raw(query, cutoff, cutoff, cutoff, cutoff).Scan(&observations).Error; err != nil {
		log.Error().Err(err).Msg("path quality query failed")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "path quality query failed"})
		return
	}
	groups := aggregatePathQuality(observations)
	shadowCounts := map[string]int{"production_candidate": 0, "standby_candidate": 0, "observation_only": 0}
	for _, group := range groups {
		shadowCounts[group.ShadowClass]++
	}
	c.JSON(http.StatusOK, gin.H{
		"identity_kind":                 "ingress_fingerprint",
		"window_hours":                  hours,
		"total_paths":                   len(groups),
		"total_nodes":                   len(observations),
		"shadow_counts":                 shadowCounts,
		"routing_changed":               false,
		"stable_for_scheduling":         stableForScheduling(groups),
		"stable_for_scheduling_reasons": schedulingReadinessReasons(groups),
		"paths":                         groups,
	})
}

func aggregatePathQuality(observations []pathQualityObservation) []pathQualityItem {
	byKey := make(map[string]*pathQualityItem)
	for _, observation := range observations {
		item := byKey[observation.PathKey]
		if item == nil {
			item = &pathQualityItem{PathKey: observation.PathKey, Capabilities: make(map[string]pathCapabilityCount), FailReasons: make(map[string]int)}
			byKey[observation.PathKey] = item
		}
		item.Nodes++
		if observation.ObservedRuns > item.ObservedRuns {
			item.ObservedRuns = observation.ObservedRuns
		}
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
		var failures map[string]int
		if json.Unmarshal([]byte(observation.FailReasonsJSON), &failures) == nil {
			for reason, count := range failures {
				item.FailReasons[reason] += count
			}
		}
		if observation.CheckedAt.After(item.LastObserved) {
			item.LastObserved = observation.CheckedAt
		}
		item.Protocols = appendUnique(item.Protocols, observation.Protocol)
		item.Regions = appendUnique(item.Regions, observation.Region)
		item.RegionSources = appendUnique(item.RegionSources, observation.RegionSource)
		if observation.RegionSource == "native_egress" && observation.EgressIP != "" {
			item.EgressObservationNodes++
			if observation.EgressObservedRuns > item.EgressObservedRuns {
				item.EgressObservedRuns = observation.EgressObservedRuns
			}
			item.EgressIPs = appendUnique(item.EgressIPs, observation.EgressIP)
			item.EgressProviders = appendUnique(item.EgressProviders, observation.EgressProvider)
			if observation.EgressASN > 0 {
				item.EgressASNs = appendUniqueInt(item.EgressASNs, observation.EgressASN)
			}
			item.EgressASNOrgs = appendUnique(item.EgressASNOrgs, observation.EgressASNOrg)
		}
		var streaming map[string]*bool
		if json.Unmarshal([]byte(observation.StreamingResult), &streaming) == nil {
			for name, passed := range streaming {
				if passed == nil {
					continue
				}
				count := item.Capabilities[name]
				if *passed {
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
		item.ShadowClass, item.ShadowReasons = classifyShadow(item)
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

func stableForScheduling(groups []pathQualityItem) bool {
	return len(schedulingReadinessReasons(groups)) == 0
}

func schedulingReadinessReasons(groups []pathQualityItem) []string {
	reasons := []string{}
	if len(groups) == 0 {
		return []string{"no_observations"}
	}
	production := 0
	for _, group := range groups {
		if group.ShadowClass != "production_candidate" {
			// Deliberately no production++ here. Counting a non-production group as
			// production made the "no_production_candidates" reason below
			// unreachable: an all-observation set reported only
			// "non_production_groups_present" and hid the more fundamental fact
			// that there was no usable path at all.
			reasons = appendUnique(reasons, "non_production_groups_present")
			continue
		}
		production++
		if group.EgressObservedRuns < 3 {
			reasons = appendUnique(reasons, "egress_runs_below_3")
		}
		if group.EgressObservationNodes < group.Nodes {
			reasons = appendUnique(reasons, "egress_node_coverage_incomplete")
		}
	}
	if production == 0 {
		reasons = appendUnique(reasons, "no_production_candidates")
	}
	return reasons
}

func hasTrustedRegionEvidence(sources []string) bool {
	if len(sources) == 0 {
		return false
	}
	for _, source := range sources {
		switch source {
		case "native_egress", "legacy_egress":
		default:
			return false
		}
	}
	return true
}

func classifyShadow(item *pathQualityItem) (string, []string) {
	reasons := make([]string, 0, 4)
	if item.Nodes == 0 {
		return "observation_only", []string{"no_nodes"}
	}
	aliveRatio := float64(item.Alive) / float64(item.Nodes)
	if aliveRatio < 0.5 {
		reasons = append(reasons, "alive_ratio_below_50pct")
	}
	if item.AvgLatencyMs == nil {
		reasons = append(reasons, "no_latency_sample")
	} else if *item.AvgLatencyMs > 1500 {
		reasons = append(reasons, "avg_latency_over_1500ms")
	}
	chat := item.Capabilities["access-chatgpt"]
	if chat.Failed > 0 {
		reasons = append(reasons, "chatgpt_failures_observed")
	}
	if chat.Unknown > 0 {
		reasons = append(reasons, "chatgpt_unmeasured")
	}
	if !hasTrustedRegionEvidence(item.RegionSources) {
		reasons = append(reasons, "region_evidence_untrusted")
	}
	if item.ObservedRuns < 3 {
		reasons = append(reasons, "fewer_than_3_checker_runs")
	}
	if len(reasons) == 0 && aliveRatio == 1 && item.AvgLatencyMs != nil && *item.AvgLatencyMs <= 1500 && chat.Failed == 0 && chat.Unknown == 0 {
		return "production_candidate", reasons
	}
	if aliveRatio >= 0.5 && item.AvgLatencyMs != nil && *item.AvgLatencyMs <= 3000 {
		return "standby_candidate", reasons
	}
	return "observation_only", reasons
}

func appendUniqueInt(values []int, value int) []int {
	if value == 0 {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
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

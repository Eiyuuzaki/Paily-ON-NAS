package scoring

import (
	"context"
	"fmt"

	"gorm.io/gorm"
)

// Confidence tiers describe how much independent DIRECT evidence supports a
// node's current grade. They say nothing about quality: a slow node measured
// every round is high confidence in a bad reading; a fast node measured once a
// week is low confidence in a good one.
//
// Thresholds are calibrated for this deployment's 2h check cadence (approved
// 2026-11-18). An earlier proposal of "3 rounds within 6h" was rejected on
// data: at 2h/round that needs a node to be picked every single round, which
// only 1.5% of the pool achieves, so the tier would have been unreachable.
//
// Counting is by DISTINCT check_run_id, never by HTTP sample: the five samples
// inside one deep check share a run, a moment and a network state, so they are
// one observation, not five.
const (
	ConfidenceHigh    = "high"
	ConfidenceMedium  = "medium"
	ConfidenceLow     = "low"
	ConfidenceUnknown = "unknown"

	// High needs two independent rounds inside the 6h window AND a reading from
	// the last 2h, so the tier means "recent and repeated", not just "repeated".
	confHighRounds     = 2
	confHighWindowH    = 6
	confHighFreshH     = 2
	confMediumWindowH  = 6
	confLowWindowH     = 24
)

// RecomputeConfidence refreshes nodes.confidence for the whole alive pool from
// direct check history. Only rows whose tier actually changes are written.
//
// Failures count as evidence: a node that was measured and failed still proves
// someone looked at it, so the window is counted from all DIRECT rows rather
// than only successful ones. (Deep-check rows with latency -1 are filtered out
// upstream by filter_dead_deep, so in practice this means "attempted" rounds.)
func RecomputeConfidence(ctx context.Context, db *gorm.DB) error {
	q := fmt.Sprintf(`
WITH rounds AS (
  SELECT node_id,
         count(DISTINCT check_run_id) FILTER (WHERE checked_at > now() - interval '%dhours') AS r6,
         count(DISTINCT check_run_id) FILTER (WHERE checked_at > now() - interval '%dhours') AS r24
  FROM node_deep_checks
  WHERE source = 'direct'
  GROUP BY node_id
), tiered AS (
  SELECT n.id,
    CASE
      WHEN n.last_direct_check_at IS NULL THEN '%s'
      WHEN coalesce(r.r6,0) >= %d AND n.last_direct_check_at > now() - interval '%dhours' THEN '%s'
      WHEN coalesce(r.r6,0) >= 1 THEN '%s'
      WHEN coalesce(r.r24,0) >= 1 THEN '%s'
      ELSE '%s'
    END AS tier
  FROM nodes n LEFT JOIN rounds r ON r.node_id = n.id
  WHERE n.archived_at IS NULL
)
UPDATE nodes n SET confidence = t.tier
FROM tiered t
WHERE n.id = t.id AND n.confidence IS DISTINCT FROM t.tier`,
		confHighWindowH, confLowWindowH,
		ConfidenceUnknown,
		confHighRounds, confHighFreshH, ConfidenceHigh,
		ConfidenceMedium,
		ConfidenceLow,
		ConfidenceUnknown,
	)
	return db.WithContext(ctx).Exec(q).Error
}
package checker

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	api "github.com/Eiyuuzaki/paily-on-nas/internal/api/generated"
	"github.com/Eiyuuzaki/paily-on-nas/internal/api/middleware"
	"github.com/Eiyuuzaki/paily-on-nas/internal/api/response"
	"github.com/Eiyuuzaki/paily-on-nas/internal/cfquota"
	"github.com/Eiyuuzaki/paily-on-nas/internal/checkqueue"
	"github.com/Eiyuuzaki/paily-on-nas/internal/db/model"
	"github.com/Eiyuuzaki/paily-on-nas/internal/entryip"
	nodePkg "github.com/Eiyuuzaki/paily-on-nas/internal/node"
	"github.com/Eiyuuzaki/paily-on-nas/internal/scoring"
	sourcePkg "github.com/Eiyuuzaki/paily-on-nas/internal/source"
	"github.com/Eiyuuzaki/paily-on-nas/internal/workerhealth"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

// Handler implements /api/v1/check/* endpoints.
type Handler struct {
	db        *gorm.DB
	sourceSvc *sourcePkg.Service
	nodeSvc   *nodePkg.Service
	scoring   ScoringPort
	cache     CacheRebuildPort
	tagMu     sync.Map // key: string (checker_tag) → *sync.Mutex
}

// NewHandler creates a checker Handler.
func NewHandler(
	db *gorm.DB,
	sourceSvc *sourcePkg.Service,
	nodeSvc *nodePkg.Service,
	scoring ScoringPort,
	cache CacheRebuildPort,
) *Handler {
	return &Handler{
		db:        db,
		sourceSvc: sourceSvc,
		nodeSvc:   nodeSvc,
		scoring:   scoring,
		cache:     cache,
	}
}

// tagLock returns (or lazily creates) the per-tag mutex for serialising
// concurrent submissions from the same checker tag.
func (h *Handler) tagLock(tag string) *sync.Mutex {
	v, _ := h.tagMu.LoadOrStore(tag, &sync.Mutex{})
	return v.(*sync.Mutex)
}

// readConfigInt reads a named key from the configs table.
// Falls back to defaultVal if the record is missing or unparseable.
// (Same pattern as source/dead.go and cleanup/cleanup.go.)
func readConfigInt(db *gorm.DB, key string, defaultVal int) int {
	var cfg model.Config
	if err := db.Where("key = ?", key).First(&cfg).Error; err != nil {
		return defaultVal
	}
	v, err := strconv.Atoi(cfg.Value)
	if err != nil {
		return defaultVal
	}
	return v
}

// GetNodes handles GET /api/v1/check/nodes.
// Returns a tiered probe set (NOT every node) for checker to pull:
//
//	tier 1 — alive nodes: probed every round (fast failover detection)
//	tier 2 — unproven nodes (fewer than check_dead_proof_rounds initial
//	         checks, alive or not): probed every round until they either
//	         turn alive or accumulate enough failed rounds. New nodes from
//	         the fetcher start at zero checks here, so discovery is not
//	         broken by the tiering.
//	tier 3 — dead nodes: rotation, not a fixed sweep. The interval since a
//	         node's last success stretches its probe cadence (<2h every
//	         round, 2-6h every second, 6-24h every fourth, beyond that and
//	         never-successful every twelfth). A node that stops working for
//	         reasons probing cannot fix therefore costs about one probe a
//	         day instead of one per round. Rotation still writes check
//	         history, so the 4-day source-dead window keeps working.
//
// Without tiering, a pool of ~1500 nodes with ~4% alive wastes 96% of each
// round's connections on dead nodes (measured 2026-10-24: 8m24s per round).
// Tunables live in the configs table; deleting the rows restores defaults.
func (h *Handler) GetNodes(c *gin.Context) {
	proofRounds := readConfigInt(h.db, "check_dead_proof_rounds", 4)
	// Probation window for dead nodes. "Probe every node with fewer than
	// proofRounds checks" was meant to give a new node a fair chance, but it
	// never expired: measured 2026-11-22, 220 of 1,374 dead nodes sat under
	// proofRounds forever and were re-probed every round, each costing a full
	// 6-second timeout — that is where half of the 18.8-minute initial phase
	// went. Bounding the rule by age keeps the intent (a node that just entered
	// the pool is checked until it has evidence) and drops the cost (a node that
	// failed months ago waits for its rotation turn instead).
	proofHours := readConfigInt(h.db, "check_dead_probation_hours", 24)
	if proofHours < 0 {
		proofHours = 0
	}
	// Dead-node probing is governed by rotation, not by a grace window: see the
	// dead_due CTE below. A node is re-probed on a cadence that stretches with
	// the time since its last success, so a fleet that has stopped working (a
	// quota-exhausted CF pool, say) costs one probe a day instead of one per
	// round until the archive window closes.
	// Representative probing (2026-11-15): alive nodes that share a server are
	// the same backend — one entry IP × one Worker — so their availability is
	// linked and probing all of them every round buys nothing (431 alive HKG
	// nodes = 6 entrances × 1 Worker). Probe 2 rotating representatives per
	// server per round instead; hashtext(id || slot) rotates the pick so every
	// node still gets probed every few rounds, and a server's death surfaces
	// within a couple of rounds. Servers with few alive nodes (unique origins:
	// paid airports, self-built singles) are probed in full — no redundancy to
	// exploit there. Measured on 2026-11-15 pool: alive 978 → 320 reps, script
	// checks phase 992 → ~200 (the 100-minute phase shrinks to ~25 min).
	repPerServer := readConfigInt(h.db, "check_rep_per_server", 2)
	repMinGroup := readConfigInt(h.db, "check_rep_min_group", 3)
	if repPerServer < 1 {
		repPerServer = 1
	}
	if repMinGroup < 0 {
		repMinGroup = 0
	}
	if proofRounds < 1 {
		proofRounds = 1
	}

	var nodes []model.Node
	// Representative probing with coverage-aware rotation (2026-11-17).
	//
	// The previous hashtext(id || 2h-bucket) rotation had two defects, both
	// measured tonight: a 44-node entry covered only 7 of its nodes in 8 hours
	// (16%), and a handful of nodes were probed in >=10 of the 12 daily rounds
	// while 93 were never probed at all. Rotation by hash is blind to how long a
	// node has actually gone without a measurement.
	//
	// New order inside each entry: nodes whose own last DIRECT measurement is
	// oldest go first (never-measured count as infinitely old), then round-robin
	// by round bucket so different representatives are picked each round. A
	// 10%-of-round random exploration slice is added on top for entries whose
	// full roster is not otherwise covered, so long-tail nodes still get
	// independent measurements instead of waiting forever.
	//
	// hashtext bucketing and window functions are PostgreSQL-specific; this
	// deployment targets PostgreSQL only.
	//
	// Placeholder order: repMinGroup, repPerServer, exploreSlots bind inside the
	// reps CTE; proofRounds, deadGraceMin, sweepSlots, sweepSlots bind in the
	// dead-side branch.
	exploreSlots := readConfigInt(h.db, "check_explore_slots", 6)
	if exploreSlots < 0 {
		exploreSlots = 0
	}
	// A round is either the scheduled rotation or the operator queue, never a mix.
	//
	// Adding queued ids to the rotation looked convenient and measured badly
	// (2026-11-22): a one-node enqueue produced a 484-node round, so the operator
	// still paid the full sweep for one answer. Queued work instead runs in its
	// own small round during the idle stretch after the scheduled one — the
	// rotation occupies the first part of each two-hour window and leaves the
	// rest empty.
	if c.Query("mode") == "urgent" {
		h.getUrgentNodes(c)
		return
	}
	if err := h.db.WithContext(c.Request.Context()).Raw(`
		WITH eg AS (
		    -- Egress identity groups nodes that share one Cloudflare Worker.
		    -- A CF entry pool publishes hundreds of entry IPs that all dial the
		    -- same Worker (measured 2026-11-21: 325 alive CF nodes, 321 distinct
		    -- servers, 2 distinct servernames). Partitioning those by the entry
		    -- IP treats 321 copies of one backend as independent evidence, so
		    -- every entry was probed every round. Grouping by servername+region
		    -- collapses them to a handful of representatives (325 -> 12) while
		    -- leaving every non-CF node on its own server key, unchanged.
		    -- Decoded via jsonb->>'name', never a substring test on raw: the entry
		    -- swap re-serialises the JSON with spaces (see egressIdentity).
		    SELECT id, server, region, last_direct_check_at, score,
		           CASE
		               -- Judge by "has a real SNI and the dial target is an IP", never by
		               -- the node's name prefix: one Worker is published under several
		               -- naming schemes (CF-US-58, HKG-013, ...), and keying on 'CF-%'
		               -- left 149 of 439 entries as their own egress key, so all of them
		               -- were probed every round. Measured 2026-11-22: all 439 carry
		               -- servername=lencf.dpdns.org across distinct entry IPs, i.e. one
		               -- Worker and one quota. Matches distribution.egressIdentity(),
		               -- which has always used the SNI+IP test.
		               WHEN coalesce(nullif(raw::jsonb->>'servername',''),
		                             nullif(raw::jsonb->>'sni',''), '') <> ''
		                    AND (server ~ '^[0-9]{1,3}(\.[0-9]{1,3}){3}$' OR server LIKE '%:%')
		               THEN coalesce(nullif(raw::jsonb->>'servername',''),
		                             nullif(raw::jsonb->>'sni',''),
		                             server) || '|' || coalesce(region,'')
		               ELSE server
		           END AS egress_key
		    FROM nodes
		    WHERE archived_at IS NULL AND alive = true
		), ranked AS (
		    SELECT id, egress_key,
		           row_number() OVER (PARTITION BY egress_key
		               ORDER BY last_direct_check_at NULLS FIRST,
		                        hashtext(id::text || floor(extract(epoch from now()) / 7200)::text)) AS rn,
		           count(*) OVER (PARTITION BY egress_key) AS total
		    FROM eg
		), reps AS (
		    SELECT id FROM ranked WHERE total <= ? OR rn <= ?
		    UNION
		    SELECT id FROM ranked WHERE rn <= ? * 5
		    UNION
		    SELECT id FROM (
		        SELECT id,
		            row_number() OVER (PARTITION BY region ORDER BY score DESC, hashtext(id::text || floor(extract(epoch from now()) / 7200)::text)) AS rgn,
		            count(*) OVER (PARTITION BY region) AS rtotal
		        FROM nodes
		        WHERE archived_at IS NULL AND alive = true AND coalesce(region, '') <> ''
		    ) region_ranked WHERE rtotal <= 3 OR rgn <= 1
		), dead_stats AS (
		    SELECT n.id,
		           n.cf_account_id,
		           n.created_at,
		           count(c.*) AS checks,
		           max(c.checked_at) FILTER (WHERE c.latency_ms >= 0) AS last_ok
		    FROM nodes n
		    LEFT JOIN node_initial_checks c ON c.node_id = n.id
		    WHERE n.archived_at IS NULL AND n.alive = false
		    GROUP BY n.id
		), dead_due AS (
		    -- Rotation: the longer a node has gone without a successful check, the
		    -- less often it is worth a probe slot. This replaces the previous
		    -- "retest every recently-failed node, capped at N" rule, which still
		    -- spent a full probe per dead node per round until the 7-day archive
		    -- window closed. A quota-exhausted CF fleet would burn ~84 rounds each
		    -- that way; at the 12-slot tier it costs one probe per day instead.
		    SELECT id FROM dead_stats
		    -- Probation only for entries that are actually new. Without the age
		    -- bound the clause below never expires and every long-dead node with
		    -- few samples is probed every round forever.
		    WHERE (checks < ? AND created_at > now() - make_interval(hours => ?))
		       -- An account whose quota just reset is retested in full, once.
		       -- Recovery must not depend on the rotation tier: a fleet that sat
		       -- out an outage has no recent success to schedule on, so waiting
		       -- for its turn would stretch recovery across many hours.
		       OR EXISTS (SELECT 1 FROM configs r WHERE r.key = 'cf_retest_ids'
		                  AND strpos(',' || r.value || ',',
		                             ',' || dead_stats.cf_account_id::text || ',') > 0)
		       OR mod(floor(extract(epoch from now()) / 7200)::int,
		              CASE
		                  WHEN last_ok IS NULL OR now() - last_ok > interval '24 hours' THEN 12
		                  WHEN now() - last_ok > interval '6 hours' THEN 4
		                  WHEN now() - last_ok > interval '2 hours' THEN 2
		                  ELSE 1
		              END) = 0
		)
		SELECT * FROM nodes n
		WHERE n.archived_at IS NULL
		  AND (n.cf_account_id IS NULL OR NOT EXISTS (
		      -- Skip accounts that have spent their Workers daily quota. Probing
		      -- them records failures for a node whose only problem is its owner's
		      -- traffic, and rotation keys on time since last success, so those
		      -- recorded failures would keep the fleet out for hours after the
		      -- quota reset. Leaving the node unprobed keeps its state untouched
		      -- and costs nothing while the account cannot serve anyway.
		      --
		      -- The set is read as a plain list, never by casting the usage JSON:
		      -- configs contains blank values and Postgres may evaluate a cast on
		      -- rows this query would not have selected, which turned a blank
		      -- config into a failed node query on 2026-11-21.
		      SELECT 1 FROM configs q
		      WHERE q.key = 'cf_exhausted_ids'
		        AND strpos(',' || q.value || ',', ',' || n.cf_account_id::text || ',') > 0
		  ))
		  AND (
		       -- One rule for dead nodes, in dead_due. A second "fewer than
		       -- proofRounds checks" test used to sit here and, being unbounded by
		       -- age, re-probed every long-dead node every round: measured
		       -- 2026-11-22, 220 nodes never escaped it and dominated the
		       -- initial phase. Two rules for one decision is how that happened.
		       (n.alive = false AND n.id IN (SELECT id FROM dead_due))
		       OR (n.alive = true AND n.id IN (SELECT id FROM reps))
		  )`, repMinGroup, repPerServer, exploreSlots, proofRounds, proofHours).Scan(&nodes).Error; err != nil {
		response.InternalError(c, "failed to query nodes")
		return
	}
	// The retest queue has now been consumed. Clearing it here keeps the promise
	// the key makes - exactly one full round - and means a node that is still
	// dead afterwards returns to the ordinary rotation instead of being retested
	// forever. Best effort: if this fails, the next round simply retests again.
	cfquota.ClearRetest(h.db)

	type nodeEntry struct {
		Hash     string                  `json:"hash"`
		Id       uuid.UUID               `json:"id"`
		Protocol string                  `json:"protocol"`
		Raw      *map[string]interface{} `json:"raw,omitempty"`
		Server   string                  `json:"server"`
	}

	entries := make([]nodeEntry, 0, len(nodes))
	for _, n := range nodes {
		e := nodeEntry{
			Hash:     n.Hash,
			Id:       n.ID,
			Protocol: n.Protocol,
			Server:   n.Server,
		}
		if n.Raw != "" {
			var rawMap map[string]interface{}
			if err := json.Unmarshal([]byte(n.Raw), &rawMap); err == nil {
				e.Raw = &rawMap
			}
		}
		entries = append(entries, e)
	}

	c.JSON(http.StatusOK, gin.H{"nodes": entries})
}

// getUrgentNodes serves the operator queue as a round of its own.
//
// Contract: it returns the queued nodes and nothing else, then clears the queue,
// so one enqueue is exactly one small round. It deliberately does not fall back
// to the rotation when the queue is empty — an empty queue must cost nothing,
// otherwise every poll would trigger a full sweep.
//
// The exhausted-account filter is kept. Probing a fleet whose Workers quota is
// spent records failures for nodes whose only fault is the owner's traffic, and
// those failures then hold the fleet out for hours after the reset. A queue
// entry pointing at such an account is dropped here rather than turned into
// misleading evidence; UrgentEnqueue reports the count so the operator is not
// left guessing why nothing happened.
func (h *Handler) getUrgentNodes(c *gin.Context) {
	ids := checkqueue.Pending(h.db)
	if len(ids) == 0 {
		c.JSON(http.StatusOK, gin.H{"nodes": []any{}})
		return
	}

	var nodes []model.Node
	if err := h.db.WithContext(c.Request.Context()).Raw(`
		SELECT * FROM nodes n
		WHERE n.archived_at IS NULL
		  AND n.id IN ?
		  AND (n.cf_account_id IS NULL OR NOT EXISTS (
		      SELECT 1 FROM configs q
		      WHERE q.key = 'cf_exhausted_ids'
		        AND strpos(',' || q.value || ',', ',' || n.cf_account_id::text || ',') > 0
		  ))`, ids).Scan(&nodes).Error; err != nil {
		response.InternalError(c, "failed to query urgent nodes")
		log.Error().Err(err).Msg("check: urgent node query failed")
		return
	}

	// Consume the queue only after a successful query. A failed read leaves it
	// intact so the next poll retries instead of losing the request.
	checkqueue.Clear(h.db)
	if len(nodes) < len(ids) {
		log.Warn().
			Int("queued", len(ids)).
			Int("usable", len(nodes)).
			Msg("check: urgent queue partially dropped (archived or quota-exhausted)")
	}

	entries := make([]gin.H, 0, len(nodes))
	for _, n := range nodes {
		e := gin.H{
			"hash":     n.Hash,
			"id":       n.ID,
			"protocol": n.Protocol,
			"server":   n.Server,
		}
		if n.Raw != "" {
			var rawMap map[string]interface{}
			if err := json.Unmarshal([]byte(n.Raw), &rawMap); err == nil {
				e["raw"] = rawMap
			}
		}
		entries = append(entries, e)
	}
	c.JSON(http.StatusOK, gin.H{"nodes": entries})
}

// UrgentEnqueue handles POST /api/v1/check/urgent.
//
// The operator's "check this now" switch: named nodes, or every active node of a
// named source, are put in front of the rotation for exactly one round. This is
// what makes adding a source a seconds-long operation instead of restarting the
// checker and re-probing the whole rotation.
//
// Nothing here probes anything. It only records intent; the checker picks it up
// on its next poll (see cmd/checker), which is also why this endpoint stays cheap
// and cannot be used to amplify load.
type urgentRequest struct {
	NodeIDs   []string `json:"node_ids"`
	SourceIDs []string `json:"source_ids"`
}

func (h *Handler) UrgentEnqueue(c *gin.Context) {
	var req urgentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	ids := make([]uuid.UUID, 0, len(req.NodeIDs)+8)
	var invalid int
	for _, raw := range req.NodeIDs {
		id, err := uuid.Parse(strings.TrimSpace(raw))
		if err != nil {
			invalid++
			continue
		}
		ids = append(ids, id)
	}

	// Source expansion is resolved here, not in the checker: the checker works in
	// node ids and has no source vocabulary.
	var expanded int
	if len(req.SourceIDs) > 0 {
		sourceIDs := make([]uuid.UUID, 0, len(req.SourceIDs))
		for _, raw := range req.SourceIDs {
			id, err := uuid.Parse(strings.TrimSpace(raw))
			if err != nil {
				invalid++
				continue
			}
			sourceIDs = append(sourceIDs, id)
		}
		if len(sourceIDs) > 0 {
			var nodeIDs []uuid.UUID
			if err := h.db.WithContext(c.Request.Context()).Raw(
				"SELECT ns.node_id FROM node_sources ns JOIN nodes n ON n.id = ns.node_id"+
					" WHERE ns.source_id IN ? AND n.archived_at IS NULL", sourceIDs).
				Scan(&nodeIDs).Error; err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "source lookup failed"})
				log.Error().Err(err).Msg("check: urgent enqueue source lookup failed")
				return
			}
			expanded = len(nodeIDs)
			ids = append(ids, nodeIDs...)
		}
	}

	if len(ids) == 0 {
		c.JSON(http.StatusOK, gin.H{"queued": 0, "pending": checkqueue.Count(h.db), "invalid": invalid})
		return
	}

	// Tell the operator up front what will be skipped, instead of queueing work
	// that the round is going to discard: a node whose CF account has spent its
	// quota is not probed at all (see getUrgentNodes).
	var exhausted int
	if err := h.db.WithContext(c.Request.Context()).Raw(`
		SELECT count(*) FROM nodes n
		WHERE n.id IN ?
		  AND n.cf_account_id IS NOT NULL
		  AND EXISTS (
		      SELECT 1 FROM configs q
		      WHERE q.key = 'cf_exhausted_ids'
		        AND strpos(',' || q.value || ',', ',' || n.cf_account_id::text || ',') > 0
		  )`, ids).Scan(&exhausted).Error; err != nil {
		exhausted = 0
	}

	added, total, err := checkqueue.Enqueue(h.db, ids)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "queue write failed"})
		log.Error().Err(err).Msg("check: urgent enqueue failed")
		return
	}
	// The queue is bounded: say so instead of silently dropping the tail.
	c.JSON(http.StatusOK, gin.H{
		"queued":              added,
		"pending":             total,
		"from_sources":        expanded,
		"skipped_exhausted":   exhausted,
		"invalid":             invalid,
		"capped_at_max_queue": total >= checkqueue.MaxQueue,
	})
}

// UrgentPending handles GET /api/v1/check/pending.
//
// The checker polls this instead of waiting for its cron tick, so an enqueue is
// acted on within a poll interval. It is deliberately a count: the checker has no
// business reading the ids, and a count cannot leak node identity to a caller
// holding only a checker secret.
func (h *Handler) UrgentPending(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"pending": checkqueue.Count(h.db)})
}

// PostResults handles POST /api/v1/check/results.
//
// Processing is fully asynchronous and uses a per-tag mutex to serialise
// concurrent submissions from the same checker. The four phases are:
//  1. Atomic DB transaction: insert initial checks, deep checks, checker_run.
//  2. Batch alive decision: single DISTINCT-ON query + two batch UPDATEs.
//  3. Async scoring: fire-and-forget goroutine per node that has deep data.
//  4. Dead-source evaluation + cache rebuild trigger.
func (h *Handler) PostResults(c *gin.Context) {
	var req api.CheckResultsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request body")
		return
	}
	if len(req.Results) == 0 {
		c.Status(http.StatusNoContent)
		return
	}

	// Read checker_tag from context BEFORE spawning goroutine (gin context
	// must not be accessed from other goroutines after the handler returns).
	checkerTag := middleware.CheckerTagFromCtx(c)

	go func(tag string, results []api.CheckResult) {
		mu := h.tagLock(tag)
		mu.Lock()
		defer mu.Unlock()

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()

		checkRunID := uuid.New()
		now := time.Now()
		start := now

		log.Info().Str("tag", tag).Str("run_id", checkRunID.String()).
			Int("count", len(results)).Msg("checker: processing started")

		// ── Phase 1: atomic transaction ───────────────────────────────────
		initialChecks, deepChecks, nodeIDs, regionUpdates, regionSources := h.buildCheckRecords(results, tag, checkRunID, now)

		err := h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := tx.CreateInBatches(initialChecks, 500).Error; err != nil {
				return fmt.Errorf("insert initial checks: %w", err)
			}
			if len(deepChecks) > 0 {
				if err := tx.CreateInBatches(deepChecks, 500).Error; err != nil {
					return fmt.Errorf("insert deep checks: %w", err)
				}
			}
			run := &model.CheckerRun{
				ID:          checkRunID,
				CheckerTag:  tag,
				NodeCount:   len(results),
				CompletedAt: now,
			}
			return tx.Create(run).Error
		})
		if err != nil {
			log.Error().Err(err).Str("tag", tag).Str("run_id", checkRunID.String()).
				Msg("checker: transaction failed")
			return
		}

		// Freshness describes what this node itself was observed doing, so only
		// DIRECT rows may advance it. A borrowed or inferred value must never make
		// an unmeasured node look freshly measured.
		if len(deepChecks) > 0 {
			if err := h.db.WithContext(ctx).Exec(`
UPDATE nodes n SET last_direct_check_at = d.last
FROM (SELECT node_id, max(checked_at) AS last
      FROM node_deep_checks
      WHERE check_run_id = ? AND source = 'direct'
      GROUP BY node_id) d
WHERE n.id = d.node_id
  AND (n.last_direct_check_at IS NULL OR n.last_direct_check_at < d.last)`, checkRunID).Error; err != nil {
				log.Warn().Err(err).Msg("checker: last_direct_check_at update failed")
			}
		}

		// Refresh the evidence tier for the whole pool. Cheap (one aggregate over
		// the direct rows) and order-independent, so running it per tag is fine.
		if err := scoring.RecomputeConfidence(ctx, h.db); err != nil {
			log.Warn().Err(err).Msg("checker: confidence recompute failed")
		}

		// Advance the Worker circuit breaker with this run's aggregates. Kept
		// outside the insert transaction: breaker bookkeeping must not roll back
		// measurement persistence, and its own failure must not fail the cycle.
		if err := workerhealth.EvaluateRun(ctx, h.db, checkRunID.String()); err != nil {
			log.Warn().Err(err).Msg("checker: worker health evaluation failed")
		}

		// Region enrichment: prefer the measured EGRESS IP (accurate exit
		// country); then fall back to the entry IP for nodes without egress
		// evidence. This self-heals missing regions every cycle.
		enrichRegionFromEgress(ctx, collectEgressIPs(results), regionUpdates, regionSources)
		enrichRegionGeoIP(ctx, h.db, nodeIDs, regionUpdates, regionSources)
		// A Cloudflare Worker runs at the edge that accepted the connection and its
		// outbound sockets leave from that same edge, so the exit location IS the
		// entry colo. GeoIP of the egress socket is useless here: Cloudflare's
		// 104.28.x pool is reported as "US" by every database regardless of which
		// colo it actually belongs to, which is what made labels contradict the
		// measured path. Measured entry colo wins for CF nodes.
		h.overrideCFRegionFromEntryColo(ctx, nodeIDs, regionUpdates, regionSources)

		// Apply region updates outside the transaction (best-effort).
		h.applyRegionUpdates(ctx, regionUpdates, regionSources)
		// A node that merely reaches chatgpt.com from an unsupported country cannot
		// actually be used for it: OpenAI serves the landing page and refuses the
		// account/session from there. Hong Kong is one of those regions, and it is
		// where most of the self-built pool exits.
		h.gateAICapabilities(ctx)

		// ── Phase 2: batch alive decision ─────────────────────────────────
		windowMinutes := h.readConfigInt(ctx, "node_alive_check_window_minutes", 240)
		deadIDs := h.batchAliveDecision(ctx, nodeIDs, tag, windowMinutes)

		// ── Phase 3: async scoring ────────────────────────────────────────
		nodesWithDeep := h.nodeIDsWithDeep(results)
		if len(nodesWithDeep) > 0 {
			go func(ids []uuid.UUID, t string) {
				sCtx, sCancel := context.WithTimeout(context.Background(), 10*time.Minute)
				defer sCancel()
				for _, id := range ids {
					if err := h.scoring.ComputeAndUpdateTagScore(sCtx, id, t); err != nil {
						log.Warn().Err(err).Str("node", id.String()).Str("tag", t).
							Msg("checker: scoring failed")
					}
				}
			}(nodesWithDeep, tag)
		}

		// ── Phase 4: dead-source evaluation + cache rebuild ───────────────
		h.handleDeadNodes(ctx, deadIDs)
		h.cache.TriggerRebuild(ctx)

		log.Info().Str("tag", tag).Str("run_id", checkRunID.String()).
			Dur("elapsed", time.Since(start)).Msg("checker: processing done")
	}(checkerTag, req.Results)

	c.Status(http.StatusNoContent)
}

// normalizeRegionSource prevents unknown wire values from becoming trusted provenance.
func normalizeRegionSource(source *string) string {
	if source == nil {
		return "checker_unspecified"
	}
	switch *source {
	case "native_egress", "legacy_egress", "inbound_mmdb":
		return *source
	default:
		return "checker_unspecified"
	}
}

// buildCheckRecords converts API results into DB model slices.
// Returns: initial checks, deep checks, all node IDs, and region updates.
func stringValue(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func (h *Handler) buildCheckRecords(
	results []api.CheckResult,
	tag string,
	checkRunID uuid.UUID,
	now time.Time,
) (
	initialChecks []model.NodeInitialCheck,
	deepChecks []model.NodeDeepCheck,
	nodeIDs []uuid.UUID,
	regionUpdates map[uuid.UUID]string,
	regionSources map[uuid.UUID]string,
) {
	regionUpdates = make(map[uuid.UUID]string)
	regionSources = make(map[uuid.UUID]string)
	seen := make(map[uuid.UUID]struct{}, len(results))

	for _, result := range results {
		nodeID := uuid.UUID(result.NodeId)

		initialChecks = append(initialChecks, model.NodeInitialCheck{
			NodeID:     nodeID,
			CheckRunID: checkRunID,
			CheckerTag: tag,
			LatencyMs:  result.Initial.LatencyMs,
			FailReason: stringValue(result.Initial.FailReason),
			CheckedAt:  now,
		})

		if _, ok := seen[nodeID]; !ok {
			seen[nodeID] = struct{}{}
			nodeIDs = append(nodeIDs, nodeID)
		}

		if result.Deep != nil {
			d := result.Deep
			streamJSON := "{}"
			if d.Streaming != nil {
				if b, err := json.Marshal(d.Streaming); err == nil {
					streamJSON = string(b)
				}
			}
			metaJSON := "{}"
			if d.Metadata != nil {
				if b, err := json.Marshal(d.Metadata); err == nil {
					metaJSON = string(b)
				}
			}
			speedKbps := 0
			if d.SpeedKbps != nil {
				speedKbps = *d.SpeedKbps
			}

			deepChecks = append(deepChecks, model.NodeDeepCheck{
				NodeID:          nodeID,
				CheckRunID:      checkRunID,
				CheckerTag:      tag,
				Source:          "direct", // written only for nodes the checker actually measured
				AvgLatencyMs:    d.AvgLatencyMs,
				JitterMs:        d.JitterMs,
				SpeedKbps:       speedKbps,
				StreamingResult: streamJSON,
				Metadata:        metaJSON,
				CheckedAt:       now,
			})

			if d.Region != nil && *d.Region != "" {
				regionUpdates[nodeID] = *d.Region
				regionSources[nodeID] = normalizeRegionSource(d.RegionSource)
			}
		}
	}
	return
}

type regionProbeRow struct {
	ID        uuid.UUID
	RawServer string
}

type pathRegion struct {
	region, source string
	conflict       bool
}

func groupRegionObservations(probes []regionProbeRow, regions, sources map[uuid.UUID]string) map[string]pathRegion {
	grouped := make(map[string]pathRegion, len(probes))
	for _, p := range probes {
		region := regions[p.ID]
		if p.RawServer == "" || region == "" {
			continue
		}
		source := sources[p.ID]
		if source == "" {
			source = "unknown"
		}
		previous, ok := grouped[p.RawServer]
		if !ok {
			grouped[p.RawServer] = pathRegion{region: region, source: source}
			continue
		}
		if previous.region != region || previous.source != source {
			previous.conflict = true
			grouped[p.RawServer] = previous
		}
	}
	return grouped
}

// applyRegionUpdates writes the measured exit region for probed nodes and
// propagates it to every alive sibling on the same server. All nodes on one
// server dial the same entry IP and the same Worker, so their exit behavior
// is identical — but under representative probing only ~2 nodes per server
// are measured per round, and without propagation the other siblings keep a
// stale region for days (2026-11-15: subscription showed US for nodes whose
// daytime exit was HK, because their last probe was a night round).
// overrideCFRegionFromEntryColo replaces the region of self-built Cloudflare
// nodes with the country of the edge colo their entry IP actually lands on.
func (h *Handler) overrideCFRegionFromEntryColo(ctx context.Context, ids []uuid.UUID, regionUpdates, regionSources map[uuid.UUID]string) {
	if len(ids) == 0 {
		return
	}
	type row struct {
		ID   uuid.UUID
		Name string
		Colo string
	}
	var rows []row
	if err := h.db.WithContext(ctx).Raw(`
SELECT n.id,
       coalesce(n.raw::jsonb->>'name','') AS name,
       coalesce(e.colo,'') AS colo
FROM nodes n
LEFT JOIN entry_ips e ON e.ip = n.server
WHERE n.id IN ?`, ids).Scan(&rows).Error; err != nil {
		return
	}
	for _, r := range rows {
		if !strings.HasPrefix(r.Name, "CF-") || r.Colo == "" {
			continue
		}
		region := entryip.RegionForColo(r.Colo)
		if region == "" {
			continue
		}
		regionUpdates[r.ID] = region
		regionSources[r.ID] = "entry_colo"
	}
}

// unsupportedAIRegions are countries OpenAI does not serve. Reaching chatgpt.com
// from one of them is not "access", so the capability flag is cleared.
var unsupportedAIRegions = []string{"CN", "HK", "MO", "RU", "IR", "KP", "CU", "SY"}

// gateAICapabilities clears access-chatgpt for nodes whose measured exit region
// is not served by OpenAI. Runs after region updates so it sees the final value.
func (h *Handler) gateAICapabilities(ctx context.Context) {
	// NOTE: GORM Exec does NOT expand a []string into an IN list the way Where
	// does; "= ANY(?)" reached Postgres as ANY('CN','HK') and failed with
	// "syntax error at or near ","" on every checker round (2026-11-21), so the
	// gate never ran and nodes in OpenAI-blocked regions kept a stale
	// access-chatgpt=true. Build the placeholder list explicitly instead.
	if len(unsupportedAIRegions) == 0 {
		return
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(unsupportedAIRegions)), ",")
	args := make([]any, 0, len(unsupportedAIRegions))
	for _, region := range unsupportedAIRegions {
		args = append(args, region)
	}
	query := `
UPDATE node_deep_checks d
SET streaming_result = (d.streaming_result::jsonb || '{"access-chatgpt": false}'::jsonb)::text
WHERE d.checked_at > now() - interval '3 days'
  AND d.streaming_result::jsonb->>'access-chatgpt' = 'true'
  AND d.node_id IN (SELECT id FROM nodes WHERE region IN (` + placeholders + `))`
	if err := h.db.WithContext(ctx).Exec(query, args...).Error; err != nil {
		log.Warn().Err(err).Msg("checker: ai capability gate failed")
	}
}

func (h *Handler) applyRegionUpdates(ctx context.Context, regionUpdates map[uuid.UUID]string, regionSources map[uuid.UUID]string) {
	if len(regionUpdates) == 0 {
		return
	}
	// Propagate: join each measured (node -> region) to all alive nodes whose
	// RAW dial target (raw.server) matches the measured node's raw server.
	// The server COLUMN is a rotation label that may diverge from the raw
	// dial target (2026-11-15: 1053 nodes with column != raw.server) — the
	// checker dials raw, so exit geo follows raw, and grouping by column
	// painted JP-named nodes with US verdicts from unrelated US-v6 nodes.
	ids := make([]uuid.UUID, 0, len(regionUpdates))
	for id := range regionUpdates {
		ids = append(ids, id)
	}
	var probes []regionProbeRow
	if err := h.db.WithContext(ctx).Model(&model.Node{}).
		Select("id, COALESCE(raw::jsonb->>'server', server) AS raw_server").
		Where("id IN ?", ids).Scan(&probes).Error; err != nil {
		log.Warn().Err(err).Int("nodes", len(ids)).Msg("checker: region probe lookup failed")
		return
	}
	byRawServer := groupRegionObservations(probes, regionUpdates, regionSources)
	for rawServer, value := range byRawServer {
		if value.conflict {
			log.Warn().Str("raw_server", rawServer).Msg("checker: conflicting region observations; skip sibling propagation")
			continue
		}
		if err := h.db.WithContext(ctx).Exec(`
			UPDATE nodes SET region = ?, region_source = ?, updated_at = NOW()
			WHERE archived_at IS NULL AND alive = true
			  AND COALESCE(raw::jsonb->>'server', server) = ?
			  AND (region IS DISTINCT FROM ? OR region_source IS DISTINCT FROM ?)`, value.region, value.source, rawServer, value.region, value.source).Error; err != nil {
			log.Warn().Err(err).Str("raw_server", rawServer).Msg("checker: region sibling update failed")
		}
	}
}

// batchAliveDecision decides nodes.alive from every check recorded inside the
// window, then batch-updates the two verdict sets.
//
// The verdict is the majority of the window, not the newest sample. Reading
// only the newest sample (the previous behaviour) promoted a single bad round
// into a permanent verdict: on 2026-11-21 a burst of dial_failed at 02:13
// killed 1044 nodes that had passed 98-99% of the seven preceding rounds, and
// they stayed dead for thirteen hours.
//
// Nodes with no check inside the window are left untouched. Silence is not
// evidence of death: a healthy node may simply not be due yet, and treating
// absence as failure would flip live nodes dead as soon as probing is thinned.
//
// Returns the node IDs this round judged dead.
func (h *Handler) batchAliveDecision(
	ctx context.Context,
	nodeIDs []uuid.UUID,
	tag string,
	windowMinutes int,
) []uuid.UUID {
	if len(nodeIDs) == 0 {
		return nil
	}

	type row struct {
		NodeID   uuid.UUID
		OkCount  int
		BadCount int
	}

	windowStart := time.Now().Add(-time.Duration(windowMinutes) * time.Minute)
	const batchSize = 10000

	// Aggregate every check inside the window across ALL tags: one checker
	// seeing the node work is enough to keep it in the pool.
	var rows []row
	for _, batch := range chunkUUIDs(nodeIDs, batchSize) {
		var batchRows []row
		if err := h.db.WithContext(ctx).Raw(`
			SELECT node_id,
			       count(*) FILTER (WHERE latency_ms >= 0) AS ok_count,
			       count(*) FILTER (WHERE latency_ms < 0)  AS bad_count
			FROM node_initial_checks
			WHERE node_id IN (?)
			  AND checked_at >= ?
			GROUP BY node_id
		`, batch, windowStart).Scan(&batchRows).Error; err != nil {
			log.Warn().Err(err).Int("batch_size", len(batch)).Msg("checker: batchAliveDecision query failed")
			return nil
		}
		rows = append(rows, batchRows...)
	}

	// Partition into alive / dead sets; a node absent from rows keeps its
	// current state because it was simply not probed this window.
	verdict := make(map[uuid.UUID]bool, len(rows))
	for _, r := range rows {
		verdict[r.NodeID] = r.OkCount > 0 && r.OkCount >= r.BadCount
	}

	var aliveIDs, deadIDs []uuid.UUID
	for _, id := range nodeIDs {
		alive, seen := verdict[id]
		if !seen {
			continue
		}
		if alive {
			aliveIDs = append(aliveIDs, id)
		} else {
			deadIDs = append(deadIDs, id)
		}
	}

	if len(aliveIDs) > 0 {
		for _, batch := range chunkUUIDs(aliveIDs, batchSize) {
			if err := h.db.WithContext(ctx).Model(&model.Node{}).
				Where("id IN (?)", batch).
				Update("alive", true).Error; err != nil {
				log.Warn().Err(err).Int("batch_size", len(batch)).Msg("checker: batch alive update failed")
			}
		}
	}
	if len(deadIDs) > 0 {
		for _, batch := range chunkUUIDs(deadIDs, batchSize) {
			if err := h.db.WithContext(ctx).Model(&model.Node{}).
				Where("id IN (?)", batch).
				Update("alive", false).Error; err != nil {
				log.Warn().Err(err).Int("batch_size", len(batch)).Msg("checker: batch dead update failed")
			}
		}
	}

	return deadIDs
}

// nodeIDsWithDeep returns the unique node IDs that have a deep check result.
func (h *Handler) nodeIDsWithDeep(results []api.CheckResult) []uuid.UUID {
	seen := make(map[uuid.UUID]struct{})
	var ids []uuid.UUID
	for _, r := range results {
		if r.Deep == nil {
			continue
		}
		id := uuid.UUID(r.NodeId)
		if _, ok := seen[id]; !ok {
			seen[id] = struct{}{}
			ids = append(ids, id)
		}
	}
	return ids
}

// handleDeadNodes runs dead-source logic for each dead node's source mappings.
func (h *Handler) handleDeadNodes(ctx context.Context, deadIDs []uuid.UUID) {
	if len(deadIDs) == 0 {
		return
	}
	const batchSize = 10000

	affectedSources := make(map[uuid.UUID]struct{})
	var mappings []model.NodeSource
	for _, batch := range chunkUUIDs(deadIDs, batchSize) {
		var batchMappings []model.NodeSource
		if err := h.db.WithContext(ctx).
			Where("node_id IN (?)", batch).
			Find(&batchMappings).Error; err != nil {
			log.Warn().Err(err).Int("batch_size", len(batch)).Msg("checker: failed to query source mappings for dead nodes")
			return
		}
		mappings = append(mappings, batchMappings...)
	}
	for _, m := range mappings {
		affectedSources[m.SourceID] = struct{}{}
	}

	for sourceID := range affectedSources {
		dead, err := sourcePkg.CheckNodeDead(ctx, h.db, sourceID)
		if err != nil {
			log.Warn().Err(err).Str("source_id", sourceID.String()).Msg("checker: CheckNodeDead error")
			continue
		}
		if dead {
			if err := h.sourceSvc.MarkDeadAndClean(ctx, sourceID); err != nil {
				log.Warn().Err(err).Str("source_id", sourceID.String()).Msg("checker: MarkDeadAndClean error")
			} else {
				log.Info().Str("source_id", sourceID.String()).Msg("checker: source marked dead")
			}
		}
	}
}

// readConfigInt reads an integer config value from the DB; returns def on error.
func (h *Handler) readConfigInt(ctx context.Context, key string, def int) int {
	var cfg model.Config
	if err := h.db.WithContext(ctx).Where("key = ?", key).First(&cfg).Error; err != nil {
		return def
	}
	v, err := strconv.Atoi(cfg.Value)
	if err != nil {
		return def
	}
	return v
}

func chunkUUIDs(ids []uuid.UUID, batchSize int) [][]uuid.UUID {
	if len(ids) == 0 {
		return nil
	}
	if batchSize <= 0 || len(ids) <= batchSize {
		return [][]uuid.UUID{ids}
	}
	batches := make([][]uuid.UUID, 0, (len(ids)+batchSize-1)/batchSize)
	for start := 0; start < len(ids); start += batchSize {
		end := start + batchSize
		if end > len(ids) {
			end = len(ids)
		}
		batches = append(batches, ids[start:end])
	}
	return batches
}

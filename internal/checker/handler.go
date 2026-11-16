package checker

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	api "github.com/Eiyuuzaki/paily-on-nas/internal/api/generated"
	"github.com/Eiyuuzaki/paily-on-nas/internal/api/middleware"
	"github.com/Eiyuuzaki/paily-on-nas/internal/api/response"
	"github.com/Eiyuuzaki/paily-on-nas/internal/db/model"
	nodePkg "github.com/Eiyuuzaki/paily-on-nas/internal/node"
	sourcePkg "github.com/Eiyuuzaki/paily-on-nas/internal/source"
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
//	tier 3 — proven-dead nodes: deterministic hash-bucket sweep; each node
//	         is probed when its bucket matches the current time slot, i.e.
//	         once per check_dead_sweep_slots rounds (= once a day at the
//	         default 2h cadence). Failed sweeps still write check history,
//	         so the 4-day source-dead window keeps working unchanged.
//
// Without tiering, a pool of ~1500 nodes with ~4% alive wastes 96% of each
// round's connections on dead nodes (measured 2026-10-24: 8m24s per round).
// Tunables live in the configs table; deleting the rows restores defaults.
func (h *Handler) GetNodes(c *gin.Context) {
	proofRounds := readConfigInt(h.db, "check_dead_proof_rounds", 4)
	sweepSlots := readConfigInt(h.db, "check_dead_sweep_slots", 12)
	// deadGraceMin keeps recently-dead nodes on the full-probe list for a
	// grace window (default 6h = ~3 checker rounds at the 2h cron). Without
	// this a single timeout verdict drops the node to the 1/12 sweep for the
	// rest of its life, and transient failures (CGNAT route flip, cold ECH
	// handshake, CDN hiccup) become permanent silent death. See AGENTS.md
	// 2026-11-11: HidenCloud app2301 was alive the whole time but invisible.
	deadGraceMin := readConfigInt(h.db, "check_dead_grace_minutes", 360)
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
	if sweepSlots < 1 {
		sweepSlots = 1
	}
	if deadGraceMin < 0 {
		deadGraceMin = 0
	}

	var nodes []model.Node
	// hashtext bucketing and window functions are PostgreSQL-specific; this
	// deployment targets PostgreSQL only (see README: static builds ship
	// without sqlite).
	//
	// Placeholder order matches the CTE-first text: repMinGroup, repPerServer
	// bind inside the reps CTE; proofRounds, deadGraceMin, sweepSlots, sweepSlots
	// bind in the dead-side branch.
	if err := h.db.WithContext(c.Request.Context()).Raw(`
		WITH ranked AS (
		    SELECT id, server,
		           row_number() OVER (PARTITION BY server
		               ORDER BY hashtext(id::text || floor(extract(epoch from now()) / 7200)::text)) AS rn,
		           count(*) OVER (PARTITION BY server) AS total
		    FROM nodes
		    WHERE archived_at IS NULL AND alive = true
		), reps AS (
		    SELECT id FROM ranked WHERE total <= ? OR rn <= ?
		    UNION
		    SELECT id FROM (
		        SELECT id,
		            row_number() OVER (PARTITION BY region ORDER BY score DESC, hashtext(id::text || floor(extract(epoch from now()) / 7200)::text)) AS rgn,
		            count(*) OVER (PARTITION BY region) AS rtotal
		        FROM nodes
		        WHERE archived_at IS NULL AND alive = true AND coalesce(region, '') <> ''
		    ) region_ranked WHERE rtotal <= 3 OR rgn <= 1
		)
		SELECT * FROM nodes n
		WHERE n.archived_at IS NULL
		  AND (
		       (n.alive = false AND (
		            (SELECT count(*) FROM node_initial_checks WHERE node_id = n.id) < ?
		            OR EXISTS (SELECT 1 FROM node_initial_checks
		               WHERE node_id = n.id AND latency_ms = -1
		               AND checked_at > now() - make_interval(mins => ?))
		            OR mod(abs(hashtext(n.id::text)), ?) = mod(floor(extract(epoch from now()) / 7200)::int, ?)
		       ))
		       OR (n.alive = true AND n.id IN (SELECT id FROM reps))
		  )`, repMinGroup, repPerServer, proofRounds, deadGraceMin, sweepSlots, sweepSlots).Scan(&nodes).Error; err != nil {
		response.InternalError(c, "failed to query nodes")
		return
	}

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

		// Region enrichment: prefer the measured EGRESS IP (accurate exit
		// country); then fall back to the entry IP for nodes without egress
		// evidence. This self-heals missing regions every cycle.
		enrichRegionFromEgress(ctx, collectEgressIPs(results), regionUpdates, regionSources)
		enrichRegionGeoIP(ctx, h.db, nodeIDs, regionUpdates, regionSources)

		// Apply region updates outside the transaction (best-effort).
		h.applyRegionUpdates(ctx, regionUpdates, regionSources)

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

// batchAliveDecision executes a single DISTINCT-ON query to find the latest
// initial check for each node within the alive window, then batch-updates
// nodes.alive in two UPDATE statements (alive / dead).
// Returns the list of node IDs that are now dead.
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
		NodeID    uuid.UUID
		LatencyMs int
	}

	windowStart := time.Now().Add(-time.Duration(windowMinutes) * time.Minute)
	const batchSize = 10000

	// Collect all recent checks per node across ALL tags within the window.
	// A node is alive if ANY checker reported it alive recently.
	var rows []row
	for _, batch := range chunkUUIDs(nodeIDs, batchSize) {
		var batchRows []row
		if err := h.db.WithContext(ctx).Raw(`
			SELECT DISTINCT ON (node_id, checker_tag) node_id, latency_ms
			FROM node_initial_checks
			WHERE node_id IN (?)
			  AND checked_at >= ?
			ORDER BY node_id, checker_tag, checked_at DESC
		`, batch, windowStart).Scan(&batchRows).Error; err != nil {
			log.Warn().Err(err).Int("batch_size", len(batch)).Msg("checker: batchAliveDecision query failed")
			return nil
		}
		rows = append(rows, batchRows...)
	}

	// Partition into alive / dead sets.
	aliveSet := make(map[uuid.UUID]struct{}, len(rows))
	for _, r := range rows {
		if r.LatencyMs != -1 {
			aliveSet[r.NodeID] = struct{}{}
		}
	}

	var aliveIDs, deadIDs []uuid.UUID
	for _, id := range nodeIDs {
		if _, ok := aliveSet[id]; ok {
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

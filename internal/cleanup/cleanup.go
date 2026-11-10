// Package cleanup runs a periodic goroutine that prunes expired history records
// from the database according to the configurable *_history_days settings.
package cleanup

import (
	"context"
	"strconv"
	"time"

	"github.com/Eiyuuzaki/paily-on-nas/internal/db/model"
	"github.com/rs/zerolog/log"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const deleteBatchSize = 5000

// Start launches a background goroutine that deletes expired history rows.
// It runs immediately once on startup, then on the configured interval.
// The goroutine exits when ctx is cancelled.
// RebuildTrigger invalidates the in-memory node cache after rows change so a
// distribution request cannot serve a node that cleanup just deleted.
type RebuildTrigger interface {
	TriggerRebuild(context.Context)
}

func Start(ctx context.Context, db *gorm.DB, rebuild RebuildTrigger) {
	go run(ctx, db, rebuild)
}

func run(ctx context.Context, db *gorm.DB, rebuild RebuildTrigger) {
	// Initial run.
	doCleanup(db, rebuild)

	for {
		intervalMinutes := readConfigInt(db, "cleanup_interval_minutes", 30)
		ticker := time.NewTicker(time.Duration(intervalMinutes) * time.Minute)
		select {
		case <-ctx.Done():
			ticker.Stop()
			return
		case <-ticker.C:
			ticker.Stop()
			doCleanup(db, rebuild)
		}
	}
}

func doCleanup(db *gorm.DB, rebuild RebuildTrigger) {
	ctx := context.Background()
	now := time.Now()

	// ── Source fetch history ──────────────────────────────────────────────
	fetchWindowDays := readConfigInt(db, "fetch_history_window_days", 7)
	fetchDeadDays := readConfigInt(db, "fetch_dead_window_days", 7)
	effectiveFetchDays := maxInt(fetchWindowDays, fetchDeadDays) + 1
	fetchCutoff := now.AddDate(0, 0, -effectiveFetchDays)

	if rows, err := deleteOldUintRows[model.SourceFetchHistory](ctx, db, "fetched_at", fetchCutoff); err != nil {
		log.Warn().Err(err).Msg("cleanup: delete fetch history failed")
	} else if rows > 0 {
		log.Info().Int64("rows", rows).Msg("cleanup: pruned source_fetch_histories")
	}

	// ── Checker runs (cascades to initial + deep checks) ──────────────────
	// Retention must cover the node-dead evaluation window so the dead-source
	// evaluator never sees incomplete history. Add one day as safety buffer.
	runHistoryDays := readConfigInt(db, "check_run_history_days", 7)
	nodeDeadDays := readConfigInt(db, "node_dead_window_days", 4)
	effectiveRunDays := maxInt(runHistoryDays, nodeDeadDays) + 1
	runCutoff := now.AddDate(0, 0, -effectiveRunDays)

	if rows, err := deleteOldUintRows[model.NodeInitialCheck](ctx, db, "checked_at", runCutoff); err != nil {
		log.Warn().Err(err).Msg("cleanup: delete node_initial_checks failed")
	} else if rows > 0 {
		log.Info().Int64("rows", rows).Msg("cleanup: pruned node_initial_checks")
	}

	if rows, err := deleteOldUintRows[model.NodeDeepCheck](ctx, db, "checked_at", runCutoff); err != nil {
		log.Warn().Err(err).Msg("cleanup: delete node_deep_checks failed")
	} else if rows > 0 {
		log.Info().Int64("rows", rows).Msg("cleanup: pruned node_deep_checks")
	}

	if rows, err := deleteOldCheckerRuns(ctx, db, runCutoff); err != nil {
		log.Warn().Err(err).Msg("cleanup: delete checker_runs failed")
	} else if rows > 0 {
		log.Info().Int64("rows", rows).Msg("cleanup: pruned checker_runs")
	}

	// ── Never-alive nodes ─────────────────────────────────────────────
	// The ingress gate (internal/node.gateIncoming) stops new junk from
	// entering, but the pool still holds whatever slipped in beforehand.
	// A node that has never once passed a check within the retention
	// window is dead weight: free-list entries either work when first
	// seen or they never do (observed: 95% of dead nodes never succeeded).
	neverAliveDays := readConfigInt(db, "node_never_alive_retention_days", 3)
	if neverAliveDays > 0 {
		archived, err := archiveNeverAliveNodes(ctx, db, now.AddDate(0, 0, -neverAliveDays))
		if err != nil {
			log.Warn().Err(err).Msg("cleanup: archive never-alive nodes failed")
		} else if archived > 0 {
			log.Info().Int64("rows", archived).Int("older_than_days", neverAliveDays).Msg("cleanup: archived never-alive nodes")
			if rebuild != nil {
				rebuild.TriggerRebuild(ctx)
			}
		}
	}

	// ── Time-of-day health aggregation ─────────────────────────────
	// Pre-aggregate per-hour alive rates (7-day window) so the
	// distribution hot path never aggregates node_initial_checks.
	if err := refreshTimeHealth(ctx, db); err != nil {
		log.Warn().Err(err).Msg("cleanup: refresh time health failed")
	}
}

// refreshTimeHealth recomputes node_time_health from the last 7 days of
// initial checks, bucketed by the UTC hour of checked_at. Full refresh:
// small table (nodes x 24) and correctness beats incrementality here.
func refreshTimeHealth(ctx context.Context, db *gorm.DB) error {
	var rows []struct {
		NodeID     uuid.UUID
		Hour       int
		AliveCount int64
		TotalCount int64
	}
	err := db.WithContext(ctx).Table("node_initial_checks").
		Select("node_id, EXTRACT(hour FROM checked_at)::int AS hour, " +
			"COUNT(*) FILTER (WHERE latency_ms > 0) AS alive_count, COUNT(*) AS total_count").
		Where("checked_at > ?", time.Now().AddDate(0, 0, -7)).
		Group("node_id, hour").
		Scan(&rows).Error
	if err != nil {
		return err
	}
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("DELETE FROM node_time_health").Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		now := time.Now()
		recs := make([]model.NodeTimeHealth, 0, len(rows))
		for _, r := range rows {
			recs = append(recs, model.NodeTimeHealth{NodeID: r.NodeID, Hour: r.Hour,
				AliveCount: int(r.AliveCount), TotalCount: int(r.TotalCount), UpdatedAt: now})
		}
		return tx.CreateInBatches(recs, 1000).Error
	})
}

func deleteOldUintRows[T any](ctx context.Context, db *gorm.DB, timeColumn string, cutoff time.Time) (int64, error) {
	var total int64
	for {
		var ids []uint
		if err := db.WithContext(ctx).
			Model(new(T)).
			Where(timeColumn+" < ?", cutoff).
			Order("id ASC").
			Limit(deleteBatchSize).
			Pluck("id", &ids).Error; err != nil {
			return total, err
		}
		if len(ids) == 0 {
			return total, nil
		}
		res := db.WithContext(ctx).Where("id IN ?", ids).Delete(new(T))
		if res.Error != nil {
			return total, res.Error
		}
		total += res.RowsAffected
		if len(ids) < deleteBatchSize {
			return total, nil
		}
	}
}

func deleteOldCheckerRuns(ctx context.Context, db *gorm.DB, cutoff time.Time) (int64, error) {
	var total int64
	for {
		var ids []string
		if err := db.WithContext(ctx).
			Model(&model.CheckerRun{}).
			Where("completed_at < ?", cutoff).
			Order("completed_at ASC").
			Limit(deleteBatchSize).
			Pluck("id", &ids).Error; err != nil {
			return total, err
		}
		if len(ids) == 0 {
			return total, nil
		}
		res := db.WithContext(ctx).Where("id IN ?", ids).Delete(&model.CheckerRun{})
		if res.Error != nil {
			return total, res.Error
		}
		total += res.RowsAffected
		if len(ids) < deleteBatchSize {
			return total, nil
		}
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// readConfigInt reads an integer config value; falls back to def on any error.
func readConfigInt(db *gorm.DB, key string, def int) int {
	var cfg model.Config
	if err := db.Where("key = ?", key).First(&cfg).Error; err != nil {
		return def
	}
	v, err := strconv.Atoi(cfg.Value)
	if err != nil {
		return def
	}
	return v
}

// archiveNeverAliveNodes retires nodes created before cutoff that have no
// successful initial check on record. Rows are kept rather than deleted so the
// unique hash index keeps blocking re-ingestion: free sources republish the same
// dead endpoints every round, and a hard delete would only recycle them.
func archiveNeverAliveNodes(ctx context.Context, db *gorm.DB, cutoff time.Time) (int64, error) {
	query := "UPDATE nodes SET archived_at = NOW() WHERE id IN (" +
		"SELECT id FROM nodes n WHERE n.alive = false AND n.archived_at IS NULL" +
		" AND n.created_at < ? AND NOT EXISTS (SELECT 1 FROM node_initial_checks c" +
		" WHERE c.node_id = n.id AND c.latency_ms <> -1) LIMIT ?)"
	var total int64
	for {
		res := db.WithContext(ctx).Exec(query, cutoff, deleteBatchSize)
		if res.Error != nil {
			return total, res.Error
		}
		total += res.RowsAffected
		if res.RowsAffected < deleteBatchSize {
			return total, nil
		}
	}
}

package workerhealth
import (
	"context"
	"fmt"
	"time"

	"github.com/Eiyuuzaki/paily-on-nas/internal/db/model"
	"gorm.io/gorm"
)
type State string

const (
	Healthy  State = "healthy"
	Degraded State = "degraded"
	Open     State = "open"
	HalfOpen State = "half_open"
)

// Calibration constants (2026-11-18 replay; see package comment).
const (
	DegradeRatio     = 1.5
	DegradeAbsMs     = 400
	RecoverRatio     = 1.2
	MinCfSamples     = 20
	MinAirSamples    = 5
	OpenAfterBad     = 2
	HealthyAfterGood = 2
)

type RoundKind string

const (
	RoundBad          RoundKind = "bad"
	RoundNormal       RoundKind = "normal"
	RoundGood         RoundKind = "good"
	RoundInconclusive RoundKind = "inconclusive"
)

func Classify(cfMed float64, cfN, airMed, airN int) RoundKind {
	if cfN < MinCfSamples || airN < MinAirSamples || airMed <= 0 {
		return RoundInconclusive
	}
	ratio := float64(cfMed) / float64(airMed)
	if ratio > DegradeRatio && cfMed > DegradeAbsMs {
		return RoundBad
	}
	if ratio <= RecoverRatio {
		return RoundGood
	}
	return RoundNormal
}

func Next(state State, kind RoundKind, consBad, consGood int) (State, int, int) {
	if kind == RoundInconclusive {
		return state, consBad, consGood
	}
	switch state {
	case Healthy:
		if kind == RoundBad {
			return Degraded, 1, 0
		}
		return Healthy, 0, 0
	case Degraded:
		if kind == RoundBad {
			consBad++
			if consBad >= OpenAfterBad {
				return Open, 0, 0
			}
			return Degraded, consBad, 0
		}
		return Healthy, 0, 0
	case Open:
		if kind == RoundGood {
			// Enter half_open; the good-round counter restarts from zero here so
			// promotion requires two good rounds counted while inside half_open,
			// matching the reviewed state machine.
			return HalfOpen, 0, 0
		}
		return Open, 0, 0
	case HalfOpen:
		if kind == RoundBad {
			return Open, 0, 0
		}
		if kind == RoundGood {
			consGood++
			if consGood >= HealthyAfterGood {
				return Healthy, 0, 0
			}
			return HalfOpen, 0, consGood
		}
		return HalfOpen, 0, 0
	}
	return state, consBad, consGood
}
type Aggregates struct {
	WorkerID  string
	CfN       int
	CfMedEff  int
	AirN      int
	AirMedEff int
	EntryN    int
}
func EvaluateRun(ctx context.Context, db *gorm.DB, runID string) error {
	type cfRow struct {
		SNI string
		N   int
		Ents int
		Med *float64
	}
	var cfs []cfRow
	q1 := "WITH dc AS (SELECT node_id, avg_latency_ms, jitter_ms FROM node_deep_checks WHERE check_run_id = ? AND source = 'direct'), nd AS (SELECT n.id, n.server, coalesce(nullif(n.raw::jsonb->>'servername',''),'') sni, (n.raw::jsonb->>'name') LIKE 'CF-%' is_cf FROM nodes n WHERE n.archived_at IS NULL) SELECT nd.sni, count(*), count(DISTINCT nd.server), percentile_disc(0.5) WITHIN GROUP (ORDER BY CASE WHEN dc.avg_latency_ms > 0 THEN dc.avg_latency_ms + 2*dc.jitter_ms END) FROM dc JOIN nd ON nd.id = dc.node_id WHERE nd.is_cf GROUP BY nd.sni"
	if err := db.WithContext(ctx).Raw(q1, runID).Scan(&cfs).Error; err != nil {
		return fmt.Errorf("workerhealth: cf aggregate: %w", err)
	}
	var air struct { N int; Med *float64 }
	q2 := "WITH dc AS (SELECT d.node_id, d.avg_latency_ms, d.jitter_ms FROM node_deep_checks d WHERE d.check_run_id = ? AND d.source = 'direct'), nd AS (SELECT n.id, coalesce(nullif(n.raw::jsonb->>'servername',''),'') sni, (n.raw::jsonb->>'name') LIKE 'CF-%' is_cf, bool_or(s.origin IN ('paid-airport','public-airport')) is_airport FROM node_deep_checks d JOIN nodes n ON n.id=d.node_id LEFT JOIN node_sources ns ON ns.node_id=n.id LEFT JOIN sources s ON s.id=ns.source_id WHERE n.archived_at IS NULL GROUP BY n.id) SELECT count(*), percentile_disc(0.5) WITHIN GROUP (ORDER BY CASE WHEN dc.avg_latency_ms > 0 THEN dc.avg_latency_ms + 2*dc.jitter_ms END) FROM dc JOIN nd ON nd.id = dc.node_id WHERE NOT nd.is_cf AND nd.is_airport"
	if err := db.WithContext(ctx).Raw(q2, runID).Scan(&air).Error; err != nil {
		return fmt.Errorf("workerhealth: airport aggregate: %w", err)
	}
	airMed, airN := 0, air.N
	if air.Med != nil { airMed = int(*air.Med) }
	now := time.Now().UTC()
	for _, row := range cfs {
		if row.SNI == "" { continue }
		cfMed := 0
		if row.Med != nil { cfMed = int(*row.Med) }
		kind := Classify(float64(cfMed), row.N, airMed, airN)
		var cur model.WorkerHealth
		found := true
		if err := db.WithContext(ctx).Where("worker_id = ?", row.SNI).First(&cur).Error; err != nil {
			found = false
			cur = model.WorkerHealth{WorkerID: row.SNI, State: string(Healthy)}
		}
		state := State(cur.State)
		next, _, _ := Next(state, kind, cur.ConsBad, cur.ConsGood)
		if found && next == state {
			db.WithContext(ctx).Model(&model.WorkerHealth{}).Where("worker_id = ?", row.SNI).Update("updated_at", now)
			continue
		}
		reason := fmt.Sprintf("cf_med=%dms air_med=%dms cf_n=%d air_n=%d kind=%s", cfMed, airMed, row.N, airN, kind)
		upSQL := "INSERT INTO worker_health (worker_id, state, baseline_eff, updated_at) VALUES (?, ?, 0, ?) ON CONFLICT (worker_id) DO UPDATE SET state = EXCLUDED.state, updated_at = EXCLUDED.updated_at"
		if err := db.WithContext(ctx).Exec(upSQL, row.SNI, string(next), now).Error; err != nil {
			return fmt.Errorf("workerhealth: upsert %s: %w", row.SNI, err)
		}
		db.WithContext(ctx).Create(&model.WorkerHealthHistory{WorkerID: row.SNI, FromState: string(state), ToState: string(next), Reason: reason, CfN: row.N, CfMedEff: cfMed, AirN: airN, CreatedAt: now})
	}
	return nil
}

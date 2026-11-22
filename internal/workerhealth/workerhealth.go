package workerhealth

import (
	"context"
	"errors"
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

// EvaluateRun aggregates one run's DIRECT deep checks into a per-Worker
// breaker verdict and advances worker_health.
//
// Two defects kept this function a no-op until 2026-11-22:
//
//  1. GORM's Scan maps by COLUMN NAME, never by position: a result column
//     with no matching struct field (or db name) is silently dropped into a
//     throwaway variable. count(*)/count(DISTINCT ...)/percentile_disc carried
//     no aliases, so every row arrived with N=0 and Med=nil, Classify() always
//     answered RoundInconclusive, and the breaker never ran.
//  2. Next() returns the advance counters, but they were discarded and the
//     upsert never wrote cons_bad/cons_good. half_open promotes only after two
//     consecutive good rounds, and open fires on the second consecutive bad
//     one, so pinning the counters at zero made the machine unable to leave
//     half_open — or to reach open even once the aliases were fixed.
func EvaluateRun(ctx context.Context, db *gorm.DB, runID string) error {
	type cfRow struct {
		SNI  string   `gorm:"column:sni"`
		N    int      `gorm:"column:n"`
		Ents int      `gorm:"column:ents"`
		Med  *float64 `gorm:"column:med"`
	}
	var cfs []cfRow
	// Scope: which nodes count as CF for the breaker. Deliberately left on the
	// CF- name prefix for now. The stricter-looking "servername non-empty and
	// server is an IP" test (distribution.egressIdentity) is the wrong test
	// here: it also matches airport nodes that point an IP at a decoy SNI
	// (AGENTS 6b), and a bogus Worker opening the breaker would drop every CF
	// node at distribution time. Narrowing this to the real Worker scope needs
	// an origin=self-built qualifier, which is a separate change.
	q1 := "WITH dc AS (SELECT node_id, avg_latency_ms, jitter_ms FROM node_deep_checks WHERE check_run_id = ? AND source = 'direct'), nd AS (SELECT n.id, n.server, coalesce(nullif(n.raw::jsonb->>'servername',''),'') sni, (n.raw::jsonb->>'name') LIKE 'CF-%' is_cf FROM nodes n WHERE n.archived_at IS NULL) SELECT nd.sni AS sni, count(*) AS n, count(DISTINCT nd.server) AS ents, percentile_disc(0.5) WITHIN GROUP (ORDER BY CASE WHEN dc.avg_latency_ms > 0 THEN dc.avg_latency_ms + 2*dc.jitter_ms END) AS med FROM dc JOIN nd ON nd.id = dc.node_id WHERE nd.is_cf GROUP BY nd.sni"
	if err := db.WithContext(ctx).Raw(q1, runID).Scan(&cfs).Error; err != nil {
		return fmt.Errorf("workerhealth: cf aggregate: %w", err)
	}
	var air struct {
		N   int      `gorm:"column:n"`
		Med *float64 `gorm:"column:med"`
	}
	q2 := "WITH dc AS (SELECT d.node_id, d.avg_latency_ms, d.jitter_ms FROM node_deep_checks d WHERE d.check_run_id = ? AND d.source = 'direct'), nd AS (SELECT n.id, coalesce(nullif(n.raw::jsonb->>'servername',''),'') sni, (n.raw::jsonb->>'name') LIKE 'CF-%' is_cf, bool_or(s.origin IN ('paid-airport','public-airport')) is_airport FROM node_deep_checks d JOIN nodes n ON n.id=d.node_id LEFT JOIN node_sources ns ON ns.node_id=n.id LEFT JOIN sources s ON s.id=ns.source_id WHERE n.archived_at IS NULL GROUP BY n.id) SELECT count(*) AS n, percentile_disc(0.5) WITHIN GROUP (ORDER BY CASE WHEN dc.avg_latency_ms > 0 THEN dc.avg_latency_ms + 2*dc.jitter_ms END) AS med FROM dc JOIN nd ON nd.id = dc.node_id WHERE NOT nd.is_cf AND nd.is_airport"
	if err := db.WithContext(ctx).Raw(q2, runID).Scan(&air).Error; err != nil {
		return fmt.Errorf("workerhealth: airport aggregate: %w", err)
	}
	airMed, airN := 0, air.N
	if air.Med != nil {
		airMed = int(*air.Med)
	}
	now := time.Now().UTC()
	for _, row := range cfs {
		if row.SNI == "" {
			continue
		}
		cfMed := 0
		if row.Med != nil {
			cfMed = int(*row.Med)
		}
		kind := Classify(float64(cfMed), row.N, airMed, airN)
		var cur model.WorkerHealth
		found := true
		if err := db.WithContext(ctx).Where("worker_id = ?", row.SNI).First(&cur).Error; err != nil {
			// Only a genuine miss may reset the row. Treating every error as
			// "absent" would turn one transient DB failure into a fresh
			// healthy/0/0 row and silently reopen a Worker that is mid-breaker.
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("workerhealth: load %s: %w", row.SNI, err)
			}
			found = false
			cur = model.WorkerHealth{WorkerID: row.SNI, State: string(Healthy)}
		}
		state := State(cur.State)
		next, nextBad, nextGood := Next(state, kind, cur.ConsBad, cur.ConsGood)
		// A counter that moved is itself a verdict: only skip the write when
		// nothing about the decision changed.
		if found && next == state && nextBad == cur.ConsBad && nextGood == cur.ConsGood {
			db.WithContext(ctx).Model(&model.WorkerHealth{}).Where("worker_id = ?", row.SNI).Update("updated_at", now)
			continue
		}
		cur.State = string(next)
		cur.ConsBad = nextBad
		cur.ConsGood = nextGood
		switch {
		case next == Open && state != Open:
			cur.OpenedAt = &now
		case next != Open:
			cur.OpenedAt = nil
		}
		cur.UpdatedAt = now
		if err := db.WithContext(ctx).Save(&cur).Error; err != nil {
			return fmt.Errorf("workerhealth: upsert %s: %w", row.SNI, err)
		}
		reason := fmt.Sprintf("cf_med=%dms air_med=%dms cf_n=%d air_n=%d kind=%s", cfMed, airMed, row.N, airN, kind)
		db.WithContext(ctx).Create(&model.WorkerHealthHistory{WorkerID: row.SNI, FromState: string(state), ToState: string(next), Reason: reason, CfN: row.N, CfMedEff: cfMed, AirN: airN, CreatedAt: now})
	}
	return nil
}

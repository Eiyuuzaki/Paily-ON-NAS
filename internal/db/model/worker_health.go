package model

import "time"

// WorkerHealth is the circuit-breaker state for one shared execution domain
// (currently: one Cloudflare Worker identity = one SNI such as lencf.dpdns.org).
//
// Why this exists: on 2026-11-17 the shared Worker degraded for several hours
// (median eff 500ms -> 6-15s across two rounds) and the distribution system could
// not see it — per-node scores all moved together, so relative ranking stayed
// neutral while every CF node became unusable. Infra health is a property of the
// Worker, not of any single node, so it gets its own table and its own breaker.
//
// Thresholds (approved 2026-11-18 after a 44-round historical replay):
//   degraded: CF median eff > 1.5 x same-round airport-control median AND > 400ms
//   open:     degraded condition met in 2 consecutive valid rounds
//   recovery: half_open after 1 valid round with CF med <= 1.2x airport,
//             healthy after 2 such consecutive rounds
type WorkerHealth struct {
	WorkerID    string     `gorm:"primaryKey;size:128"` // Worker SNI, e.g. lencf.dpdns.org
	State       string     `gorm:"size:16;not null;default:'healthy'"`
	ConsBad     int        `gorm:"not null;default:0"` // consecutive degraded-qualifying rounds
	ConsGood    int        `gorm:"not null;default:0"` // consecutive recovery-qualifying rounds (half_open)
	BaselineEff int        `gorm:"not null;default:0"` // last healthy median eff — monitoring only, not a trigger
	OpenedAt    *time.Time
	UpdatedAt   time.Time  `gorm:"not null"`
}

func (WorkerHealth) TableName() string { return "worker_health" }

// WorkerHealthHistory keeps one row per state transition with the evidence that
// produced it, so every breaker decision stays auditable (2026-11-18 GPT review).
type WorkerHealthHistory struct {
	ID         int64     `gorm:"primaryKey;autoIncrement"`
	WorkerID   string    `gorm:"size:128;index;not null"`
	FromState  string    `gorm:"size:16;not null"`
	ToState    string    `gorm:"size:16;not null"`
	Reason     string    `gorm:"size:255;not null"`
	CfN        int       `gorm:"not null;default:0"` // CF nodes measured this round
	CfMedEff   int       `gorm:"not null;default:0"`
	AirN       int       `gorm:"not null;default:0"` // airport control nodes measured
	AirMedEff  int       `gorm:"not null;default:0"`
	Ratio      float64   `gorm:"not null;default:0"`
	EntryCount int       `gorm:"not null;default:0"` // distinct entry IPs among CF samples
	CreatedAt  time.Time `gorm:"not null"`
}

func (WorkerHealthHistory) TableName() string { return "worker_health_history" }

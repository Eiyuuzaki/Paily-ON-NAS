package model

import (
	"time"

	"github.com/google/uuid"
)

// NodeTimeHealth is the pre-aggregated per-hour alive rate for one node over
// a rolling window. Rows are refreshed by the cleanup job; the distribution
// hot path only reads this table (never aggregates node_initial_checks).
type NodeTimeHealth struct {
	NodeID     uuid.UUID `gorm:"type:uuid;primaryKey" json:"node_id"`
	Hour       int       `gorm:"primaryKey" json:"hour"` // 0-23 UTC
	AliveCount int       `json:"alive_count"`
	TotalCount int       `json:"total_count"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// AliveRate returns the observed survival rate for this hour; 1.0 when no data.
func (h NodeTimeHealth) AliveRate() float64 {
	if h.TotalCount <= 0 {
		return 1.0
	}
	return float64(h.AliveCount) / float64(h.TotalCount)
}

// TableName pins the table name.
func (NodeTimeHealth) TableName() string { return "node_time_health" }

package model

import (
	"time"

	"github.com/google/uuid"
)

// CheckerRun records a single atomic submission batch from one checker.
type CheckerRun struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey"`
	CheckerTag  string    `gorm:"not null;index"`
	NodeCount   int       `gorm:"not null"`
	CompletedAt time.Time `gorm:"not null;index"`
}

// NodeTagScore stores the per-tag EWMA score for a node (UPSERT table).
type NodeTagScore struct {
	NodeID     uuid.UUID `gorm:"type:uuid;primaryKey"`
	CheckerTag string    `gorm:"primaryKey"`
	Score      float64   `gorm:"not null;default:0"`
	UpdatedAt  time.Time
}

// Node represents a single proxy node in the pool.
type Node struct {
	ID           uuid.UUID `gorm:"type:uuid;primaryKey"`
	Hash         string    `gorm:"uniqueIndex;not null"` // sha256(protocol|server:port|auth)
	Server       string    `gorm:"index;not null"`
	Port         int       `gorm:"not null;default:0"`
	Password     string    // primary auth field (uuid/password/psk)
	Protocol     string    `gorm:"not null"`
	Raw          string    `gorm:"not null"` // JSON compact of full clash proxy fields
	Region       string    // latest region from checker
	RegionSource string    `gorm:"not null;default:unknown"` // native_egress, legacy_egress, inbound_mmdb, checker_unspecified, unknown
	Alive        bool      `gorm:"index;not null;default:false"`
	Score        float64   `gorm:"not null;default:0"` // float64 [0,1]; ordering only
	// EffLatencyMs is the experience-equivalent latency (mean + jitter penalty +
	// failure cost). Grade is the band cut from it, in milliseconds.
	EffLatencyMs int    `gorm:"not null;default:0"`
	Grade        string `gorm:"size:2;not null;default:''"`
	// Confidence describes how much independent DIRECT evidence backs the
	// current grade: high/medium/low/unknown. It is a property of the evidence,
	// not of quality — a slow node measured every round is high confidence in a
	// bad reading, and a fast node measured once a week is low confidence in a
	// good one. Deliberately separate from Score so staleness is never punished
	// twice.
	Confidence string `gorm:"size:8;not null;default:'unknown';index"`
	// CFAccountID links this node to the Cloudflare account whose Worker serves
	// it, when one can be identified from the servername. Nil means "unknown
	// owner" and the node is never filtered by account state: quota is an
	// account-level fact, and guessing an owner is worse than not filtering.
	CFAccountID *uuid.UUID `gorm:"type:uuid;index"`
	// TrustTier tracks infrastructure authorization: public = published for
	// public use (free lists, own builds); T0 = unauthorized third-party relay
	// (observed, NOT for distribution); T2 = authorized shadow; T3 = owned.
	TrustTier string `gorm:"size:8;not null;default:'public';index"`
	// LastDirectCheckAt is the timestamp of this node's own most recent DIRECT
	// measurement. Inferred or borrowed data must never advance it: freshness has
	// to describe what was actually observed on this node.
	LastDirectCheckAt *time.Time `gorm:"index"`
	CreatedAt         time.Time
	UpdatedAt         time.Time
	// ArchivedAt marks a node retired from probing. The row is kept so the
	// unique hash index keeps blocking re-ingestion: sources republish the
	// same dead endpoints every round, and a hard delete would re-admit them
	// on the very next fetch.
	ArchivedAt *time.Time `gorm:"index"`
}

// NodeSource is the many-to-many join table between nodes and sources.
type NodeSource struct {
	NodeID   uuid.UUID `gorm:"type:uuid;primaryKey"`
	SourceID uuid.UUID `gorm:"type:uuid;primaryKey"`
}

// NodeInitialCheck records each initial latency check for a node.
type NodeInitialCheck struct {
	ID         uint      `gorm:"primaryKey;autoIncrement"`
	NodeID     uuid.UUID `gorm:"type:uuid;not null;index"`
	CheckRunID uuid.UUID `gorm:"type:uuid;not null;default:'00000000-0000-0000-0000-000000000000';index"`
	CheckerTag string    `gorm:"not null;default:'default';index"`
	LatencyMs  int       `gorm:"not null"`            // -1 = dead
	FailReason string    `gorm:"not null;default:''"` // dead reason: timeout/conn_refused/dns/tls_handshake/...
	CheckedAt  time.Time `gorm:"not null"`
}

// NodeDeepCheck records each comprehensive check result for a node.
type NodeDeepCheck struct {
	ID              uint      `gorm:"primaryKey;autoIncrement"`
	NodeID          uuid.UUID `gorm:"type:uuid;not null;index"`
	CheckRunID      uuid.UUID `gorm:"type:uuid;not null;default:'00000000-0000-0000-0000-000000000000';index"`
	CheckerTag      string    `gorm:"not null;default:'default';index"`
	AvgLatencyMs    int       `gorm:"not null"`
	JitterMs        int       `gorm:"not null"`
	SpeedKbps       int       `gorm:"not null;default:0"`    // 0 = not measured
	StreamingResult string    `gorm:"not null;default:'{}'"` // JSON {"netflix":true,...}
	Metadata        string    `gorm:"not null;default:'{}'"` // JSON extra fields
	// Source records how this row came to exist. DIRECT = the checker measured
	// this node in this run. Future INFERRED rows would be derived from a sibling
	// on the same entry, and must never feed score/grade or refresh freshness.
	Source    string    `gorm:"size:16;not null;default:'direct'"`
	CheckedAt time.Time `gorm:"not null"`
}

package model

import "time"

// EntryIP is one Cloudflare anycast entry address, measured from this line.
//
// A node's server column is an entry IP, and many nodes share one. CF routing
// decides per-BGP which colo an entry lands on, so the colo changes over time:
// of 12 mapped entries re-checked on 2026-11-17, four that used to land HKG were
// landing LAX. Labels therefore have to come from measurement, not from a name
// or a static table.
type EntryIP struct {
	IP string `gorm:"primaryKey;size:64"`
	// Colo is the measured Cloudflare edge (HKG / LAX / LHR ...), empty if unknown.
	Colo string `gorm:"size:8;not null;default:''"`
	// RTTMs is the median latency measured through this entry by the checker
	// (tunnel-inclusive, i.e. what a client actually feels), 0 if not measured.
	RTTMs int `gorm:"not null;default:0"`
	// ScanRTTMs is the bare TCP handshake RTT seen by the discovery scan. Kept
	// separate because it is measured on a different path and must not be
	// compared with RTTMs.
	ScanRTTMs int `gorm:"not null;default:0"`
	// SuccessRate is the share of the latest node samples that answered (0..1).
	SuccessRate float64 `gorm:"not null;default:0"`
	// Samples is how many nodes contributed to the passive aggregate.
	Samples int `gorm:"not null;default:0"`
	// Source records where the row came from: "passive" (node traffic) or "scan".
	Source     string    `gorm:"size:16;not null;default:'passive'"`
	MeasuredAt time.Time `gorm:"not null"`
}

func (EntryIP) TableName() string { return "entry_ips" }

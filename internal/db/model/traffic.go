package model

import (
	"time"

	"github.com/google/uuid"
)

// SourceTrafficStat records aggregated traffic for a source,
// collected from the mihomo external-controller API.
//
// Traffic is attributed to a source by matching node names in the
// subscription output against nodes in the nodes table, then
// aggregating by source_id via the node_sources join table.
type SourceTrafficStat struct {
	ID         uint      `gorm:"primaryKey;autoIncrement"`
	SourceID   uuid.UUID `gorm:"type:uuid;not null;index"`
	Upload     int64     `gorm:"not null;default:0"` // 累计上传字节
	Download   int64     `gorm:"not null;default:0"` // 累计下载字节
	RecordedAt time.Time `gorm:"not null"`
}

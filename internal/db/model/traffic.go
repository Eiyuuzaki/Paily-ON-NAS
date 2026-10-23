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
//
// 这张表只存「最新一次快照」，每轮轮询会 Delete + Create 覆写。
// 它的 Upload/Download 是 mihomo 的累计计数器值，含义是「自 mihomo 启动
// 以来该来源用了多少」，会随 mihomo 重启归零。
//
// 需要按时间范围统计时请查 SourceTrafficDaily。
type SourceTrafficStat struct {
	ID         uint      `gorm:"primaryKey;autoIncrement"`
	SourceID   uuid.UUID `gorm:"type:uuid;not null;index"`
	Upload     int64     `gorm:"not null;default:0"` // 累计上传字节
	Download   int64     `gorm:"not null;default:0"` // 累计下载字节
	RecordedAt time.Time `gorm:"not null"`
}

// SourceTrafficDaily stores per-source traffic deltas grouped by local date.
//
// 每轮轮询计算「本轮计数器值 - 上轮计数器值」得到增量，累加到当天的行上。
// 这样即使 mihomo 重启导致计数器归零，历史数据也不会丢。
//
// 与 SourceTrafficStat 的区别：
//   - SourceTrafficStat：一张最新快照，会被覆写，等于「自 mihomo 启动以来」
//   - SourceTrafficDaily：按天累加的历史，可以回答「今天/本周/本月/累计」
type SourceTrafficDaily struct {
	ID       uint      `gorm:"primaryKey;autoIncrement"`
	SourceID uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_std_source_date"`
	// Day 存本地时区的日期（YYYY-MM-DD），按天聚合用。
	Day       time.Time `gorm:"type:date;not null;uniqueIndex:idx_std_source_date"`
	Upload    int64     `gorm:"not null;default:0"`
	Download  int64     `gorm:"not null;default:0"`
	UpdatedAt time.Time `gorm:"not null"`
}

// TableName 固定表名。
// 不指定的话 GORM 会把 Daily 复数化成语义通顺但难记的
// source_traffic_dailies，和手写 SQL 里的 source_traffic_daily 对不上。
func (SourceTrafficDaily) TableName() string {
	return "source_traffic_daily"
}

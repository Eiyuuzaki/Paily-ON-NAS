package model

import (
	"time"

	"github.com/google/uuid"
)

// Source represents a proxy node source (subscribe URL or raw node text).
type Source struct {
	ID         uuid.UUID `gorm:"type:uuid;primaryKey"`
	Type       string    `gorm:"not null"` // "subscribe" | "node"
	Identifier string    // e.g. "tg:channel_abc"
	Info       string    // free-form description
	Content    string    `gorm:"not null"`                // URL for subscribe, raw text for node
	Status     string    `gorm:"not null;default:active"` // "active" | "dead"
	IgnoreDead bool      `gorm:"not null;default:false"`

	// Origin classifies where the nodes come from. Values:
	//   self-built      自己的机器/账号（最高优先）
	//   paid-airport    付费机场
	//   public-airport  公益机场
	//   free-list       免费一手维护者（根）
	//   aggregator      二次聚合他人源（枝，可能纯重复）
	//   wild            打野临时源（随时死）
	// Distribution prefixes map 1:1: [自建]/[付费]/[公益]/[一手]/[聚合]/[打野]。
	Origin string `gorm:"default:free-list"`

	// 优先级：0=公益, 1=机场/付费, 2=最高
	Priority int `gorm:"not null;default:0"`

	// 额度信息（来自订阅 HTTP 响应头的 subscription-userinfo）
	Upload   int64 `gorm:"not null;default:0"` // 累计上传字节
	Download int64 `gorm:"not null;default:0"` // 累计下载字节
	Total    int64 `gorm:"not null;default:0"` // 总量字节
	Expire   int64 `gorm:"not null;default:0"` // 到期 unix timestamp, 0=无

	CreatedAt time.Time
	UpdatedAt time.Time
}

// SourceFetchHistory records each fetch attempt for a subscribe-type source.
type SourceFetchHistory struct {
	ID        uint      `gorm:"primaryKey;autoIncrement"`
	SourceID  uuid.UUID `gorm:"type:uuid;not null;index"`
	Success   bool      `gorm:"not null"`
	NodeCount int       `gorm:"not null;default:0"`
	FetchedAt time.Time `gorm:"not null"`
}

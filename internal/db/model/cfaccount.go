package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// CFAccount is one Cloudflare account managed by the CF module.
// Activation is done by pasting a GitHub repo URL that hosts the worker
// source plus optionally an API token file reference; credentials never
// leave the NAS. The module is intentionally minimal: read-mostly.
type CFAccount struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	Name        string    `gorm:"not null" json:"name"`            // C, D, ...
	Role        string    `gorm:"not null;default:backup" json:"role"` // main | backup | asset
	WorkerURL   string    `gorm:"" json:"worker_url"`              // full worker URL
	WorkerName  string    `gorm:"" json:"worker_name"`
	Subdomain   string    `gorm:"" json:"subdomain"`               // workers.dev subdomain
	UUID        string    `gorm:"" json:"uuid"`                    // vless uuid
	RepoURL     string    `gorm:"" json:"repo_url"`                // GitHub repo (activation)
	Enabled     bool      `gorm:"not null;default:true" json:"enabled"`
	Health      string    `gorm:"not null;default:unknown" json:"health"` // ok | degraded | down | unknown
	LastCheckAt *time.Time `gorm:"" json:"last_check_at"`
	Note        string    `gorm:"" json:"note"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (CFAccount) TableName() string { return "cf_accounts" }

// CFModuleConfig is stored in configs as JSON under cf_module.
type CFModuleConfig struct {
	Activated bool   `json:"activated"`
	RepoURL   string `json:"repo_url"`
	ActivatedAt string `json:"activated_at,omitempty"`
}

// BeforeCreate sets the UUID primary key.
func (a *CFAccount) BeforeCreate(tx *gorm.DB) error {
	if a.ID == uuid.Nil {
		a.ID = uuid.New()
	}
	return nil
}
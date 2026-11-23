// Package cfhealth answers one question for the CF module: is an account's
// Worker currently backed by a living node pool?
//
// The verdict is derived from the node pool, never from probing the Worker URL
// directly: after the SNI blacklist era a direct dial to workers.dev/dpdns.org
// is RST even when the client path works, so a probe would report every healthy
// account as dead.
//
// It lives in its own package because two callers need the same rule: the panel
// endpoint (operator-initiated recompute) and the checker cycle (automatic
// refresh once the alive flags have settled). Two copies of the rule would drift
// apart, and a drifted health flag sends the operator hunting the wrong fault.
package cfhealth

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Eiyuuzaki/paily-on-nas/internal/db/model"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

// Account health values stored in cf_accounts.health.
const (
	OK       = "ok"       // every node in the pool is alive
	Degraded = "degraded" // some nodes alive
	Down     = "down"     // pool exists, nothing alive
	Unknown  = "unknown"  // no nodes in the pool: no evidence either way
)

// HostOf pulls the host part of a worker URL for node matching.
func HostOf(workerURL string) string {
	s := strings.TrimPrefix(workerURL, "https://")
	s = strings.TrimPrefix(s, "http://")
	if i := strings.IndexAny(s, "/:"); i >= 0 {
		s = s[:i]
	}
	return s
}

// Compute derives health from the node pool of one account.
//
// Matching mirrors the panel summary (server OR raw): a node built from a domain
// entry stores the host in "server", while an IP entry only carries it inside
// the raw JSON, so matching raw alone silently undercounts.
//
// An account with no nodes at all reports Unknown, not Down: "this account is
// not deployed" and "this account is broken" are different facts, and the panel
// should not present an unused backup account as a failure.
func Compute(db *gorm.DB, workerURL string) (health string, alive, total int64, err error) {
	host := HostOf(workerURL)
	if host == "" {
		return "", 0, 0, fmt.Errorf("cfhealth: worker_url has no host")
	}
	pattern := "%" + host + "%"

	if err = db.Model(&model.Node{}).
		Where("archived_at IS NULL AND (server LIKE ? OR raw LIKE ?)", pattern, pattern).
		Count(&total).Error; err != nil {
		return "", 0, 0, err
	}
	if total == 0 {
		return Unknown, 0, 0, nil
	}
	if err = db.Model(&model.Node{}).
		Where("archived_at IS NULL AND alive = true AND (server LIKE ? OR raw LIKE ?)", pattern, pattern).
		Count(&alive).Error; err != nil {
		return "", 0, 0, err
	}

	switch {
	case alive == 0:
		return Down, alive, total, nil
	case alive < total:
		return Degraded, alive, total, nil
	default:
		return OK, alive, total, nil
	}
}

// RefreshAll recomputes health for every account in one pass.
//
// Called once per checker cycle, after the alive flags for that run have been
// written, so the panel stops depending on an operator pressing the check
// button. It issues no network request: the cost is two COUNTs over the node
// table per account, which is why it can ride along on every round.
func RefreshAll(ctx context.Context, db *gorm.DB) (int, error) {
	var accounts []model.CFAccount
	if err := db.WithContext(ctx).Find(&accounts).Error; err != nil {
		return 0, err
	}
	now := time.Now()
	updated := 0
	for _, a := range accounts {
		health, _, _, err := Compute(db.WithContext(ctx), a.WorkerURL)
		if err != nil {
			// An empty worker_url is a configuration gap, not a cycle failure:
			// log it and keep refreshing the remaining accounts.
			log.Warn().Err(err).Str("account", a.Name).Msg("cfhealth: skip account")
			continue
		}
		if err := db.WithContext(ctx).Model(&model.CFAccount{}).
			Where("id = ?", a.ID).
			Updates(map[string]any{"health": health, "last_check_at": now}).Error; err != nil {
			log.Warn().Err(err).Str("account", a.Name).Msg("cfhealth: update failed")
			continue
		}
		updated++
	}
	return updated, nil
}

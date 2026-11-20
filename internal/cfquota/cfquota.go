// Package cfquota answers one question for the rest of the service: is a given
// Cloudflare account currently able to serve requests, or has it spent its
// daily Workers quota?
//
// The answer is deliberately kept out of the node-health domain. A 429 from an
// exhausted account means the account cannot serve right now; it says nothing
// about the node that points at it. Folding that into a node's alive verdict or
// score would punish a healthy node for its owner's traffic, and the damage
// would outlive the outage: rotation schedules dead nodes by time since last
// success, so a quota blip would push the whole fleet to once-a-day probing and
// delay recovery long after the quota reset.
//
// The write side (the GraphQL call that produces the figures) lives with the CF
// handler, which already owned it. This package holds only the shared contract:
// the cache key and the read rule, so distribution and the checker can both ask
// the question without importing the HTTP layer.
package cfquota

import (
	"encoding/json"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// CacheKey is the configs key holding one account's most recent usage reading.
// The refresh writer and every reader must agree on it; it lives here so they
// cannot drift apart.
func CacheKey(accountID uuid.UUID) string {
	return "cf_usage_" + accountID.String()
}

// entry mirrors what the writer stores: when the figure was fetched, and the
// payload itself.
type entry struct {
	FetchedAt string          `json:"fetched_at"`
	Payload   json.RawMessage `json:"payload"`
}

// payload is the part of the reading that decides over-quota.
type payload struct {
	TotalToday int64 `json:"total_today"`
	QuotaDaily int64 `json:"quota_daily"`
}

// Exhausted reports the accounts whose daily quota is spent.
//
// An account with no cached reading, an unparsable one, or a non-positive
// quota is treated as NOT exhausted. The filter only ever removes nodes, so an
// unknown account must keep its nodes in the pool: wrongly hiding a working
// fleet is worse than briefly showing one that is out of quota, and the refresh
// loop fills the gap within one interval anyway.
func Exhausted(db *gorm.DB) map[uuid.UUID]struct{} {
	out := make(map[uuid.UUID]struct{})
	var rows []struct {
		ID    uuid.UUID
		Value string
	}
	// Joined through cf_accounts so a stale cf_usage_ row for a deleted account
	// cannot keep filtering anything.
	if err := db.Raw(
		"SELECT a.id, c.value FROM cf_accounts a" +
			" JOIN configs c ON c.key = 'cf_usage_' || a.id::text" +
			" WHERE a.enabled = true").Scan(&rows).Error; err != nil {
		return out
	}
	for _, r := range rows {
		var e entry
		if json.Unmarshal([]byte(r.Value), &e) != nil || len(e.Payload) == 0 {
			continue
		}
		var p payload
		if json.Unmarshal(e.Payload, &p) != nil || p.QuotaDaily <= 0 {
			continue
		}
		if p.TotalToday >= p.QuotaDaily {
			out[r.ID] = struct{}{}
		}
	}
	return out
}

// IDsKey holds the exhausted account IDs as a plain comma-separated list.
//
// Readers must not parse the usage JSON in SQL: the configs table holds rows
// with empty values, and Postgres is free to evaluate a ::jsonb cast on rows
// the query would never have selected, so a JSON cast there turns one unrelated
// blank config into a failed node query. A plain list has no such failure mode,
// and the refresher republishes it on every cycle.
const IDsKey = "cf_exhausted_ids"

// StoreExhausted republishes the exhausted set for SQL readers and reports which
// accounts left it since the previous publish - those are the accounts whose
// quota just reset, and the ones that deserve an immediate full retest.
func StoreExhausted(db *gorm.DB, set map[uuid.UUID]struct{}) ([]uuid.UUID, error) {
	previous := ExhaustedSet(db)
	var released []uuid.UUID
	for id := range previous {
		if _, still := set[id]; !still {
			released = append(released, id)
		}
	}
	ids := make([]string, 0, len(set))
	for id := range set {
		ids = append(ids, id.String())
	}
	err := db.Exec(
		"INSERT INTO configs (key, value, updated_at) VALUES (?, ?, now()) "+
			"ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now()",
		IDsKey, strings.Join(ids, ",")).Error
	return released, err
}

// ExhaustedSet reads back the published list. A missing or blank value yields an
// empty set, which keeps every node in play: the filter may only ever remove
// nodes, so a cold start must not hide the pool.
func ExhaustedSet(db *gorm.DB) map[uuid.UUID]struct{} {
	out := make(map[uuid.UUID]struct{})
	var stored string
	if err := db.Raw("SELECT value FROM configs WHERE key = ?", IDsKey).Scan(&stored).Error; err != nil {
		return out
	}
	for _, part := range strings.Split(stored, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if id, err := uuid.Parse(part); err == nil {
			out[id] = struct{}{}
		}
	}
	return out
}

// RetestKey holds the accounts that just left the exhausted set. Their nodes are
// due for one full probe round regardless of the rotation tier, so recovery is a
// single event instead of an eight-hour trickle: rotation schedules by time
// since last success, and a fleet that sat out an outage has nothing recent to
// schedule on. GetNodes clears the key after using it, which is what makes it
// exactly one round.
const RetestKey = "cf_retest_ids"

// StoreRetest queues a one-round full retest for the given accounts.
func StoreRetest(db *gorm.DB, ids []uuid.UUID) error {
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		parts = append(parts, id.String())
	}
	return db.Exec(
		"INSERT INTO configs (key, value, updated_at) VALUES (?, ?, now()) "+
			"ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now()",
		RetestKey, strings.Join(parts, ",")).Error
}

// ClearRetest drops the queued retest after a round consumed it.
func ClearRetest(db *gorm.DB) {
	db.Exec("UPDATE configs SET value = '', updated_at = now() WHERE key = ?", RetestKey)
}

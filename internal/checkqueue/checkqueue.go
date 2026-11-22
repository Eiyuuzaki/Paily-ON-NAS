// Package checkqueue holds the nodes the operator wants probed right now
// rather than on the next rotation turn.
//
// GetNodes() decides what to probe from rotation tiers: a live node is probed
// once every few rounds, a dead one on a cadence that stretches with its time
// since last success. That is correct for steady state and wrong the moment a
// human adds a node or a source — a brand-new entry has no measurement at all,
// so it stays out of distribution until its rotation turn arrives, which can be
// two hours. The only workaround used to be restarting the checker, which
// re-probes the entire rotation and wastes the round.
//
// This queue is the narrow override: ids listed here are handed to the checker
// on its next round regardless of rotation, then cleared. One enqueue therefore
// means exactly one round — the same contract as cfquota.RetestKey.
package checkqueue

import (
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Key is the configs row that holds the queue as a comma-separated id list.
//
// A plain list, never JSON: configs carries blank values and Postgres is free
// to evaluate a ::jsonb cast on rows the query would not have selected, which
// turned an unrelated blank config into a failed query on 2026-11-21.
const Key = "check_urgent_ids"

// MaxQueue bounds the queue. A source with thousands of nodes is a bulk import,
// not an urgent recheck; refusing at the edge is better than handing the checker
// a round it cannot finish inside its interval.
const MaxQueue = 2000

// Enqueue merges ids into the queue, dropping duplicates, and reports how many
// were newly added together with the resulting queue length. Ids already queued
// are not counted twice, so repeated clicks do not grow the next round.
func Enqueue(db *gorm.DB, ids []uuid.UUID) (added, total int, err error) {
	existing := Pending(db)
	merged, added := merge(existing, ids, MaxQueue)
	if added == 0 {
		return 0, len(existing), nil
	}
	if err := write(db, merged); err != nil {
		return 0, len(existing), err
	}
	return added, len(merged), nil
}

// merge appends the ids not already present, preserving order and stopping at
// max. It is pure so the queue's contract — never duplicate, never exceed the
// cap, keep what was already queued — can be tested without a database.
func merge(existing, incoming []uuid.UUID, max int) (out []uuid.UUID, added int) {
	seen := make(map[uuid.UUID]struct{}, len(existing)+len(incoming))
	out = make([]uuid.UUID, 0, len(existing)+len(incoming))
	for _, id := range existing {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	for _, id := range incoming {
		if _, ok := seen[id]; ok {
			continue
		}
		if len(out) >= max {
			break
		}
		seen[id] = struct{}{}
		out = append(out, id)
		added++
	}
	return out, added
}

// Pending reads the queue. A missing or blank row yields an empty slice: the
// queue may only ever add work, so a cold start must not fail a round.
func Pending(db *gorm.DB) []uuid.UUID {
	var stored string
	if err := db.Raw("SELECT value FROM configs WHERE key = ?", Key).Scan(&stored).Error; err != nil {
		return nil
	}
	parts := strings.Split(stored, ",")
	out := make([]uuid.UUID, 0, len(parts))
	seen := map[uuid.UUID]struct{}{}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		id, err := uuid.Parse(part)
		if err != nil {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

// Count reports the queue length.
func Count(db *gorm.DB) int { return len(Pending(db)) }

// Clear empties the queue after a round consumed it. Best effort by design: a
// failed clear means the next round repeats the urgent set, which is harmless
// and far better than dropping the queue on a transient write error.
func Clear(db *gorm.DB) {
	db.Exec("UPDATE configs SET value = '', updated_at = now() WHERE key = ?", Key)
}

func write(db *gorm.DB, ids []uuid.UUID) error {
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		parts = append(parts, id.String())
	}
	return db.Exec(
		"INSERT INTO configs (key, value, updated_at) VALUES (?, ?, now()) "+
			"ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now()",
		Key, strings.Join(parts, ",")).Error
}

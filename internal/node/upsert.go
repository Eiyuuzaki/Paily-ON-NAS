package node

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Eiyuuzaki/paily-on-nas/internal/db/model"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// UpsertService implements fetcher.NodeUpsertPort.
type UpsertService struct {
	db *gorm.DB
}

// NewUpsertService creates a new UpsertService.
func NewUpsertService(db *gorm.DB) *UpsertService {
	return &UpsertService{db: db}
}

// UpsertFromFetchResult processes a fetch result:
//   - failure       → no-op (existing nodes are preserved)
//   - success+empty → clear all source mappings, delete orphaned nodes
//   - success+nodes → upsert nodes/mappings, clean up stale mappings + orphans
//
// Dead sources are silently skipped so that an in-flight fetch result (submitted
// after checker already condemned the source) cannot resurrect zombie nodes that
// would otherwise never be cleaned up (CheckNodeDead short-circuits on dead sources).
func (u *UpsertService) UpsertFromFetchResult(
	ctx context.Context,
	sourceID uuid.UUID,
	success bool,
	nodes []json.RawMessage,
) error {
	if !success {
		return nil
	}

	// Guard: if this source is already dead, reject the submission so that
	// in-flight fetcher results cannot re-create node mappings for a condemned
	// source.  Those mappings would become permanent zombies because
	// CheckNodeDead returns false early for dead sources.
	var src model.Source
	if err := u.db.WithContext(ctx).Select("status").First(&src, "id = ?", sourceID).Error; err == nil {
		if src.Status == "dead" {
			log.Debug().Str("source_id", sourceID.String()).Msg("node upsert: skipping dead source")
			return nil
		}
	}

	if len(nodes) == 0 {
		return removeSourceMappings(ctx, u.db, sourceID)
	}

	// Parse incoming nodes.
	incoming := make([]parsedNode, 0, len(nodes))
	for _, raw := range nodes {
		var fields map[string]interface{}
		if err := json.Unmarshal(raw, &fields); err != nil {
			log.Warn().Err(err).Msg("node upsert: skipping unparseable node")
			continue
		}
		proto := strings.ToLower(strField(fields, "type"))
		srv := strField(fields, "server")
		auth := nodeAuth(proto, fields)
		compact, _ := json.Marshal(fields)
		incoming = append(incoming, parsedNode{
			hash:     nodeHash(fields),
			protocol: proto,
			server:   srv,
			port:     clashPort(fields),
			password: auth,
			rawJSON:  compact,
		})
	}

	// ── Ingress gate (B) ────────────────────────────────────────────────
	// Free sources re-publish the same dead endpoints every round. Without a
	// gate the pool grows without bound and the checker spends most of its
	// budget re-probing nodes that have never once succeeded.
	incoming = u.gateIncoming(ctx, incoming)

	// Load existing mappings for this source.
	var oldMappings []model.NodeSource
	if err := u.db.WithContext(ctx).Where("source_id = ?", sourceID).Find(&oldMappings).Error; err != nil {
		return fmt.Errorf("node upsert: load old mappings: %w", err)
	}

	// Upsert each incoming node.
	// Each node is handled inside a transaction so the SELECT-then-INSERT is
	// atomic: concurrent callers that share a node cannot both take the "insert
	// new node" branch, preventing a lost NodeSource mapping when the second
	// INSERT fails with a UNIQUE constraint violation.
	newNodeIDs := make(map[uuid.UUID]struct{}, len(incoming))
	for _, p := range incoming {
		var nodeID uuid.UUID
		err := u.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			var existing model.Node
			if err := tx.Where("hash = ?", p.hash).First(&existing).Error; err == nil {
				// Exists: refresh raw content and ensure mapping.
				tx.Model(&existing).Updates(map[string]interface{}{
					"raw":        string(p.rawJSON),
					"port":       p.port,
					"updated_at": time.Now(),
				})
				nodeID = existing.ID
			} else {
				// New node: insert.
				n := &model.Node{
					ID:        uuid.New(),
					Hash:      p.hash,
					Server:    p.server,
					Port:      p.port,
					Password:  p.password,
					Protocol:  p.protocol,
					Raw:       string(p.rawJSON),
					Alive:     false,
					Score:     0,
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
				}
				if err2 := tx.Create(n).Error; err2 != nil {
					return err2
				}
				nodeID = n.ID
			}
			// Ensure source → node mapping (idempotent).
			tx.Clauses(clause.OnConflict{DoNothing: true}).
				Create(&model.NodeSource{NodeID: nodeID, SourceID: sourceID})
			return nil
		})
		if err != nil {
			log.Warn().Err(err).Str("hash", p.hash).Msg("node upsert: transaction failed, skipping")
			continue
		}
		newNodeIDs[nodeID] = struct{}{}
	}

	// Remove stale mappings and orphaned nodes.
	for _, old := range oldMappings {
		if _, ok := newNodeIDs[old.NodeID]; ok {
			continue
		}
		u.db.WithContext(ctx).Delete(&model.NodeSource{},
			"node_id = ? AND source_id = ?", old.NodeID, sourceID)
		// Delete the node only if it is now truly orphaned. The DELETE
		// WHERE NOT EXISTS is atomic — no concurrent caller can insert a
		// new mapping in the gap between our count check and the delete.
		u.db.WithContext(ctx).Exec(
			"DELETE FROM nodes WHERE id = ? AND NOT EXISTS (SELECT 1 FROM node_sources WHERE node_id = ?)",
			old.NodeID, old.NodeID,
		)
	}

	return nil
}

// removeSourceMappings deletes all NodeSource rows for sourceID and removes orphaned nodes
// along with all their associated check history.
// Shared by UpsertService (empty result path) and Service.RemoveSourceMappings (source dead path).
func removeSourceMappings(ctx context.Context, db *gorm.DB, sourceID uuid.UUID) error {
	var mappings []model.NodeSource
	if err := db.WithContext(ctx).Where("source_id = ?", sourceID).Find(&mappings).Error; err != nil {
		return fmt.Errorf("node cleanup: load mappings: %w", err)
	}
	if len(mappings) == 0 {
		return nil
	}
	if err := db.WithContext(ctx).Delete(&model.NodeSource{}, "source_id = ?", sourceID).Error; err != nil {
		return fmt.Errorf("node cleanup: delete mappings: %w", err)
	}
	for _, m := range mappings {
		// Only proceed with deeper cleanup if the node has become truly orphaned.
		var count int64
		db.WithContext(ctx).Model(&model.NodeSource{}).Where("node_id = ?", m.NodeID).Count(&count)
		if count > 0 {
			continue // node still referenced by another source, skip
		}
		// Clean up all check history before deleting the node.
		db.WithContext(ctx).Delete(&model.NodeInitialCheck{}, "node_id = ?", m.NodeID)
		db.WithContext(ctx).Delete(&model.NodeDeepCheck{}, "node_id = ?", m.NodeID)
		db.WithContext(ctx).Delete(&model.NodeTagScore{}, "node_id = ?", m.NodeID)
		// Atomic guard: only delete the node if it is still orphaned.
		db.WithContext(ctx).Exec(
			"DELETE FROM nodes WHERE id = ? AND NOT EXISTS (SELECT 1 FROM node_sources WHERE node_id = ?)",
			m.NodeID, m.NodeID,
		)
	}
	return nil
}

// nodeHash returns a stable, content-based identifier for a proxy node.
// Algorithm mirrors paily-fetch's node.Hash:
//
//	canonical = "type|server|port_int|auth_credential"
//	digest    = hex(sha256(canonical)[:16])  — 32-char hex string
func nodeHash(clash map[string]any) string {
	ptype, _ := clash["type"].(string)
	server, _ := clash["server"].(string)
	port := clashPort(clash)
	if ptype == "" || server == "" || port == 0 {
		return ""
	}
	auth := normalizeCredential(nodeAuth(ptype, clash))
	canonical := fmt.Sprintf("%s|%s|%d|%s", ptype, server, port, auth)
	sum := sha256.Sum256([]byte(canonical))
	return hex.EncodeToString(sum[:16])
}

// normalizeCredential repeatedly percent-decodes a credential string until
// it stabilises. This ensures that the same underlying credential encoded
// at different nesting depths (e.g. %25-escaped multiple times) always
// produces the same canonical form for hashing.
func normalizeCredential(s string) string {
	for {
		decoded, err := url.PathUnescape(s)
		if err != nil || decoded == s {
			return s
		}
		s = decoded
	}
}

// nodeAuth returns the primary authentication credential for the given protocol.
// Mirrors paily-fetch's node.authField.
func nodeAuth(ptype string, clash map[string]any) string {
	switch ptype {
	case "vmess", "vless":
		s, _ := clash["uuid"].(string)
		return s
	case "trojan", "anytls", "hysteria", "hysteria2":
		s, _ := clash["password"].(string)
		return s
	case "tuic":
		// TUIC V5 uses uuid; V4 uses only password.
		if uuid, ok := clash["uuid"].(string); ok && uuid != "" {
			s, _ := clash["password"].(string)
			return uuid + ":" + s
		}
		s, _ := clash["password"].(string)
		return s
	case "ss", "ssr":
		s, _ := clash["password"].(string)
		return s
	case "wireguard":
		s, _ := clash["private-key"].(string)
		return s
	case "socks5", "http":
		user, _ := clash["username"].(string)
		pass, _ := clash["password"].(string)
		return user + ":" + pass
	case "mieru":
		user, _ := clash["username"].(string)
		pass, _ := clash["password"].(string)
		return user + ":" + pass
	case "sudoku":
		s, _ := clash["key"].(string)
		return s
	case "snell":
		s, _ := clash["psk"].(string)
		return s
	default:
		return ""
	}
}

// clashPort extracts the port field as int (handles float64/int/int64).
// Mirrors paily-fetch's node.clashPort.
func clashPort(clash map[string]any) int {
	switch v := clash["port"].(type) {
	case int:
		return v
	case float64:
		return int(v)
	case int64:
		return int(v)
	default:
		return 0
	}
}

// strField extracts a string value from a map[string]interface{}.
func strField(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// parsedNode is one node decoded from a fetch payload, ready for upsert.
type parsedNode struct {
	hash     string
	protocol string
	server   string
	port     int
	password string
	rawJSON  []byte
}

// zeroYieldProtocols are protocols whose free-list entries have never passed
// a single check in this deployment (observed n>=600, all i/o timeout at the
// TCP layer). They are not rejected outright — a genuine revival would then
// be invisible forever — but they are held to a small pool quota so one dead
// protocol family cannot crowd out the probe budget.
var zeroYieldProtocols = map[string]bool{
	"ss": true, "ssr": true, "hysteria2": true, "socks5": true,
	"tuic": true, "anytls": true, "wireguard": true,
}

func endpointKey(server string, port int) string {
	return server + ":" + strconv.Itoa(port)
}

// gateIncoming applies the ingress cap described at the call site.
// Ordering is preserved: nodes that already exist pass first, then new
// entries are admitted until their endpoint or protocol quota is spent.
func (u *UpsertService) gateIncoming(ctx context.Context, incoming []parsedNode) []parsedNode {
	if len(incoming) == 0 {
		return incoming
	}
	maxPerEndpoint := readCfgInt(u.db, "ingress_max_per_endpoint", 3)
	zeroQuota := readCfgInt(u.db, "ingress_zero_yield_quota", 20)
	if maxPerEndpoint <= 0 {
		maxPerEndpoint = 3
	}

	// Which of these hashes are already in the pool? Those are refreshes,
	// not additions, and must never be dropped by a quota.
	hashes := make([]string, 0, len(incoming))
	for _, p := range incoming {
		hashes = append(hashes, p.hash)
	}
	existing := make(map[string]struct{})
	retired := make(map[string]struct{})
	var rows []struct {
		Hash       string
		IsArchived bool
	}
	if err := u.db.WithContext(ctx).Model(&model.Node{}).
		Select("hash, archived_at IS NOT NULL AS is_archived").
		Where("hash IN ?", hashes).Scan(&rows).Error; err == nil {
		for _, r := range rows {
			if r.IsArchived {
				retired[r.Hash] = struct{}{}
			} else {
				existing[r.Hash] = struct{}{}
			}
		}
	}

	// Current pool pressure, keyed the way the rules reason about it.
	var epRows []struct {
		Server string
		Port   int
		N      int
	}
	u.db.WithContext(ctx).Model(&model.Node{}).
		Select("server, port, count(*) as n").Group("server, port").Scan(&epRows)
	epCount := make(map[string]int, len(epRows))
	for _, r := range epRows {
		epCount[endpointKey(r.Server, r.Port)] = r.N
	}
	var protoRows []struct {
		Protocol string
		N        int
	}
	u.db.WithContext(ctx).Model(&model.Node{}).
		Select("protocol, count(*) as n").Group("protocol").Scan(&protoRows)
	protoCount := make(map[string]int, len(protoRows))
	for _, r := range protoRows {
		protoCount[r.Protocol] = r.N
	}

	out := make([]parsedNode, 0, len(incoming))
	seen := make(map[string]struct{}, len(incoming))
	droppedEP, droppedProto, droppedDup, droppedRetired := 0, 0, 0, 0
	for _, p := range incoming {
		if _, dup := seen[p.hash]; dup {
			droppedDup++
			continue
		}
		seen[p.hash] = struct{}{}
		if _, ok := retired[p.hash]; ok {
			droppedRetired++
			continue
		}
		if _, ok := existing[p.hash]; ok {
			out = append(out, p)
			continue
		}
		if zeroYieldProtocols[p.protocol] && protoCount[p.protocol] >= zeroQuota {
			droppedProto++
			continue
		}
		k := endpointKey(p.server, p.port)
		if epCount[k] >= maxPerEndpoint {
			droppedEP++
			continue
		}
		epCount[k]++
		protoCount[p.protocol]++
		out = append(out, p)
	}
	if droppedEP+droppedProto+droppedDup+droppedRetired > 0 {
		log.Info().
			Int("in", len(incoming)).Int("kept", len(out)).
			Int("endpoint_cap", droppedEP).Int("protocol_quota", droppedProto).Int("dup", droppedDup).Int("retired", droppedRetired).
			Msg("node upsert: ingress gate filtered")
	}
	return out
}

// readCfgInt reads an integer config value, falling back to def.
func readCfgInt(db *gorm.DB, key string, def int) int {
	var cfg model.Config
	if err := db.Where("key = ?", key).First(&cfg).Error; err != nil {
		return def
	}
	v, err := strconv.Atoi(cfg.Value)
	if err != nil {
		return def
	}
	return v
}

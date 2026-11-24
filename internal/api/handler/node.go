package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	api "github.com/Eiyuuzaki/paily-on-nas/internal/api/generated"
	"github.com/Eiyuuzaki/paily-on-nas/internal/api/response"
	"github.com/Eiyuuzaki/paily-on-nas/internal/cache"
	"github.com/Eiyuuzaki/paily-on-nas/internal/db/model"
	nodePkg "github.com/Eiyuuzaki/paily-on-nas/internal/node"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"
	"gorm.io/gorm"
)

// NodeHandler implements admin node management HTTP handlers.
type NodeHandler struct {
	svc *nodePkg.Service
	db  *gorm.DB
	// cache is the distribution snapshot. Deleting a node must invalidate it,
	// otherwise the removed node keeps being served until something else
	// triggers a rebuild.
	cache *cache.AliveCache
}

// List handles GET /api/v1/nodes.
func (h *NodeHandler) List(c *gin.Context, params api.NodeListParams) {
	f := nodePkg.NodeListFilter{}
	if params.Alive != nil {
		f.Alive = params.Alive
	}
	if params.Region != nil {
		f.Region = params.Region
	}
	if params.Protocol != nil {
		f.Protocol = params.Protocol
	}
	if params.Server != nil {
		f.Server = params.Server
	}
	if params.SourceId != nil {
		id := uuid.UUID(*params.SourceId)
		f.SourceID = &id
	}
	if params.MinScore != nil {
		f.MinScore = params.MinScore
	}
	if params.MaxScore != nil {
		f.MaxScore = params.MaxScore
	}
	if params.HasStreaming != nil && *params.HasStreaming != "" {
		f.HasStreaming = splitCSV(*params.HasStreaming)
	}
	if params.Tag != nil {
		f.CheckerTag = params.Tag
	}
	if params.Page != nil {
		f.Page = *params.Page
	}
	if params.Limit != nil {
		f.Limit = *params.Limit
	}
	if params.SortBy != nil {
		f.SortBy = string(*params.SortBy)
	}
	if params.SortDir != nil {
		f.SortDir = string(*params.SortDir)
	}

	nodes, total, err := h.svc.List(c.Request.Context(), f)
	if err != nil {
		response.InternalError(c, "failed to list nodes")
		return
	}

	nodeIDs := make([]uuid.UUID, len(nodes))
	for i, n := range nodes {
		nodeIDs[i] = n.ID
	}
	initChecks, deepChecks, nodeSrcs, err := loadNodeBatches(c.Request.Context(), h.db, nodeIDs)
	if err != nil {
		response.InternalError(c, "failed to load node check details")
		return
	}

	summaries := make([]api.NodeSummary, len(nodes))
	for i := range nodes {
		summaries[i] = nodeToSummary(&nodes[i], initChecks[nodes[i].ID], deepChecks[nodes[i].ID], nodeSrcs[nodes[i].ID])
	}

	page, limit := normaliseNodeListPageLimit(f.Page, f.Limit)
	totalPages := 0
	if limit > 0 {
		totalPages = int((total + int64(limit) - 1) / int64(limit))
	}

	c.JSON(http.StatusOK, api.NodeListResponse{
		Data: summaries,
		Pagination: api.PaginationInfo{
			Page:       page,
			Limit:      limit,
			Total:      total,
			TotalPages: totalPages,
		},
	})
}

// Get handles GET /api/v1/nodes/:id.
func (h *NodeHandler) Get(c *gin.Context, id openapi_types.UUID) {
	ctx := c.Request.Context()
	nodeID := uuid.UUID(id)
	node, sources, err := h.svc.Get(ctx, nodeID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			response.NotFound(c, "node not found")
		} else {
			response.InternalError(c, "failed to get node")
		}
		return
	}
	latestDeep, err := h.svc.GetLatestDeepCheck(ctx, nodeID)
	if err != nil {
		response.InternalError(c, "failed to load latest deep check")
		return
	}
	latestInitial, err := latestInitialChecksBatch(ctx, h.db, []uuid.UUID{nodeID})
	if err != nil {
		response.InternalError(c, "failed to load latest initial check")
		return
	}

	// Query per-tag scores.
	var tagScoreRows []model.NodeTagScore
	h.db.WithContext(ctx).Where("node_id = ?", nodeID).Find(&tagScoreRows)
	tagScores := make(map[string]float64, len(tagScoreRows))
	for _, ts := range tagScoreRows {
		tagScores[ts.CheckerTag] = ts.Score
	}

	// Query latest deep check per tag (correlated subquery).
	var deepByTagRows []model.NodeDeepCheck
	h.db.WithContext(ctx).Raw(`
		SELECT * FROM node_deep_checks dc
		WHERE node_id = ?
		AND checked_at = (
			SELECT MAX(dc2.checked_at) FROM node_deep_checks dc2
			WHERE dc2.node_id = dc.node_id AND dc2.checker_tag = dc.checker_tag
		)
	`, nodeID).Scan(&deepByTagRows)
	latestDeepByTag := make(map[string]*model.NodeDeepCheck, len(deepByTagRows))
	for i := range deepByTagRows {
		latestDeepByTag[deepByTagRows[i].CheckerTag] = &deepByTagRows[i]
	}

	c.JSON(http.StatusOK, nodeToDetail(node, sources, latestDeep, latestInitial[nodeID], tagScores, latestDeepByTag))
}

// Delete handles DELETE /api/v1/nodes/:id.
func (h *NodeHandler) Delete(c *gin.Context, id openapi_types.UUID) {
	if err := h.svc.Delete(c.Request.Context(), uuid.UUID(id)); err != nil {
		if err == gorm.ErrRecordNotFound {
			response.NotFound(c, "node not found")
		} else {
			response.InternalError(c, "failed to delete node")
		}
		return
	}
	if h.cache != nil {
		h.cache.TriggerRebuild(c.Request.Context())
	}
	response.NoContent(c)
}

func nodeToSummary(n *model.Node, latestInitial *model.NodeInitialCheck, latestDeep *model.NodeDeepCheck, sources []model.Source) api.NodeSummary {
	s := api.NodeSummary{
		Id:       openapi_types.UUID(n.ID),
		Hash:     n.Hash,
		Server:   n.Server,
		Protocol: n.Protocol,
		Region:   n.Region,
		Alive:    n.Alive,
		Score:    n.Score,
	}
	if n.RegionSource != "" {
		s.RegionSource = &n.RegionSource
	}
	if latestInitial != nil {
		s.LatestLatencyMs = &latestInitial.LatencyMs
	}
	if latestDeep != nil {
		s.LatestAvgLatencyMs = &latestDeep.AvgLatencyMs
		s.LatestJitterMs = &latestDeep.JitterMs
	}
	if len(sources) > 0 {
		sums := make([]api.SourceSummary, len(sources))
		for i := range sources {
			// 节点详情里只展示来源的基本信息，不做节点统计。
			sums[i] = sourceToSummary(&sources[i], nil, nil)
		}
		s.Sources = &sums
	}
	return s
}

func nodeToDetail(n *model.Node, sources []model.Source, latestDeep *model.NodeDeepCheck, latestInitial *model.NodeInitialCheck, tagScores map[string]float64, latestDeepByTag map[string]*model.NodeDeepCheck) api.NodeDetail {
	pw := n.Password
	createdAt := n.CreatedAt
	updatedAt := n.UpdatedAt

	var rawMap map[string]interface{}
	if n.Raw != "" {
		_ = json.Unmarshal([]byte(n.Raw), &rawMap)
	}

	detail := api.NodeDetail{
		Id:        openapi_types.UUID(n.ID),
		Hash:      n.Hash,
		Server:    n.Server,
		Protocol:  n.Protocol,
		Region:    n.Region,
		Alive:     n.Alive,
		Score:     n.Score,
		Password:  &pw,
		Raw:       &rawMap,
		CreatedAt: &createdAt,
		UpdatedAt: &updatedAt,
	}
	if n.RegionSource != "" {
		detail.RegionSource = &n.RegionSource
	}

	if latestDeep != nil {
		rec := deepCheckToRecord(latestDeep)
		detail.LatestDeepCheck = &rec
		detail.LatestAvgLatencyMs = &latestDeep.AvgLatencyMs
		detail.LatestJitterMs = &latestDeep.JitterMs
	}

	if latestInitial != nil {
		detail.LatestLatencyMs = &latestInitial.LatencyMs
	}

	if len(tagScores) > 0 {
		detail.TagScores = &tagScores
	}

	if len(latestDeepByTag) > 0 {
		byTag := make(map[string]api.DeepCheckRecord, len(latestDeepByTag))
		for tag, dc := range latestDeepByTag {
			byTag[tag] = deepCheckToRecord(dc)
		}
		detail.LatestDeepCheckByTag = &byTag
	}

	if len(sources) > 0 {
		sums := make([]api.SourceSummary, len(sources))
		for i := range sources {
			sums[i] = sourceToSummary(&sources[i], nil, nil)
		}
		detail.Sources = &sums
	}

	return detail
}

// splitCSV splits a comma-separated string, trimming whitespace and dropping blanks.
func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func normaliseNodeListPageLimit(page, limit int) (int, int) {
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 20
	}
	if limit > 200 {
		limit = 200
	}
	return page, limit
}

// batchQueryError wraps a failure from one batch of a batched load.
//
// A batched load spans several queries, and returning a partial map would make
// "the query for these nodes failed" indistinguishable from "these nodes have no
// row yet" - the second of which is also a valid state. Every batch failure is
// therefore surfaced, not skipped.
type batchQueryError struct {
	what  string
	batch int
	err   error
}

func (e *batchQueryError) Error() string {
	return fmt.Sprintf("%s: batch %d: %v", e.what, e.batch, e.err)
}

func (e *batchQueryError) Unwrap() error { return e.err }

// latestInitialChecksBatch returns the most recent initial check per node ID.
//
// The correlated subquery here is deliberate and must not be "optimised" into
// the GROUP BY MAX(id) + JOIN form that filter.go uses. That form wins only when
// there is no node_id filter, because it aggregates the whole table first.
// Measured on the production table (82,903 rows) with ids supplied:
//
//	no filter:            correlated 466.7ms   grouped 102.3ms   (grouped wins)
//	200 node_id filter:   correlated  22.7ms   grouped  84.1ms   (correlated wins)
//
// This function always filters by node_id, so PostgreSQL narrows through
// idx_node_initial_checks_node_id and runs the subquery only len(batch) times.
// Switching it to the grouped form measured 3.7x slower.
func latestInitialChecksBatch(ctx context.Context, db *gorm.DB, nodeIDs []uuid.UUID) (map[uuid.UUID]*model.NodeInitialCheck, error) {
	if len(nodeIDs) == 0 {
		return nil, nil
	}
	var rows []model.NodeInitialCheck
	for i, batch := range chunkUUIDs(nodeIDs, 10000) {
		var batchRows []model.NodeInitialCheck
		if err := db.WithContext(ctx).
			Where("node_id IN ? AND checked_at = (SELECT MAX(ic2.checked_at) FROM node_initial_checks ic2 WHERE ic2.node_id = node_initial_checks.node_id)", batch).
			Find(&batchRows).Error; err != nil {
			return nil, &batchQueryError{what: "load initial checks", batch: i, err: err}
		}
		rows = append(rows, batchRows...)
	}
	m := make(map[uuid.UUID]*model.NodeInitialCheck, len(rows))
	for i := range rows {
		m[rows[i].NodeID] = &rows[i]
	}
	return m, nil
}

// latestDeepChecksBatch returns the most recent deep check per node ID.
// Same filtered-correlated form as latestInitialChecksBatch, for the same
// measured reason - see the note there before changing it.
func latestDeepChecksBatch(ctx context.Context, db *gorm.DB, nodeIDs []uuid.UUID) (map[uuid.UUID]*model.NodeDeepCheck, error) {
	if len(nodeIDs) == 0 {
		return nil, nil
	}
	var rows []model.NodeDeepCheck
	for i, batch := range chunkUUIDs(nodeIDs, 10000) {
		var batchRows []model.NodeDeepCheck
		if err := db.WithContext(ctx).
			Where("node_id IN ? AND checked_at = (SELECT MAX(dc2.checked_at) FROM node_deep_checks dc2 WHERE dc2.node_id = node_deep_checks.node_id)", batch).
			Find(&batchRows).Error; err != nil {
			return nil, &batchQueryError{what: "load deep checks", batch: i, err: err}
		}
		rows = append(rows, batchRows...)
	}
	m := make(map[uuid.UUID]*model.NodeDeepCheck, len(rows))
	for i := range rows {
		m[rows[i].NodeID] = &rows[i]
	}
	return m, nil
}

// nodeSourcesBatch returns all sources per node ID for a batch of nodes.
func nodeSourcesBatch(ctx context.Context, db *gorm.DB, nodeIDs []uuid.UUID) (map[uuid.UUID][]model.Source, error) {
	if len(nodeIDs) == 0 {
		return nil, nil
	}
	type nsRow struct {
		NodeID   uuid.UUID
		SourceID uuid.UUID
	}
	var ns []nsRow
	for i, batch := range chunkUUIDs(nodeIDs, 10000) {
		var batchNS []nsRow
		if err := db.WithContext(ctx).Table("node_sources").Where("node_id IN ?", batch).Scan(&batchNS).Error; err != nil {
			return nil, &batchQueryError{what: "load node-source links", batch: i, err: err}
		}
		ns = append(ns, batchNS...)
	}
	if len(ns) == 0 {
		return nil, nil
	}
	srcSet := make(map[uuid.UUID]struct{}, len(ns))
	for _, r := range ns {
		srcSet[r.SourceID] = struct{}{}
	}
	srcIDs := make([]uuid.UUID, 0, len(srcSet))
	for id := range srcSet {
		srcIDs = append(srcIDs, id)
	}
	var sources []model.Source
	for i, batch := range chunkUUIDs(srcIDs, 10000) {
		var batchSources []model.Source
		if err := db.WithContext(ctx).Where("id IN ?", batch).Find(&batchSources).Error; err != nil {
			return nil, &batchQueryError{what: "load sources", batch: i, err: err}
		}
		sources = append(sources, batchSources...)
	}
	srcByID := make(map[uuid.UUID]*model.Source, len(sources))
	for i := range sources {
		srcByID[sources[i].ID] = &sources[i]
	}
	m := make(map[uuid.UUID][]model.Source)
	for _, r := range ns {
		if src, ok := srcByID[r.SourceID]; ok {
			m[r.NodeID] = append(m[r.NodeID], *src)
		}
	}
	return m, nil
}

// loadNodeBatches loads the three per-node detail maps together.
//
// Both list-shaped handlers (NodeHandler.List and SearchHandler.NodeSearch) need
// all three and share one error path, so the combination lives here instead of
// being spelled out twice.
func loadNodeBatches(ctx context.Context, db *gorm.DB, ids []uuid.UUID) (
	map[uuid.UUID]*model.NodeInitialCheck,
	map[uuid.UUID]*model.NodeDeepCheck,
	map[uuid.UUID][]model.Source,
	error,
) {
	initChecks, err := latestInitialChecksBatch(ctx, db, ids)
	if err != nil {
		return nil, nil, nil, err
	}
	deepChecks, err := latestDeepChecksBatch(ctx, db, ids)
	if err != nil {
		return nil, nil, nil, err
	}
	nodeSrcs, err := nodeSourcesBatch(ctx, db, ids)
	if err != nil {
		return nil, nil, nil, err
	}
	return initChecks, deepChecks, nodeSrcs, nil
}

func chunkUUIDs(ids []uuid.UUID, batchSize int) [][]uuid.UUID {
	if len(ids) == 0 {
		return nil
	}
	if batchSize <= 0 || len(ids) <= batchSize {
		return [][]uuid.UUID{ids}
	}
	batches := make([][]uuid.UUID, 0, (len(ids)+batchSize-1)/batchSize)
	for start := 0; start < len(ids); start += batchSize {
		end := start + batchSize
		if end > len(ids) {
			end = len(ids)
		}
		batches = append(batches, ids[start:end])
	}
	return batches
}

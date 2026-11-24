package handler

import (
	"net/http"

	api "github.com/Eiyuuzaki/paily-on-nas/internal/api/generated"
	"github.com/Eiyuuzaki/paily-on-nas/internal/api/response"
	"github.com/Eiyuuzaki/paily-on-nas/internal/db/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// StatsHandler returns aggregate statistics for the admin dashboard.
type StatsHandler struct {
	db *gorm.DB
}

func (h *StatsHandler) Get(c *gin.Context) {
	var totalNodes, aliveNodes, totalSources, deadSources int64

	// archived_at is not a gorm.DeletedAt, so GORM does not filter it: the model
	// keeps retired rows on purpose so the unique hash index keeps blocking
	// re-ingestion, which means the archived set only ever grows. Counting them
	// made "total nodes" drift further from the real pool every day, with no error
	// anywhere and no line of output to suggest the number had stopped meaning what
	// its label says.
	// Every count is checked. A discarded error here returned 200 with "total 0,
	// alive 0", which reads as a legitimately empty pool rather than as a failed
	// query - nobody goes looking in the log for a response that looks valid.
	queries := []struct {
		dst *int64
		q   *gorm.DB
	}{
		{&totalNodes, h.db.Model(&model.Node{}).Where("archived_at IS NULL")},
		{&aliveNodes, h.db.Model(&model.Node{}).Where("archived_at IS NULL AND alive = ?", true)},
		{&totalSources, h.db.Model(&model.Source{})},
		{&deadSources, h.db.Model(&model.Source{}).Where("status = ?", "dead")},
	}
	for _, item := range queries {
		if err := item.q.Count(item.dst).Error; err != nil {
			response.InternalError(c, "failed to count nodes and sources")
			return
		}
	}

	c.JSON(http.StatusOK, api.StatsResponse{
		TotalNodes:   int(totalNodes),
		AliveNodes:   int(aliveNodes),
		TotalSources: int(totalSources),
		DeadSources:  int(deadSources),
	})
}

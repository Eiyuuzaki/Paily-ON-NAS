package handler

import (
	"net/http"

	api "github.com/Eiyuuzaki/paily-on-nas/internal/api/generated"
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
	h.db.Model(&model.Node{}).Where("archived_at IS NULL").Count(&totalNodes)
	h.db.Model(&model.Node{}).Where("archived_at IS NULL AND alive = ?", true).Count(&aliveNodes)
	h.db.Model(&model.Source{}).Count(&totalSources)
	h.db.Model(&model.Source{}).Where("status = ?", "dead").Count(&deadSources)

	c.JSON(http.StatusOK, api.StatsResponse{
		TotalNodes:   int(totalNodes),
		AliveNodes:   int(aliveNodes),
		TotalSources: int(totalSources),
		DeadSources:  int(deadSources),
	})
}

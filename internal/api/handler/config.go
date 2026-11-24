package handler

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	api "github.com/Eiyuuzaki/paily-on-nas/internal/api/generated"
	"github.com/Eiyuuzaki/paily-on-nas/internal/api/response"
	"github.com/Eiyuuzaki/paily-on-nas/internal/db/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// numericConfigRules bounds the config keys that drive a duration or a
// cleanup cutoff. Both failure modes are silent until they hurt:
//
//   - a NEGATIVE window moves the cutoff into the FUTURE (now.AddDate(0,0,-(-5))
//     is five days ahead), so the next prune deletes the entire table;
//   - a NON-POSITIVE cleanup interval panics time.NewTicker, and it is called
//     from a background goroutine with no recover, so the whole process dies.
//
// allowZero is true where 0 is the documented way to disable a rule.
var numericConfigRules = []struct {
	key       string
	allowZero bool
}{
	{"cleanup_interval_minutes", false},
	{"fetch_history_window_days", true},
	{"fetch_dead_window_days", true},
	{"check_run_history_days", true},
	{"node_dead_window_days", true},
	{"node_never_alive_retention_days", true},
	{"node_dead_retention_days", true},
	{"node_exhausted_archive_hours", true},
	{"node_never_alive_min_checks", true},
	{"node_exhausted_min_checks", true},
}

// validateNumericConfigs rejects unparsable, negative, or (where required)
// non-positive values for the keys in numericConfigRules. Only keys present in
// the request are checked; untouched keys keep whatever the DB already holds.
func validateNumericConfigs(incoming map[string]string) error {
	for _, rule := range numericConfigRules {
		v, present := incoming[rule.key]
		if !present {
			continue
		}
		n, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil {
			return fmt.Errorf("config %q must be an integer, got %q", rule.key, v)
		}
		if n < 0 {
			return fmt.Errorf("config %q must not be negative: a negative window moves the cleanup cutoff into the future and would delete every row", rule.key)
		}
		if !rule.allowZero && n == 0 {
			return fmt.Errorf("config %q must be greater than zero", rule.key)
		}
	}
	return nil
}

// windowConstraints lists pairs where the history (cleanup) window must be >=
// the dead-evaluation window so the cleanup goroutine never deletes records
// that the dead-evaluation logic still needs.
var windowConstraints = [][2]string{
	{"fetch_history_window_days", "fetch_dead_window_days"},
	{"check_run_history_days", "node_dead_window_days"},
}

// validateWindowConstraints checks that all history windows are >= their paired
// dead-eval windows given the effective merged config (current DB values
// overridden by the incoming update request).
func validateWindowConstraints(db *gorm.DB, incoming map[string]string) error {
	// Build effective values: start with DB values, override with incoming.
	effective := make(map[string]int)
	for _, pair := range windowConstraints {
		for _, key := range pair {
			if _, ok := effective[key]; ok {
				continue
			}
			if v, inReq := incoming[key]; inReq {
				n, err := strconv.Atoi(v)
				// 0 is the documented way to disable these rules: all four window keys
				// are marked allowZero in numericConfigRules. Rejecting 0 here made that
				// impossible through PUT /api/v1/config, which is the only entry point -
				// validateNumericConfigs accepted the value and this check then refused
				// it, so the two validators disagreed about the same key.
				if err != nil || n < 0 {
					return fmt.Errorf("config %q must not be negative", key)
				}
				effective[key] = n
			} else {
				var cfg model.Config
				if err := db.Where("key = ?", key).First(&cfg).Error; err == nil {
					if n, err2 := strconv.Atoi(cfg.Value); err2 == nil {
						effective[key] = n
					}
				}
			}
		}
	}

	for _, pair := range windowConstraints {
		histKey, deadKey := pair[0], pair[1]
		histVal, histOK := effective[histKey]
		deadVal, deadOK := effective[deadKey]
		if histOK && deadOK && histVal < deadVal {
			return fmt.Errorf(
				"%s (%d) must be >= %s (%d): "+
					"the cleanup window must cover the dead-evaluation window",
				histKey, histVal, deadKey, deadVal,
			)
		}
	}
	return nil
}

// ConfigHandler implements GET/PUT /api/v1/config.
type ConfigHandler struct {
	db *gorm.DB
}

// Get handles GET /api/v1/config — returns the full KV config map.
func (h *ConfigHandler) Get(c *gin.Context) {
	var rows []model.Config
	if err := h.db.Find(&rows).Error; err != nil {
		response.InternalError(c, "failed to get configs")
		return
	}
	cm := make(api.ConfigMap, len(rows))
	for _, r := range rows {
		cm[r.Key] = r.Value
	}
	c.JSON(http.StatusOK, cm)
}

// Update handles PUT /api/v1/config — upserts each key/value pair and returns
// the full updated config map.
func (h *ConfigHandler) Update(c *gin.Context) {
	var req api.ConfigUpdateJSONRequestBody
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request body")
		return
	}
	// Validate and write inside one transaction: a half-applied batch would
	// leave the cleanup window shorter than the dead-evaluation window, and the
	// cleanup goroutine would then delete records the dead logic still needs.
	var validationErr error
	now := time.Now()
	txErr := h.db.WithContext(c).Transaction(func(tx *gorm.DB) error {
		if err := validateNumericConfigs(req); err != nil {
			validationErr = err
			return err
		}
		if err := validateWindowConstraints(tx, req); err != nil {
			validationErr = err
			return err
		}
		for k, v := range req {
			row := model.Config{Key: k, Value: v, UpdatedAt: now}
			if err := tx.Save(&row).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if txErr != nil {
		if validationErr != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": validationErr.Error()})
			return
		}
		response.InternalError(c, "failed to update config")
		return
	}
	// Return the full config after update.
	var rows []model.Config
	if err := h.db.Find(&rows).Error; err != nil {
		response.InternalError(c, "failed to get configs")
		return
	}
	cm := make(api.ConfigMap, len(rows))
	for _, r := range rows {
		cm[r.Key] = r.Value
	}
	c.JSON(http.StatusOK, cm)
}

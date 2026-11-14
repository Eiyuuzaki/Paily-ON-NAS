package handler

import (
	"bytes"
	"io"
	"encoding/json"
	"fmt"
	"os"
	"math"
	"net/http"
	"path/filepath"
	neturl "net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/Eiyuuzaki/paily-on-nas/internal/api/response"
	"github.com/Eiyuuzaki/paily-on-nas/internal/db/model"
	"gorm.io/gorm"
)

const cfModuleConfigKey = "cf_module"

// CFHandler implements the CF module endpoints (hand-registered, not in
// the openapi spec - this is an opt-in advanced module).
type CFHandler struct {
	db *gorm.DB
}

func NewCFHandler(db *gorm.DB) *CFHandler { return &CFHandler{db: db} }

type cfModuleState struct {
	HasToken bool   `json:"has_token"`
	Activated   bool   `json:"activated"`
	RepoURL     string `json:"repo_url"`
	ActivatedAt string `json:"activated_at,omitempty"`
}

func (h *CFHandler) loadState(c *gin.Context) cfModuleState {
	var st cfModuleState
	var rec model.Config
	if err := h.db.WithContext(c).Where("key = ?", cfModuleConfigKey).First(&rec).Error; err == nil {
		_ = json.Unmarshal([]byte(rec.Value), &st)
	}
	return st
}

func (h *CFHandler) saveState(c *gin.Context, st cfModuleState) error {
	b, _ := json.Marshal(st)
	return h.db.WithContext(c).Exec(
		"INSERT INTO configs (key, value, updated_at) VALUES (?, ?, now()) " +
		"ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now()",
		cfModuleConfigKey, string(b)).Error
}
// Status handles GET /api/v1/cf/module.
func (h *CFHandler) Status(c *gin.Context) {
	st := h.loadState(c)
	response.OK(c, gin.H{"activated": st.Activated, "repo_url": st.RepoURL})
}

// Activate handles POST /api/v1/cf/module/activate.
// Only stored; the repo URL is a provenance marker for the worker source,
// never fetched server-side (GitHub may be unreachable from the NAS).
func (h *CFHandler) Activate(c *gin.Context) {
	var req struct {
		RepoURL string `json:"repo_url"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.RepoURL) == "" {
		response.BadRequest(c, "repo_url is required")
		return
	}
	url := strings.TrimSpace(req.RepoURL)
	if !strings.HasPrefix(url, "https://github.com/") {
		response.BadRequest(c, "repo_url must be a GitHub repository URL")
		return
	}
	st := cfModuleState{Activated: true, RepoURL: url, ActivatedAt: time.Now().UTC().Format(time.RFC3339)}
	if err := h.saveState(c, st); err != nil {
		response.InternalError(c, "failed to save activation")
		return
	}
	response.OK(c, gin.H{"activated": true, "repo_url": url})
}

// Deactivate handles POST /api/v1/cf/module/deactivate.
func (h *CFHandler) Deactivate(c *gin.Context) {
	st := h.loadState(c)
	st.Activated = false
	if err := h.saveState(c, st); err != nil {
		response.InternalError(c, "failed to save state")
		return
	}
	response.OK(c, gin.H{"activated": false})
}

// ListAccounts handles GET /api/v1/cf/accounts.
func (h *CFHandler) ListAccounts(c *gin.Context) {
	var rows []model.CFAccount
	if err := h.db.WithContext(c).Order("created_at ASC").Find(&rows).Error; err != nil {
		response.InternalError(c, "failed to load accounts")
		return
	}
	response.OK(c, gin.H{"data": rows})
}
// CreateAccount handles POST /api/v1/cf/accounts.
func (h *CFHandler) CreateAccount(c *gin.Context) {
	var req model.CFAccount
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request body")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		response.BadRequest(c, "name is required")
		return
	}
	req.Role = strings.ToLower(strings.TrimSpace(req.Role))
	if req.Role == "" { req.Role = "backup" }
	if req.Health == "" { req.Health = "unknown" }
	if err := h.db.WithContext(c).Create(&req).Error; err != nil {
		response.InternalError(c, "failed to create account")
		return
	}
	response.OK(c, req)
}

// UpdateAccount handles PUT /api/v1/cf/accounts/{id}.
func (h *CFHandler) UpdateAccount(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid id")
		return
	}
	var row model.CFAccount
	if err := h.db.WithContext(c).First(&row, "id = ?", id).Error; err != nil {
		response.NotFound(c, "account not found")
		return
	}
	var req struct {
		Name       *string `json:"name"`
		Role       *string `json:"role"`
		WorkerURL  *string `json:"worker_url"`
		WorkerName *string `json:"worker_name"`
		Subdomain  *string `json:"subdomain"`
		UUID       *string `json:"uuid"`
		RepoURL    *string `json:"repo_url"`
		Enabled    *bool   `json:"enabled"`
		Note       *string `json:"note"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request body")
		return
	}
	if req.Name != nil { row.Name = *req.Name }
	if req.Role != nil { row.Role = *req.Role }
	if req.WorkerURL != nil { row.WorkerURL = *req.WorkerURL }
	if req.WorkerName != nil { row.WorkerName = *req.WorkerName }
	if req.Subdomain != nil { row.Subdomain = *req.Subdomain }
	if req.UUID != nil { row.UUID = *req.UUID }
	if req.RepoURL != nil { row.RepoURL = *req.RepoURL }
	if req.Enabled != nil { row.Enabled = *req.Enabled }
	if req.Note != nil { row.Note = *req.Note }
	if err := h.db.WithContext(c).Save(&row).Error; err != nil {
		response.InternalError(c, "failed to save account")
		return
	}
	response.OK(c, row)
}

// DeleteAccount handles DELETE /api/v1/cf/accounts/{id}.
func (h *CFHandler) DeleteAccount(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid id")
		return
	}
	if err := h.db.WithContext(c).Delete(&model.CFAccount{}, "id = ?", id).Error; err != nil {
		response.InternalError(c, "failed to delete account")
		return
	}
	// Remove the credential too: an orphan token file would keep a deleted
	// account's Cloudflare access usable on disk.
	_ = os.Remove(cfTokenPath(id))
	response.OK(c, gin.H{"deleted": true})
}
// CheckAccount handles POST /api/v1/cf/accounts/{id}/check - probes the
// worker root URL server-side and records health. A single safe GET.
func (h *CFHandler) CheckAccount(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid id")
		return
	}
	var row model.CFAccount
	if err := h.db.WithContext(c).First(&row, "id = ?", id).Error; err != nil {
		response.NotFound(c, "account not found")
		return
	}
	// 健康判定 = 节点池聚合（语义正确：号是节点池的容器，节点活即号活）。
	// 直连 WorkerURL 探测在 SNI 黑名单时代不可信（GFW 封 workers.dev/dpdns.org，
	// 客户端走 ECH 能通但直连必 RST），只作辅助信号。
	// An empty worker URL would turn the LIKE pattern into '%%' and count every
	// node in the pool as belonging to this account.
	host := extractWorkerHost(row.WorkerURL)
	if host == "" {
		response.BadRequest(c, "worker_url is not configured for this account")
		return
	}
	pattern := "%" + host + "%"
	var aliveCnt, totalCnt int64
	if err := h.db.WithContext(c).Model(&model.Node{}).
		Where("archived_at IS NULL AND raw LIKE ?", pattern).
		Count(&totalCnt).Error; err != nil {
		response.InternalError(c, "failed to count nodes")
		return
	}
	if err := h.db.WithContext(c).Model(&model.Node{}).
		Where("archived_at IS NULL AND alive = true AND raw LIKE ?", pattern).
		Count(&aliveCnt).Error; err != nil {
		response.InternalError(c, "failed to count alive nodes")
		return
	}
	health := "down"
	if aliveCnt > 0 {
		health = "ok"
		if aliveCnt < totalCnt {
			health = "degraded"
		}
	}
	now := time.Now()
	if err := h.db.WithContext(c).Model(&row).
		Updates(map[string]interface{}{"health": health, "last_check_at": now}).Error; err != nil {
		response.InternalError(c, "failed to record health")
		return
	}
	response.OK(c, gin.H{"id": id, "health": health, "checked_at": now})
}
// extractWorkerHost pulls the host part of a worker URL for node raw matching.
func extractWorkerHost(workerURL string) string {
	s := strings.TrimPrefix(workerURL, "https://")
	s = strings.TrimPrefix(s, "http://")
	if i := strings.IndexAny(s, "/:"); i >= 0 {
		s = s[:i]
	}
	return s
}

// Summary handles GET /api/v1/cf/summary - aggregates pool stats,
// per-account node counts, probe burn and quota countdown for the panel.
func (h *CFHandler) Summary(c *gin.Context) {
	// 1. accounts
	var accounts []model.CFAccount
	h.db.WithContext(c).Order("created_at ASC").Find(&accounts)

	type accInfo struct {
		model.CFAccount
		NodeAlive int `json:"node_alive"`
		NodeTotal int `json:"node_total"`
	}
	infos := make([]accInfo, 0, len(accounts))
	cfAlive, cfTotal := 0, 0
	for _, a := range accounts {
		info := accInfo{CFAccount: a}
		if a.WorkerURL != "" {
			domain := strings.TrimPrefix(strings.TrimPrefix(a.WorkerURL, "https://"), "http://")
			domain = strings.TrimSuffix(domain, "/")
			type cnt struct{ Alive, Total int }
			var cc cnt
			h.db.WithContext(c).Raw(
				"SELECT count(*) FILTER (WHERE alive) AS alive, count(*) AS total FROM nodes " +
				"WHERE archived_at IS NULL AND (server LIKE ? OR raw LIKE ?)",
				"%"+domain+"%", "%"+domain+"%").Scan(&cc)
			info.NodeAlive = cc.Alive
			info.NodeTotal = cc.Total
			cfAlive += cc.Alive
			cfTotal += cc.Total
		}
		infos = append(infos, info)
	}

	// 2. today (UTC) probe burn on CF nodes
	var probes int64
	h.db.WithContext(c).Raw(
		"SELECT count(*) FROM node_initial_checks ic JOIN nodes n ON n.id = ic.node_id " +
		"WHERE ic.checked_at > (date_trunc('day', now() AT TIME ZONE 'UTC') AT TIME ZONE 'UTC') " +
		"AND (n.server LIKE '%lencf%' OR n.raw LIKE '%CF-HKG%' OR n.raw LIKE '%CF-LHR%' OR n.raw LIKE '%dcf1%')").Scan(&probes)

	// 3. quota reset = next UTC midnight
	now := time.Now().UTC()
	reset := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	if !reset.After(now) { reset = reset.AddDate(0, 0, 1) }
	hoursLeft := reset.Sub(now).Hours()

	// 4. ech toggle
	var ech string
	h.db.WithContext(c).Raw("SELECT value FROM configs WHERE key = 'ech_enabled'").Scan(&ech)

	// CF 节点的出口地区分布（alive 且带 CF 特征的节点，按 region 统计）
	type regionRow struct {
		Region string `json:"region"`
		Cnt    int64  `json:"cnt"`
	}
	var regions []regionRow
	h.db.WithContext(c).Raw(
		"SELECT CASE WHEN region IS NULL OR region = '' THEN '其他' ELSE region END AS region, COUNT(*) AS cnt FROM nodes " +
		"WHERE alive = true AND archived_at IS NULL AND (raw LIKE '%lencf%' OR raw LIKE '%dcf1%') " +
		"GROUP BY region ORDER BY cnt DESC LIMIT 8").Scan(&regions)

	response.OK(c, gin.H{
		"accounts":      infos,
		"cf_alive":      cfAlive,
		"cf_total":      cfTotal,
		"probes_today":  probes,
		"quota_daily":   100000,
		"quota_reset_hours": math.Round(hoursLeft*10) / 10,
		"ech_enabled":   ech == "1",
		"egress_regions": regions,
	})
}

// cfTokenDir is where per-account Cloudflare Analytics tokens live.
// It is overridable so the container path matches the Compose bind mount: a
// hardcoded host path silently wrote into the container's ephemeral layer and
// the token was lost on every recreate.
func cfTokenDir() string {
	if dir := strings.TrimSpace(os.Getenv("CF_TOKEN_DIR")); dir != "" {
		return dir
	}
	// Default lives under the app directory so it lines up with the Compose
	// bind mount. Override with CF_TOKEN_DIR for a custom location.
	return "/app/tokens"
}

// cfTokenPath returns the token file path for one account. The id must already
// be a parsed UUID so it can never escape the token directory.
func cfTokenPath(id uuid.UUID) string {
	return filepath.Join(cfTokenDir(), id.String()+".token")
}

// writeTokenFile replaces the token atomically and keeps 0600 permissions.
func writeTokenFile(id uuid.UUID, token string) error {
	dir := cfTokenDir()
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".token-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.WriteString(strings.TrimSpace(token)); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, cfTokenPath(id))
}

// SetToken handles POST /api/v1/cf/accounts/{id}/token.
// Per-account Cloudflare Analytics token, written to a root-only file.
func (h *CFHandler) SetToken(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid id")
		return
	}
	var body struct {
		Token string `json:"token"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || strings.TrimSpace(body.Token) == "" {
		response.BadRequest(c, "token is required")
		return
	}
	var acc model.CFAccount
	if err := h.db.WithContext(c).First(&acc, "id = ?", id).Error; err != nil {
		response.NotFound(c, "account not found")
		return
	}
	if err := writeTokenFile(id, body.Token); err != nil {
		response.InternalError(c, "write token file failed: "+err.Error())
		return
	}
	note := acc.Note
	if note != "" {
		note += " | "
	}
	// 记录 has_token 标记（不写 token 本体）
	note += "has_token=true"
	if err := h.db.WithContext(c).Model(&acc).Update("note", note).Error; err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.OK(c, gin.H{"account_id": id.String(), "has_token": true})
}

// TokenStatus handles GET /api/v1/cf/accounts/{id}/token.
func (h *CFHandler) TokenStatus(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid id")
		return
	}
	_, statErr := os.Stat(cfTokenPath(id))
	response.OK(c, gin.H{"account_id": id.String(), "has_token": statErr == nil})
}




// SelfCheck handles POST /api/v1/cf/selfcheck.
// Performs a 4-way egress self-test from the NAS host, mirroring the
// edgetunnel panel pattern: direct-CN, direct-CF(landing), proxied, wall test.
func (h *CFHandler) SelfCheck(c *gin.Context) {
	client := &http.Client{Timeout: 8 * time.Second}
	type checkResult struct {
		Name    string `json:"name"`
		OK      bool   `json:"ok"`
		IP      string `json:"ip,omitempty"`
		Loc     string `json:"loc,omitempty"`
		Colo    string `json:"colo,omitempty"`
		Latency int    `json:"latency_ms"`
		Detail  string `json:"detail,omitempty"`
	}
	do := func(name, url string, useProxy bool) checkResult {
		res := checkResult{Name: name}
		req, _ := http.NewRequestWithContext(c.Request.Context(), "GET", url, nil)
		start := time.Now()
		var resp *http.Response
		var err error
		if useProxy {
			proxyURL, perr := neturl.Parse("http://172.17.0.1:7890")
			if perr == nil {
				tr := &http.Transport{Proxy: http.ProxyURL(proxyURL)}
				resp, err = (&http.Client{Timeout: 8 * time.Second, Transport: tr}).Do(req)
			} else {
				err = perr
			}
		} else {
			resp, err = client.Do(req)
		}
		res.Latency = int(time.Since(start).Milliseconds())
		if err != nil {
			res.Detail = err.Error()
			return res
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		res.OK = resp.StatusCode == http.StatusOK
		// cloudflare trace format: ip=..&loc=..&colo=..
		for _, kv := range strings.Split(string(body), "\n") {
			parts := strings.SplitN(strings.TrimSpace(kv), "=", 2)
			if len(parts) == 2 {
				switch parts[0] {
				case "ip":
					res.IP = parts[1]
				case "loc":
					res.Loc = parts[1]
				case "colo":
					res.Colo = parts[1]
				}
			}
		}
		if res.IP == "" && res.OK {
			res.Detail = strings.TrimSpace(string(body))[:min(60, len(strings.TrimSpace(string(body))))]
		}
		return res
	}
	r1 := make(chan checkResult, 1)
	r2 := make(chan checkResult, 1)
	r3 := make(chan checkResult, 1)
	go func() { r1 <- do("direct_cn", "https://www.cloudflare.com/cdn-cgi/trace", false) }()
	go func() { r2 <- do("direct_cf", "https://www.cloudflare.com/cdn-cgi/trace", false) }()
	go func() { r3 <- do("proxied", "https://www.cloudflare.com/cdn-cgi/trace", true) }()
	results := []checkResult{<-r1, <-r2, <-r3}
	response.OK(c, gin.H{"results": results, "checked_at": time.Now()})
}

// AccountUsage handles GET /api/v1/cf/accounts/{id}/usage.
// Reads the account's Analytics token file, resolves its CF accountTag,
// queries workersInvocationsAdaptive for today (UTC) totals and hourly
// distribution. Cached for 5 minutes to avoid hammering the CF API.
func (h *CFHandler) AccountUsage(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid id")
		return
	}
	tb, err := os.ReadFile(cfTokenPath(id))
	if err != nil {
		response.NotFound(c, "token not configured for this account")
		return
	}
	token := strings.TrimSpace(string(tb))
	// 5-min cache in configs table
	var cached string
	cacheKey := "cf_usage_" + id.String()
	h.db.WithContext(c).Raw("SELECT value FROM configs WHERE key = ?", cacheKey).Scan(&cached)
	if cached != "" {
		var entry struct {
			FetchedAt time.Time       `json:"fetched_at"`
			Payload   json.RawMessage `json:"payload"`
		}
		if json.Unmarshal([]byte(cached), &entry) == nil && time.Since(entry.FetchedAt) < 5*time.Minute {
			var payload map[string]any
			json.Unmarshal(entry.Payload, &payload)
			response.OK(c, payload)
			return
		}
	}
	client := &http.Client{Timeout: 20 * time.Second}
	// 1) resolve accountTag
	req, _ := http.NewRequestWithContext(c.Request.Context(), "GET", "https://api.cloudflare.com/client/v4/accounts", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := client.Do(req)
	if err != nil {
		response.InternalError(c, "CF accounts API failed: "+err.Error())
		return
	}
	var accts struct {
		Result []struct {
			ID string `json:"id"`
		} `json:"result"`
	}
	json.NewDecoder(resp.Body).Decode(&accts)
	resp.Body.Close()
	if len(accts.Result) == 0 {
		response.InternalError(c, "token has no account access")
		return
	}
	atag := accts.Result[0].ID
	// 2) GraphQL: today totals + hourly
	todayUTC := time.Now().UTC().Format("2006-01-02T00:00:00Z")
	gqlQuery := fmt.Sprintf(`{ viewer { accounts(filter: {accountTag: "%s"}) { workersInvocationsAdaptive(limit: 24, filter: {datetime_geq: "%s"}) { dimensions { datetimeHour } sum { requests } } } } }`, atag, todayUTC)
	gqlBody, _ := json.Marshal(map[string]string{"query": gqlQuery})
	gqlReq, _ := http.NewRequestWithContext(c.Request.Context(), "POST",
		"https://api.cloudflare.com/client/v4/graphql", bytes.NewReader(gqlBody))
	gqlReq.Header.Set("Authorization", "Bearer "+token)
	gqlReq.Header.Set("Content-Type", "application/json")
	gresp, err := client.Do(gqlReq)
	if err != nil {
		response.InternalError(c, "GraphQL failed: "+err.Error())
		return
	}
	defer gresp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(gresp.Body, 16384))

	var gresp2 struct {
		Data struct {
			Viewer struct {
				Accounts []struct {
					WorkersInvocationsAdaptive []struct {
						Dimensions struct {
							DatetimeHour string `json:"datetimeHour"`
						} `json:"dimensions"`
						Sum struct {
							Requests int64 `json:"requests"`
						} `json:"sum"`
					} `json:"workersInvocationsAdaptive"`
				} `json:"accounts"`
			} `json:"viewer"`
		} `json:"data"`
	}
	json.Unmarshal(body, &gresp2)
	hourly := []gin.H{}
	total := int64(0)
	if len(gresp2.Data.Viewer.Accounts) > 0 {
		for _, u := range gresp2.Data.Viewer.Accounts[0].WorkersInvocationsAdaptive {
			hourly = append(hourly, gin.H{"hour": u.Dimensions.DatetimeHour, "requests": u.Sum.Requests})
			total += u.Sum.Requests
		}
	}
	payload := gin.H{
		"total_today": total,
		"quota_daily": 100000,
		"percent":     float64(total) / 1000.0 / 100.0,
		"hourly":      hourly,
		"account_tag": atag,
	}
	// cache
	cb, _ := json.Marshal(map[string]any{"fetched_at": time.Now(), "payload": payload})
	h.db.WithContext(c).Exec(
		"INSERT INTO configs (key, value, updated_at) VALUES (?, ?, now()) "+
		"ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now()",
		cacheKey, string(cb))
	response.OK(c, payload)
}
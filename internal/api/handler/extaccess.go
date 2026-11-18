package handler

import (
	"crypto/rand"
	"encoding/base64"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/Eiyuuzaki/paily-on-nas/internal/api/response"
	"gorm.io/gorm"
)

const subscribeTokenKey = "subscribe_token"
const extAccessHostKey = "extaccess_public_host"

// SubscribeTokenManager is the runtime source of truth for the subscription
// access token. The yaml value (auth.subscribe_token) is the boot-time
// fallback: if the configs table has no row yet it is seeded from yaml, after
// which the panel owns the value. Reads are cached for 10s so the hot
// subscription path does not hit the database on every request.
type SubscribeTokenManager struct {
	db       *gorm.DB
	yamlVal  string
	mu       sync.RWMutex
	cache    string
	cachedAt time.Time
}

func NewSubscribeTokenManager(db *gorm.DB, yamlVal string) *SubscribeTokenManager {
	return &SubscribeTokenManager{db: db, yamlVal: yamlVal}
}

// Get returns the active token: configs table first, yaml fallback second.
// An explicitly stored empty value means open access — callers keep firing
// their boot-time warning in that case.
func (m *SubscribeTokenManager) Get() string {
	m.mu.RLock()
	if time.Since(m.cachedAt) < 10*time.Second {
		v := m.cache
		m.mu.RUnlock()
		return v
	}
	m.mu.RUnlock()

	var stored string
	res := m.db.Raw("SELECT value FROM configs WHERE key = ?", subscribeTokenKey).Scan(&stored)
	if res.Error == nil && res.RowsAffected > 0 {
		m.mu.Lock()
		m.cache, m.cachedAt = stored, time.Now()
		m.mu.Unlock()
		return stored
	}
	// key missing: seed the table from the yaml value so the panel sees it.
	if m.yamlVal != "" {
		m.db.Exec("INSERT INTO configs (key, value, updated_at) VALUES (?, ?, now()) ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now()",
			subscribeTokenKey, m.yamlVal)
	}
	m.mu.Lock()
	m.cache, m.cachedAt = m.yamlVal, time.Now()
	m.mu.Unlock()
	return m.yamlVal
}

// Set stores a new token and refreshes the read cache immediately so the next
// subscription request is judged against the new value.
func (m *SubscribeTokenManager) Set(tok string) error {
	if err := m.db.Exec("INSERT INTO configs (key, value, updated_at) VALUES (?, ?, now()) ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now()",
		subscribeTokenKey, tok).Error; err != nil {
		return err
	}
	m.mu.Lock()
	m.cache, m.cachedAt = tok, time.Now()
	m.mu.Unlock()
	return nil
}

func (m *SubscribeTokenManager) publicHost() string {
	var host string
	m.db.Raw("SELECT value FROM configs WHERE key = ?", extAccessHostKey).Scan(&host)
	if host == "" {
		host = "sub.lencf.dpdns.org"
	}
	return host
}

func subURLs(host, token string) map[string]string {
	return map[string]string{
		"clash":   "https://" + host + "/clash?token=" + token,
		"singbox": "https://" + host + "/singbox?token=" + token,
		"base64":  "https://" + host + "/base64?token=" + token,
	}
}

// ExtAccessHandler serves the panel's external-access card: subscription
// token management and the public subscription URLs. Hand-registered like
// the CF module (not in the openapi spec).
type ExtAccessHandler struct {
	db  *gorm.DB
	mgr *SubscribeTokenManager
}

func NewExtAccessHandler(db *gorm.DB, mgr *SubscribeTokenManager) *ExtAccessHandler {
	return &ExtAccessHandler{db: db, mgr: mgr}
}

// GET /api/v1/extaccess — full token included: the card copies URLs for the
// admin, and the route is behind adminJWT.
func (h *ExtAccessHandler) Root(c *gin.Context) {
	tok := h.mgr.Get()
	host := h.mgr.publicHost()
	response.OK(c, gin.H{
		"token":       tok,
		"enabled":     tok != "",
		"public_host": host,
		"urls":        subURLs(host, tok),
		"lan_port":    21380,
	})
}

type extSetTokenReq struct {
	Token string `json:"token"`
}

// PUT /api/v1/extaccess/token
func (h *ExtAccessHandler) SetToken(c *gin.Context) {
	var req extSetTokenReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid body")
		return
	}
	tok := strings.TrimSpace(req.Token)
	if len(tok) < 16 {
		response.BadRequest(c, "token must be at least 16 characters")
		return
	}
	if err := h.mgr.Set(tok); err != nil {
		response.InternalError(c, "failed to store token: "+err.Error())
		return
	}
	response.OK(c, gin.H{"token": tok})
}

// POST /api/v1/extaccess/token/rotate — generate a 43-char urlsafe token.
func (h *ExtAccessHandler) RotateToken(c *gin.Context) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		response.InternalError(c, "rand failed: "+err.Error())
		return
	}
	tok := base64.RawURLEncoding.EncodeToString(buf)
	if err := h.mgr.Set(tok); err != nil {
		response.InternalError(c, "failed to store token: "+err.Error())
		return
	}
	response.OK(c, gin.H{"token": tok})
}

type extSetHostReq struct {
	PublicHost string `json:"public_host"`
}

// PUT /api/v1/extaccess/host
func (h *ExtAccessHandler) SetHost(c *gin.Context) {
	var req extSetHostReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid body")
		return
	}
	host := strings.TrimPrefix(strings.TrimSpace(req.PublicHost), "https://")
	host = strings.TrimSuffix(host, "/")
	if host == "" || strings.ContainsAny(host, " /\\") {
		response.BadRequest(c, "invalid host")
		return
	}
	if err := h.db.Exec("INSERT INTO configs (key, value, updated_at) VALUES (?, ?, now()) ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now()",
		extAccessHostKey, host).Error; err != nil {
		response.InternalError(c, "failed to store host: "+err.Error())
		return
	}
	response.OK(c, gin.H{"public_host": host})
}

// POST /api/v1/extaccess/test — server-side probe of the public URL. The
// browser cannot test it directly (cross-origin without CORS on the tunnel),
// so core does the fetch and reports status plus latency.
func (h *ExtAccessHandler) Test(c *gin.Context) {
	host := h.mgr.publicHost()
	tok := h.mgr.Get()
	client := &http.Client{Timeout: 20 * time.Second}
	start := time.Now()
	req, _ := http.NewRequestWithContext(c.Request.Context(), http.MethodGet,
		"https://"+host+"/clash?token="+tok, nil)
	resp, err := client.Do(req)
	if err != nil {
		response.OK(c, gin.H{
			"ok": false, "http_code": 0,
			"latency_ms": time.Since(start).Milliseconds(),
			"error":      err.Error(),
		})
		return
	}
	defer resp.Body.Close()
	buf := make([]byte, 512)
	_, _ = io.ReadFull(resp.Body, buf)
	response.OK(c, gin.H{
		"ok":         resp.StatusCode == http.StatusOK,
		"http_code":  resp.StatusCode,
		"latency_ms": time.Since(start).Milliseconds(),
	})
}

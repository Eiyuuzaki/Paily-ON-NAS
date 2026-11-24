package handler

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/Eiyuuzaki/paily-on-nas/internal/api/response"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
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
	db         *gorm.DB
	yamlVal    string
	mu         sync.RWMutex
	cache      string
	cachedAt   time.Time
	cacheValid bool
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
	if res.Error != nil {
		log.Error().Err(res.Error).Msg("subscribe token: config read failed")
		m.mu.RLock()
		valid, cached := m.cacheValid, m.cache
		m.mu.RUnlock()
		if valid {
			return cached
		}
		if m.yamlVal != "" {
			return m.yamlVal
		}
		return "unavailable-paily-token"
	}
	if res.RowsAffected > 0 {
		m.mu.Lock()
		m.cache, m.cachedAt, m.cacheValid = stored, time.Now(), true
		m.mu.Unlock()
		return stored
	}
	// key missing: seed the table from the yaml value so the panel sees it.
	if m.yamlVal != "" {
		m.db.Exec("INSERT INTO configs (key, value, updated_at) VALUES (?, ?, now()) ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now()",
			subscribeTokenKey, m.yamlVal)
	}
	m.mu.Lock()
	m.cache, m.cachedAt, m.cacheValid = m.yamlVal, time.Now(), true
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
	m.cache, m.cachedAt, m.cacheValid = tok, time.Now(), true
	m.mu.Unlock()
	return nil
}

// defaultPublicHost is the fallback when no host has been configured yet.
// It is only for "never set"; a failed read must not silently land here, or the
// panel shows a real-looking domain the deployment does not actually answer on.
const defaultPublicHost = "sub.lencf.dpdns.org"

// publicHost returns the configured public host.
//
// The error is returned rather than folded into the fallback: /extaccess/test
// probes whatever this returns, so a silent fallback made the operator see
// "unreachable" for a domain problem that was really a database one.
func (m *SubscribeTokenManager) publicHost() (string, error) {
	var host string
	if err := m.db.Raw("SELECT value FROM configs WHERE key = ?", extAccessHostKey).Scan(&host).Error; err != nil {
		return "", fmt.Errorf("read %s: %w", extAccessHostKey, err)
	}
	if host == "" {
		return defaultPublicHost, nil
	}
	return host, nil
}

func subURLs(host, token string) map[string]string {
	return map[string]string{
		"clash":   "https://" + host + "/clash?token=" + url.QueryEscape(token),
		"singbox": "https://" + host + "/singbox?token=" + url.QueryEscape(token),
		"base64":  "https://" + host + "/base64?token=" + url.QueryEscape(token),
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
	host, err := h.mgr.publicHost()
	if err != nil {
		response.InternalError(c, "failed to read public host")
		return
	}
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
	host, err := h.mgr.publicHost()
	if err != nil {
		response.InternalError(c, "failed to read public host")
		return
	}
	tok := h.mgr.Get()
	client := &http.Client{Timeout: 20 * time.Second}
	start := time.Now()
	// Checked: an empty or unparsable host yields a nil request, and client.Do(nil)
	// dereferences it. The old code discarded this error, so a host the operator
	// typed wrongly (SetHost only rejects empty and spaces) turned every test into
	// a panic that the recovery middleware reported as a 500 with a panic stack.
	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet,
		"https://"+host+"/clash?token="+url.QueryEscape(tok), nil)
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
	response.OK(c, gin.H{
		"ok":         resp.StatusCode == http.StatusOK,
		"http_code":  resp.StatusCode,
		"latency_ms": time.Since(start).Milliseconds(),
	})
}

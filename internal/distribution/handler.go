package distribution

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	api "github.com/Eiyuuzaki/paily-on-nas/internal/api/generated"
	"github.com/Eiyuuzaki/paily-on-nas/internal/cache"
	"github.com/Eiyuuzaki/paily-on-nas/internal/config"
	"github.com/Eiyuuzaki/paily-on-nas/internal/db/model"
	"github.com/Eiyuuzaki/paily-on-nas/internal/naming"
	"github.com/Eiyuuzaki/paily-on-nas/third_party/paily-fetch/format"
	node "github.com/Eiyuuzaki/paily-on-nas/third_party/paily-fetch/node"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

// Handler implements the GET /{format} distribution endpoint.
type Handler struct {
	cache *cache.AliveCache
	db    *gorm.DB
	cfg   *config.Config
}

// NewHandler creates a distribution Handler.
func NewHandler(c *cache.AliveCache, db *gorm.DB, cfg *config.Config) *Handler {
	return &Handler{cache: c, db: db, cfg: cfg}
}

// Distribute handles GET /{format} with region, filter_unlock, filter_mode params.
func (h *Handler) Distribute(
	c *gin.Context,
	formatName api.DistributeParamsFormat,
	params api.DistributeParams,
) {
	factory, ok := format.Get(string(formatName))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "unknown format: " + string(formatName)})
		return
	}

	// ── 1. Collect alive entries and apply unlock filter ──────────────────────
	tag := ""
	if params.Tag != nil && *params.Tag != "" {
		tag = *params.Tag
	}
	all := h.cache.ForTag(tag)

	// Unlock filter + mode.
	var requestedUnlocks []string
	if params.FilterUnlock != nil && *params.FilterUnlock != "" {
		for _, s := range strings.Split(*params.FilterUnlock, "|") {
			if t := strings.TrimSpace(s); t != "" {
				requestedUnlocks = append(requestedUnlocks, strings.ToLower(t))
			}
		}
	}

	filterMode := ""
	if params.FilterMode != nil {
		filterMode = string(*params.FilterMode)
	}

	var candidates []*cache.NodeCacheEntry
	for _, e := range all {
		// Entry-only nodes are relays, not exits: emitting them as standalone
		// proxies would expose the plaintext hop the exit filter rejects.
		if e.ChainEntryOnly {
			continue
		}
		// CN nodes are relays too: their egress is domestic, useless as exits,
		// and pushing the same node as both entry and exit renders duplicate
		// proxy names, which mihomo clients reject outright.
		if e.Region == "CN" {
			continue
		}
		if len(requestedUnlocks) > 0 {
			switch filterMode {
			case "select":
				// Intersection: node must unlock ALL requested services.
				if !nodeMatchesUnlocks(e, requestedUnlocks) {
					continue
				}
			case "mark":
				// Union: node must unlock AT LEAST ONE requested service.
				if !nodeMatchesAnyUnlock(e, requestedUnlocks) {
					continue
				}
			}
		}
		candidates = append(candidates, e)
	}

	// ── 2. Preset-based selection ─────────────────────────────────────────────
	// Priority: explicit param > tag default_preset > global default_preset > "common"
	preset := "common"
	if p := h.cfg.DefaultPreset; p != "" {
		preset = p
	}
	if tag != "" {
		for _, e := range h.cfg.Checkers {
			if e.Tag == tag && e.DefaultPreset != "" {
				preset = e.DefaultPreset
				break
			}
		}
	}
	if params.Preset != nil && params.Preset.Valid() {
		preset = string(*params.Preset)
	}
	maxDistribute := configInt(h.db, "max_distribute", 200)
	priorityRatio := configFloat(h.db, "priority_ratio", 0.6)
	selected := SelectWithPreset(candidates, maxDistribute, preset, priorityRatio)

	log.Debug().
		Str("format", string(formatName)).
		Int("candidates", len(candidates)).
		Int("selected", len(selected)).
		Msg("distribution: nodes selected")

	// ── 3. Build unlock flags and generate names, then push to generator ──────
	// Domestic (CN) survivors act as entry relays for the rest: a free node
	// reachable from a mainland line usually has to be dialled *through* a
	// domestic hop, otherwise the client times out and the node looks dead.
	// When no CN node survived, chainPlan is nil and the output is flat —
	// identical to the pre-chain behaviour.
	// Chain entries: every alive CN node, including entry-only ones that the
	// exit filter rejected (they exist purely to relay).
	cnEntries := make([]*cache.NodeCacheEntry, 0, len(all))
	for _, e := range all {
		if e.Region == "CN" {
			cnEntries = append(cnEntries, e)
		}
	}
	chainPlan, entryNames := buildChainPlan(
		c.Request.Context(), selected, cnEntries,
		chainRenderFunc(c.Request.Context(), h.db, preset, filterMode),
	)

	gen := factory.NewGenerator()
	// Entries are emitted first so every "dialer-proxy" target exists in the
	// output before the exits that reference it (mihomo resolves by name).
	for i, name := range entryNames {
		if i < len(cnEntries) {
			gen.Push(cnEntries[i].ProxyNode, name)
		}
	}
	// Resolve source origins once for the whole batch: a subscription may emit
	// hundreds of nodes, and a per-node query would mean hundreds of round-trips
	// on the hottest request path.
	originPrefix := h.originPrefixResolver(c.Request.Context(), selected)
	for i, sn := range selected {
		unlockFlags := assembleUnlockFlags(sn.Entry, requestedUnlocks, filterMode)
		name := originPrefix(sn.Entry) + " " + naming.Render(c.Request.Context(), sn.Entry, sn.SeqNo, unlockFlags, filterMode, preset, h.db)
		proxy := proxyWithChain(sn.Entry.ProxyNode, chainPlan[i])
		if rg, ok := gen.(interface {
			PushWithRegion(n *node.ProxyNode, name, region string)
		}); ok {
			rg.PushWithRegion(proxy, name, sn.Entry.Region)
		} else {
			gen.Push(proxy, name)
		}
	}

	// ── 4. Load format config and generate output ─────────────────────────────
	fmtConfig := loadFormatConfig(h.db, string(formatName), factory)

	output, err := gen.Generate(fmtConfig)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "generate failed: " + err.Error()})
		return
	}

	contentType := contentTypeFor(string(formatName))
	if filename := configString(h.db, "filename_"+string(formatName), ""); filename != "" {
		c.Header("Content-Disposition", contentDisposition(filename))
	}
	if ui := h.subscriptionUserinfo(); ui != "" {
		c.Header("Subscription-Userinfo", ui)
		c.Header("Profile-Update-Interval",
			strconv.Itoa(configInt(h.db, "profile_update_interval", 24)))
	}
	c.Data(http.StatusOK, contentType, output)
}

// subscriptionUserinfo aggregates airport quota data into the
// subscription-userinfo header format understood by Clash / Sing-box /
// Shadowrocket: upload=<B>; download=<B>; total=<B>; expire=<unix seconds>.
//
// Only sources that actually report a quota (total > 0) are counted — free
// GitHub / TG sources never consume any account quota, so mixing them in
// would dilute the signal. A shrinking displayed quota therefore means
// airport nodes are carrying traffic; zero movement means everything is
// riding free nodes. Expire is the soonest expiry across all quota-bearing
// sources, so the client warns before the earliest renewal is due.
// Returns "" when no source reports a quota (header omitted entirely —
// a "0B / 0B" display would be noise).

// originRank orders origins from most to least authoritative. A node linked to
// several sources is labelled with the best one (e.g. a paid-airport node that
// an aggregator also republishes is still [付费]).
var originRank = map[string]int{
	"self-built": 6, "paid-airport": 5, "public-airport": 4,
	"free-list": 3, "aggregator": 2, "wild": 1,
}

// originPrefixResolver loads every source origin referenced by entries in ONE
// query and returns a per-entry labeller.
//
// Distribution is the hottest request path (clients poll subscriptions), and a
// single response can contain hundreds of nodes, so the previous
// query-per-node implementation multiplied one request into hundreds of
// round-trips.
func (h *Handler) originPrefixResolver(ctx context.Context, selected []SelectedNode) func(*cache.NodeCacheEntry) string {
	seen := make(map[string]struct{}, 64)
	ids := make([]string, 0, 64)
	for _, sn := range selected {
		if sn.Entry == nil {
			continue
		}
		for _, id := range sn.Entry.SourceIDs {
			if _, ok := seen[id]; !ok {
				seen[id] = struct{}{}
				ids = append(ids, id)
			}
		}
	}
	origin := make(map[string]string, len(ids))
	isApex := make(map[string]bool, len(ids))
	if len(ids) > 0 {
		var rows []struct {
			ID         string
			Identifier string
			Origin     string
		}
		h.db.WithContext(ctx).Table("sources").
			Select("id, identifier, origin").Where("id IN ?", ids).Scan(&rows)
		for _, r := range rows {
			origin[r.ID] = r.Origin
			if r.Identifier == "cf-apex-sub" {
				isApex[r.ID] = true
			}
		}
	}

	return func(entry *cache.NodeCacheEntry) string {
		if len(entry.SourceIDs) == 0 {
			return "[免费]"
		}
		best, bestRank := "", -1
		for _, id := range entry.SourceIDs {
			if isApex[id] {
				return "[CF]"
			}
			if r := originRank[origin[id]]; r > bestRank {
				bestRank, best = r, origin[id]
			}
		}
		switch best {
		case "self-built":
			if strings.Contains(entry.Raw, "ech-opts") {
				return "[自建🔒]"
			}
			return "[自建]"
		case "paid-airport":
			return "[付费]"
		case "public-airport":
			return "[公益]"
		case "free-list":
			return "[一手]"
		case "aggregator":
			return "[聚合]"
		case "wild":
			return "[打野]"
		}
		return "[免费]"
	}
}
func (h *Handler) subscriptionUserinfo() string {
	var agg struct {
		Upload   int64
		Download int64
		Total    int64
		Expire   *int64
	}
	h.db.Raw(`
		SELECT COALESCE(SUM(upload), 0)   AS upload,
		       COALESCE(SUM(download), 0) AS download,
		       COALESCE(SUM(total), 0)    AS total,
		       MIN(CASE WHEN expire > 0 THEN expire END) AS expire
		FROM sources
		WHERE status = 'active' AND total > 0
	`).Scan(&agg)
	if agg.Total <= 0 {
		return ""
	}
	ui := fmt.Sprintf("upload=%d; download=%d; total=%d", agg.Upload, agg.Download, agg.Total)
	if agg.Expire != nil && *agg.Expire > 0 {
		ui += fmt.Sprintf("; expire=%d", *agg.Expire)
	}
	return ui
}

// ── helpers ───────────────────────────────────────────────────────────────────

// nodeMatchesAnyUnlock returns true if the node's StreamingResult unlocks AT
// LEAST ONE of the requested services (union semantics for mark mode).
func nodeMatchesAnyUnlock(e *cache.NodeCacheEntry, unlocks []string) bool {
	for _, u := range unlocks {
		for svc, ok := range e.StreamingResult {
			if ok && strings.EqualFold(svc, u) {
				return true
			}
		}
	}
	return false
}

// nodeMatchesUnlocks returns true if the node's StreamingResult unlocks ALL of
// the requested services (intersection semantics for select mode).
func nodeMatchesUnlocks(e *cache.NodeCacheEntry, unlocks []string) bool {
	if len(e.StreamingResult) == 0 {
		return false
	}
	for _, u := range unlocks {
		found := false
		for svc, ok := range e.StreamingResult {
			if ok && strings.EqualFold(svc, u) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// assembleUnlockFlags builds the unlock_flags string per §6 rules.
func assembleUnlockFlags(e *cache.NodeCacheEntry, requested []string, filterMode string) string {
	// No filterMode specified, or select-mode with ≤1 unlock requested → no flags shown.
	if filterMode == "" {
		return ""
	}
	if filterMode == "select" && len(requested) <= 1 {
		return ""
	}
	// Collect services actually unlocked by this node.
	var flags []string
	if len(requested) == 0 {
		// mark mode with no specific filter: show all unlocked services.
		for svc, ok := range e.StreamingResult {
			if ok {
				flags = append(flags, capitalizeFirst(svc))
			}
		}
	} else {
		for _, u := range requested {
			for svc, ok := range e.StreamingResult {
				if ok && strings.EqualFold(svc, u) {
					flags = append(flags, capitalizeFirst(svc))
					break
				}
			}
		}
	}
	return strings.Join(flags, " | ")
}

func capitalizeFirst(s string) string {
	if s == "" {
		return s
	}

	r, size := utf8.DecodeRuneInString(s)
	if r == utf8.RuneError && size == 1 {
		return s
	}

	return string(unicode.ToUpper(r)) + s[size:]
}

// loadFormatConfig reads a format's JSON config from DB, falling back to the factory default.
func loadFormatConfig(db *gorm.DB, formatName string, factory format.FormatFactory) format.FormatConfig {
	var row model.FormatConfig
	if err := db.First(&row, "format_name = ?", formatName).Error; err == nil && row.Config != "" {
		return format.FormatConfig(row.Config)
	}
	return factory.DefaultConfig()
}

// configInt reads an integer config value from the DB config table.
// configInt reads an integer config value from the DB config table.
func configInt(db *gorm.DB, key string, def int) int {
	var row model.Config
	if err := db.First(&row, "key = ?", key).Error; err != nil {
		return def
	}
	v, err := strconv.Atoi(row.Value)
	if err != nil {
		return def
	}
	return v
}

// configFloat reads a float config value from the DB config table.
func configFloat(db *gorm.DB, key string, def float64) float64 {
	var row model.Config
	if err := db.First(&row, "key = ?", key).Error; err != nil {
		return def
	}
	v, err := strconv.ParseFloat(row.Value, 64)
	if err != nil {
		return def
	}
	return v
}

// configString reads a string config value from the DB config table.
func configString(db *gorm.DB, key string, def string) string {
	var row model.Config
	if err := db.First(&row, "key = ?", key).Error; err != nil {
		return def
	}
	return row.Value
}

// contentDisposition builds a Content-Disposition header value that supports
// UTF-8 filenames per RFC 6266 / RFC 5987.
// Always emits both filename= (ASCII fallback) and filename*=UTF-8” (RFC 5987
// percent-encoded, no surrounding quotes) so that clients supporting RFC 5987
// never treat the surrounding double-quotes as part of the name - which fixes
// "Paily Connect" being displayed as '"Paily Connect"' by some subscription
// clients that do not strip RFC 2183 quoting from the plain filename= token.
func contentDisposition(filename string) string {
	// ASCII fallback: replace non-ASCII and control chars with '?'.
	ascii := strings.Map(func(r rune) rune {
		if r > 0x7e || r < 0x20 {
			return '?'
		}
		return r
	}, filename)

	// RFC 5987 percent-encoding: spaces become %20, no surrounding quotes.
	encoded := url.PathEscape(filename)

	// Always emit both tokens. RFC 5987-aware clients use the encoded form;
	// legacy clients fall back to the quoted filename= token.
	return `attachment; filename="` + ascii + `"; filename*=UTF-8''` + encoded
}

// contentTypeFor returns the appropriate HTTP Content-Type for a format.
func contentTypeFor(formatName string) string {
	switch formatName {
	case "clash":
		return "text/yaml; charset=utf-8"
	case "singbox":
		return "application/json; charset=utf-8"
	default:
		return "text/plain; charset=utf-8"
	}
}

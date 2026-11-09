package main

import (
	"context"
	"flag"
	"fmt"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	api "github.com/Eiyuuzaki/paily-on-nas/internal/api/generated"
	"github.com/Eiyuuzaki/paily-on-nas/internal/api/handler"
	"github.com/Eiyuuzaki/paily-on-nas/internal/api/middleware"
	"github.com/Eiyuuzaki/paily-on-nas/internal/auth"
	"github.com/Eiyuuzaki/paily-on-nas/internal/cache"
	"github.com/Eiyuuzaki/paily-on-nas/internal/checker"
	"github.com/Eiyuuzaki/paily-on-nas/internal/cleanup"
	"github.com/Eiyuuzaki/paily-on-nas/internal/config"
	"github.com/Eiyuuzaki/paily-on-nas/internal/db"
	"github.com/Eiyuuzaki/paily-on-nas/internal/db/model"
	"github.com/Eiyuuzaki/paily-on-nas/internal/fetcher"
	"github.com/Eiyuuzaki/paily-on-nas/internal/migrate"
	"github.com/Eiyuuzaki/paily-on-nas/internal/node"
	"github.com/Eiyuuzaki/paily-on-nas/internal/scoring"
	"github.com/Eiyuuzaki/paily-on-nas/internal/source"
	"github.com/Eiyuuzaki/paily-on-nas/internal/sponsor"
	"github.com/Eiyuuzaki/paily-on-nas/internal/templates"
	"github.com/Eiyuuzaki/paily-on-nas/internal/traffic"
	"github.com/Eiyuuzaki/paily-on-nas/third_party/paily-fetch/format"

	// Trigger format plugin registration via init().
	_ "github.com/Eiyuuzaki/paily-on-nas/third_party/paily-fetch/format/base64"
	_ "github.com/Eiyuuzaki/paily-on-nas/third_party/paily-fetch/format/clash"
	_ "github.com/Eiyuuzaki/paily-on-nas/third_party/paily-fetch/format/singbox"
)

func main() {
	cfgFile := flag.String("config", "", "path to config file (default: ./config.yaml)")
	migrateFlag := flag.Bool("migrate", false, "migrate config tables from SQLite to PostgreSQL, then exit")
	migrateFrom := flag.String("migrate-from", "", "path to source SQLite database file (required with -migrate)")
	flag.Parse()
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr}).Level(zerolog.InfoLevel)

	// ── Migration mode ────────────────────────────────────────────────────────
	if *migrateFlag {
		if *migrateFrom == "" {
			log.Fatal().Msg("-migrate-from is required when using -migrate")
		}
		cfg, err := config.Load(*cfgFile)
		if err != nil {
			log.Fatal().Err(err).Msg("failed to load config")
		}
		if cfg.Database.Type != "postgres" {
			log.Fatal().Msg("target database must be type=postgres in config")
		}
		if err := migrate.Run(*migrateFrom, cfg.Database.DSN); err != nil {
			log.Fatal().Err(err).Msg("migration failed")
		}
		return
	}

	cfg, err := config.Load(*cfgFile)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to load config")
	}

	database, err := db.Init(cfg)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to init database")
	}
	sqlDB, _ := database.DB()
	defer sqlDB.Close()

	// ── Dependency wiring ────────────────────────────────────────────────────
	nodeSvc := node.NewService(database)
	upsertSvc := node.NewUpsertService(database)
	sourceSvc := source.NewService(database, nodeSvc)
	fetchHandler := fetcher.NewHandler(sourceSvc, upsertSvc)
	scoringSvc := scoring.NewService(database)
	aliveCache := cache.New(database, cfg)
	checkerHandler := checker.NewHandler(database, sourceSvc, nodeSvc, scoringSvc, aliveCache)

	// Start periodic history cleanup goroutine.
	cleanup.Start(context.Background(), database, aliveCache)

	// Start traffic reporter (polls mihomo external-controller for per-source traffic).
	mihomoURL := os.Getenv("MIHOMO_API_URL")
	mihomoSecret := os.Getenv("MIHOMO_API_SECRET")
	if mihomoURL != "" {
		trafficReporter := traffic.New(mihomoURL, mihomoSecret, database)
		go trafficReporter.Run(context.Background())
	}

	sponsorSvc := sponsor.NewService(database)

	// Seed default format configs for every registered format. A format with a
	// built-in template uses the embedded one; otherwise the factory default is
	// used. Customized configs are preserved; never-customized ones are upgraded.
	for _, f := range format.All() {
		defaultCfg := []byte(f.DefaultConfig())
		if embedded, ok := templates.Default(f.Name()); ok {
			defaultCfg = embedded
		}
		db.SeedFormatConfig(database, f.Name(), defaultCfg)
	}

	// Trigger an initial cache rebuild so distribution is ready immediately on startup.
	aliveCache.TriggerRebuild(context.Background())

	// ── HTTP engine ──────────────────────────────────────────────────────────
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	engine.Use(middleware.Logger(cfg.Server.Verbose))
	engine.Use(middleware.Recovery())
	engine.Use(corsMiddleware(cfg.Server.CORSOrigins))

	// Service credentials are restricted to the machine-to-machine endpoints.
	// All other API routes require an administrator JWT.
	svcMW := auth.ServiceBearerMiddleware(cfg.Auth.ServiceSecret)
	adminMW := auth.AdminJWTMiddleware(cfg.Auth.JWTSecret)
	sourceCreateMW := auth.AdminOrServiceMiddleware(cfg.Auth.JWTSecret, cfg.Auth.ServiceSecret)

	engine.Use(func(c *gin.Context) {
		p := c.FullPath()
		m := c.Request.Method
		switch {
		case p == "" ||
			p == "/:format" ||
			p == "/health" ||
			strings.HasPrefix(p, "/api/v1/auth/"):
			// public — no auth required
		case strings.HasPrefix(p, "/api/v1/check/"):
			// checker-specific: authenticates via checker secret and injects checker_tag
			middleware.CheckerTagMiddleware(cfg)(c)
		case strings.HasPrefix(p, "/api/v1/fetch/"):
			// service-facing endpoints
			svcMW(c)
		case p == "/api/v1/sources" && m == http.MethodPost:
			// Source creation is shared by the administrator UI and Catcher clients.
			sourceCreateMW(c)
		default:
			adminMW(c)
		}
		if !c.IsAborted() {
			c.Next()
		}
	})

	srv := handler.New(cfg.Auth.AdminPassword, cfg.Auth.JWTSecret, sourceSvc, fetchHandler, nodeSvc, checkerHandler, sponsorSvc, aliveCache, database, cfg)
	api.RegisterHandlers(engine, srv)

	// Health probe — always returns 200, no auth required.
	engine.GET("/health", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	// Traffic stats endpoint — returns per-source traffic aggregated from mihomo.
	//
	// 支持 ?range=day|week|month|all（默认 all）。四个范围统一从
	// source_traffic_daily 求和，区别只在时间起点：
	//   day   -> 今天 00:00
	//   week  -> 今天往前 6 天
	//   month -> 今天往前 29 天
	//   all   -> 不带时间条件，累加全部历史
	//
	// 全部范围也走 daily 而不是快照表，是因为快照存的是 mihomo 的实时
	// 计数器，mihomo 一重启就归零；daily 存的是增量，重启不受影响。
	engine.GET("/api/v1/traffic/stats", func(c *gin.Context) {
		type trafficItem struct {
			SourceID   string `json:"source_id"`
			Identifier string `json:"identifier"`
			Priority   int    `json:"priority"`
			Upload     int64  `json:"upload"`
			Download   int64  `json:"download"`
			RecordedAt string `json:"recorded_at"`
		}

		rng := c.DefaultQuery("range", "all")
		now := time.Now()
		today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

		// 计算起始日期；all 为 nil 表示不加时间条件。
		var since *time.Time
		switch rng {
		case "day":
			since = &today
		case "week":
			d := today.AddDate(0, 0, -6)
			since = &d
		case "month":
			d := today.AddDate(0, 0, -29)
			since = &d
		case "all":
			since = nil
		default:
			// 未知值按 all 处理，避免 400 打断前端。
			rng = "all"
			since = nil
		}

		type aggRow struct {
			SourceID uuid.UUID `gorm:"column:source_id"`
			Upload   int64     `gorm:"column:upload"`
			Download int64     `gorm:"column:download"`
		}
		var rows []aggRow

		q := database.WithContext(c.Request.Context()).Table("source_traffic_daily").
			Select("source_id, SUM(upload) AS upload, SUM(download) AS download").
			Group("source_id")
		if since != nil {
			q = q.Where("day >= ?", *since)
		}
		if err := q.Scan(&rows).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "query failed"})
			return
		}

		perSource := make(map[uuid.UUID]*trafficItem, len(rows))
		for _, r := range rows {
			perSource[r.SourceID] = &trafficItem{
				SourceID:   r.SourceID.String(),
				Upload:     r.Upload,
				Download:   r.Download,
				RecordedAt: now.Format(time.RFC3339),
			}
		}
		recordedAt := now.Format(time.RFC3339)

		// 补齐来源信息（identifier / priority），并丢掉没有流量的条目。
		items := make([]trafficItem, 0, len(perSource))
		var totalUp, totalDown int64
		if len(perSource) > 0 {
			ids := make([]uuid.UUID, 0, len(perSource))
			for id := range perSource {
				ids = append(ids, id)
			}
			var sources []model.Source
			database.WithContext(c.Request.Context()).Where("id IN ?", ids).Find(&sources)
			srcMap := make(map[uuid.UUID]model.Source, len(sources))
			for _, s := range sources {
				srcMap[s.ID] = s
			}
			for id, it := range perSource {
				if it.Upload == 0 && it.Download == 0 {
					continue
				}
				src := srcMap[id]
				it.Identifier = src.Identifier
				it.Priority = src.Priority
				items = append(items, *it)
				totalUp += it.Upload
				totalDown += it.Download
			}
		}

		// mihomo_configured 让前端能区分"没配 reporter"和"配了但还没流量"，
		// 两者的处理方式完全不同：前者要改 .env 重启，后者要把设备代理指过来。
		c.JSON(http.StatusOK, gin.H{
			"sources":           items,
			"total_upload":      totalUp,
			"total_download":    totalDown,
			"recorded_at":       recordedAt,
			"range":             rng,
			"mihomo_configured": mihomoURL != "",
		})
	})

	// Observability endpoints (read-only, same exposure model as traffic/stats).
	// 服务于管理面板「可观测性」卡片：失败原因分布 + 池子健康趋势。
	// fail_reason 空串 = 旧二进制历史数据，归为 unknown；趋势按 check_run
	// 聚合（GROUP BY + JOIN，禁相关子查询——热表教训）。
	parseObsHours := func(c *gin.Context) int {
		h := 48
		if v := c.Query("hours"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n >= 1 && n <= 720 {
				h = n
			}
		}
		return h
	}

	// GET /api/v1/observability/fail-reasons?hours=48
	engine.GET("/api/v1/observability/fail-reasons", func(c *gin.Context) {
		hours := parseObsHours(c)
		type reasonRow struct {
			Reason string `gorm:"column:reason"`
			Count  int64  `gorm:"column:count"`
		}
		var rows []reasonRow
		if err := database.WithContext(c.Request.Context()).
			Table("node_initial_checks").
			Select("COALESCE(NULLIF(fail_reason, ''), 'unknown') AS reason, COUNT(*) AS count").
			Where("latency_ms = -1 AND checked_at >= ?", time.Now().Add(-time.Duration(hours)*time.Hour)).
			Group("reason").Order("count DESC").
			Scan(&rows).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "query failed"})
			return
		}
		var total int64
		for _, r := range rows {
			total += r.Count
		}
		type reasonItem struct {
			Reason string  `json:"reason"`
			Count  int64   `json:"count"`
			Pct    float64 `json:"pct"`
		}
		items := make([]reasonItem, 0, len(rows))
		for _, r := range rows {
			pct := 0.0
			if total > 0 {
				pct = math.Round(float64(r.Count)/float64(total)*1000) / 10
			}
			items = append(items, reasonItem{Reason: r.Reason, Count: r.Count, Pct: pct})
		}
		c.JSON(http.StatusOK, gin.H{
			"window_hours":   hours,
			"total_failures": total,
			"reasons":        items,
		})
	})

	// GET /api/v1/observability/health-trend?hours=48
	engine.GET("/api/v1/observability/health-trend", func(c *gin.Context) {
		hours := parseObsHours(c)
		type runRow struct {
			RunID       uuid.UUID `gorm:"column:run_id"`
			CompletedAt time.Time `gorm:"column:completed_at"`
			NodeCount   int64     `gorm:"column:node_count"`
			Alive       int64     `gorm:"column:alive"`
			P50Latency  *float64  `gorm:"column:p50_latency"`
		}
		// 先取窗口内最新的 200 轮，再按时间正序输出。若直接 ORDER BY ASC + LIMIT，
		// 长窗口（如 30 天 ≈360 轮）会截掉最新的轮次、只留下最旧的——与本接口
		// "看最近健康度"的语义相反。total 也用 COUNT(nic.id) 与 alive 同口径，
		// 避免 node_count 与实际写入行数不一致时算出 >100% 的存活率。
		const trendSQL = `
WITH recent AS (
    SELECT id, completed_at
    FROM checker_runs
    WHERE completed_at >= ?
    ORDER BY completed_at DESC
    LIMIT 200
)
SELECT r.id AS run_id,
       r.completed_at,
       COUNT(nic.id) AS node_count,
       COUNT(nic.id) FILTER (WHERE nic.latency_ms <> -1) AS alive,
       PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY nic.latency_ms)
           FILTER (WHERE nic.latency_ms <> -1) AS p50_latency
FROM recent r
JOIN node_initial_checks nic ON nic.check_run_id = r.id
GROUP BY r.id, r.completed_at
ORDER BY r.completed_at ASC`
		var rows []runRow
		if err := database.WithContext(c.Request.Context()).
			Raw(trendSQL, time.Now().Add(-time.Duration(hours)*time.Hour)).
			Scan(&rows).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "query failed"})
			return
		}
		type runItem struct {
			CompletedAt string   `json:"completed_at"`
			Total       int64    `json:"total"`
			Alive       int64    `json:"alive"`
			AlivePct    float64  `json:"alive_pct"`
			P50Latency  *float64 `json:"p50_latency_ms"`
		}
		items := make([]runItem, 0, len(rows))
		for _, r := range rows {
			pct := 0.0
			if r.NodeCount > 0 {
				pct = math.Round(float64(r.Alive)/float64(r.NodeCount)*1000) / 10
			}
			items = append(items, runItem{
				CompletedAt: r.CompletedAt.Format(time.RFC3339),
				Total:       r.NodeCount,
				Alive:       r.Alive,
				AlivePct:    pct,
				P50Latency:  r.P50Latency,
			})
		}
		c.JSON(http.StatusOK, gin.H{
			"window_hours": hours,
			"runs":         items,
		})
	})
	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	log.Info().Str("addr", addr).Msg("paily-core starting")

	if err := engine.Run(addr); err != nil {
		log.Fatal().Err(err).Msg("server exited")
	}
}

func corsMiddleware(allowedOrigins []string) gin.HandlerFunc {
	originSet := make(map[string]struct{}, len(allowedOrigins))
	for _, o := range allowedOrigins {
		o = strings.TrimRight(o, "/")
		if o != "" && o != "*" {
			originSet[o] = struct{}{}
		}
	}

	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin == "" {
			c.Next()
			return
		}

		_, allowed := originSet[strings.TrimRight(origin, "/")]

		if allowed {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Api-Key, X-Request-ID")
			c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		}

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

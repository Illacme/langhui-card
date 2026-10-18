package main

import (
	"log"

	"langhui/internal/config"
	"langhui/internal/handler"
	"langhui/internal/middleware"
	"langhui/internal/repository"
	"langhui/internal/setting"
	"langhui/internal/storage"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func main() {
	cfg := config.Load()

	// 上线配置自检：默认 JWT 密钥 / 默认超管口令会在此明确告警，提醒及时修改
	for _, w := range cfg.InsecureWarnings() {
		log.Printf("[SECURITY] %s", w)
	}

	db, err := gorm.Open(mysql.Open(cfg.DBDSN), &gorm.Config{})
	if err != nil {
		log.Fatalf("db connect: %v", err)
	}

	handler.SeedAdmin(db, cfg) // 首启动若 admin 表为空则创建初始超管

	// 系统设置：建表 + seed 缺省值 + 载入内存缓存（后台「系统设置」界面维护运行参数）
	if err := setting.Init(db); err != nil {
		log.Fatalf("init settings: %v", err)
	}
	// 系统设置覆盖环境变量（空串=界面未设置→保留环境变量），让云存储 / CDN / 微信凭据
	// 也能在后台维护；这些客户端在启动时构造，故界面标记为「需重启生效」。
	cfg.OverlayFromSettings()
	// 上传限制从系统设置读取（后台改上限即时生效、无需重启）；未配置则回落 10MB / 默认白名单
	storage.UploadLimitBytes = func() int64 {
		return int64(setting.GetInt("upload_max_mb", 10)) * 1024 * 1024
	}
	storage.AllowExtProvider = func() map[string]bool {
		sl := setting.GetStringSlice("upload_allow_ext", nil)
		if len(sl) == 0 {
			return storage.DefaultAllowExt
		}
		m := make(map[string]bool, len(sl))
		for _, e := range sl {
			m[e] = true
		}
		return m
	}

	// 机会性清理过期的会话黑名单行（expires_at 已过的吊销记录，令牌自身也已过期）。
	// 生产可按需另配定时任务：DELETE FROM revoked_sessions WHERE expires_at < NOW();
	if n, err := repository.PurgeExpiredSessions(db); err != nil {
		log.Printf("[WARN] purge expired revoked sessions: %v", err)
	} else if n > 0 {
		log.Printf("[INFO] purged %d expired revoked sessions", n)
	}

	r := handler.New(db)
	// 可观测性：请求 ID + 访问日志（方法/路径/状态/耗时/IP，5xx 单独前缀便于告警匹配）
	r.Use(middleware.RequestID(), middleware.AccessLog())
	// 指标端点：采集器始终注册，开关交给系统设置 metrics_enabled（留空回退 METRICS_ENABLED）——
	// 后台改完即时生效、无需重启；关闭时 /metrics 返回 404，避免公网暴露流量规模。
	metrics := middleware.NewMetrics()
	metrics.Enabled = func() bool {
		return setting.GetBool("metrics_enabled", cfg.MetricsEnabled)
	}
	r.Use(metrics.Middleware())
	r.GET("/metrics", metrics.Handler())
	if metrics.Enabled() {
		log.Printf("[INFO] metrics enabled: GET /metrics")
	}
	handler.RegisterAuth(r, db, cfg) // 员工/超管登录（JWT 签发）+ 刷新 + 登出
	handler.RegisterApply(r, db)     // 开通申请 / 名片修改审核 / 员工管理 / 统计
	handler.RegisterContent(r, db)   // 企业内容管理（公司信息/简介/分类/项目/服务/布局，按租户隔离）
	handler.RegisterCompanies(r, db) // 企业管理（平台超管）：开通/停用企业、企业管理员管理
	handler.RegisterAdmins(r, db)    // 平台超管账号管理：列表/新增/改用户名/重置密码/删除
	handler.RegisterAuditLogs(r, db) // 操作审计日志（只读查询：平台超管）

	// 文件上传：存储驱动可配（local 默认 / minio / oss），业务代码不变。
	// 驱动初始化失败时降级本地（而不是 Fatal）：凭据在界面里改错的情况下，
	// 进程必须还能起来，管理员才进得去后台把它改回来（「改错可自愈」的底线）。
	up := newUploaderWithFallback(cfg)
	// CDN 刷新器（可选）：启用后上传/覆盖写会触发边缘缓存失效
	var refresher storage.CDNRefresher
	if cfg.CDNEnabled && cfg.CDNAccessKeyID != "" && cfg.CDNAccessKeySecret != "" {
		refresher = storage.NewAliyunCDNRefresher(cfg.CDNAccessKeyID, cfg.CDNAccessKeySecret, cfg.CDNEndpoint)
		log.Printf("cdn refresh enabled (endpoint=%s)", cfg.CDNEndpoint)
	}
	// /uploads 静态托管：local 驱动下用户文件与小程序码均落本地并由本路由托管；
	// minio/oss 驱动下用户文件/小程序码走对象存储，此路由仅作空目录兜底，无副作用。
	r.Static("/uploads", cfg.UploadDir)
	handler.RegisterUpload(r, db, up, refresher)       // POST /api/admin/upload + /api/employee/upload
	handler.RegisterStorage(r, db, up, refresher, cfg) // 直传预签名 / 私有桶签名 / CDN 刷新 / 本地私有文件流
	handler.RegisterAssets(r, db, up)                  // 常用素材库（内嵌图标/封面/LOGO）：清单 / 采用 / 预览图
	handler.RegisterWechat(r, cfg, up)                 // GET /api/wechat/wxacode（小程序码，按 staffId）
	handler.RegisterSettings(r, db, cfg)               // 系统设置：GET/PUT /api/admin/settings + GET /api/public/settings

	log.Printf("listen :%s (storage driver=%s)", cfg.Port, cfg.StorageDriver)
	if err := r.Run(":" + cfg.Port); err != nil {
		log.Fatal(err)
	}
}

// newUploaderWithFallback 构造存储实现；失败时降级为本地磁盘并打印醒目告警。
// 不 Fatal 是刻意的：凭据可能是在后台「系统设置」里填错的，若进程起不来，
// 管理员就再也进不去后台改回来——降级保证「改错可自愈」。
func newUploaderWithFallback(cfg config.Config) storage.Uploader {
	up, err := newUploader(cfg)
	if err == nil {
		return up
	}
	log.Printf("[WARN] 存储驱动 %q 初始化失败：%v；已降级为本地磁盘存储。"+
		"请到「系统设置 → 文件存储」修正后重启服务。", cfg.StorageDriver, err)
	return &storage.LocalUploader{
		Dir:      cfg.UploadDir,
		BaseURL:  cfg.UploadBaseURL,
		MaxBytes: 10 * 1024 * 1024,
		AllowExt: storage.DefaultAllowExt,
	}
}

// newUploader 按存储驱动构造文件存储实现（local / minio / oss），统一返回 storage.Uploader
func newUploader(cfg config.Config) (storage.Uploader, error) {
	const maxBytes = 10 * 1024 * 1024 // 10MB
	switch cfg.StorageDriver {
	case "minio":
		return storage.NewMinioUploader(cfg.MinioEndpoint, cfg.MinioAccessKey, cfg.MinioSecretKey,
			cfg.MinioBucket, cfg.UploadBaseURL, cfg.MinioUseSSL, maxBytes, storage.DefaultAllowExt)
	case "oss":
		return storage.NewOssUploader(cfg.OssEndpoint, cfg.OssAccessKeyID, cfg.OssAccessKeySecret,
			cfg.OssBucket, cfg.UploadBaseURL, maxBytes, storage.DefaultAllowExt)
	default:
		return &storage.LocalUploader{
			Dir:      cfg.UploadDir,
			BaseURL:  cfg.UploadBaseURL,
			MaxBytes: maxBytes,
			AllowExt: storage.DefaultAllowExt,
		}, nil
	}
}

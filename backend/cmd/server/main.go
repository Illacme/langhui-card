package main

import (
	"log"

	"langhui/internal/config"
	"langhui/internal/handler"
	"langhui/internal/middleware"
	"langhui/internal/repository"
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
	// 指标端点默认关闭（METRICS_ENABLED=1 开启）：避免公网暴露流量规模，
	// 生产建议仅内网 / 反代白名单可达，配 Prometheus 抓取 /metrics。
	if cfg.MetricsEnabled {
		metrics := middleware.NewMetrics()
		r.Use(metrics.Middleware())
		r.GET("/metrics", metrics.Handler())
		log.Printf("[INFO] metrics enabled: GET /metrics")
	}
	handler.RegisterAuth(r, db, cfg) // 员工/超管登录（JWT 签发）+ 刷新 + 登出
	handler.RegisterApply(r, db)     // 开通申请 / 名片修改审核 / 员工管理 / 统计
	handler.RegisterContent(r, db)   // 企业内容管理（公司信息/简介/分类/项目/服务/布局，按租户隔离）
	handler.RegisterCompanies(r, db) // 企业管理（平台超管）：开通/停用企业、企业管理员管理

	// 文件上传：存储驱动可配（local 默认 / minio / oss），业务代码不变
	up, err := newUploader(cfg)
	if err != nil {
		log.Fatalf("init uploader: %v", err)
	}
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
	handler.RegisterWechat(r, cfg, up)                 // GET /api/wechat/wxacode（小程序码，按 staffId）

	log.Printf("listen :%s (storage driver=%s)", cfg.Port, cfg.StorageDriver)
	if err := r.Run(":" + cfg.Port); err != nil {
		log.Fatal(err)
	}
}

// newUploader 按 STORAGE_DRIVER 构造文件存储实现（local / minio / oss），统一返回 storage.Uploader
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

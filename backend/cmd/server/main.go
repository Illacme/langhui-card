package main

import (
	"log"
	"os"

	"weichuan/internal/config"
	"weichuan/internal/dbopen"
	"weichuan/internal/handler"
	"weichuan/internal/middleware"
	"weichuan/internal/model"
	"weichuan/internal/repository"
	"weichuan/internal/setting"
	"weichuan/internal/setup"
	"weichuan/internal/storage"
)

func main() {
	cfg := config.Load()

	// 首次部署引导：连库必需的 DB_DSN 根本没提供时，不再直接 Fatal
	// （运维只看到一行日志、不知道下一步做什么），而是以免数据库方式起来，
	// 只提供只读引导页：说清缺什么、生成可复制的配置、提示改完重启。
	// 判据刻意是「DB_DSN 未提供」而不是「连不上库」：后者是线上真故障，
	// 若也进引导页，一次数据库抖动就会让终端用户看到安装页。
	if setup.Unconfigured() {
		log.Printf("[SETUP] 未检测到 DB_DSN，进入初始配置模式：不加载业务功能，仅提供引导页")
		log.Printf("[SETUP] 请访问 http://<服务器IP>:%s/setup 查看并复制需要配置的内容", cfg.Port)
		if err := setup.Engine().Run(":" + cfg.Port); err != nil {
			log.Fatal(err)
		}
		return
	}

	// 安全硬门槛：带内置缺省的 JWT 密钥 / 超管口令上线，等于把后台开放给所有人——
	// 这两个默认值都原样写在公开文档里，属于已知凭据，因此这里拒绝启动而不是只告警。
	// 自愈方式很简单（改一个环境变量重启），不会像 db_dsn 那样把「改回来的入口」弄丢。
	// 本地联调确需放行：ALLOW_INSECURE=1。
	if gates := cfg.InsecureGates(); len(gates) > 0 {
		if os.Getenv("ALLOW_INSECURE") != "1" {
			for _, g := range gates {
				log.Printf("[SECURITY][FATAL] %s", g)
			}
			log.Fatal("拒绝以不安全的默认凭据启动：请设置 JWT_KEY / ADMIN_PASS 后重启" +
				"（本地联调确需放行请加 ALLOW_INSECURE=1，切勿用于生产）")
		}
		for _, g := range gates {
			log.Printf("[SECURITY][ALLOW_INSECURE] 已放行：%s", g)
		}
	}

	// 其余不安全配置只提示不拦截（如超管口令偏弱），由运维自行判断。
	for _, w := range cfg.InsecureWarnings() {
		log.Printf("[SECURITY] %s", w)
	}

	// 驱动由 DB_DSN 形态决定：默认是 MySQL；写成 sqlite://<路径> / file:<URI> /
	// *.db 则走 SQLite（详见 docs/deploy-runbook.md「路径 D：SQLite 单机部署」）。
	// 注意 db_dsn 是系统设置里的只读项——改错会连不上库、把「改回来的入口」一起弄丢，
	// 所以库地址只能在启动时用环境变量定好，上线后改不动。
	db, driver, err := dbopen.Open(cfg.DBDSN)
	if err != nil {
		// 走到这里说明 DB_DSN 给了但连不上——是配置错误 / 库不可用，不是「没配置」，
		// 所以仍然 Fatal（不能让终端用户对着一个半成品服务）。但要把排查路径写明，
		// 而不是留一句 "db connect: ..." 让人自己去猜。
		// 注意：**不打印 DSN 原文**，里面含数据库口令，打印即泄密。
		log.Printf("[FATAL] 数据库连接失败（driver=%s）：%v", driver, err)
		log.Printf("[FATAL] 排查顺序：1) 数据库是否已启动、地址/端口是否可达；" +
			"2) DB_DSN 的账号口令与库名是否正确（日志不打印 DSN，避免泄露口令）；" +
			"3) 先跑 scripts/preflight.sh 做静态自检；4) 详见 docs/deploy-runbook.md")
		log.Fatal("db connect failed")
	}

	// SQLite 没有可执行的 migrations：`migrations/*.sql` 是 MySQL 方言
	// （ENGINE=InnoDB / AUTO_INCREMENT / 反引号），这里改为按模型建表。
	// MySQL 侧不动——生产库的表结构由迁移脚本管，AutoMigrate 擅自改列类型风险更大。
	if driver == dbopen.DriverSQLite {
		if err := db.AutoMigrate(model.All()...); err != nil {
			log.Fatalf("auto migrate: %v", err)
		}
	}
	log.Printf("database driver=%s", driver)
	if driver == dbopen.DriverSQLite {
		log.Printf("[INFO] SQLite 为单写者且不支持多副本：请勿将库文件放在 NFS 等网络盘，" +
			"备份请用 VACUUM INTO 取在线一致性快照（详见 docs/deploy-runbook.md）")
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
	// 首次配置状态查询（只读）：已配置时返回 configured=true；未配置时进程根本进不到这里
	// （见开头 setup.Unconfigured 分支）。前端据此决定是否显示配置引导。
	// /setup 别名：安装完成后还有人手动敲 /setup 时 302 回首页，不给裸 404。
	setup.RegisterStatus(r)
	setup.RegisterSetupAlias(r)

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

// newUploader 按存储驱动构造文件存储实现（local / minio / oss / s3），统一返回 storage.Uploader
func newUploader(cfg config.Config) (storage.Uploader, error) {
	const maxBytes = 10 * 1024 * 1024 // 10MB
	switch cfg.StorageDriver {
	// minio 与 s3 共用同一套 S3 客户端实现：端点 / 桶 / 凭据 / SSL / 区域全部来自
	// 同一批界面字段（storage_region 对两者都生效），枚举分开只为界面上的归属说明
	// ——s3 面向腾讯云 COS / 华为云 OBS / 七牛云 / AWS S3 / R2 等 S3 兼容云，
	// 这类云常需要显式指定区域，minio-go 的自动探测对它们并非总可靠。
	case "minio", "s3":
		return storage.NewMinioUploader(cfg.MinioEndpoint, cfg.MinioAccessKey, cfg.MinioSecretKey,
			cfg.MinioBucket, cfg.UploadBaseURL, cfg.StorageRegion, cfg.MinioUseSSL, maxBytes, storage.DefaultAllowExt)
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

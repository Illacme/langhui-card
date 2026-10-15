package main

import (
	"log"

	"langhui/internal/config"
	"langhui/internal/handler"
	"langhui/internal/storage"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func main() {
	cfg := config.Load()

	db, err := gorm.Open(mysql.Open(cfg.DBDSN), &gorm.Config{})
	if err != nil {
		log.Fatalf("db connect: %v", err)
	}

	handler.SeedAdmin(db, cfg) // 首启动若 admin 表为空则创建初始超管

	r := handler.New(db)
	handler.RegisterAuth(r, db)      // 员工/超管登录（账号密码，JWT 签发）
	handler.RegisterApply(r, db)     // 开通申请 / 名片修改审核 / 员工管理 / 统计
	handler.RegisterContent(r, db)   // 企业内容管理（公司信息/简介/分类/项目/服务/布局，超管）

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
	handler.RegisterUpload(r, up, refresher)   // POST /api/admin/upload + /api/employee/upload
	handler.RegisterStorage(r, up, refresher, cfg) // 直传预签名 / 私有桶签名 / CDN 刷新 / 本地私有文件流
	handler.RegisterWechat(r, cfg, up)         // GET /api/wechat/wxacode（小程序码，按 staffId）

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

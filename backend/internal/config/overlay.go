package config

import (
	"strings"

	"weichuan/internal/setting"
)

// str 取界面设置值；**空串一律视同「未设置」**，保留环境变量值。
// 不能直接用 setting.GetString(key, def)：seed 会把这些 key 以空串写进库，
// GetString 读到的是「有这条记录、值为空」，会把环境变量值覆盖成空——
// 这正是「只配了环境变量的老部署升级后凭空失效」的成因。
func str(key, def string) string {
	if v := strings.TrimSpace(setting.GetString(key, "")); v != "" {
		return v
	}
	return def
}

// OverlayFromSettings 用系统设置（sys_setting）覆盖环境变量加载出的配置。
//
// 语义：**空串 = 未在界面设置 → 保留环境变量值**。
// 这样同一份部署既能纯环境变量跑（界面全空），也能在界面里逐项覆盖（界面优先），
// 且把界面上的值清空后会自动退回环境变量，不会出现「改坏了退不回去」的死角。
//
// 调用时机：setting.Init(db) 之后、构造上传器 / CDN 刷新器之前（见 cmd/server）。
// 覆盖结果只影响本次进程：存储与 CDN 客户端在启动时构造，因此这些项标记为
// 「需重启生效」（setting 的 Restart 元数据），界面会明确提示。
func (c *Config) OverlayFromSettings() {
	// 文件存储（MinIO / S3 兼容 与 阿里云 OSS 共用同一组界面字段，按驱动二选一）
	c.StorageDriver = str("storage_driver", c.StorageDriver)
	c.StoragePublic = setting.GetBool("storage_public", c.StoragePublic)
	c.MinioEndpoint = str("storage_endpoint", c.MinioEndpoint)
	c.MinioBucket = str("storage_bucket", c.MinioBucket)
	c.MinioAccessKey = str("storage_access_key", c.MinioAccessKey)
	c.MinioSecretKey = str("storage_secret_key", c.MinioSecretKey)
	c.MinioUseSSL = setting.GetBool("storage_use_ssl", c.MinioUseSSL)
	// storage_region 对 minio / s3 两个 S3 兼容驱动生效（oss 客户端没有区域参数）
	c.StorageRegion = str("storage_region", c.StorageRegion)
	c.OssEndpoint = str("storage_endpoint", c.OssEndpoint)
	c.OssBucket = str("storage_bucket", c.OssBucket)
	c.OssAccessKeyID = str("storage_access_key", c.OssAccessKeyID)
	c.OssAccessKeySecret = str("storage_secret_key", c.OssAccessKeySecret)

	c.UploadDir = str("upload_dir", c.UploadDir)
	c.UploadBaseURL = str("upload_base_url", c.UploadBaseURL)

	// CDN 缓存刷新
	c.CDNEnabled = setting.GetBool("cdn_enabled", c.CDNEnabled)
	c.CDNAccessKeyID = str("cdn_access_key_id", c.CDNAccessKeyID)
	c.CDNAccessKeySecret = str("cdn_access_key_secret", c.CDNAccessKeySecret)
	c.CDNEndpoint = str("cdn_endpoint", c.CDNEndpoint)

	// 微信：AppID / AppSecret 另有请求期解析（resolveWXAppID / resolveWXAppSecret，改完即时生效），
	// 此处同步一份到 cfg，保证启动日志与其它读取点看到的值一致。
	c.WXAppID = str("wx_appid", c.WXAppID)
	c.WXAppSecret = str("wx_appsecret", c.WXAppSecret)
}

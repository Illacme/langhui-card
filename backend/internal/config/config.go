package config

import (
	"os"
	"strconv"
	"time"
)

// Config 通过环境变量注入，不写死（保证可平滑迁云托管）
type Config struct {
	DBDSN           string
	Port            string
	JWTKey          string
	UploadDir       string // 本地上传目录（local 驱动用，小程序码缓存也落这里）
	UploadBaseURL   string // 上传文件公开访问基址（小程序/前端取此拼 URL）
	StorageDriver   string // 文件存储驱动：local(默认) | minio | oss
	StoragePublic   bool   // 是否为公开桶；false=私有桶，需限时签名访问（STORAGE_PUBLIC）
	MinioEndpoint   string // MinIO / S3 兼容端点（不含协议头）
	MinioAccessKey  string
	MinioSecretKey  string
	MinioBucket     string
	MinioUseSSL     bool
	OssEndpoint     string // 阿里云 OSS 端点，如 oss-cn-hangzhou.aliyuncs.com
	OssAccessKeyID  string
	OssAccessKeySecret string
	OssBucket       string
	CDNEnabled      bool   // 是否启用 CDN 刷新（覆盖写后使边缘缓存失效）
	CDNAccessKeyID  string // CDN 刷新凭据（阿里云，可与 OSS 凭据相同）
	CDNAccessKeySecret string
	CDNEndpoint     string // CDN 刷新端点，默认 cdn.aliyuncs.com
	WXAppID         string // 小程序 AppID（小程序码用）
	WXAppSecret     string // 小程序 AppSecret（小程序码用）
	AdminUser       string // 初始超管用户名（首启动 seed 用）
	AdminPass       string // 初始超管密码（首启动 seed 用，上线务必修改）
	WxacodeTTLHours int    // 小程序码缓存有效期（小时）
	WxacodeTTL      time.Duration
}

func Load() Config {
	ttlHours := atoi(getEnv("WXACODE_TTL_HOURS", "720"), 720) // 默认 30 天
	return Config{
		DBDSN:           getEnv("DB_DSN", "root:root@tcp(127.0.0.1:3306)/langhui?charset=utf8mb4&parseTime=true"),
		Port:            getEnv("PORT", "8080"),
		JWTKey:          getEnv("JWT_KEY", "change-me"),
		UploadDir:       getEnv("UPLOAD_DIR", "./uploads"),
		UploadBaseURL:   getEnv("UPLOAD_BASE_URL", "http://127.0.0.1:8080/uploads"),
		StorageDriver:   getEnv("STORAGE_DRIVER", "local"),
		StoragePublic:   parseBool(getEnv("STORAGE_PUBLIC", "true")),
		MinioEndpoint:   getEnv("MINIO_ENDPOINT", ""),
		MinioAccessKey:  getEnv("MINIO_ACCESS_KEY", ""),
		MinioSecretKey:  getEnv("MINIO_SECRET_KEY", ""),
		MinioBucket:     getEnv("MINIO_BUCKET", ""),
		MinioUseSSL:     parseBool(getEnv("MINIO_USE_SSL", "false")),
		OssEndpoint:     getEnv("OSS_ENDPOINT", ""),
		OssAccessKeyID:  getEnv("OSS_ACCESS_KEY_ID", ""),
		OssAccessKeySecret: getEnv("OSS_ACCESS_KEY_SECRET", ""),
		OssBucket:       getEnv("OSS_BUCKET", ""),
		CDNEnabled:      parseBool(getEnv("CDN_ENABLED", "false")),
		CDNAccessKeyID:  getEnv("CDN_ACCESS_KEY_ID", ""),
		CDNAccessKeySecret: getEnv("CDN_ACCESS_KEY_SECRET", ""),
		CDNEndpoint:     getEnv("CDN_ENDPOINT", "cdn.aliyuncs.com"),
		WXAppID:         getEnv("WX_APPID", ""),
		WXAppSecret:     getEnv("WX_APPSECRET", ""),
		AdminUser:       getEnv("ADMIN_USER", "admin"),
		AdminPass:       getEnv("ADMIN_PASS", "admin123456"),
		WxacodeTTLHours: ttlHours,
		WxacodeTTL:      time.Duration(ttlHours) * time.Hour,
	}
}

func getEnv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func atoi(s string, def int) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}

func parseBool(s string) bool {
	b, err := strconv.ParseBool(s)
	if err != nil {
		return false
	}
	return b
}

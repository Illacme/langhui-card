package config

import (
	"strconv"
	"time"
)

// 安全相关常量：默认密钥 / 默认超管口令与其判定下限。
//
// 默认口令之所以能被「硬门槛」拦住，是因为它连同演示账号一起写进了 README 与联调
// 文档——它已经不是秘密了。带它上线等于任何人都能进后台，所以必须是致命项而非提示项。
const (
	DefaultJWTKey    = "change-me"   // JWT_KEY 内置缺省值
	MinJWTKeyLen     = 16            // 低于此长度视为可暴力伪造
	DefaultAdminPass = "admin123456" // ADMIN_PASS 内置缺省值（首启动 seed 用）
	MinAdminPassLen  = 8             // 低于此长度仅告警：口令是给人记的，不宜卡太死
)

// Config 通过环境变量注入，不写死（保证可平滑迁云托管）
type Config struct {
	DBDSN              string
	Port               string
	JWTKey             string
	UploadDir          string // 本地上传目录（local 驱动用，小程序码缓存也落这里）
	UploadBaseURL      string // 上传文件公开访问基址（小程序/前端取此拼 URL）
	StorageDriver      string // 文件存储驱动：local(默认) | minio | oss
	StoragePublic      bool   // 是否为公开桶；false=私有桶，需限时签名访问（STORAGE_PUBLIC）
	MinioEndpoint      string // MinIO / S3 兼容端点（不含协议头）
	MinioAccessKey     string
	MinioSecretKey     string
	MinioBucket        string
	MinioUseSSL        bool
	OssEndpoint        string // 阿里云 OSS 端点，如 oss-cn-hangzhou.aliyuncs.com
	OssAccessKeyID     string
	OssAccessKeySecret string
	OssBucket          string
	CDNEnabled         bool   // 是否启用 CDN 刷新（覆盖写后使边缘缓存失效）
	CDNAccessKeyID     string // CDN 刷新凭据（阿里云，可与 OSS 凭据相同）
	CDNAccessKeySecret string
	CDNEndpoint        string // CDN 刷新端点，默认 cdn.aliyuncs.com
	WXAppID            string // 小程序 AppID（小程序码用）
	WXAppSecret        string // 小程序 AppSecret（小程序码用）
	AdminUser          string // 初始超管用户名（首启动 seed 用）
	AdminPass          string // 初始超管密码（首启动 seed 用，上线务必修改）
	CookieSecure       bool   // 鉴权 Cookie 是否加 Secure 标记（生产 HTTPS 设 true；本地 http 需 false）
	CookieDomain       string // 鉴权 Cookie 作用域（默认空=仅当前主机；跨子域共享登录态时才设）
	MetricsEnabled     bool   // 是否开放 GET /metrics（Prometheus 文本格式；默认关，避免公网暴露流量规模）
	WxacodeTTLHours    int    // 小程序码缓存有效期（小时）
	WxacodeTTL         time.Duration
}

func Load() Config {
	ttlHours := atoi(getEnv("WXACODE_TTL_HOURS", "720"), 720) // 默认 30 天
	return Config{
		DBDSN:              getEnv("DB_DSN", "root:root@tcp(127.0.0.1:3306)/langhui?charset=utf8mb4&parseTime=true"),
		Port:               getEnv("PORT", "8080"),
		JWTKey:             getEnv("JWT_KEY", DefaultJWTKey),
		UploadDir:          getEnv("UPLOAD_DIR", "./uploads"),
		UploadBaseURL:      getEnv("UPLOAD_BASE_URL", "http://127.0.0.1:8080/uploads"),
		StorageDriver:      getEnv("STORAGE_DRIVER", "local"),
		StoragePublic:      parseBool(getEnv("STORAGE_PUBLIC", "true")),
		MinioEndpoint:      getEnv("MINIO_ENDPOINT", ""),
		MinioAccessKey:     getEnv("MINIO_ACCESS_KEY", ""),
		MinioSecretKey:     getEnv("MINIO_SECRET_KEY", ""),
		MinioBucket:        getEnv("MINIO_BUCKET", ""),
		MinioUseSSL:        parseBool(getEnv("MINIO_USE_SSL", "false")),
		OssEndpoint:        getEnv("OSS_ENDPOINT", ""),
		OssAccessKeyID:     getEnv("OSS_ACCESS_KEY_ID", ""),
		OssAccessKeySecret: getEnv("OSS_ACCESS_KEY_SECRET", ""),
		OssBucket:          getEnv("OSS_BUCKET", ""),
		CDNEnabled:         parseBool(getEnv("CDN_ENABLED", "false")),
		CDNAccessKeyID:     getEnv("CDN_ACCESS_KEY_ID", ""),
		CDNAccessKeySecret: getEnv("CDN_ACCESS_KEY_SECRET", ""),
		CDNEndpoint:        getEnv("CDN_ENDPOINT", "cdn.aliyuncs.com"),
		WXAppID:            getEnv("WX_APPID", ""),
		WXAppSecret:        getEnv("WX_APPSECRET", ""),
		AdminUser:          getEnv("ADMIN_USER", "admin"),
		AdminPass:          getEnv("ADMIN_PASS", DefaultAdminPass),
		CookieSecure:       parseBool(getEnv("COOKIE_SECURE", "false")),
		CookieDomain:       getEnv("COOKIE_DOMAIN", ""),
		MetricsEnabled:     parseBool(getEnv("METRICS_ENABLED", "false")),
		WxacodeTTLHours:    ttlHours,
		WxacodeTTL:         time.Duration(ttlHours) * time.Hour,
	}
}

// InsecureWarnings 返回上线前应修正的不安全配置提示（超管口令偏弱）。
//
// 与 InsecureGates 的分工：Warnings 只提示、不拦启动（改不改都不影响系统可用）；
// Gates 命中则 cmd/server 拒绝启动。判断依据是「这个默认值是否已经不 secret」——
// JWT 密钥与初始口令都原样写在公开文档里，属于已知凭据，必须拦；
// 口令长度只是强弱问题，交给运维判断，不卡启动。
func (c Config) InsecureWarnings() []string {
	var ws []string
	if len(c.AdminPass) < MinAdminPassLen {
		ws = append(ws, "初始超管口令长度不足 8 位，建议通过 ADMIN_PASS 设置更强口令并在首登后及时改密")
	}
	return ws
}

// InsecureGates 返回「命中即拒绝在生产启动」的安全硬门槛。
//
// 设计取舍：这里只放「带默认凭据上线等于把后台开放给所有人」的项，且每一项都能靠
// 改一个环境变量自愈（不像 db_dsn / cookie_* 那样会把「改回来的入口」一起弄丢），
// 所以「拒绝启动」是安全且不危险的。逃生门是 ALLOW_INSECURE=1（仅本地联调用）。
func (c Config) InsecureGates() []string {
	var gs []string
	if c.JWTKey == DefaultJWTKey {
		gs = append(gs, "JWT_KEY 仍为内置缺省值 change-me：任何人都能伪造任意身份的 token，请用 openssl rand -base64 32 生成后设置 JWT_KEY")
	} else if len(c.JWTKey) < MinJWTKeyLen {
		gs = append(gs, "JWT_KEY 长度不足 16 位，可被暴力破解，请用 openssl rand -base64 32 生成后设置 JWT_KEY")
	}
	if c.AdminPass == DefaultAdminPass {
		gs = append(gs, "初始超管口令仍为内置缺省值（该口令已随演示账号写入公开文档）：任何人都能登录后台，请设置 ADMIN_PASS 后启动")
	}
	return gs
}

// getEnv 取配置值：环境变量 > 配置文件（CONFIG_FILE）> 内置缺省值。
// 走 lookup 而不是直接读 os.Getenv，是为了让安装向导写进 runtime.env 的值
// 在重启后生效；空串/纯空白一律视为未设置（见 file.go 的优先级说明）。
func getEnv(k, def string) string {
	if v, ok := lookup(k); ok {
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

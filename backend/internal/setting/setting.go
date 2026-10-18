// Package setting 系统运行参数的「后台可视化配置」基础设施。
//
// 设计目标（与产品诉求对齐）：
//   - 把过去散落在 config.go / main.go / storage.go / auth.go 等处的「写死值」
//     收敛为一张 sys_setting 表，平台超管可在 PC 后台「系统设置」界面统一维护。
//
// 配置项分三档（按「改错能否自愈」与「泄漏爆炸半径」划分，不是按「是否敏感」一刀切）：
//  1. 常规项：保存即时生效（运行时内存缓存 + 即时回写），如品牌名 / 上传上限 / 特性开关；
//  2. 需重启项（Restart）：云存储与 CDN 凭据、上传基址等。它们要在启动时建客户端，
//     保存后需重启进程才生效；凭据写错不会让系统不可达——改回来重启即可自愈；
//  3. 只读项（ReadOnly）：DB_DSN / JWT_KEY / Cookie 安全。它们一旦改错就会把
//     「改回来的入口」一起弄丢（连不上库 → 读不出配置；会话全失效 → 登不进后台），
//     因此只能在界面**掩码展示当前值**并提示改法，实际修改走环境变量 + 重启。
//
// 约定：
//   - 所有「可后台配置项」必须在 registry 登记（含类型与默认值），前端分组表单与之对应；
//     新增可配置项 = 在 registry 追加一行 + 前端加一项，缺省值会在 Init 时自动 seed 进库。
//   - 取值一律带默认值：库未初始化 / 该 key 缺失 时回落默认值，保证任意阶段调用都安全。
//   - 空串 = 「未在界面设置」，读取时回退同名环境变量（见 config.OverlayFromSettings）。
package setting

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"

	"gorm.io/gorm"
)

// Setting 系统设置表（key-value 带类型与分组）。
type Setting struct {
	ID          uint   `gorm:"primaryKey" json:"-"`
	Key         string `gorm:"uniqueIndex;size:64" json:"key"`
	Group       string `gorm:"size:32;index" json:"group"`
	Value       string `gorm:"type:text" json:"value"`
	Type        string `gorm:"size:16" json:"type"` // int | string | bool | string_slice
	Description string `gorm:"type:text" json:"description"`
}

// TableName 显式指定表名（避免 GORM 复数化）。
func (Setting) TableName() string { return "sys_setting" }

// def 单条可配置项的静态定义（类型契约 + 默认值 + 文案 + 生效方式元数据）。
type def struct {
	Key, Group, Type, Value, Desc string
	// Secret：敏感项。接口只回传掩码（Mask），真实值永不出现在响应里；
	// 界面提交掩码或缺省表示「不改动」，提交空串表示「清掉、回退环境变量」。
	Secret bool
	// Restart：保存后需重启进程才生效（需在启动时建客户端的项，如对象存储 / CDN）。
	Restart bool
	// ReadOnly：只读项。不落库、不可通过接口修改，只能改环境变量后重启
	//（改错会把「改回来的入口」一起弄丢的项，见包注释第 3 档）。
	ReadOnly bool
	// EnvVar：对应环境变量名。界面据此提示改法；未设置时回退该环境变量。
	EnvVar string
}

// Mask 敏感项在接口响应里的掩码（界面据此显示「已配置」，不回传真实值）。
const Mask = "******"

// Item 后台「系统设置」渲染用 DTO。
// 与 DB 模型 Setting 分开，避免把 secret 的真实值随表结构一并序列化出去。
type Item struct {
	Key         string `json:"key"`
	Group       string `json:"group"`
	Type        string `json:"type"` // int | string | bool | string_slice
	Description string `json:"description"`
	// Value 当前值：敏感项恒为 Mask（已配置）或空串（未配置）。
	Value      string `json:"value"`
	Secret     bool   `json:"secret"`
	Restart    bool   `json:"restart"`
	ReadOnly   bool   `json:"readOnly"`
	EnvVar     string `json:"envVar"`
	Configured bool   `json:"configured"` // 是否有有效值（库值 / 环境变量 / 进程内缺省）
}

// registry 全量「可后台配置项」清单：新增可配置项只在此追加一行即可。
// 分组（group）直接对应前端「系统设置」的分组表单：
//   general    通用（品牌名）
//   miniprogram 小程序（默认基址 / 独立版锁定租户）
//   upload     上传（单文件上限 / 允许扩展名 / 各类签名有效期）
//   wechat     微信（小程序码 TTL / AppID / AppSecret）
//   auth       登录与令牌（令牌有效期 / 登录与刷新限流）
//   storage    文件存储与 CDN（驱动 / 端点 / 桶 / 凭据 / 上传基址 / CDN 刷新）
//   security   安全与基础设施（只读：DB 连接串 / JWT 密钥 / Cookie）
//   feature    特性开关（Prometheus 指标 / 独立版企业锁定）
var registry = []def{
	{Key: "brand_name", Group: "general", Type: "string", Value: "智能名片",
		Desc: "后台侧边栏 / 页标题品牌名（未设置回落此值）"},
	{Key: "miniprogram_base_url", Group: "miniprogram", Type: "string",
		Desc: "小程序端默认接口基址；空则小程序用自身内置 baseUrl"},
	{Key: "pinned_company_id", Group: "miniprogram", Type: "int", Value: "0",
		Desc: "企业独立版锁定租户：>0 锁定该企业，0=平台模式"},
	{Key: "upload_max_mb", Group: "upload", Type: "int", Value: "10",
		Desc: "单文件上传上限（MB）"},
	{Key: "upload_allow_ext", Group: "upload", Type: "string_slice", Value: "png,jpg,jpeg,gif,webp,mp4",
		Desc: "允许上传的扩展名（小写，逗号分隔）"},
	// 签名有效期：直传 PUT 与私有桶 GET 的限时链接时长。太短前端还没传完就过期，
	// 太长等于把对象存储写成公开读，故留界面可调，但代码内有下限兜底。
	{Key: "presign_put_ttl_minutes", Group: "upload", Type: "int", Value: "15",
		Desc: "直传预签名 PUT 链接有效期（分钟），最短 1 分钟"},
	{Key: "sign_get_ttl_minutes", Group: "upload", Type: "int", Value: "30",
		Desc: "私有桶限时签名 GET 链接有效期（分钟），最短 1 分钟"},
	{Key: "wxacode_ttl_hours", Group: "wechat", Type: "int", Value: "720",
		Desc: "小程序码缓存有效期（小时）"},
	{Key: "wxacode_sign_ttl_minutes", Group: "wechat", Type: "int", Value: "60",
		Desc: "私有桶下小程序码签名链接有效期（分钟），最短 1 分钟"},
	{Key: "wx_appid", Group: "wechat", Type: "string", EnvVar: "WX_APPID",
		Desc: "小程序 AppID（小程序码与微信登录用）；留空回退环境变量 WX_APPID"},
	{Key: "wx_appsecret", Group: "wechat", Type: "string", Secret: true, EnvVar: "WX_APPSECRET",
		Desc: "小程序 AppSecret（小程序码用，敏感）；留空回退环境变量 WX_APPSECRET"},

	// ---- 登录与令牌（即时生效）----
	// 令牌有效期设太短会把人一直踢下线；代码内保留**安全下限**，
	// 界面填 0 / 负数时回落缺省，避免管理员把自己锁在系统外（改错可自愈）。
	{Key: "access_token_ttl_minutes", Group: "auth", Type: "int", Value: "15",
		Desc: "access 令牌有效期（分钟），最短 1 分钟；改后新签发的令牌立即按新值，已签发的不追溯"},
	{Key: "refresh_token_ttl_days", Group: "auth", Type: "int", Value: "30",
		Desc: "refresh 令牌有效期（天），最短 1 天；决定多久要重新登录一次"},
	{Key: "login_rate_limit_per_min", Group: "auth", Type: "int", Value: "10",
		Desc: "每 IP 每分钟允许的登录尝试次数（防爆破），最小 1"},
	{Key: "refresh_rate_limit_per_min", Group: "auth", Type: "int", Value: "60",
		Desc: "每 IP 每分钟允许的令牌刷新次数（刷新比登录频繁，配额更宽松），最小 1"},

	// ---- 文件存储与 CDN（改后需重启：客户端在启动时构造）----
	// 凭据写错最坏结果是「上传失败」，改回来重启即可自愈，不会把自己锁在系统外，
	// 因此允许在界面维护；留空则回退同名环境变量。
	{Key: "storage_driver", Group: "storage", Type: "string", Restart: true, EnvVar: "STORAGE_DRIVER",
		Desc: "存储驱动：local(本地磁盘) | minio(MinIO/S3 兼容) | oss(阿里云 OSS)；留空回退环境变量"},
	{Key: "storage_endpoint", Group: "storage", Type: "string", Restart: true, EnvVar: "MINIO_ENDPOINT",
		Desc: "对象存储端点（不含协议头）；MinIO 对应 MINIO_ENDPOINT，OSS 对应 OSS_ENDPOINT"},
	{Key: "storage_bucket", Group: "storage", Type: "string", Restart: true, EnvVar: "MINIO_BUCKET",
		Desc: "对象存储桶名（MinIO 对应 MINIO_BUCKET，OSS 对应 OSS_BUCKET）"},
	{Key: "storage_access_key", Group: "storage", Type: "string", Restart: true, EnvVar: "MINIO_ACCESS_KEY",
		Desc: "对象存储 AccessKey（MinIO 的 access key / OSS 的 AccessKeyId）"},
	{Key: "storage_secret_key", Group: "storage", Type: "string", Secret: true, Restart: true, EnvVar: "MINIO_SECRET_KEY",
		Desc: "对象存储 SecretKey（敏感，界面只回传掩码）"},
	{Key: "storage_use_ssl", Group: "storage", Type: "bool", Restart: true, EnvVar: "MINIO_USE_SSL",
		Desc: "MinIO 是否走 HTTPS；留空回退环境变量 MINIO_USE_SSL"},
	{Key: "storage_public", Group: "storage", Type: "bool", Restart: true, EnvVar: "STORAGE_PUBLIC",
		Desc: "是否公开桶：false=私有桶，已存图片需限时签名才能访问"},
	{Key: "upload_dir", Group: "storage", Type: "string", Restart: true, EnvVar: "UPLOAD_DIR",
		Desc: "本地存储目录（仅 local 驱动；小程序码缓存也落这里）"},
	{Key: "upload_base_url", Group: "storage", Type: "string", Restart: true, EnvVar: "UPLOAD_BASE_URL",
		Desc: "上传文件公开访问基址（小程序 / 前端拼绝对 URL 用）"},
	{Key: "cdn_enabled", Group: "storage", Type: "bool", Restart: true, EnvVar: "CDN_ENABLED",
		Desc: "是否启用 CDN 缓存刷新（覆盖写后使边缘缓存失效）"},
	{Key: "cdn_access_key_id", Group: "storage", Type: "string", Restart: true, EnvVar: "CDN_ACCESS_KEY_ID",
		Desc: "CDN 刷新 AccessKeyId（阿里云，可与 OSS 凭据相同）"},
	{Key: "cdn_access_key_secret", Group: "storage", Type: "string", Secret: true, Restart: true, EnvVar: "CDN_ACCESS_KEY_SECRET",
		Desc: "CDN 刷新 AccessKeySecret（敏感）"},
	{Key: "cdn_endpoint", Group: "storage", Type: "string", Restart: true, EnvVar: "CDN_ENDPOINT",
		Desc: "CDN 刷新端点，留空默认 cdn.aliyuncs.com"},

	// ---- 安全与基础设施（只读：只能改环境变量后重启）----
	// 这三项改错会把「改回来的入口」一起弄丢（连不上库 / 登不进后台），故界面只展示。
	{Key: "db_dsn", Group: "security", Type: "string", Secret: true, Restart: true, ReadOnly: true, EnvVar: "DB_DSN",
		Desc: "数据库连接串（敏感，只读）。配置本身存在库里，改错就连不上库、再也读不出配置，因此只能改环境变量 DB_DSN 后重启"},
	{Key: "jwt_key", Group: "security", Type: "string", Secret: true, Restart: true, ReadOnly: true, EnvVar: "JWT_KEY",
		Desc: "JWT 签名密钥（敏感，只读）。改错会让所有已登录会话立即失效且无法回退，只能改环境变量 JWT_KEY 后重启"},
	{Key: "cookie_secure", Group: "security", Type: "bool", Restart: true, ReadOnly: true, EnvVar: "COOKIE_SECURE",
		Desc: "鉴权 Cookie 是否加 Secure 标记（生产 HTTPS 设 true）。改错会导致 http 环境登录不上，只能改环境变量 COOKIE_SECURE 后重启"},
	{Key: "cookie_domain", Group: "security", Type: "string", Restart: true, ReadOnly: true, EnvVar: "COOKIE_DOMAIN",
		Desc: "鉴权 Cookie 作用域（默认空=仅当前主机；跨子域共享登录态时才设）"},

	// 缺省空串 = 「未设置」，读取时回退环境变量 METRICS_ENABLED；
	// 一旦在界面显式设为 true/false 就以界面为准（避免升级后让既有部署的开关行为突变）。
	{Key: "metrics_enabled", Group: "feature", Type: "bool", EnvVar: "METRICS_ENABLED",
		Desc: "是否开放 /metrics（Prometheus 指标）；留空则跟随环境变量 METRICS_ENABLED"},
	// 开启后企业管理页整体只读：列表/管理员列表可查，但新建企业、编辑企业、
	// 启停企业、新建企业管理员一律拒绝（见 handler/companies.go 的 writeLocked）。
	{Key: "independent_lock", Group: "feature", Type: "bool", Value: "false",
		Desc: "独立版企业锁定：开启后企业管理页只读（不可新建/编辑/启停企业、不可加企业管理员）"},
}

// registryByKey 供按 key 快速查类型。
var registryByKey = func() map[string]def {
	m := make(map[string]def, len(registry))
	for _, d := range registry {
		m[d.Key] = d
	}
	return m
}()

// publicKeys 公开接口仅下发这些非敏感项（小程序 / 前端引导用）。
var publicKeys = map[string]bool{
	"brand_name":          true,
	"miniprogram_base_url": true,
	"pinned_company_id":   true,
}

// Store 内存缓存 + DB 持久化的设置存储。
type Store struct {
	db     *gorm.DB
	mu     sync.RWMutex
	values map[string]string
}

var (
	global *Store
	gmu    sync.RWMutex
)

// Init 首次启动时建表（AutoMigrate）、seed 缺省值、载入内存缓存，并置为全局实例。
// 必须在 handler 装配之前调用（cmd/server 与 devserver 均已接入）。
func Init(db *gorm.DB) error {
	s := &Store{db: db, values: map[string]string{}}
	if err := db.AutoMigrate(&Setting{}); err != nil {
		return fmt.Errorf("auto migrate sys_setting: %w", err)
	}
	if err := s.seedDefaults(); err != nil {
		return err
	}
	if err := s.reload(); err != nil {
		return err
	}
	gmu.Lock()
	global = s
	gmu.Unlock()
	return nil
}

// seedDefaults 把 registry 中尚未落库的 key 以默认值写入（幂等）。
// 只读项（ReadOnly）不落库：它们只能由环境变量决定，写进库反而会造成「界面没改、库里有值」的误解。
func (s *Store) seedDefaults() error {
	for _, d := range registry {
		if d.ReadOnly {
			continue
		}
		var cnt int64
		if err := s.db.Model(&Setting{}).Where("key = ?", d.Key).Count(&cnt).Error; err != nil {
			return err
		}
		if cnt > 0 {
			continue
		}
		if err := s.db.Create(&Setting{
			Key: d.Key, Group: d.Group, Value: d.Value, Type: d.Type, Description: d.Desc,
		}).Error; err != nil {
			return err
		}
	}
	return nil
}

// reload 全量载入当前值到内存（Put 后会调用，保证下次读取即时生效）。
func (s *Store) reload() error {
	var rows []Setting
	if err := s.db.Find(&rows).Error; err != nil {
		return err
	}
	m := make(map[string]string, len(rows))
	for _, r := range rows {
		m[r.Key] = r.Value
	}
	s.mu.Lock()
	s.values = m
	s.mu.Unlock()
	return nil
}

func (s *Store) getRaw(key string) (string, bool) {
	s.mu.RLock()
	v, ok := s.values[key]
	s.mu.RUnlock()
	return v, ok
}

// GetString 取字符串；缺失回落 def。
func (s *Store) GetString(key, def string) string {
	if v, ok := s.getRaw(key); ok {
		return v
	}
	return def
}

// GetInt 取整数；缺失 / 非法回落 def。
func (s *Store) GetInt(key string, def int) int {
	v, ok := s.getRaw(key)
	if !ok || v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

// GetBool 取布尔；缺失 / 非法回落 def（识别 1/true/yes/on 与 0/false/no/off）。
func (s *Store) GetBool(key string, def bool) bool {
	v, ok := s.getRaw(key)
	if !ok || v == "" {
		return def
	}
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return def
	}
}

// GetStringSlice 取逗号分隔的切片；缺失 / 空回落 def。
func (s *Store) GetStringSlice(key string, def []string) []string {
	v, ok := s.getRaw(key)
	if !ok || v == "" {
		return def
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return def
	}
	return out
}

// Set 校验类型后写入 DB 并刷新内存（key 必须已在 registry 登记）。
// 用于后台「系统设置」提交；任何一项非法整体失败，由调用方提示。
func (s *Store) Set(key, value string) error {
	d, ok := registryByKey[key]
	if !ok {
		return fmt.Errorf("未知设置项: %s", key)
	}
	if d.ReadOnly {
		return fmt.Errorf("%s 为只读项，只能通过环境变量 %s 修改后重启", key, d.EnvVar)
	}
	if d.Secret && value == Mask {
		return fmt.Errorf("%s 不能提交掩码值；留空表示回退环境变量", key)
	}
	if err := validateType(d.Type, value); err != nil {
		return fmt.Errorf("%s: %w", key, err)
	}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var existing Setting
		if err := tx.Where("key = ?", key).First(&existing).Error; err == nil {
			existing.Value = value
			return tx.Save(&existing).Error
		} else if !isNotFound(err) {
			return err
		}
		return tx.Create(&Setting{
			Key: d.Key, Group: d.Group, Value: value, Type: d.Type, Description: d.Desc,
		}).Error
	})
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.values[key] = value
	s.mu.Unlock()
	return nil
}

func isNotFound(err error) bool {
	return err != nil && (err == gorm.ErrRecordNotFound ||
		strings.Contains(strings.ToLower(err.Error()), "record not found"))
}

func validateType(typ, value string) error {
	switch typ {
	case "int":
		if _, err := strconv.Atoi(value); err != nil {
			return fmt.Errorf("必须为整数，收到 %q", value)
		}
	case "bool":
		if strings.TrimSpace(value) == "" {
			// 空串 = 「未在界面设置」，由调用方回退环境变量（见 metrics_enabled）
			return nil
		}
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "true", "false", "1", "0", "yes", "no", "on", "off":
		default:
			return fmt.Errorf("必须为布尔值(true/false)，收到 %q", value)
		}
	case "string_slice":
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("扩展名列表不能为空")
		}
	case "string", "":
		// 任意字符串均合法
	default:
		return fmt.Errorf("未知类型 %q", typ)
	}
	return nil
}

// ---- 包级便捷函数（读写全局实例；未初始化时读取安全回落默认值，写入报错） ----

func getStore() *Store {
	gmu.RLock()
	s := global
	gmu.RUnlock()
	if s == nil {
		return &Store{values: map[string]string{}}
	}
	return s
}

// GetString / GetInt / GetBool / GetStringSlice 包级封装。
func GetString(key, def string) string            { return getStore().GetString(key, def) }
func GetInt(key string, def int) int             { return getStore().GetInt(key, def) }
func GetBool(key string, def bool) bool          { return getStore().GetBool(key, def) }
func GetStringSlice(key string, def []string) []string {
	return getStore().GetStringSlice(key, def)
}

// Reload 重新从 DB 载入内存（Put 已即时刷新，此处供外部强制刷新）。
func Reload() error { return getStore().reload() }

// envSnapshot 进程启动时的「环境变量类配置」实际值快照（仅用于只读项展示，不落库）。
// 由 handler.RegisterSettings 注入（见 SetEnvSnapshot）：DB_DSN 等项即使环境变量没设，
// 进程内也有缺省值，只有注入快照才能如实告诉管理员「当前到底配了什么」。
var (
	envSnapshot   = map[string]string{}
	envSnapshotMu sync.RWMutex
)

// SetEnvSnapshot 注入环境变量类配置项的当前实际值（键为 registry 的 key，如 db_dsn）。
func SetEnvSnapshot(m map[string]string) {
	envSnapshotMu.Lock()
	envSnapshot = m
	envSnapshotMu.Unlock()
}

func envValue(key, envVar string) string {
	envSnapshotMu.RLock()
	v, ok := envSnapshot[key]
	envSnapshotMu.RUnlock()
	if ok && v != "" {
		return v
	}
	if envVar == "" {
		return ""
	}
	return os.Getenv(envVar)
}

// All 返回全部可配置项（含类型 / 描述 / 当前值 / 生效方式），供后台「系统设置」渲染分组表单。
// 以 registry 为序，保证界面稳定；敏感项只回传掩码，真实值不出接口。
func All() []Item {
	g := getStore()
	out := make([]Item, 0, len(registry))
	for _, d := range registry {
		stored := g.GetString(d.Key, d.Value)
		// 只读项不落库，值一律取进程内的环境变量实际值（由 envSnapshot 注入）。
		if d.ReadOnly {
			stored = envValue(d.Key, d.EnvVar)
		}
		it := Item{
			Key: d.Key, Group: d.Group, Type: d.Type,
			Description: d.Desc,
			Secret:      d.Secret, Restart: d.Restart, ReadOnly: d.ReadOnly, EnvVar: d.EnvVar,
		}
		it.Configured = configured(d, stored)
		if d.Secret {
			// 敏感项：已配置回传掩码（界面显示「已配置」），未配置回传空串
			if it.Configured {
				it.Value = Mask
			}
		} else {
			it.Value = stored
		}
		out = append(out, it)
	}
	return out
}

// IsRestart 该项保存后是否需重启进程才生效（云存储 / CDN 客户端在启动时构造）。
// PUT 接口据此在返回 message 里点名提示，避免管理员误以为「改了没生效」。
func IsRestart(key string) bool {
	d, ok := registryByKey[key]
	return ok && d.Restart
}

// configured 判断该项当前是否已有有效值：库值非空，或环境变量 / 进程快照非空。
// 布尔项只看「有没有被显式设置」——缺省 false 不应当显示成已配置。
func configured(d def, stored string) bool {
	if strings.TrimSpace(stored) != "" {
		return true
	}
	// 布尔项的「未设置」是空串而不是 "false"：缺省关的开关不应显示成已配置
	return envValue(d.Key, d.EnvVar) != ""
}

// Upsert 包级写入（校验类型）。未初始化时返回错误。
func Upsert(key, value string) error {
	s := getStore()
	if s.db == nil {
		return fmt.Errorf("系统设置未初始化")
	}
	return s.Set(key, value)
}

// Public 返回非敏感公开项（小程序 / 前端引导用）。
func Public() map[string]string {
	g := getStore()
	m := make(map[string]string, len(publicKeys))
	for k := range publicKeys {
		m[k] = g.GetString(k, "")
	}
	return m
}

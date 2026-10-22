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
	// Name 中文名：界面主标签。商业产品的配置页不能把 snake_case 的 key
	// 当标签甩给运营看（key 降级为次要灰字，排查日志/环境变量时仍可对照）。
	Name string
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
	// Options 枚举候选值（可选）：界面据此渲染下拉而不是文本框，
	// 与 Validate 的 oneOf 同源——写在一处旁边，改值时一起改，
	// 避免出现「下拉能选、保存被拒」或反之（2026-10-22 用户实测：
	// storage_driver 有 oneOf 校验却渲染成文本框，填错只能靠保存时报错）。
	Options []string
	// Validate 业务校验（可选）：**类型之外**的取值约束（上下限 / 枚举 / URL 前缀 /
	// 租户是否存在）。只给「填错会造成大面积故障或把自己锁住」的项加，不做普遍校验——
	// 处处拦截会让运营改个配置处处碰壁，不如让它自愈（见 validate.go 的包注释）。
	// 需要查库的校验也走这里，db 为当前库连接（未初始化时为 nil，校验需自行跳过）。
	Validate func(db *gorm.DB, value string) error
}

// Mask 敏感项在接口响应里的掩码（界面据此显示「已配置」，不回传真实值）。
const Mask = "******"

// Item 后台「系统设置」渲染用 DTO。
// 与 DB 模型 Setting 分开，避免把 secret 的真实值随表结构一并序列化出去。
type Item struct {
	Key         string `json:"key"`
	Name        string `json:"name"` // 中文名（界面主标签；为空时前端回落显示 key）
	Group       string `json:"group"`
	Type        string `json:"type"` // int | string | bool | string_slice
	Description string `json:"description"`
	// Value 当前值：敏感项恒为 Mask（已配置）或空串（未配置）。
	Value      string `json:"value"`
	Secret     bool   `json:"secret"`
	Restart    bool   `json:"restart"`
	ReadOnly   bool   `json:"readOnly"`
	EnvVar     string `json:"envVar"`
	// Options 枚举候选值（仅枚举项有）：界面据此渲染下拉。omitempty 保证
	// 旧客户端（不认这个字段）与大多数项（无候选值）的响应形状不变。
	Options    []string `json:"options,omitempty"`
	Configured bool     `json:"configured"` // 是否有有效值（库值 / 环境变量 / 进程内缺省）
}

// registry 全量「可后台配置项」清单：新增可配置项只在此追加一行即可。
// 分组（group）直接对应前端「系统设置」的分组表单：
//
//	general    通用（品牌名）
//	miniprogram 小程序（默认基址 / 独立版锁定租户）
//	upload     上传（单文件上限 / 允许扩展名 / 各类签名有效期）
//	wechat     微信（小程序码 TTL / AppID / AppSecret）
//	auth       登录与令牌（令牌有效期 / 登录与刷新限流）
//	storage    文件存储与 CDN（驱动 / 端点 / 桶 / 凭据 / 上传基址 / CDN 刷新）
//	security   安全与基础设施（只读：DB 连接串 / JWT 密钥 / Cookie）
//	feature    特性开关（Prometheus 指标 / 独立版企业锁定）
var registry = []def{
	{Key: "brand_name", Group: "general", Type: "string", Value: "微传名片",
		Name: "品牌名称",
		Desc: "显示在后台侧边栏、登录页和浏览器标签页上的名字。改完立即生效。"},
	{Key: "miniprogram_base_url", Group: "miniprogram", Type: "string",
		Name:     "小程序接口基址",
		Validate: httpURLOrEmpty(),
		// 说清「填错会怎样」比只说格式有用：这是首配最容易填错、且错了表现最远的一项
		Desc: "小程序访问后端的地址，必须是 http:// 或 https:// 开头的完整地址（例：https://api.example.com）。留空则小程序用自己内置的地址——首次部署务必填对，填错小程序会连不上后端。"},
	{Key: "pinned_company_id", Group: "miniprogram", Type: "int", Value: "0",
		Name:     "独立版锁定企业",
		Validate: companyEnabledOrZero,
		Desc:     "把整套系统锁定成某一个企业的独立版：填企业 ID 后，小程序只展示该企业的内容（该企业必须已存在且启用）；填 0 表示平台模式，不锁定。"},
	{Key: "upload_max_mb", Group: "upload", Type: "int", Value: "10",
		Name:     "单张图片大小上限",
		Validate: intRange(1, 1024),
		Desc:     "单张图片最大能传多大（MB），可填 1~1024。超过这个大小的图片会被直接拒绝。"},
	{Key: "upload_allow_ext", Group: "upload", Type: "string_slice", Value: "png,jpg,jpeg,gif,webp,mp4",
		Name: "允许上传的文件类型",
		Desc: "允许上传的文件后缀，小写、用英文逗号分隔（例：png,jpg,webp,mp4）。不在这个名单里的文件会被拒绝。"},
	// 视频单独一个上限：图片 10MB 够用，视频动辄几十上百 MB。
	// 若复用 upload_max_mb，为迁就视频把上限拉到 100MB，等于把图片的限制一起放开
	// （误传超大图片、占满磁盘都更难收场），故两条线分开。
	// 注意：本值只约束「前端选文件时的预校验」——直传预签名由对象存储直接收文件，
	// 应用服务器看不到内容，无法在服务端强拦；要真正封顶需在对象存储侧配 Post Policy。
	{Key: "upload_video_max_mb", Group: "upload", Type: "int", Value: "100",
		Name:     "单个视频大小上限",
		Validate: intRange(1, 2048),
		Desc:     "单个视频最大能传多大（MB），可填 1~2048。视频单独设上限，图片仍受「单张图片大小上限」约束。"},
	// 签名有效期：直传 PUT 与私有桶 GET 的限时链接时长。太短前端还没传完就过期，
	// 太长等于把对象存储写成公开读，故留界面可调，但代码内有下限兜底。
	{Key: "presign_put_ttl_minutes", Group: "upload", Type: "int", Value: "15",
		Name: "上传链接有效期",
		// 兜底语义写准：填 0 / 负数不是夹到 1 分钟，而是回落 registry 的缺省值
		// （见 signTTL）。写「最短 1 分钟」会让人以为 0 会被当成 1，与实际不符。
		Desc: "上传文件时生成的临时直传链接能用多久（分钟）。太短可能传到一半就过期；填 0 或负数会回到默认 15 分钟。"},
	{Key: "sign_get_ttl_minutes", Group: "upload", Type: "int", Value: "30",
		Name: "私有桶下载链接有效期",
		Desc: "私有存储里的文件，生成的临时访问链接能用多久（分钟）。填 0 或负数会回到默认 30 分钟。"},
	{Key: "wxacode_ttl_hours", Group: "wechat", Type: "int", Value: "720",
		Name: "小程序码缓存时长",
		Desc: "生成的小程序码图片缓存多久（小时），到期后会自动重新生成。未设置或填 0 会用部署环境的默认值（默认 720 小时，约 30 天）。"},
	{Key: "wxacode_sign_ttl_minutes", Group: "wechat", Type: "int", Value: "60",
		Name: "小程序码签名链接有效期",
		Desc: "私有存储里小程序码的临时访问链接能用多久（分钟）。填 0 或负数会回到默认 60 分钟。"},
	{Key: "wx_appid", Group: "wechat", Type: "string", EnvVar: "WX_APPID",
		Name: "小程序 AppID",
		Desc: "微信小程序的 AppID，用来生成小程序码和做微信登录。留空则使用部署环境里配置的值（环境变量 WX_APPID 或配置文件）。"},
	{Key: "wx_appsecret", Group: "wechat", Type: "string", Secret: true, EnvVar: "WX_APPSECRET",
		Name: "小程序 AppSecret",
		Desc: "微信小程序的 AppSecret，用来生成小程序码。属于敏感信息，界面不会显示已保存的内容，填新值即覆盖。留空则使用部署环境里配置的值（环境变量 WX_APPSECRET 或配置文件）。"},

	// ---- 登录与令牌（即时生效）----
	// 令牌有效期设太短会把人一直踢下线；代码内保留**安全下限**，
	// 界面填 0 / 负数时回落缺省，避免管理员把自己锁在系统外（改错可自愈）。
	{Key: "access_token_ttl_minutes", Group: "auth", Type: "int", Value: "15",
		Name: "登录凭证有效期",
		Desc: "登录后凭证能用多久（分钟），到期后需要换新的。改完对新登录的人立即生效，已经登录的人不受影响。填 0 或负数会回到默认 15 分钟。"},
	{Key: "refresh_token_ttl_days", Group: "auth", Type: "int", Value: "30",
		Name: "免登录保持时长",
		Desc: "「保持登录」最多能维持多少天，到期后必须重新登录。填 0 或负数会回到默认 30 天。"},
	{Key: "login_rate_limit_per_min", Group: "auth", Type: "int", Value: "10",
		Name: "登录尝试频率限制",
		Desc: "同一个 IP 每分钟最多允许试几次登录，用来防暴力破解。填 0 或负数会回到默认 10 次——不会把这道防护关掉。"},
	{Key: "refresh_rate_limit_per_min", Group: "auth", Type: "int", Value: "60",
		Name: "凭证刷新频率限制",
		Desc: "同一个 IP 每分钟最多允许刷新几次凭证。刷新比登录频繁，所以配额给得更宽。填 0 或负数会回到默认 60 次。"},

	// ---- 文件存储与 CDN（改后需重启：客户端在启动时构造）----
	// 凭据写错最坏结果是「上传失败」，改回来重启即可自愈，不会把自己锁在系统外，
	// 因此允许在界面维护；留空则回退同名环境变量。
	{Key: "storage_driver", Group: "storage", Type: "string", Restart: true, EnvVar: "STORAGE_DRIVER",
		Name:     "存储方式",
		Options:  []string{"local", "minio", "oss"},
		Validate: oneOf("local", "minio", "oss"),
		Desc:     "图片和视频存到哪里：local=服务器本地磁盘，minio=MinIO 或 S3 兼容存储，oss=阿里云 OSS。留空则使用部署环境里配置的值。改完要重启服务才生效。"},
	{Key: "storage_endpoint", Group: "storage", Type: "string", Restart: true, EnvVar: "MINIO_ENDPOINT",
		Name: "存储服务端点",
		Desc: "对象存储的服务地址，只填域名或 IP:端口，不要带 http://（例：127.0.0.1:9000）。MinIO 和阿里云 OSS 共用这一项。"},
	{Key: "storage_bucket", Group: "storage", Type: "string", Restart: true, EnvVar: "MINIO_BUCKET",
		Name: "存储空间名称",
		Desc: "对象存储里存放文件的桶名（MinIO 与阿里云 OSS 都叫 bucket）。上传的文件都会存进这个桶。"},
	{Key: "storage_access_key", Group: "storage", Type: "string", Restart: true, EnvVar: "MINIO_ACCESS_KEY",
		Name: "存储 AccessKey",
		Desc: "对象存储的用户名（阿里云 OSS 里叫 AccessKeyId）。改完要重启服务才生效。"},
	{Key: "storage_secret_key", Group: "storage", Type: "string", Secret: true, Restart: true, EnvVar: "MINIO_SECRET_KEY",
		Name: "存储 SecretKey",
		Desc: "对象存储的密码（阿里云 OSS 里叫 AccessKeySecret）。界面不会显示已保存的内容，填新值即覆盖。改完要重启服务才生效。"},
	{Key: "storage_use_ssl", Group: "storage", Type: "bool", Restart: true, EnvVar: "MINIO_USE_SSL",
		Name: "存储启用 HTTPS",
		Desc: "连接对象存储时是否走 HTTPS。留空则使用部署环境里配置的值。改完要重启服务才生效。"},
	{Key: "storage_public", Group: "storage", Type: "bool", Restart: true, EnvVar: "STORAGE_PUBLIC",
		Name: "存储空间公开访问",
		Desc: "存储桶是否允许公开访问。选「关」（私有桶）时，已上传的图片只能通过限时签名链接访问。改完要重启服务才生效。"},
	{Key: "upload_dir", Group: "storage", Type: "string", Restart: true, EnvVar: "UPLOAD_DIR",
		Name: "本地上传目录",
		Desc: "存储方式选 local 时，文件保存在服务器上的哪个目录（小程序码缓存也放这里）。换成其它存储方式后这一项不再起作用。改完要重启服务才生效。"},
	{Key: "upload_base_url", Group: "storage", Type: "string", Restart: true, EnvVar: "UPLOAD_BASE_URL",
		Name:     "文件访问基址",
		Validate: httpURLOrEmpty(),
		Desc:     "已上传文件对外访问的根地址，必须是 http:// 或 https:// 开头的完整地址（例：https://cdn.example.com）。小程序和后台都用它拼出完整的图片链接，填错的话所有图片都会打不开。改完要重启服务才生效。"},
	{Key: "cdn_enabled", Group: "storage", Type: "bool", Restart: true, EnvVar: "CDN_ENABLED",
		Name: "启用 CDN 刷新",
		Desc: "覆盖同名文件后是否自动刷新 CDN 缓存，让访客立刻看到新文件。要同时填好下面的 CDN 凭据才有用。改完要重启服务才生效。"},
	{Key: "cdn_access_key_id", Group: "storage", Type: "string", Restart: true, EnvVar: "CDN_ACCESS_KEY_ID",
		Name: "CDN AccessKeyId",
		Desc: "阿里云 CDN 刷新用的 AccessKeyId，可以和 OSS 的凭据相同。改完要重启服务才生效。"},
	{Key: "cdn_access_key_secret", Group: "storage", Type: "string", Secret: true, Restart: true, EnvVar: "CDN_ACCESS_KEY_SECRET",
		Name: "CDN AccessKeySecret",
		Desc: "阿里云 CDN 刷新用的 AccessKeySecret。界面不会显示已保存的内容，填新值即覆盖。改完要重启服务才生效。"},
	{Key: "cdn_endpoint", Group: "storage", Type: "string", Restart: true, EnvVar: "CDN_ENDPOINT",
		Name: "CDN 服务端点",
		Desc: "阿里云 CDN 的刷新接口地址，留空默认 cdn.aliyuncs.com，一般不用改。"},

	// ---- 安全与基础设施（只读：只能改环境变量后重启）----
	// 这三项改错会把「改回来的入口」一起弄丢（连不上库 / 登不进后台），故界面只展示。
	{Key: "db_dsn", Group: "security", Type: "string", Secret: true, Restart: true, ReadOnly: true, EnvVar: "DB_DSN",
		Name: "数据库连接串",
		// 改法要写两条：环境变量**或**配置文件。多数部署走安装向导，DB_DSN 就
		// 落在 data/runtime.env 里而没有同名环境变量，只提环境变量等于指错路。
		// 「不保存在数据库里」必须说清：否则会被读成「连接串存在库里」，与本项
		// ReadOnly 不落库（seedDefaults 跳过、Set 拒绝）的实现正好相反。
		Desc: "当前服务连的是哪个数据库（下方可查看，密码部分已隐藏）。这一项在界面上不能改：其余配置都保存在数据库里，连接串一旦填错，重启后连不上数据库，连改回来的入口都会一起消失。如需修改，请到部署环境改环境变量 DB_DSN 或配置文件（默认 data/runtime.env），再重启服务。"},
	{Key: "jwt_key", Group: "security", Type: "string", Secret: true, Restart: true, ReadOnly: true, EnvVar: "JWT_KEY",
		Name: "登录签名密钥",
		Desc: "给登录凭证签名的密钥（下方可查看首尾，中间已隐藏）。这一项在界面上不能改：改错会让所有已登录的人立刻掉线，而且没法回退。如需修改，请到部署环境改环境变量 JWT_KEY 或配置文件（默认 data/runtime.env），再重启服务。"},
	{Key: "cookie_secure", Group: "security", Type: "bool", Restart: true, ReadOnly: true, EnvVar: "COOKIE_SECURE",
		Name: "登录 Cookie 强制 HTTPS",
		Desc: "登录 Cookie 是否只在 HTTPS 下传输。正式上线（HTTPS）应为「开」；本地 http 环境选「开」会导致登录不上。界面不能改，请到部署环境改环境变量 COOKIE_SECURE 或配置文件（默认 data/runtime.env）后重启。"},
	{Key: "cookie_domain", Group: "security", Type: "string", Restart: true, ReadOnly: true, EnvVar: "COOKIE_DOMAIN",
		Name: "登录 Cookie 作用域",
		Desc: "登录 Cookie 在哪个范围内有效。留空只对当前域名生效；需要在多个子域名之间共享登录状态时才填（例：.example.com）。界面不能改，请到部署环境改环境变量 COOKIE_DOMAIN 或配置文件（默认 data/runtime.env）后重启。"},

	// 缺省空串 = 「未设置」，读取时回退环境变量 METRICS_ENABLED；
	// 一旦在界面显式设为 true/false 就以界面为准（避免升级后让既有部署的开关行为突变）。
	{Key: "metrics_enabled", Group: "feature", Type: "bool", EnvVar: "METRICS_ENABLED",
		Name: "开放监控指标",
		Desc: "是否开放 /metrics 监控接口，供 Prometheus 抓取数据。留空则使用部署环境里配置的值。"},
	// 开启后企业管理页整体只读：列表/管理员列表可查，但新建企业、编辑企业、
	// 启停企业、新建企业管理员一律拒绝（见 handler/companies.go 的 writeLocked）。
	{Key: "independent_lock", Group: "feature", Type: "bool", Value: "false",
		Name: "独立版企业锁定",
		Desc: "开启后企业管理页变成只读：不能新建、编辑、启停企业，也不能添加企业管理员。适合交付给单一企业使用的独立版。"},
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
	"brand_name":           true,
	"miniprogram_base_url": true,
	"pinned_company_id":    true,
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
//
// 建表加 HasTable 门闩的原因（2026-10-21 实证，别改回去）：
// MySQL 下 sys_setting 由 migrations/0014 建（UNIQUE KEY uk_setting_key），
// 而模型 Key 用的是 uniqueIndex（gorm 自动名 uni_sys_setting_key）而非 unique
// tag——field.Unique=false。gorm 的 MigrateColumnUnique 看到「库里的 key 列带
// 唯一索引（column_key=UNI）而模型没有 unique tag」，误判为「模型删掉了唯一
// 约束」，按命名策略名执行 DROP CONSTRAINT uni_sys_setting_key；mysql driver
// v1.5.2 的 DropConstraint 无条件发 ALTER TABLE ... DROP FOREIGN KEY（v1.3.6
// 还有 nil 守卫，5.x 删了），而库里实际叫 uk_setting_key → Error 1091，
// 安装向导与 MySQL 形态启动全部卡死在这。
// 表已存在时跳过 AutoMigrate 即绕开该假阳性；MySQL 的表结构本就归 migrations
// 管（与 main.go 对 MySQL 不 AutoMigrate 同一策略），缺列应补迁移而非靠
// AutoMigrate 擅自改。SQLite 下 HasTable 恒同步，行为不变。
func Init(db *gorm.DB) error {
	s := &Store{db: db, values: map[string]string{}}
	if !db.Migrator().HasTable(&Setting{}) {
		if err := db.AutoMigrate(&Setting{}); err != nil {
			return fmt.Errorf("auto migrate sys_setting: %w", err)
		}
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
		// key 是 MySQL 保留字：必须用 map/struct 条件让 gorm 按方言加引号
		// （裸字符串 "key = ?" 不会被 gorm 补反引号，MySQL 直接 1064；
		// SQLite 宽容所以潜伏至今——2026-10-21 parity 首跑抓出）
		if err := s.db.Model(&Setting{}).Where(map[string]interface{}{"key": d.Key}).Count(&cnt).Error; err != nil {
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
		// 措辞不能只说「环境变量」：优先级是 环境变量 > 配置文件 > 内置缺省，
	// 而安装向导正是把 DB_DSN / JWT_KEY / COOKIE_* 写进配置文件（data/runtime.env）。
	// 走向导装的部署根本没有环境变量，说「只能改环境变量」会让人找不到东西改。
	return fmt.Errorf("%s 为只读项，只能改环境变量 %s 或配置文件（默认 data/runtime.env）后重启", key, d.EnvVar)
	}
	if d.Secret && value == Mask {
		return fmt.Errorf("%s 不能提交掩码值；留空表示回退环境变量", key)
	}
	if err := validateType(d.Type, value); err != nil {
		return fmt.Errorf("%s: %w", key, err)
	}
	// 业务校验（值域 / 枚举 / 租户存在性）：类型合法不代表取值合法，
	// 例如上传上限填 0 会让所有上传失败，基址漏写协议头会让小程序永远拿不到正确地址。
	if d.Validate != nil {
		if err := d.Validate(s.db, value); err != nil {
			return fmt.Errorf("%s: %w", key, err)
		}
	}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var existing Setting
		// 同 seedDefaults：key 是 MySQL 保留字，用 map 条件让 gorm 加引号
		if err := tx.Where(map[string]interface{}{"key": key}).First(&existing).Error; err == nil {
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
func GetString(key, def string) string  { return getStore().GetString(key, def) }
func GetInt(key string, def int) int    { return getStore().GetInt(key, def) }
func GetBool(key string, def bool) bool { return getStore().GetBool(key, def) }
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
			Key: d.Key, Name: d.Name, Group: d.Group, Type: d.Type,
			Description: d.Desc,
			Secret:      d.Secret, Restart: d.Restart, ReadOnly: d.ReadOnly, EnvVar: d.EnvVar,
			Options:     d.Options,
		}
		it.Configured = configured(d, stored)
		if d.Secret {
			// 敏感项：已配置的**可写项**回传掩码（界面显示「已配置」，填新值才覆盖）。
			// 只读项例外——它界面改不了，展示它的全部意义就是让人确认「当前配的是不是这个」，
			// 六颗星等于什么都没说，出问题时无法核对连的是哪个库 / 用的是哪个密钥。
			// 但也不能整串明文：连接串含数据库口令、JWT 密钥可伪造登录态。
			// 折中为「可辨认预览」：保留结构与首尾、只遮中段（与云厂商控制台同一做法）。
			if it.Configured {
				it.Value = Mask
				if d.ReadOnly {
					it.Value = preview(stored)
				}
			}
		} else {
			it.Value = stored
		}
		out = append(out, it)
	}
	return out
}

// preview 敏感值的「可辨认预览」：能核对是不是自己配的那一个，但不整串泄露。
//
// 只用于只读项（db_dsn / jwt_key）。它们界面改不了，展示的目的就是核对；
// 而六颗星无法核对，整串明文又是把数据库口令和签名密钥摊在页面上
// （截图、肩窥、录屏都会泄露）。折中规则：
//   - 连接串（user:pass@tcp(host:port)/db）：保留结构，只遮口令那一段；
//   - 无凭据形态（sqlite:///path 等含 "://"）：原样返回，本来就没有秘密；
//   - 其它（如密钥）：首尾各留 4 位，中间遮掉。
func preview(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	// sqlite:///xxx、file:///xxx 这类不含凭据，直接显示更好核对
	if strings.Contains(v, "://") {
		return v
	}
	// user:password@tcp(host:port)/db?param → user:******@tcp(host:port)/db?param
	if at := strings.IndexByte(v, '@'); at > 0 {
		if c := strings.IndexByte(v[:at], ':'); c > 0 {
			return v[:c+1] + Mask + v[at:]
		}
	}
	// 露头尾的位数随长度伸缩：长密钥露 4 位足矣；短值（如 dev 缺省密钥）露多了
	// 约等于全文，只露 2 位，既保留「能认出是不是它」，又不至于几乎完整泄露。
	keep := 4
	if len(v) < 12 {
		keep = 2
	}
	if len(v) < 8 {
		// 太短：怎么露都约等于全文，退回掩码
		return Mask
	}
	return v[:keep] + "****" + v[len(v)-keep:]
}

// IsSecret 该项是否为敏感项（密钥 / 连接串）：接口只回传掩码，审计详情也不能记明文。
func IsSecret(key string) bool {
	d, ok := registryByKey[key]
	return ok && d.Secret
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

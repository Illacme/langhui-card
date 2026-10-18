// Package setting 系统运行参数的「后台可视化配置」基础设施。
//
// 设计目标（与产品诉求对齐）：
//   - 把过去散落在 config.go / main.go / storage.go / auth.go 等处的「写死值」
//     收敛为一张 sys_setting 表，平台超管可在 PC 后台「系统设置」界面统一维护，
//     无需改代码、无需重启即可生效（运行时内存缓存 + 即时回写）。
//   - 密钥类（DB DSN / JWT / 云存储与 CDN 凭据 / 微信 AppSecret / Cookie 安全）
//     始终留环境变量（轨 B），不进本表，避免敏感信息落入可界面编辑的范围。
//
// 约定：
//   - 所有「可后台配置项」必须在 registry 登记（含类型与默认值），前端分组表单与之对应；
//     新增可配置项 = 在 registry 追加一行 + 前端加一项，缺省值会在 Init 时自动 seed 进库。
//   - 取值一律带默认值：库未初始化 / 该 key 缺失 时回落默认值，保证任意阶段调用都安全。
package setting

import (
	"fmt"
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

// def 单条可配置项的静态定义（类型契约 + 默认值 + 文案）。
type def struct {
	Key, Group, Type, Value, Desc string
}

// registry 全量「可后台配置项」清单：新增可配置项只在此追加一行即可。
// 分组（group）直接对应前端「系统设置」的分组表单：
//   general    通用（品牌名）
//   miniprogram 小程序（默认基址 / 独立版锁定租户）
//   upload     上传（单文件上限 / 允许扩展名）
//   wechat     微信（小程序码 TTL / AppID）
//   feature    特性开关（Prometheus 指标 / 独立版企业锁定）
var registry = []def{
	{"brand_name", "general", "string", "智能名片", "后台侧边栏 / 页标题品牌名（未设置回落此值）"},
	{"miniprogram_base_url", "miniprogram", "string", "", "小程序端默认接口基址；空则小程序用自身内置 baseUrl"},
	{"pinned_company_id", "miniprogram", "int", "0", "企业独立版锁定租户：>0 锁定该企业，0=平台模式"},
	{"upload_max_mb", "upload", "int", "10", "单文件上传上限（MB）"},
	{"upload_allow_ext", "upload", "string_slice", "png,jpg,jpeg,gif,webp,mp4", "允许上传的扩展名（小写，逗号分隔）"},
	{"wxacode_ttl_hours", "wechat", "int", "720", "小程序码缓存有效期（小时）"},
	{"wx_appid", "wechat", "string", "", "小程序 AppID（小程序码与微信登录用；留空回退环境变量 WX_APPID。AppSecret 仍留环境变量）"},
	// 缺省空串 = 「未设置」，读取时回退环境变量 METRICS_ENABLED；
	// 一旦在界面显式设为 true/false 就以界面为准（避免升级后让既有部署的开关行为突变）。
	{"metrics_enabled", "feature", "bool", "", "是否开放 /metrics（Prometheus 指标）；留空则跟随环境变量 METRICS_ENABLED"},
	// TODO(未接线)：仅登记到界面，尚无任何代码读取——界面可存但当前不影响行为。
	// 待产品定义「锁定后具体禁掉哪些操作」（企业启停？企业管理页只读？）后再实现；
	// 在此之前保留可见，是为了让管理员看到规划中的开关，而不是假装已生效。
	{"independent_lock", "feature", "bool", "false", "独立版企业锁定（暂未生效：锁定范围待定义，当前开启不影响任何行为）"},
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
func (s *Store) seedDefaults() error {
	for _, d := range registry {
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

// All 返回全部可配置项（含类型 / 描述 / 当前值），供后台「系统设置」渲染分组表单。
// 以 registry 为序，保证界面稳定；值取内存当前值（缺失则缺省值）。
func All() []Setting {
	g := getStore()
	out := make([]Setting, 0, len(registry))
	for _, d := range registry {
		out = append(out, Setting{
			Key: d.Key, Group: d.Group, Type: d.Type,
			Description: d.Desc, Value: g.GetString(d.Key, d.Value),
		})
	}
	return out
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

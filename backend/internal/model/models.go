package model

import (
	"time"

	"gorm.io/gorm"
)

// Employee 员工即名片主体；staff_id 为对外唯一编号
// Landline（座机）为员工个人办公电话，与 company_info.tel（公司总机）是两个层级：
// 小程序名片「座机」行优先展示本人的，为空时回退到公司总机。
// CompanyID 为租户外键：员工归属企业；登录令牌携带该值实现后台按租户隔离。
type Employee struct {
	StaffID      int64     `gorm:"primaryKey;autoIncrement;column:staff_id" json:"staffId"`
	CompanyID    int64     `gorm:"column:company_id;index" json:"companyId"`
	Name         string    `gorm:"column:name" json:"name"`
	Title        string    `gorm:"column:title" json:"title"` // 职位（对外展示）
	Avatar       string    `gorm:"column:avatar" json:"avatar"`
	Phone        string    `gorm:"column:phone" json:"phone"`       // 手机
	Landline     string    `gorm:"column:landline" json:"landline"` // 座机
	Email        string    `gorm:"column:email" json:"email"`
	OpenID       string    `gorm:"column:openid" json:"openid"`     // 微信 openid（登录时采集，供后台核实身份）
	Nickname     string    `gorm:"column:nickname" json:"nickname"` // 微信昵称（登录时授权采集）
	PasswordHash string    `gorm:"column:password_hash" json:"-"`
	Status       string    `gorm:"column:status;default:enabled" json:"status"` // enabled / disabled
	TokenVersion int       `gorm:"column:token_version;default:0" json:"-"`     // 安全戳：改密/禁用自增，令旧令牌失效
	CreatedAt    time.Time `gorm:"column:created_at" json:"createdAt"`
}

// Company 租户（企业）。多租户第一期：朗汇机电为 1 号租户，存量数据全部归属。
// 与 CompanyInfo 是 1:1——CompanyInfo.company_id 指向本表主键，企业基础信息
// （全称/简称/地址/电话/LOGO）按租户隔离。
// status: enabled / disabled；停用后该企业的管理员与员工均不能登录（见 handler/auth.go）。
type Company struct {
	ID        int64     `gorm:"primaryKey;autoIncrement;column:id" json:"id"`
	Name      string    `gorm:"column:name" json:"name"`
	ShortName string    `gorm:"column:short_name" json:"shortName"` // 企业简称：PC 后台侧边栏品牌名 + 公司信息简称的初始值；公司信息里的简称仅用于小程序展示，两者相互独立
	Status    string    `gorm:"column:status;default:enabled" json:"status"`
	CreatedAt time.Time `gorm:"column:created_at" json:"created_at"`
}

// TableName 显式指定单数表名：GORM 默认会把 Company 复数化为 companies，
// 与 migrations/0011 的 company 表不一致（开发/生产两张名）。
func (Company) TableName() string { return "company" }

// CompanyInfo 企业基础信息（首页固定展示）。
// company_id 为租户外键（唯一）：一个企业一行；历史 id=1 单行迁移为 company_id=1（朗汇）。
type CompanyInfo struct {
	ID        int64  `gorm:"primaryKey;column:id" json:"id"`
	CompanyID int64  `gorm:"column:company_id;uniqueIndex" json:"companyId"`
	FullName  string `gorm:"column:full_name" json:"fullName"`
	ShortName string `gorm:"column:short_name" json:"shortName"` // 企业简称（小程序顶栏居中展示）
	Address   string `gorm:"column:address" json:"address"`
	Tel       string `gorm:"column:tel" json:"tel"`
	Logo      string `gorm:"column:logo" json:"logo"`
}

// CompanyProfile 公司简介 + 视频号 + 备用 MP4（按租户隔离，company_id 唯一）
type CompanyProfile struct {
	ID             int64  `gorm:"primaryKey;column:id" json:"id"`
	CompanyID      int64  `gorm:"column:company_id;uniqueIndex" json:"companyId"`
	IntroText      string `gorm:"column:intro_text;type:text" json:"introText"`
	VideoChannelID string `gorm:"column:video_channel_id" json:"videoChannelId"`
	BackupMp4URL   string `gorm:"column:backup_mp4_url" json:"backupMp4Url"`
	Cover          string `gorm:"column:cover" json:"cover"`
}

// AchievementCategory 经典业绩分类（种子 7 类，按租户隔离）
type AchievementCategory struct {
	ID        int64          `gorm:"primaryKey;column:id" json:"id"`
	CompanyID int64          `gorm:"column:company_id;index" json:"companyId"`
	Name      string         `gorm:"column:name" json:"name"`
	Icon      string         `gorm:"column:icon" json:"icon"`
	Sort      int            `gorm:"column:sort" json:"sort"`
	IsLocked  bool           `gorm:"column:is_locked;default:false" json:"isLocked"`
	DeletedAt gorm.DeletedAt `gorm:"column:deleted_at" json:"-"`
}

// ProjectCase 项目案例（按租户隔离）
type ProjectCase struct {
	ID             int64          `gorm:"primaryKey;column:id" json:"id"`
	CompanyID      int64          `gorm:"column:company_id;index" json:"companyId"`
	CategoryID     int64          `gorm:"column:category_id;index" json:"categoryId"`
	Name           string         `gorm:"column:name" json:"name"`
	Summary        string         `gorm:"column:summary" json:"summary"`
	Detail         string         `gorm:"column:detail;type:text" json:"detail"`
	Cover          string         `gorm:"column:cover" json:"cover"`
	Album          string         `gorm:"column:album;type:json" json:"-"` // 存 JSON 数组字符串
	IsHomeFeatured bool           `gorm:"column:is_home_featured;default:false" json:"isHomeFeatured"`
	Enabled        bool           `gorm:"column:enabled;default:true" json:"enabled"`
	Sort           int            `gorm:"column:sort" json:"sort"`
	CreatedAt      time.Time      `gorm:"column:created_at" json:"createdAt"`
	UpdatedAt      time.Time      `gorm:"column:updated_at" json:"updatedAt"`
	DeletedAt      gorm.DeletedAt `gorm:"column:deleted_at" json:"-"`

	// CategoryName 非持久化：列表/详情接口回填分类名，避免前端二次查询
	// （见《跳转页组件与接口字段映射》§4）。gorm:"-" 让 GORM 忽略它。
	CategoryName string `gorm:"-" json:"categoryName,omitempty"`
}

// BizService 业务服务（富文本含视频号块，按租户隔离）
type BizService struct {
	ID        int64          `gorm:"primaryKey;column:id" json:"id"`
	CompanyID int64          `gorm:"column:company_id;index" json:"companyId"`
	Title     string         `gorm:"column:title" json:"title"`
	Icon      string         `gorm:"column:icon" json:"icon"` // 业务服务图标（首页服务区卡片展示）
	Content   string         `gorm:"column:content;type:text" json:"content"`
	Sort      int            `gorm:"column:sort" json:"sort"`
	CreatedAt time.Time      `gorm:"column:created_at" json:"createdAt"`
	UpdatedAt time.Time      `gorm:"column:updated_at" json:"updatedAt"`
	DeletedAt gorm.DeletedAt `gorm:"column:deleted_at" json:"-"`
}

// HomeLayout 首页板块顺序（JSON 数组，按租户隔离，company_id 唯一）
type HomeLayout struct {
	ID        int64  `gorm:"primaryKey;column:id" json:"id"`
	CompanyID int64  `gorm:"column:company_id;uniqueIndex" json:"companyId"`
	Order     string `gorm:"column:order_col;type:json" json:"order"`
}

// StatEvent 埋点原始事件
type StatEvent struct {
	ID        int64     `gorm:"primaryKey;column:id;autoIncrement" json:"id"`
	StaffID   int64     `gorm:"column:staff_id;index" json:"staffId"`
	EventType string    `gorm:"column:event_type;index" json:"eventType"`
	StatDate  string    `gorm:"column:stat_date;index" json:"statDate"` // YYYY-MM-DD
	CreatedAt time.Time `gorm:"column:created_at" json:"createdAt"`
}

// Admin 后台管理员（账号 + 密码登录，非微信 openid）。
// 两级角色（多租户第一期）：
//   - company_id = 0：平台超级管理员，可见/管理全部租户（企业管理入口仅对其开放）；
//   - company_id > 0：企业管理员，仅可见/管理本企业的数据（登录时校验企业未停用）。
type Admin struct {
	ID           int64     `gorm:"primaryKey;autoIncrement;column:id" json:"id"`
	CompanyID    int64     `gorm:"column:company_id;index" json:"companyId"` // 0=平台超管
	Username     string    `gorm:"column:username" json:"username"`
	PasswordHash string    `gorm:"column:password_hash" json:"-"`
	TokenVersion int       `gorm:"column:token_version;default:0" json:"-"` // 安全戳：改密自增，令旧令牌失效
	CreatedAt    time.Time `gorm:"column:created_at" json:"createdAt"`
}

// RevokedSession 已吊销会话黑名单（单设备精确吊销）。
// 一次登录对应一个 sid（见 auth.Claims.SID，access/refresh 共享）；登出时把 sid 写入本表，
// 之后该会话签发的所有令牌立即失效。与 token_version 互补：
//   - token_version：账号级，改密/禁用后该用户所有设备全部失效；
//   - 本表：会话级，只失效指定设备，同一账号的其他登录不受影响。
//
// expires_at 仅用于后台按时间清理过期记录，不参与吊销判定（判定只看 sid 是否存在）。
type RevokedSession struct {
	SID       string    `gorm:"primaryKey;column:sid" json:"sid"`
	UserType  string    `gorm:"column:user_type" json:"userType"` // admin / employee
	UserID    int64     `gorm:"column:user_id" json:"userId"`     // admin.id 或 employee.staff_id
	ExpiresAt time.Time `gorm:"column:expires_at;index" json:"expiresAt"`
	CreatedAt time.Time `gorm:"column:created_at" json:"createdAt"`
}

// OpenApply 员工自助开通申请（审核流，按租户隔离：申请归属企业，审核后员工落到同企业）
type OpenApply struct {
	ID           int64     `gorm:"primaryKey;autoIncrement;column:id" json:"id"`
	CompanyID    int64     `gorm:"column:company_id;index" json:"companyId"`
	Name         string    `gorm:"column:name" json:"name"`
	Phone        string    `gorm:"column:phone" json:"phone"`
	Email        string    `gorm:"column:email" json:"email"`
	Title        string    `gorm:"column:title" json:"title"`
	Landline     string    `gorm:"column:landline" json:"landline"` // 座机（选填）
	Avatar       string    `gorm:"column:avatar" json:"avatar"`
	OpenID       string    `gorm:"column:openid" json:"openid"`     // 提交申请时的微信 openid
	Nickname     string    `gorm:"column:nickname" json:"nickname"` // 提交申请时的微信昵称
	Status       string    `gorm:"column:status;default:pending" json:"status"` // pending/approved/rejected
	RejectReason string    `gorm:"column:reject_reason" json:"rejectReason"`
	CreatedAt    time.Time `gorm:"column:created_at" json:"createdAt"`
}

// CardModifyApply 员工名片信息修改申请（审核流，对外仍显示旧信息直到通过）。
// company_id 冗余自员工归属租户：后台按企业隔离列表时免 join。
type CardModifyApply struct {
	ID           int64     `gorm:"primaryKey;autoIncrement;column:id" json:"id"`
	CompanyID    int64     `gorm:"column:company_id;index" json:"companyId"`
	StaffID      int64     `gorm:"column:staff_id;index" json:"staffId"`
	Snapshot     string    `gorm:"column:snapshot;type:json" json:"snapshot"` // JSON：变更字段 {avatar,name,title,phone,landline,email}
	Status       string    `gorm:"column:status;default:pending" json:"status"`
	RejectReason string    `gorm:"column:reject_reason" json:"rejectReason"`
	CreatedAt    time.Time `gorm:"column:created_at" json:"createdAt"`
}

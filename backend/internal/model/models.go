package model

import "time"

// Employee 员工即名片主体；staff_id 为对外唯一编号
type Employee struct {
	StaffID      int64     `gorm:"primaryKey;autoIncrement;column:staff_id" json:"staffId"`
	Name         string    `gorm:"column:name" json:"name"`
	Title        string    `gorm:"column:title" json:"title"`
	Avatar       string    `gorm:"column:avatar" json:"avatar"`
	Phone        string    `gorm:"column:phone" json:"phone"`
	Email        string    `gorm:"column:email" json:"email"`
	PasswordHash string    `gorm:"column:password_hash" json:"-"`
	Status       string    `gorm:"column:status;default:enabled" json:"status"` // enabled / disabled
	CreatedAt    time.Time `gorm:"column:created_at" json:"createdAt"`
}

// CompanyInfo 企业基础信息（首页固定展示）
type CompanyInfo struct {
	ID       int64  `gorm:"primaryKey;column:id" json:"id"`
	FullName string `gorm:"column:full_name" json:"fullName"`
	Address  string `gorm:"column:address" json:"address"`
	Tel      string `gorm:"column:tel" json:"tel"`
	Logo     string `gorm:"column:logo" json:"logo"`
}

// CompanyProfile 公司简介 + 视频号 + 备用 MP4
type CompanyProfile struct {
	ID             int64  `gorm:"primaryKey;column:id" json:"id"`
	IntroText      string `gorm:"column:intro_text;type:text" json:"introText"`
	VideoChannelID string `gorm:"column:video_channel_id" json:"videoChannelId"`
	BackupMp4URL   string `gorm:"column:backup_mp4_url" json:"backupMp4Url"`
	Cover          string `gorm:"column:cover" json:"cover"`
}

// AchievementCategory 经典业绩分类（种子 7 类）
type AchievementCategory struct {
	ID       int64  `gorm:"primaryKey;column:id" json:"id"`
	Name     string `gorm:"column:name" json:"name"`
	Icon     string `gorm:"column:icon" json:"icon"`
	Sort     int    `gorm:"column:sort" json:"sort"`
	IsLocked bool   `gorm:"column:is_locked;default:false" json:"isLocked"`
}

// ProjectCase 项目案例
type ProjectCase struct {
	ID             int64     `gorm:"primaryKey;column:id" json:"id"`
	CategoryID     int64     `gorm:"column:category_id;index" json:"categoryId"`
	Name           string    `gorm:"column:name" json:"name"`
	Summary        string    `gorm:"column:summary" json:"summary"`
	Detail         string    `gorm:"column:detail;type:text" json:"detail"`
	Cover          string    `gorm:"column:cover" json:"cover"`
	Album          string    `gorm:"column:album;type:json" json:"-"` // 存 JSON 数组字符串
	IsHomeFeatured bool      `gorm:"column:is_home_featured;default:false" json:"isHomeFeatured"`
	Enabled        bool      `gorm:"column:enabled;default:true" json:"enabled"`
	Sort           int       `gorm:"column:sort" json:"sort"`
	CreatedAt      time.Time `gorm:"column:created_at" json:"createdAt"`
}

// BizService 业务服务（富文本含视频号块）
type BizService struct {
	ID        int64     `gorm:"primaryKey;column:id" json:"id"`
	Title     string    `gorm:"column:title" json:"title"`
	Content   string    `gorm:"column:content;type:text" json:"content"`
	Sort      int       `gorm:"column:sort" json:"sort"`
	CreatedAt time.Time `gorm:"column:created_at" json:"createdAt"`
}

// HomeLayout 首页板块顺序（JSON 数组）
type HomeLayout struct {
	ID    int64  `gorm:"primaryKey;column:id" json:"id"`
	Order string `gorm:"column:order_col;type:json" json:"order"`
}

// StatEvent 埋点原始事件
type StatEvent struct {
	ID        int64     `gorm:"primaryKey;column:id;autoIncrement" json:"id"`
	StaffID   int64     `gorm:"column:staff_id;index" json:"staffId"`
	EventType string    `gorm:"column:event_type;index" json:"eventType"`
	StatDate  string    `gorm:"column:stat_date;index" json:"statDate"` // YYYY-MM-DD
	CreatedAt time.Time `gorm:"column:created_at" json:"createdAt"`
}

// Admin 超级管理员（账号 + 密码登录，非微信 openid）
type Admin struct {
	ID           int64     `gorm:"primaryKey;autoIncrement;column:id" json:"id"`
	Username     string    `gorm:"column:username" json:"username"`
	PasswordHash string    `gorm:"column:password_hash" json:"-"`
	CreatedAt    time.Time `gorm:"column:created_at" json:"createdAt"`
}

// OpenApply 员工自助开通申请（审核流）
type OpenApply struct {
	ID           int64     `gorm:"primaryKey;autoIncrement;column:id" json:"id"`
	Name         string    `gorm:"column:name" json:"name"`
	Phone        string    `gorm:"column:phone" json:"phone"`
	Email        string    `gorm:"column:email" json:"email"`
	Title        string    `gorm:"column:title" json:"title"`
	Avatar       string    `gorm:"column:avatar" json:"avatar"`
	Status       string    `gorm:"column:status;default:pending" json:"status"` // pending/approved/rejected
	RejectReason string    `gorm:"column:reject_reason" json:"rejectReason"`
	CreatedAt    time.Time `gorm:"column:created_at" json:"createdAt"`
}

// CardModifyApply 员工名片信息修改申请（审核流，对外仍显示旧信息直到通过）
type CardModifyApply struct {
	ID           int64     `gorm:"primaryKey;autoIncrement;column:id" json:"id"`
	StaffID      int64     `gorm:"column:staff_id;index" json:"staffId"`
	Snapshot     string    `gorm:"column:snapshot;type:json" json:"snapshot"` // JSON：变更字段 {avatar,name,title,phone,email}
	Status       string    `gorm:"column:status;default:pending" json:"status"`
	RejectReason string    `gorm:"column:reject_reason" json:"rejectReason"`
	CreatedAt    time.Time `gorm:"column:created_at" json:"createdAt"`
}

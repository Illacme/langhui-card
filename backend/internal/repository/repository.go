package repository

import (
	"time"

	"langhui/internal/model"

	"gorm.io/gorm"
)

// GetCard 聚合员工 + 企业信息 + 公司简介（首页一次拉齐）
func GetCard(db *gorm.DB, staffID int64) (model.Employee, model.CompanyInfo, model.CompanyProfile, error) {
	var e model.Employee
	var c model.CompanyInfo
	var p model.CompanyProfile
	if err := db.First(&e, "staff_id = ?", staffID).Error; err != nil {
		return e, c, p, err
	}
	db.First(&c, 1) // 企业信息单行
	db.First(&p, 1) // 公司简介单行
	return e, c, p, nil
}

// GetAchievements 分类（按 sort）+ 首页精选项目
func GetAchievements(db *gorm.DB) ([]model.AchievementCategory, []model.ProjectCase, error) {
	var cats []model.AchievementCategory
	var featured []model.ProjectCase
	if err := db.Order("sort asc").Find(&cats).Error; err != nil {
		return nil, nil, err
	}
	if err := db.Where("is_home_featured = ? AND enabled = ?", true, true).Order("sort asc").Find(&featured).Error; err != nil {
		return nil, nil, err
	}
	return cats, featured, nil
}

func ListProjects(db *gorm.DB, categoryID int64) ([]model.ProjectCase, error) {
	var ps []model.ProjectCase
	err := db.Where("category_id = ? AND enabled = ?", categoryID, true).Order("sort asc").Find(&ps).Error
	return ps, err
}

func GetProject(db *gorm.DB, id int64) (model.ProjectCase, error) {
	var p model.ProjectCase
	err := db.First(&p, "id = ? AND enabled = ?", id, true).Error
	return p, err
}

func ListServices(db *gorm.DB) ([]model.BizService, error) {
	var s []model.BizService
	err := db.Order("sort asc").Find(&s).Error
	return s, err
}

func GetService(db *gorm.DB, id int64) (model.BizService, error) {
	var s model.BizService
	err := db.First(&s, "id = ?", id).Error
	return s, err
}

func GetHomeLayout(db *gorm.DB) (model.HomeLayout, error) {
	var l model.HomeLayout
	err := db.First(&l, 1).Error
	return l, err
}

// TrackStat 写入埋点事件（仅记点击，不校验真实结果）
func TrackStat(db *gorm.DB, staffID int64, eventType string) error {
	ev := model.StatEvent{
		StaffID:   staffID,
		EventType: eventType,
		StatDate:  time.Now().Format("2006-01-02"),
	}
	return db.Create(&ev).Error
}

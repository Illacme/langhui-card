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
	if err != nil {
		return nil, err
	}
	fillCategoryNames(db, ps)
	return ps, nil
}

func GetProject(db *gorm.DB, id int64) (model.ProjectCase, error) {
	var p model.ProjectCase
	err := db.First(&p, "id = ? AND enabled = ?", id, true).Error
	if err != nil {
		return p, err
	}
	// 注意：p 是值，直接传切片再回填改的是切片元素——必须把回填后的元素取回，
	// 否则 handler 里 p.CategoryName 仍是空值（Go 值语义，非指针接收）。
	ps := []model.ProjectCase{p}
	fillCategoryNames(db, ps)
	return ps[0], nil
}

// fillCategoryNames 批量回填 ProjectCase.CategoryName（非持久化字段）：
// 一次 IN 查询取全部分类名，避免 N+1；分类被删/查不到时留空，由前端兜底。
func fillCategoryNames(db *gorm.DB, ps []model.ProjectCase) {
	if len(ps) == 0 {
		return
	}
	ids := make([]int64, 0, len(ps))
	seen := map[int64]bool{}
	for _, p := range ps {
		if p.CategoryID != 0 && !seen[p.CategoryID] {
			ids = append(ids, p.CategoryID)
			seen[p.CategoryID] = true
		}
	}
	if len(ids) == 0 {
		return
	}
	var cats []model.AchievementCategory
	if err := db.Where("id IN ?", ids).Find(&cats).Error; err != nil {
		return // 回填失败不阻断主查询：分类名降级为空
	}
	nameOf := make(map[int64]string, len(cats))
	for _, c := range cats {
		nameOf[c.ID] = c.Name
	}
	for i := range ps {
		ps[i].CategoryName = nameOf[ps[i].CategoryID]
	}
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

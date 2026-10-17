package repository

import (
	"time"

	"langhui/internal/model"

	"gorm.io/gorm"
)

// GetCard 聚合员工 + 企业信息 + 公司简介（首页一次拉齐）。
// 企业信息按员工归属租户取（employee.company_id），天然支持多租户。
func GetCard(db *gorm.DB, staffID int64) (model.Employee, model.CompanyInfo, model.CompanyProfile, error) {
	var e model.Employee
	var c model.CompanyInfo
	var p model.CompanyProfile
	if err := db.First(&e, "staff_id = ?", staffID).Error; err != nil {
		return e, c, p, err
	}
	db.First(&c, "company_id = ?", e.CompanyID) // 企业信息按租户
	db.First(&p, "company_id = ?", e.CompanyID) // 公司简介按租户
	return e, c, p, nil
}

// GetAchievements 分类（按 sort）+ 首页精选项目（按租户隔离）
func GetAchievements(db *gorm.DB, companyID int64) ([]model.AchievementCategory, []model.ProjectCase, error) {
	var cats []model.AchievementCategory
	var featured []model.ProjectCase
	if err := db.Where("company_id = ?", companyID).Order("sort asc").Find(&cats).Error; err != nil {
		return nil, nil, err
	}
	if err := db.Where("company_id = ? AND is_home_featured = ? AND enabled = ?", companyID, true, true).Order("sort asc").Find(&featured).Error; err != nil {
		return nil, nil, err
	}
	return cats, featured, nil
}

func ListProjects(db *gorm.DB, companyID int64, categoryID int64) ([]model.ProjectCase, error) {
	var ps []model.ProjectCase
	q := db.Where("company_id = ?", companyID).Order("sort asc")
	if categoryID > 0 {
		q = q.Where("category_id = ? AND enabled = ?", categoryID, true)
	} else {
		// 未传分类：返回全部上架案例（供公开接口 /api/projects 无 categoryId 时列出全部）
		q = q.Where("enabled = ?", true)
	}
	if err := q.Find(&ps).Error; err != nil {
		return nil, err
	}
	fillCategoryNames(db, ps)
	return ps, nil
}

func GetProject(db *gorm.DB, companyID int64, id int64) (model.ProjectCase, error) {
	var p model.ProjectCase
	err := db.First(&p, "company_id = ? AND id = ? AND enabled = ?", companyID, id, true).Error
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

func ListServices(db *gorm.DB, companyID int64) ([]model.BizService, error) {
	var s []model.BizService
	err := db.Where("company_id = ?", companyID).Order("sort asc").Find(&s).Error
	return s, err
}

func GetService(db *gorm.DB, companyID int64, id int64) (model.BizService, error) {
	var s model.BizService
	err := db.First(&s, "company_id = ? AND id = ?", companyID, id).Error
	return s, err
}

func GetHomeLayout(db *gorm.DB, companyID int64) (model.HomeLayout, error) {
	var l model.HomeLayout
	err := db.First(&l, "company_id = ?", companyID).Error
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

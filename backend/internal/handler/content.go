package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"langhui/internal/middleware"
	"langhui/internal/model"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// RegisterContent 注册「企业内容管理」写接口（仅超管），覆盖：
// 公司信息 / 公司简介 / 业绩分类 / 项目案例 / 业务服务 / 首页布局
func RegisterContent(r *gin.Engine, db *gorm.DB) {
	// ===== 公司基础信息（固定展示） =====
	r.GET("/api/admin/company-info", middleware.AuthMiddleware(db, "admin"), func(c *gin.Context) {
		var ci model.CompanyInfo
		if db.First(&ci, "id = ?", 1).Error != nil {
			c.JSON(http.StatusOK, gin.H{"code": 0, "data": model.CompanyInfo{ID: 1}})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": ci})
	})
	r.PUT("/api/admin/company-info", middleware.AuthMiddleware(db, "admin"), func(c *gin.Context) {
		var req struct {
			FullName string `json:"fullName"`
			Address  string `json:"address"`
			Tel      string `json:"tel"`
			Logo     string `json:"logo"`
		}
		if c.ShouldBindJSON(&req) != nil {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "参数错误"})
			return
		}
		var ci model.CompanyInfo
		if db.First(&ci, "id = ?", 1).Error != nil {
			ci = model.CompanyInfo{ID: 1, FullName: req.FullName, Address: req.Address, Tel: req.Tel, Logo: req.Logo}
			db.Create(&ci)
		} else {
			db.Model(&ci).Updates(map[string]interface{}{"full_name": req.FullName, "address": req.Address, "tel": req.Tel, "logo": req.Logo})
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": ci})
	})

	// ===== 公司简介 + 视频号 =====
	r.GET("/api/admin/company-profile", middleware.AuthMiddleware(db, "admin"), func(c *gin.Context) {
		var cp model.CompanyProfile
		if db.First(&cp, "id = ?", 1).Error != nil {
			c.JSON(http.StatusOK, gin.H{"code": 0, "data": model.CompanyProfile{ID: 1}})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": cp})
	})
	r.PUT("/api/admin/company-profile", middleware.AuthMiddleware(db, "admin"), func(c *gin.Context) {
		var req struct {
			IntroText      string `json:"introText"`
			VideoChannelID string `json:"videoChannelId"`
			BackupMp4URL   string `json:"backupMp4Url"`
			Cover          string `json:"cover"`
		}
		if c.ShouldBindJSON(&req) != nil {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "参数错误"})
			return
		}
		var cp model.CompanyProfile
		if db.First(&cp, "id = ?", 1).Error != nil {
			cp = model.CompanyProfile{ID: 1, IntroText: req.IntroText, VideoChannelID: req.VideoChannelID, BackupMp4URL: req.BackupMp4URL, Cover: req.Cover}
			db.Create(&cp)
		} else {
			db.Model(&cp).Updates(map[string]interface{}{"intro_text": req.IntroText, "video_channel_id": req.VideoChannelID, "backup_mp4_url": req.BackupMp4URL, "cover": req.Cover})
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": cp})
	})

	// ===== 业绩分类 =====
	r.GET("/api/admin/categories", middleware.AuthMiddleware(db, "admin"), func(c *gin.Context) {
		var list []model.AchievementCategory
		db.Order("sort asc").Find(&list)
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": list})
	})
	r.POST("/api/admin/categories", middleware.AuthMiddleware(db, "admin"), func(c *gin.Context) {
		var req model.AchievementCategory
		if c.ShouldBindJSON(&req) != nil || req.Name == "" {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "参数错误"})
			return
		}
		db.Create(&req)
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": req})
	})
	r.PUT("/api/admin/categories/:id", middleware.AuthMiddleware(db, "admin"), func(c *gin.Context) {
		id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
		var req struct {
			Name     string `json:"name"`
			Icon     string `json:"icon"`
			Sort     int    `json:"sort"`
			IsLocked bool   `json:"isLocked"`
		}
		if c.ShouldBindJSON(&req) != nil {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "参数错误"})
			return
		}
		db.Model(&model.AchievementCategory{}).Where("id = ?", id).
			Updates(map[string]interface{}{"name": req.Name, "icon": req.Icon, "sort": req.Sort, "is_locked": req.IsLocked})
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "ok"})
	})
	r.DELETE("/api/admin/categories/:id", middleware.AuthMiddleware(db, "admin"), func(c *gin.Context) {
		id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
		var cat model.AchievementCategory
		if db.First(&cat, "id = ?", id).Error != nil {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "未找到"})
			return
		}
		// 强校验：分类下存在项目则禁止删除（避免产生孤立数据）
		var pcnt int64
		db.Model(&model.ProjectCase{}).Where("category_id = ?", id).Count(&pcnt)
		if pcnt > 0 {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "该分类下存在项目，无法删除"})
			return
		}
		if cat.IsLocked {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "锁定分类不可删除"})
			return
		}
		db.Delete(&cat)
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "ok"})
	})

	// ===== 项目案例 =====
	r.GET("/api/admin/projects", middleware.AuthMiddleware(db, "admin"), func(c *gin.Context) {
		cid, _ := strconv.ParseInt(c.Query("categoryId"), 10, 64)
		q := db.Order("sort asc")
		if cid > 0 {
			q = q.Where("category_id = ?", cid)
		}
		var list []model.ProjectCase
		q.Find(&list)
		data := make([]gin.H, 0, len(list))
		for _, p := range list {
			data = append(data, projectToMap(p))
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": data})
	})
	r.POST("/api/admin/projects", middleware.AuthMiddleware(db, "admin"), func(c *gin.Context) {
		in, ok := bindProject(c)
		if !ok {
			return
		}
		alb, _ := json.Marshal(in.Album)
		p := model.ProjectCase{
			CategoryID: in.CategoryID, Name: in.Name, Summary: in.Summary, Detail: in.Detail,
			Cover: in.Cover, Album: string(alb), IsHomeFeatured: in.IsHomeFeatured,
			Enabled: in.Enabled, Sort: in.Sort,
		}
		db.Create(&p)
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": projectToMap(p)})
	})
	// 批量排序：接收 [{id, sort}]，在事务内一次性更新，
	// 避免前端拖拽/上下箭头调整顺序时出现中间态不一致。
	// 注意：该静态路由必须注册在 PUT /:id 之前，否则会被 :id 参数路由吞掉。
	r.PUT("/api/admin/projects/sort", middleware.AuthMiddleware(db, "admin"), func(c *gin.Context) {
		var items []struct {
			ID   int64 `json:"id"`
			Sort int   `json:"sort"`
		}
		if c.ShouldBindJSON(&items) != nil || len(items) == 0 {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "参数错误"})
			return
		}
		tx := db.Begin()
		if tx.Error != nil {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "数据库错误"})
			return
		}
		for _, it := range items {
			if it.ID <= 0 {
				continue
			}
			if err := tx.Model(&model.ProjectCase{}).Where("id = ?", it.ID).
				Update("sort", it.Sort).Error; err != nil {
				tx.Rollback()
				c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "更新失败"})
				return
			}
		}
		if err := tx.Commit().Error; err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "提交失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "ok"})
	})
	r.PUT("/api/admin/projects/:id", middleware.AuthMiddleware(db, "admin"), func(c *gin.Context) {
		id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
		in, ok := bindProject(c)
		if !ok {
			return
		}
		var p model.ProjectCase
		if db.First(&p, "id = ?", id).Error != nil {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "未找到"})
			return
		}
		alb, _ := json.Marshal(in.Album)
		db.Model(&p).Updates(map[string]interface{}{
			"category_id": in.CategoryID, "name": in.Name, "summary": in.Summary,
			"detail": in.Detail, "cover": in.Cover, "album": string(alb),
			"is_home_featured": in.IsHomeFeatured, "enabled": in.Enabled, "sort": in.Sort,
		})
		db.First(&p, "id = ?", id)
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": projectToMap(p)})
	})
	r.DELETE("/api/admin/projects/:id", middleware.AuthMiddleware(db, "admin"), func(c *gin.Context) {
		id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
		if db.Delete(&model.ProjectCase{}, "id = ?", id).RowsAffected == 0 {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "未找到"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "ok"})
	})

	// ===== 业务服务 =====
	r.GET("/api/admin/services", middleware.AuthMiddleware(db, "admin"), func(c *gin.Context) {
		var list []model.BizService
		db.Order("sort asc").Find(&list)
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": list})
	})
	r.POST("/api/admin/services", middleware.AuthMiddleware(db, "admin"), func(c *gin.Context) {
		var req struct {
			Title   string `json:"title"`
			Content string `json:"content"`
			Sort    int    `json:"sort"`
		}
		if c.ShouldBindJSON(&req) != nil || req.Title == "" {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "参数错误"})
			return
		}
		s := model.BizService{Title: req.Title, Content: req.Content, Sort: req.Sort}
		db.Create(&s)
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": s})
	})
	r.PUT("/api/admin/services/:id", middleware.AuthMiddleware(db, "admin"), func(c *gin.Context) {
		id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
		var req struct {
			Title   string `json:"title"`
			Content string `json:"content"`
			Sort    int    `json:"sort"`
		}
		if c.ShouldBindJSON(&req) != nil {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "参数错误"})
			return
		}
		db.Model(&model.BizService{}).Where("id = ?", id).
			Updates(map[string]interface{}{"title": req.Title, "content": req.Content, "sort": req.Sort})
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "ok"})
	})
	r.DELETE("/api/admin/services/:id", middleware.AuthMiddleware(db, "admin"), func(c *gin.Context) {
		id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
		if db.Delete(&model.BizService{}, "id = ?", id).RowsAffected == 0 {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "未找到"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "ok"})
	})

	// ===== 首页布局（板块顺序） =====
	r.GET("/api/admin/home-layout", middleware.AuthMiddleware(db, "admin"), func(c *gin.Context) {
		var hl model.HomeLayout
		order := []string{}
		if db.First(&hl, "id = ?", 1).Error == nil && hl.Order != "" {
			_ = json.Unmarshal([]byte(hl.Order), &order)
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"order": order}})
	})
	r.PUT("/api/admin/home-layout", middleware.AuthMiddleware(db, "admin"), func(c *gin.Context) {
		var req struct {
			Order []string `json:"order"`
		}
		if c.ShouldBindJSON(&req) != nil {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "参数错误"})
			return
		}
		b, _ := json.Marshal(req.Order)
		var hl model.HomeLayout
		if db.First(&hl, "id = ?", 1).Error != nil {
			db.Create(&model.HomeLayout{ID: 1, Order: string(b)})
		} else {
			db.Model(&hl).Update("order_col", string(b))
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "ok"})
	})
}

// bindProject 解析项目案例的写请求体（album 为字符串数组，映射至 JSON 列）
func bindProject(c *gin.Context) (projectInput, bool) {
	var in projectInput
	if c.ShouldBindJSON(&in) != nil || in.Name == "" {
		c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "参数错误"})
		return in, false
	}
	return in, true
}

type projectInput struct {
	CategoryID     int64    `json:"categoryId"`
	Name           string   `json:"name"`
	Summary        string   `json:"summary"`
	Detail         string   `json:"detail"`
	Cover          string   `json:"cover"`
	Album          []string `json:"album"`
	IsHomeFeatured bool     `json:"isHomeFeatured"`
	Enabled        bool     `json:"enabled"`
	Sort           int      `json:"sort"`
}

// projectToMap 把模型转为含 album 数组的响应结构（模型 Album 为 json:"-"）
func projectToMap(p model.ProjectCase) gin.H {
	var album []string
	if p.Album != "" {
		_ = json.Unmarshal([]byte(p.Album), &album)
	}
	return gin.H{
		"id":             p.ID,
		"categoryId":     p.CategoryID,
		"name":           p.Name,
		"summary":        p.Summary,
		"detail":         p.Detail,
		"cover":          p.Cover,
		"album":          album,
		"isHomeFeatured": p.IsHomeFeatured,
		"enabled":        p.Enabled,
		"sort":           p.Sort,
		"createdAt":      p.CreatedAt,
	}
}

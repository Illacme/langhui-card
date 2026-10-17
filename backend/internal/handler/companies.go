package handler

import (
	"net/http"
	"strconv"
	"time"

	"langhui/internal/auth"
	"langhui/internal/middleware"
	"langhui/internal/model"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// RegisterCompanies 注册租户（企业）管理接口——仅平台超级管理员可用
// （middleware.PlatformAdminMiddleware：登录 + role=admin + company_id=0）。
// 覆盖：企业列表 / 开通企业（可一并建企业管理员）/ 停用启用 / 企业管理员管理。
// 企业管理员（company_id>0）调用这些接口一律 403，越权改不了别家租户。
func RegisterCompanies(r *gin.Engine, db *gorm.DB) {
	plat := middleware.PlatformAdminMiddleware(db)

	// 当前登录身份（前端 hydration 用：令牌在 httpOnly Cookie 中，JS 读不到 claims）
	r.GET("/api/admin/me", middleware.AuthMiddleware(db, "admin"), func(c *gin.Context) {
		adminID := c.GetInt64("staffId")
		var a model.Admin
		if db.First(&a, "id = ?", adminID).Error != nil {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "未找到"})
			return
		}
		companyName := ""
		companyShortName := ""
		if a.CompanyID > 0 {
			var comp model.Company
			if db.First(&comp, "id = ?", a.CompanyID).Error == nil {
				companyName = comp.Name
				companyShortName = comp.ShortName
			}
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{
			"adminId":          a.ID,
			"username":         a.Username,
			"companyId":        a.CompanyID,
			"companyName":      companyName,
			"companyShortName": companyShortName,
			"isPlatform":       a.CompanyID == 0,
		}})
	})

	// 企业列表（含员工数，供平台方总览）
	r.GET("/api/admin/companies", plat, func(c *gin.Context) {
		var comps []model.Company
		db.Order("id asc").Find(&comps)
		data := make([]gin.H, 0, len(comps))
		for _, comp := range comps {
			var empCnt, adminCnt int64
			db.Model(&model.Employee{}).Where("company_id = ?", comp.ID).Count(&empCnt)
			db.Model(&model.Admin{}).Where("company_id = ?", comp.ID).Count(&adminCnt)
			data = append(data, gin.H{
				"id": comp.ID, "name": comp.Name, "shortName": comp.ShortName, "status": comp.Status,
				"createdAt": comp.CreatedAt, "employeeCount": empCnt, "adminCount": adminCnt,
			})
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": data})
	})

	// 开通企业：建租户行 + 种子企业内容三件套（公司信息/简介/首页布局）；
	// 可选一并创建企业管理员（adminUsername + adminPassword）。
	// shortName（企业简称）：① 作为公司信息简称的初始值，企业管理员登录后可在
	// 「公司信息」页继续完善（此后仅影响小程序展示）；② 作为 PC 后台侧边栏品牌名。
	r.POST("/api/admin/companies", plat, func(c *gin.Context) {
		var req struct {
			Name          string `json:"name"`
			ShortName     string `json:"shortName"`
			AdminUsername string `json:"adminUsername"`
			AdminPassword string `json:"adminPassword"`
		}
		if c.ShouldBindJSON(&req) != nil || req.Name == "" {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "参数错误"})
			return
		}
		// 企业管理员二选一：要么都填（建账号），要么都不填（后续再建）
		if (req.AdminUsername == "") != (req.AdminPassword == "") {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "管理员账号与密码需同时填写"})
			return
		}
		if req.AdminUsername != "" {
			var cnt int64
			db.Model(&model.Admin{}).Where("username = ?", req.AdminUsername).Count(&cnt)
			if cnt > 0 {
				c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "管理员用户名已存在"})
				return
			}
		}
		comp := model.Company{Name: req.Name, ShortName: req.ShortName, Status: "enabled", CreatedAt: time.Now()}
		if err := db.Create(&comp).Error; err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "创建失败"})
			return
		}
		// 种子内容三件套：新企业开箱即有空白公司信息/简介/默认布局；
		// 简称取开通时填写的企业简称作为初始值（企业管理员后续可在公司信息页修改）
		db.Create(&model.CompanyInfo{CompanyID: comp.ID, FullName: req.Name, ShortName: req.ShortName})
		db.Create(&model.CompanyProfile{CompanyID: comp.ID})
		db.Create(&model.HomeLayout{CompanyID: comp.ID, Order: `["achievement","service"]`})
		if req.AdminUsername != "" {
			hash, _ := auth.HashPassword(req.AdminPassword)
			if err := db.Create(&model.Admin{CompanyID: comp.ID, Username: req.AdminUsername, PasswordHash: hash}).Error; err != nil {
				c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "企业已创建，但管理员创建失败，请重试或手动添加"})
				return
			}
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"id": comp.ID, "name": comp.Name, "shortName": comp.ShortName}})
	})

	// 编辑企业（名称 / 简称）：只改 company 表本身。
	// 简称改后同步侧边栏品牌名，但不回写公司信息的简称——
	// 公司信息里的简称是企业管理员在「公司信息」页维护的小程序展示值，两者相互独立。
	r.PUT("/api/admin/companies/:id", plat, func(c *gin.Context) {
		id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
		var req struct {
			Name      string `json:"name"`
			ShortName string `json:"shortName"`
		}
		if c.ShouldBindJSON(&req) != nil || req.Name == "" {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "参数错误"})
			return
		}
		if db.Model(&model.Company{}).Where("id = ?", id).
			Updates(map[string]interface{}{"name": req.Name, "short_name": req.ShortName}).RowsAffected == 0 {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "未找到"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "ok"})
	})

	// 停用 / 启用企业（停用后该企业的管理员与员工均不能登录）
	r.POST("/api/admin/companies/:id/status", plat, func(c *gin.Context) {
		id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
		var req struct {
			Status string `json:"status"`
		}
		if c.ShouldBindJSON(&req) != nil || (req.Status != "enabled" && req.Status != "disabled") {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "参数错误"})
			return
		}
		if db.Model(&model.Company{}).Where("id = ?", id).
			Update("status", req.Status).RowsAffected == 0 {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "未找到"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "ok"})
	})

	// 企业管理员列表
	r.GET("/api/admin/companies/:id/admins", plat, func(c *gin.Context) {
		id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
		var list []model.Admin
		db.Where("company_id = ?", id).Order("id asc").Find(&list)
		data := make([]gin.H, 0, len(list))
		for _, a := range list {
			data = append(data, gin.H{"id": a.ID, "username": a.Username, "createdAt": a.CreatedAt})
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": data})
	})

	// 新增企业管理员
	r.POST("/api/admin/companies/:id/admins", plat, func(c *gin.Context) {
		id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
		var req struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if c.ShouldBindJSON(&req) != nil || req.Username == "" || req.Password == "" {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "参数错误"})
			return
		}
		var comp model.Company
		if db.First(&comp, "id = ?", id).Error != nil {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "企业不存在"})
			return
		}
		var cnt int64
		db.Model(&model.Admin{}).Where("username = ?", req.Username).Count(&cnt)
		if cnt > 0 {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "用户名已存在"})
			return
		}
		hash, _ := auth.HashPassword(req.Password)
		a := model.Admin{CompanyID: id, Username: req.Username, PasswordHash: hash}
		if err := db.Create(&a).Error; err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "创建失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"id": a.ID, "username": a.Username}})
	})
}

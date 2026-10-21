package handler

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"weichuan/internal/audit"
	"weichuan/internal/auth"
	"weichuan/internal/middleware"
	"weichuan/internal/model"
	"weichuan/internal/setting"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// defaultShortName 企业简称缺省值：企业管理里未设置简称（留空/纯空白）时，
// 侧边栏品牌名回落为「微传名片」。
const defaultShortName = "微传名片"

// resolveShortName 规整企业简称：去首尾空白；为空则回落缺省值「微传名片」。
func resolveShortName(raw string) string {
	if s := strings.TrimSpace(raw); s != "" {
		return s
	}
	return defaultShortName
}

// independentLocked 是否开启「独立版企业锁定」（后台「系统设置 → 功能开关 → independent_lock」）。
// 开启后企业管理整体只读：列表 / 管理员列表照常可查，
// 但新建企业、编辑企业、启停企业、新建企业管理员一律拒绝。
func independentLocked() bool {
	return setting.GetBool("independent_lock", false)
}

// writeLocked 企业管理「写操作」的统一门禁：锁定时返回 403 并给出明确原因。
// 放在各写接口开头而非注册期判定——锁定是运行期开关，注册期判定会导致改完还得重启。
func writeLocked(c *gin.Context) bool {
	if !independentLocked() {
		return false
	}
	c.JSON(http.StatusOK, gin.H{
		"code": 403,
		"msg":  "已开启独立版企业锁定（系统设置 independent_lock），企业管理为只读",
	})
	return true
}

// countByCompany 批量统计「某企业下的记录数」：一次 group by 拿到所有目标企业的计数，
// 替代原先逐企业 COUNT 的 N+1 写法。ids 为空时返回空 map（不再查库）。
func countByCompany(db *gorm.DB, tableModel interface{}, ids []int64) map[int64]int64 {
	out := make(map[int64]int64, len(ids))
	if len(ids) == 0 {
		return out
	}
	var rows []struct {
		CompanyID int64 `gorm:"column:company_id"`
		Cnt       int64 `gorm:"column:cnt"`
	}
	db.Model(tableModel).
		Select("company_id, count(*) as cnt").
		Where("company_id IN ?", ids).
		Group("company_id").
		Scan(&rows)
	for _, r := range rows {
		out[r.CompanyID] = r.Cnt
	}
	return out
}

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
			// 独立版企业锁定：前端据此把「企业管理」页切为只读。
			// 后端各写接口也有 writeLocked 拦截——双保险，避免只靠前端隐藏按钮被绕过。
			"independentLock": independentLocked(),
		}})
	})

	// 企业列表（含员工数/管理员数，供平台方总览）
	//
	// 分页 + 关键字（名称/简称）+ 状态筛选，参数见 handler/pagination.go 的文件头约定。
	// 数量统计改为「按当前页 id 批量 group by」：原写法是每行 2 次 COUNT（N+1），
	// 租户一多就是 2N 次查询；现在固定 2 次，与页数无关。
	r.GET("/api/admin/companies", plat, func(c *gin.Context) {
		page, size := pageParams(c)

		q := db.Model(&model.Company{})
		if cond, args := keywordCond(c.Query("keyword"), "name", "short_name"); cond != "" {
			q = q.Where(cond, args...)
		}
		if cond, v := statusCond(c.Query("status"), "enabled", "disabled"); cond != "" {
			q = q.Where(cond, v)
		}

		var comps []model.Company
		total, page := pagedQuery(q, "id asc", &comps, page, size)

		ids := make([]int64, 0, len(comps))
		for _, comp := range comps {
			ids = append(ids, comp.ID)
		}
		empCnt := countByCompany(db, &model.Employee{}, ids)
		adminCnt := countByCompany(db, &model.Admin{}, ids)

		data := make([]gin.H, 0, len(comps))
		for _, comp := range comps {
			data = append(data, gin.H{
				"id": comp.ID, "name": comp.Name, "shortName": comp.ShortName, "status": comp.Status,
				"createdAt": comp.CreatedAt,
				"employeeCount": empCnt[comp.ID], "adminCount": adminCnt[comp.ID],
			})
		}
		pagedOK(c, data, total, page, size)
	})

	// 开通企业：建租户行 + 种子企业内容三件套（公司信息/简介/首页布局）；
	// 可选一并创建企业管理员（adminUsername + adminPassword）。
	// shortName（企业简称）：① 未设置时默认「微传名片」，作为 PC 后台侧边栏品牌名；
	// ② 显式填写的值同时作为公司信息简称的初始值，企业管理员登录后可在
	// 「公司信息」页继续完善（此后仅影响小程序展示）。
	r.POST("/api/admin/companies", plat, func(c *gin.Context) {
		if writeLocked(c) {
			return
		}
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
		comp := model.Company{Name: req.Name, ShortName: resolveShortName(req.ShortName), Status: "enabled", CreatedAt: time.Now()}
		if err := db.Create(&comp).Error; err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "创建失败"})
			return
		}
		// 种子内容三件套：新企业开箱即有空白公司信息/简介/默认布局；
		// 简称种子取开通时显式填写的值，未填则留空——小程序顶栏回落展示企业全称，
		// 不跟随 company.short_name 的缺省值「微传名片」（两者相互独立）
		db.Create(&model.CompanyInfo{CompanyID: comp.ID, FullName: req.Name, ShortName: strings.TrimSpace(req.ShortName)})
		db.Create(&model.CompanyProfile{CompanyID: comp.ID})
		db.Create(&model.HomeLayout{CompanyID: comp.ID, Order: `["achievement","service"]`})
		if req.AdminUsername != "" {
			hash, _ := auth.HashPassword(req.AdminPassword)
			if err := db.Create(&model.Admin{CompanyID: comp.ID, Username: req.AdminUsername, PasswordHash: hash}).Error; err != nil {
				c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "企业已创建，但管理员创建失败，请重试或手动添加"})
				return
			}
		}
		audit.Record(db, c, audit.Entry{
			Action: audit.ActionCompanyCreate, TargetType: "company", TargetID: strconv.FormatInt(comp.ID, 10),
			Detail: map[string]any{"name": comp.Name, "admin": req.AdminUsername},
		})
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"id": comp.ID, "name": comp.Name, "shortName": comp.ShortName}})
	})

	// 编辑企业（名称 / 简称）：只改 company 表本身。
	// 简称改后同步侧边栏品牌名（未设置/清空则回落缺省值「微传名片」），但不回写公司信息的简称——
	// 公司信息里的简称是企业管理员在「公司信息」页维护的小程序展示值，两者相互独立。
	r.PUT("/api/admin/companies/:id", plat, func(c *gin.Context) {
		if writeLocked(c) {
			return
		}
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
			Updates(map[string]interface{}{"name": req.Name, "short_name": resolveShortName(req.ShortName)}).RowsAffected == 0 {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "未找到"})
			return
		}
		audit.Record(db, c, audit.Entry{
			Action: audit.ActionCompanyUpdate, TargetType: "company", TargetID: strconv.FormatInt(id, 10),
			Detail: map[string]any{"name": req.Name, "shortName": req.ShortName},
		})
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "ok"})
	})

	// 停用 / 启用企业（停用后该企业的管理员与员工均不能登录）
	r.POST("/api/admin/companies/:id/status", plat, func(c *gin.Context) {
		if writeLocked(c) {
			return
		}
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
		audit.Record(db, c, audit.Entry{
			Action: audit.ActionCompanyStatus, TargetType: "company", TargetID: strconv.FormatInt(id, 10),
			Detail: map[string]any{"status": req.Status},
		})
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
		if writeLocked(c) {
			return
		}
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
		audit.Record(db, c, audit.Entry{
			Action: audit.ActionCompanyAdminAdd, TargetType: "admin", TargetID: strconv.FormatInt(a.ID, 10),
			Detail: map[string]any{"username": a.Username, "companyId": id},
		})
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"id": a.ID, "username": a.Username}})
	})
}

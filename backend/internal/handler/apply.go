package handler

import (
	"crypto/rand"
	"encoding/json"
	"math/big"
	"net/http"
	"strconv"

	"langhui/internal/auth"
	"langhui/internal/middleware"
	"langhui/internal/model"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// RegisterApply 注册写操作：开通申请 / 名片修改审核 / 员工管理 / 统计
func RegisterApply(r *gin.Engine, db *gorm.DB) {
	// 员工自助提交开通申请（公开）
	r.POST("/api/apply/open", func(c *gin.Context) {
		var req model.OpenApply
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "参数错误"})
			return
		}
		req.Status = "pending"
		if err := db.Create(&req).Error; err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "提交失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "已提交，等待审核"})
	})

	// 员工本人名片信息（需登录）
	r.GET("/api/employee/me", middleware.AuthMiddleware(db, "employee"), func(c *gin.Context) {
		staffID := c.GetInt64("staffId")
		var e model.Employee
		if err := db.First(&e, "staff_id = ?", staffID).Error; err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "未找到"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": e})
	})

	// 员工提交名片修改（待审，对外仍显示旧信息）
	r.PUT("/api/employee/me", middleware.AuthMiddleware(db, "employee"), func(c *gin.Context) {
		staffID := c.GetInt64("staffId")
		var patch struct {
			Avatar string `json:"avatar"`
			Name   string `json:"name"`
			Title  string `json:"title"`
			Phone  string `json:"phone"`
			Email  string `json:"email"`
		}
		if err := c.ShouldBindJSON(&patch); err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "参数错误"})
			return
		}
		snap, _ := json.Marshal(patch)
		apply := model.CardModifyApply{StaffID: staffID, Snapshot: string(snap), Status: "pending"}
		if err := db.Create(&apply).Error; err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "提交失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "已提交，等待审核"})
	})

	// 员工本人统计
	r.GET("/api/employee/me/stat", middleware.AuthMiddleware(db, "employee"), func(c *gin.Context) {
		staffID := c.GetInt64("staffId")
		var visit, call, save int64
		db.Model(&model.StatEvent{}).Where("staff_id = ? AND event_type = ?", staffID, "visit").Count(&visit)
		db.Model(&model.StatEvent{}).Where("staff_id = ? AND event_type = ?", staffID, "call").Count(&call)
		db.Model(&model.StatEvent{}).Where("staff_id = ? AND event_type = ?", staffID, "save").Count(&save)
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"visit": visit, "call": call, "save": save}})
	})

	// 员工修改本人密码
	r.PUT("/api/employee/password", middleware.AuthMiddleware(db, "employee"), func(c *gin.Context) {
		staffID := c.GetInt64("staffId")
		var req struct {
			Old string `json:"old"`
			New string `json:"new"`
		}
		if c.ShouldBindJSON(&req) != nil || req.New == "" {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "参数错误"})
			return
		}
		var e model.Employee
		db.First(&e, "staff_id = ?", staffID)
		if !auth.CheckPassword(e.PasswordHash, req.Old) {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "原密码错误"})
			return
		}
		h, _ := auth.HashPassword(req.New)
		// 改密同时自增安全戳 → 旧令牌立即失效，需重新登录
		db.Model(&e).Updates(map[string]interface{}{"password_hash": h, "token_version": e.TokenVersion + 1})
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "ok"})
	})

	// ===== 超管端 =====

	// 开通申请列表
	r.GET("/api/admin/apply/open", middleware.AuthMiddleware(db, "admin"), func(c *gin.Context) {
		var list []model.OpenApply
		db.Order("created_at desc").Find(&list)
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": list})
	})

	// 开通申请通过 → 自动建员工账号
	r.POST("/api/admin/apply/open/:id/approve", middleware.AuthMiddleware(db, "admin"), func(c *gin.Context) {
		id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
		var a model.OpenApply
		if db.First(&a, "id = ?", id).Error != nil {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "未找到"})
			return
		}
		if a.Status != "pending" {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "已处理"})
			return
		}
		pwd := genPassword()
		hash, _ := auth.HashPassword(pwd)
		emp := model.Employee{Name: a.Name, Phone: a.Phone, Email: a.Email, Title: a.Title, Avatar: a.Avatar, PasswordHash: hash, Status: "enabled"}
		db.Create(&emp)
		a.Status = "approved"
		db.Save(&a)
		c.JSON(http.StatusOK, gin.H{"code": 0, "staffId": emp.StaffID, "initialPassword": pwd})
	})

	// 开通申请驳回
	r.POST("/api/admin/apply/open/:id/reject", middleware.AuthMiddleware(db, "admin"), func(c *gin.Context) {
		id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
		var body struct {
			Reason string `json:"reason"`
		}
		c.ShouldBindJSON(&body)
		if db.Model(&model.OpenApply{}).Where("id = ? AND status = ?", id, "pending").
			Updates(map[string]interface{}{"status": "rejected", "reject_reason": body.Reason}).RowsAffected == 0 {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "未找到或已处理"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "ok"})
	})

	// 名片修改申请列表
	r.GET("/api/admin/apply/modify", middleware.AuthMiddleware(db, "admin"), func(c *gin.Context) {
		var list []model.CardModifyApply
		db.Order("created_at desc").Find(&list)
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": list})
	})

	// 名片修改通过 → 应用快照到员工
	r.POST("/api/admin/apply/modify/:id/approve", middleware.AuthMiddleware(db, "admin"), func(c *gin.Context) {
		id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
		var a model.CardModifyApply
		if db.First(&a, "id = ?", id).Error != nil {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "未找到"})
			return
		}
		if a.Status != "pending" {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "已处理"})
			return
		}
		var patch map[string]string
		_ = json.Unmarshal([]byte(a.Snapshot), &patch)
		var e model.Employee
		db.First(&e, "staff_id = ?", a.StaffID)
		updates := map[string]interface{}{}
		if v, ok := patch["avatar"]; ok {
			updates["avatar"] = v
		}
		if v, ok := patch["name"]; ok {
			updates["name"] = v
		}
		if v, ok := patch["title"]; ok {
			updates["title"] = v
		}
		if v, ok := patch["phone"]; ok {
			updates["phone"] = v
		}
		if v, ok := patch["email"]; ok {
			updates["email"] = v
		}
		db.Model(&e).Updates(updates)
		a.Status = "approved"
		db.Save(&a)
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "ok"})
	})

	// 名片修改驳回
	r.POST("/api/admin/apply/modify/:id/reject", middleware.AuthMiddleware(db, "admin"), func(c *gin.Context) {
		id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
		var body struct {
			Reason string `json:"reason"`
		}
		c.ShouldBindJSON(&body)
		if db.Model(&model.CardModifyApply{}).Where("id = ? AND status = ?", id, "pending").
			Updates(map[string]interface{}{"status": "rejected", "reject_reason": body.Reason}).RowsAffected == 0 {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "未找到或已处理"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "ok"})
	})

	// 超管手动新增员工（自动生成账号 + 初始密码）
	r.POST("/api/admin/employees", middleware.AuthMiddleware(db, "admin"), func(c *gin.Context) {
		var req model.Employee
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "参数错误"})
			return
		}
		pwd := genPassword()
		hash, _ := auth.HashPassword(pwd)
		emp := model.Employee{Name: req.Name, Phone: req.Phone, Email: req.Email, Title: req.Title, Avatar: req.Avatar, PasswordHash: hash, Status: "enabled"}
		db.Create(&emp)
		c.JSON(http.StatusOK, gin.H{"code": 0, "staffId": emp.StaffID, "initialPassword": pwd})
	})

	// 员工列表（超管）
	r.GET("/api/admin/employees", middleware.AuthMiddleware(db, "admin"), func(c *gin.Context) {
		var list []model.Employee
		db.Order("staff_id asc").Find(&list)
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": list})
	})

	// 启用 / 禁用（禁用同时自增安全戳 → 该员工在线会话立即失效）
	r.POST("/api/admin/employees/:id/disable", middleware.AuthMiddleware(db, "admin"), func(c *gin.Context) {
		id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
		db.Model(&model.Employee{}).Where("staff_id = ?", id).
			Updates(map[string]interface{}{"status": "disabled", "token_version": gorm.Expr("token_version + 1")})
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "ok"})
	})
	r.POST("/api/admin/employees/:id/enable", middleware.AuthMiddleware(db, "admin"), func(c *gin.Context) {
		id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
		db.Model(&model.Employee{}).Where("staff_id = ?", id).
			Updates(map[string]interface{}{"status": "enabled", "token_version": gorm.Expr("token_version + 1")})
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "ok"})
	})

	// 重置密码（返回新初始密码）
	r.POST("/api/admin/employees/:id/reset-password", middleware.AuthMiddleware(db, "admin"), func(c *gin.Context) {
		id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
		pwd := genPassword()
		hash, _ := auth.HashPassword(pwd)
		// 重置密码同时自增安全戳 → 该员工所有旧会话立即失效
		if db.Model(&model.Employee{}).Where("staff_id = ?", id).
			Updates(map[string]interface{}{"password_hash": hash, "token_version": gorm.Expr("token_version + 1")}).RowsAffected == 0 {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "未找到"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "initialPassword": pwd})
	})

	// 超管修改自己密码
	r.PUT("/api/admin/password", middleware.AuthMiddleware(db, "admin"), func(c *gin.Context) {
		adminID := c.GetInt64("staffId")
		var req struct {
			Old string `json:"old"`
			New string `json:"new"`
		}
		if c.ShouldBindJSON(&req) != nil || req.New == "" {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "参数错误"})
			return
		}
		var a model.Admin
		db.First(&a, "id = ?", adminID)
		if !auth.CheckPassword(a.PasswordHash, req.Old) {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "原密码错误"})
			return
		}
		h, _ := auth.HashPassword(req.New)
		// 改密同时自增安全戳 → 旧令牌立即失效，需重新登录
		db.Model(&a).Updates(map[string]interface{}{"password_hash": h, "token_version": a.TokenVersion + 1})
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "ok"})
	})
}

// genPassword 生成 8 位随机密码（字母+数字）
func genPassword() string {
	const chars = "abcdefghijkmnpqrstuvwxyz23456789"
	b := make([]byte, 8)
	for i := range b {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(chars))))
		b[i] = chars[n.Int64()]
	}
	return string(b)
}

package handler

import (
	"net/http"

	"langhui/internal/auth"
	"langhui/internal/model"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// RegisterAuth 注册登录接口（员工 + 超管，账号密码，JWT 签发）
func RegisterAuth(r *gin.Engine, db *gorm.DB) {
	// 员工登录（按手机号）
	r.POST("/api/employee/login", func(c *gin.Context) {
		var req struct {
			Phone    string `json:"phone"`
			Password string `json:"password"`
		}
		if err := c.ShouldBindJSON(&req); err != nil || req.Phone == "" || req.Password == "" {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "参数错误"})
			return
		}
		var e model.Employee
		if err := db.First(&e, "phone = ?", req.Phone).Error; err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "账号不存在"})
			return
		}
		if e.Status == "disabled" {
			c.JSON(http.StatusOK, gin.H{"code": 2, "msg": "账号已停用"})
			return
		}
		if !auth.CheckPassword(e.PasswordHash, req.Password) {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "密码错误"})
			return
		}
		token, _ := auth.GenerateToken(e.StaffID, "employee")
		c.JSON(http.StatusOK, gin.H{"code": 0, "token": token})
	})

	// 超管登录（按用户名）
	r.POST("/api/admin/login", func(c *gin.Context) {
		var req struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if err := c.ShouldBindJSON(&req); err != nil || req.Username == "" || req.Password == "" {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "参数错误"})
			return
		}
		var a model.Admin
		if err := db.First(&a, "username = ?", req.Username).Error; err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "账号不存在"})
			return
		}
		if !auth.CheckPassword(a.PasswordHash, req.Password) {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "密码错误"})
			return
		}
		token, _ := auth.GenerateToken(a.ID, "admin")
		c.JSON(http.StatusOK, gin.H{"code": 0, "token": token})
	})
}

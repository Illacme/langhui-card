package handler

import (
	"net/http"
	"time"

	"langhui/internal/auth"
	"langhui/internal/middleware"
	"langhui/internal/model"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// RegisterAuth 注册登录接口（员工 + 超管，账号密码，JWT 签发 access + refresh）。
// 两个登录接口均叠加登录限流中间件（按客户端 IP 固定窗口计数，防爆破）。
// 另提供 /api/refresh：用 refresh 令牌换取新的 access 令牌（避免频繁重登）。
func RegisterAuth(r *gin.Engine, db *gorm.DB) {
	loginLimiter := middleware.NewLoginRateLimiter(10, time.Minute)   // 每 IP 每分钟最多 10 次登录尝试
	refreshLimiter := middleware.NewLoginRateLimiter(60, time.Minute) // 刷新相对频繁，配额更宽松

	// 员工登录（按手机号）
	r.POST("/api/employee/login", loginLimiter.Handler(), func(c *gin.Context) {
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
		access, refresh, expiresIn, err := auth.IssueTokens(e.StaffID, "employee", e.TokenVersion)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "登录失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "token": access, "refreshToken": refresh, "expiresIn": expiresIn})
	})

	// 超管登录（按用户名）
	r.POST("/api/admin/login", loginLimiter.Handler(), func(c *gin.Context) {
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
		access, refresh, expiresIn, err := auth.IssueTokens(a.ID, "admin", a.TokenVersion)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "登录失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "token": access, "refreshToken": refresh, "expiresIn": expiresIn})
	})

	// 刷新 access 令牌：仅凭 refresh 令牌，不要求已登录（access 可能已过期）。
	r.POST("/api/refresh", refreshLimiter.Handler(), func(c *gin.Context) {
		var req struct {
			RefreshToken string `json:"refreshToken"`
		}
		if err := c.ShouldBindJSON(&req); err != nil || req.RefreshToken == "" {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "参数错误"})
			return
		}
		claims, err := auth.ParseToken(req.RefreshToken)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 401, "msg": "刷新令牌无效"})
			return
		}
		if claims.Typ != "refresh" {
			c.JSON(http.StatusOK, gin.H{"code": 401, "msg": "刷新令牌类型错误"})
			return
		}
		// 安全戳校验：refresh 同样受 token_version 约束，改密/禁用后立即失效
		currentVersion, err := middleware.TokenVersion(db, claims.Role, claims.StaffID)
		if err != nil || currentVersion != claims.VID {
			c.JSON(http.StatusOK, gin.H{"code": 401, "msg": "登录已失效，请重新登录"})
			return
		}
		access, _, expiresIn, err := auth.IssueTokens(claims.StaffID, claims.Role, currentVersion)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "签发失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "token": access, "expiresIn": expiresIn})
	})
}

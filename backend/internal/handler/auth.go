package handler

import (
	"net/http"
	"time"

	"langhui/internal/auth"
	"langhui/internal/config"
	"langhui/internal/middleware"
	"langhui/internal/model"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// RegisterAuth 注册登录 / 刷新 / 登出接口。
//   - 员工登录（小程序端）：Bearer 头鉴权，token 走响应体；
//   - 超管登录（PC 后台）：httpOnly Cookie 鉴权，token 不下发到响应体；
//   - /api/refresh：Cookie 流（读 refresh_token Cookie）或 Body 流（传 refreshToken）二选一；
//   - /api/logout：清除 httpOnly Cookie。
//
// 两个登录接口均叠加登录限流中间件（按客户端 IP 固定窗口计数，防爆破）。
func RegisterAuth(r *gin.Engine, db *gorm.DB, cfg config.Config) {
	loginLimiter := middleware.NewLoginRateLimiter(10, time.Minute)   // 每 IP 每分钟最多 10 次登录尝试
	refreshLimiter := middleware.NewLoginRateLimiter(60, time.Minute) // 刷新相对频繁，配额更宽松

	// 员工登录（按手机号）——小程序端，Bearer 头鉴权
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

	// 超管登录（按用户名）——PC 后台，httpOnly Cookie 鉴权（token 不入响应体）
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
		setAuthCookies(c, access, refresh, auth.AccessTokenTTL, auth.RefreshTokenTTL, cfg)
		c.JSON(http.StatusOK, gin.H{"code": 0, "expiresIn": expiresIn})
	})

	// 刷新 access 令牌：Cookie 流（PC 后台，读 refresh_token Cookie）或 Body 流（小程序，传 refreshToken）。
	r.POST("/api/refresh", refreshLimiter.Handler(), func(c *gin.Context) {
		var req struct {
			RefreshToken string `json:"refreshToken"`
		}
		_ = c.ShouldBindJSON(&req)
		refreshTok := req.RefreshToken
		fromCookie := false
		if refreshTok == "" {
			if ck, err := c.Cookie(refreshCookieName); err == nil && ck != "" {
				refreshTok = ck
				fromCookie = true
			}
		}
		if refreshTok == "" {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "参数错误"})
			return
		}
		claims, err := auth.ParseToken(refreshTok)
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
		access, refresh, expiresIn, err := auth.IssueTokens(claims.StaffID, claims.Role, currentVersion)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "签发失败"})
			return
		}
		if fromCookie {
			// Cookie 流：轮换下发新的 access / refresh Cookie
			setAuthCookies(c, access, refresh, auth.AccessTokenTTL, auth.RefreshTokenTTL, cfg)
			c.JSON(http.StatusOK, gin.H{"code": 0, "expiresIn": expiresIn})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "token": access, "expiresIn": expiresIn})
	})

	// 登出：清除 httpOnly 鉴权 Cookie（PC 后台）。小程序端由客户端丢弃 token 即可。
	r.POST("/api/logout", func(c *gin.Context) {
		clearAuthCookies(c, cfg)
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "ok"})
	})
}

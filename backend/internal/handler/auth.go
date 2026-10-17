package handler

import (
	"net/http"
	"time"

	"langhui/internal/auth"
	"langhui/internal/config"
	"langhui/internal/middleware"
	"langhui/internal/model"
	"langhui/internal/repository"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// RegisterAuth 注册登录 / 刷新 / 登出接口。
//   - 员工登录（小程序端）：Bearer 头鉴权，token 走响应体；
//   - 超管登录（PC 后台）：httpOnly Cookie 鉴权，token 不下发到响应体；
//   - /api/refresh：Cookie 流（读 refresh_token Cookie）或 Body 流（传 refreshToken）二选一；
//   - /api/logout：吊销当前会话 sid（单设备精确吊销）并清除 httpOnly Cookie。
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
			OpenID   string `json:"openid"`   // 微信 openid（wx.login 换取，供后台核实身份）
			Nickname string `json:"nickname"` // 微信昵称（用户授权后采集）
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
		// 记录本次登录的微信身份，供后台核实；逐字段判空，空值不覆盖已有值
		// （例如登录只静默拿到 openid 时，不能把申请阶段填好的昵称清空）。
		loginUpdates := map[string]interface{}{}
		if req.OpenID != "" {
			loginUpdates["openid"] = req.OpenID
		}
		if req.Nickname != "" {
			loginUpdates["nickname"] = req.Nickname
		}
		if len(loginUpdates) > 0 {
			db.Model(&e).Updates(loginUpdates)
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
		// 会话黑名单校验：该会话已登出 / 被踢下线时，refresh 同样失效
		if revoked, err := repository.IsSessionRevoked(db, claims.SID); err != nil || revoked {
			c.JSON(http.StatusOK, gin.H{"code": 401, "msg": "登录已失效，请重新登录"})
			return
		}
		// 刷新不换 sid：一次登录始终同一个会话，登出吊销该 sid 即终结整条令牌链
		access, refresh, expiresIn, err := auth.IssueTokensWithSID(claims.StaffID, claims.Role, currentVersion, claims.SID)
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

	// 登出：吊销当前会话 sid（单设备精确吊销），并清除 httpOnly 鉴权 Cookie（PC 后台）。
	//   - PC 后台：Cookie 流，浏览器自动携带 access_token，本接口解析出 sid 写入黑名单；
	//   - 小程序端：Bearer 流，客户端丢弃 token 即可；携带有效 Bearer 调用时同样吊销该 sid。
	// 只失效当前设备：同一账号的其他登录不受影响（那需要改密 / 禁用，走 token_version 账号级吊销）。
	// 不挂鉴权中间件：access 过期后客户端仍需能调本接口清 Cookie / 收尾会话。
	r.POST("/api/logout", func(c *gin.Context) {
		revokeCurrentSession(c, db)
		clearAuthCookies(c, cfg)
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "ok"})
	})
}

// revokeCurrentSession 从 Bearer / Cookie 中解析当前会话并写入黑名单。
// 解析失败（无 token / token 无效）时静默跳过：登出始终成功，清 Cookie 由调用方保证。
func revokeCurrentSession(c *gin.Context, db *gorm.DB) {
	// 优先解析 access（Bearer 或 access_token Cookie）；access 过期 / 缺失时
	// 退而解析 refresh_token Cookie，保证「access 已过期但 refresh 仍有效」时登出也能吊销会话。
	raw := middleware.ExtractToken(c)
	if raw == "" {
		if ck, err := c.Cookie(refreshCookieName); err == nil && ck != "" {
			raw = ck
		}
	}
	if raw == "" {
		return
	}
	claims, err := auth.ParseToken(raw)
	if err != nil || claims.SID == "" {
		return
	}
	if claims.Typ != "access" && claims.Typ != "refresh" {
		return
	}
	_ = repository.RevokeSession(db, claims.SID, claims.Role, claims.StaffID, sessionExpiry(c, claims))
}

// sessionExpiry 估算黑名单记录的过期时间：优先取 refresh 令牌的过期时间
// （PC 后台从 refresh_token Cookie 解析），都没有时保守按 now + RefreshTokenTTL。
// 该时间仅用于后台清理过期记录，不参与吊销判定。
func sessionExpiry(c *gin.Context, claims *auth.Claims) time.Time {
	if claims.Typ == "refresh" && claims.ExpiresAt != nil {
		return claims.ExpiresAt.Time
	}
	if ck, err := c.Cookie(refreshCookieName); err == nil && ck != "" {
		if rc, err := auth.ParseToken(ck); err == nil && rc.ExpiresAt != nil {
			return rc.ExpiresAt.Time
		}
	}
	return time.Now().Add(auth.RefreshTokenTTL)
}

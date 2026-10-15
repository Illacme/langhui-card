package middleware

import (
	"net/http"
	"strings"

	"langhui/internal/auth"
	"langhui/internal/model"
	"langhui/internal/repository"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// AuthMiddleware 校验 JWT；requiredRole 为空表示登录即可，否则需匹配角色。
// 登录签发见 handler/auth.go（账号 + 密码，JWT 签发 access + refresh）。
//
// 令牌来源二选一（兼容两种客户端）：
//   - Authorization: Bearer <token>（小程序端，token 由 JS 显式携带）；
//   - httpOnly Cookie access_token（PC 管理后台，浏览器自动携带，JS 不可读，防 XSS 窃取）。
//
// 除签名 / 过期外，还做三层校验：
//  1. claims.Typ 必须为 "access"（refresh 令牌只能用于 /api/refresh，不能当访问令牌用）；
//  2. claims.VID 必须与用户当前 token_version 一致——改密 / 账号禁用会使 token_version 自增，
//     从而让所有已签发的旧令牌立即失效（账号级即时吊销，无需黑名单表）；
//  3. claims.SID 必须不在会话黑名单内——登出只吊销当前设备的会话（单设备精确吊销），
//     不影响同一账号的其他登录（见 repository.IsSessionRevoked）。
//
// 代价：每个鉴权请求多两次按主键的查询（用户 + 黑名单 sid），小数据量运营场景可接受。
func AuthMiddleware(db *gorm.DB, requiredRole string) gin.HandlerFunc {
	return func(c *gin.Context) {
		raw := ExtractToken(c)
		if raw == "" {
			c.AbortWithStatusJSON(http.StatusOK, gin.H{"code": 401, "msg": "未登录"})
			return
		}
		claims, err := auth.ParseToken(raw)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusOK, gin.H{"code": 401, "msg": "登录失效"})
			return
		}
		if claims.Typ != "access" {
			c.AbortWithStatusJSON(http.StatusOK, gin.H{"code": 401, "msg": "令牌类型错误"})
			return
		}
		// 校验安全戳：令牌签发时的版本须与用户当前版本一致
		currentVersion, err := TokenVersion(db, claims.Role, claims.StaffID)
		if err != nil || currentVersion != claims.VID {
			c.AbortWithStatusJSON(http.StatusOK, gin.H{"code": 401, "msg": "登录已失效，请重新登录"})
			return
		}
		// 校验会话黑名单：登出 / 踢下线写入的 sid 立即失效（单设备精确吊销）
		if revoked, err := repository.IsSessionRevoked(db, claims.SID); err != nil || revoked {
			c.AbortWithStatusJSON(http.StatusOK, gin.H{"code": 401, "msg": "登录已失效，请重新登录"})
			return
		}
		c.Set("staffId", claims.StaffID)
		c.Set("role", claims.Role)
		if requiredRole != "" && claims.Role != requiredRole {
			c.AbortWithStatusJSON(http.StatusOK, gin.H{"code": 403, "msg": "无权限"})
			return
		}
		c.Next()
	}
}

// ExtractToken 从请求中取出裸 JWT：优先 Authorization 头，其次 access_token Cookie。
// 导出供 handler 层（如 /api/logout 吊销当前会话）复用同一套取token逻辑。
// 注：Cookie 名与 handler/cookie.go 的 accessCookieName 保持一致（两包不可互相引用，避免环）。
func ExtractToken(c *gin.Context) string {
	if h := c.GetHeader("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return strings.TrimPrefix(h, "Bearer ")
	}
	if ck, err := c.Cookie("access_token"); err == nil {
		return ck
	}
	return ""
}

// TokenVersion 按角色与主键取出用户当前的 token_version（安全戳）。
// 导出供 handler 层的 /api/refresh 复用，避免重复查询逻辑。
func TokenVersion(db *gorm.DB, role string, staffID int64) (int, error) {
	if role == "admin" {
		var a model.Admin
		if err := db.First(&a, "id = ?", staffID).Error; err != nil {
			return 0, err
		}
		return a.TokenVersion, nil
	}
	var e model.Employee
	if err := db.First(&e, "staff_id = ?", staffID).Error; err != nil {
		return 0, err
	}
	return e.TokenVersion, nil
}

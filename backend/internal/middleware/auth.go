package middleware

import (
	"net/http"
	"strings"

	"langhui/internal/auth"

	"github.com/gin-gonic/gin"
)

// AuthMiddleware 校验 JWT；requiredRole 为空表示登录即可，否则需匹配角色
// 登录签发见 handler/auth.go（需求：账号 + 密码，非微信 openid）
func AuthMiddleware(requiredRole string) gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.GetHeader("Authorization")
		if !strings.HasPrefix(h, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusOK, gin.H{"code": 401, "msg": "未登录"})
			return
		}
		claims, err := auth.ParseToken(strings.TrimPrefix(h, "Bearer "))
		if err != nil {
			c.AbortWithStatusJSON(http.StatusOK, gin.H{"code": 401, "msg": "登录失效"})
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

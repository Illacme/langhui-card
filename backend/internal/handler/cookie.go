package handler

import (
	"net/http"
	"time"

	"weichuan/internal/config"

	"github.com/gin-gonic/gin"
)

const (
	accessCookieName  = "access_token"
	refreshCookieName = "refresh_token"
)

// setAuthCookies 下发 httpOnly 的 access / refresh Cookie（PC 管理后台用）。
//   - HttpOnly：JS 不可读，防 XSS 窃取令牌；
//   - SameSite=Lax：跨站 POST/PUT/DELETE 不携带 Cookie，缓解 CSRF；
//   - Secure：由 COOKIE_SECURE 控制（生产 HTTPS 置 true，本地 http 需 false）。
func setAuthCookies(c *gin.Context, access, refresh string, accessTTL, refreshTTL time.Duration, cfg config.Config) {
	mk := func(name, val string, ttl time.Duration) *http.Cookie {
		return &http.Cookie{
			Name:     name,
			Value:    val,
			Path:     "/",
			Domain:   cfg.CookieDomain,
			MaxAge:   int(ttl.Seconds()),
			Secure:   cfg.CookieSecure,
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
		}
	}
	http.SetCookie(c.Writer, mk(accessCookieName, access, accessTTL))
	http.SetCookie(c.Writer, mk(refreshCookieName, refresh, refreshTTL))
}

// clearAuthCookies 清除鉴权 Cookie（登出用）：置空并 MaxAge=-1 立即过期。
func clearAuthCookies(c *gin.Context, cfg config.Config) {
	for _, name := range []string{accessCookieName, refreshCookieName} {
		http.SetCookie(c.Writer, &http.Cookie{
			Name:     name,
			Value:    "",
			Path:     "/",
			Domain:   cfg.CookieDomain,
			MaxAge:   -1,
			Secure:   cfg.CookieSecure,
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
		})
	}
}

package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// LoginRateLimiter 登录接口固定窗口限流（按客户端 IP）。
// 用于防爆破：同一 IP 在窗口内登录尝试超过上限即拒绝。
// 注意：生产若后端位于反向代理之后，应在 gin 中设置 TrustedProxies 以正确识别真实 IP，
// 否则此处取到的是代理 IP，限流粒度会变粗。
type LoginRateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*loginBucket
	// limitFn 每次判定前取一次上限，而不是构造时定死——后台改限流阈值即时生效、无需重启。
	limitFn func() int
	// fallback 为 limitFn 缺失 / 返回非法值（<=0）时的兜底值，保证限流器永远有效。
	fallback int
	window   time.Duration
}

type loginBucket struct {
	count   int
	resetAt time.Time
}

// NewLoginRateLimiter 创建限流器。limitFn 返回窗口内允许的最大尝试次数（每次判定前调用，
// 因此后台改阈值即时生效），fallback 在 limitFn 为 nil 或返回非法值时兜底，window 为窗口长度。
func NewLoginRateLimiter(limitFn func() int, fallback int, window time.Duration) *LoginRateLimiter {
	if fallback <= 0 {
		fallback = 10
	}
	if window <= 0 {
		window = time.Minute
	}
	return &LoginRateLimiter{
		buckets:  make(map[string]*loginBucket),
		limitFn:  limitFn,
		fallback: fallback,
		window:   window,
	}
}

// limit 当前生效的上限：limitFn 优先，缺失或非法（<=0）时回落 fallback。
func (l *LoginRateLimiter) limit() int {
	if l.limitFn != nil {
		if n := l.limitFn(); n > 0 {
			return n
		}
	}
	return l.fallback
}

// allow 判断该 IP 本次是否放行，并维护计数（窗口过期后自动重置）。
func (l *LoginRateLimiter) allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	b, ok := l.buckets[ip]
	if !ok || now.After(b.resetAt) {
		l.buckets[ip] = &loginBucket{count: 1, resetAt: now.Add(l.window)}
		return true
	}
	if b.count >= l.limit() {
		return false
	}
	b.count++
	return true
}

// Handler 返回 gin 中间件：超限返回 HTTP 200 + {code:429}，与现有鉴权约定（非 HTTP 状态）一致。
func (l *LoginRateLimiter) Handler() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !l.allow(c.ClientIP()) {
			c.AbortWithStatusJSON(http.StatusOK, gin.H{
				"code": 429,
				"msg":  "登录尝试过于频繁，请稍后再试",
			})
			return
		}
		c.Next()
	}
}

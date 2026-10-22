package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// RateLimiter 固定窗口限流（默认按客户端 IP 分桶）。
// 用于防爆破 / 防刷：同一 key 在窗口内超过上限即拒绝。
// 注意：生产若后端位于反向代理之后，应在 gin 中设置 TrustedProxies 以正确识别真实 IP，
// 否则此处取到的是代理 IP，限流粒度会变粗。
type RateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*rateBucket
	// limitFn 每次判定前取一次上限，而不是构造时定死——后台改限流阈值即时生效、无需重启。
	limitFn func() int
	// fallback 为 limitFn 缺失 / 返回非法值（<=0）时的兜底值，保证限流器永远有效。
	fallback int
	window   time.Duration
	// keyFn 分桶键（默认客户端 IP）；msg 超限文案（默认通用文案）。
	keyFn func(c *gin.Context) string
	msg   string
}

type rateBucket struct {
	count   int
	resetAt time.Time
}

// NewRateLimiter 通用固定窗口限流：默认按客户端 IP 分桶，超限文案为通用文案。
// 非 IP 维度（如「IP + 业务参数」）用 WithKey 覆盖，文案用 WithMessage 覆盖。
func NewRateLimiter(limitFn func() int, fallback int, window time.Duration) *RateLimiter {
	if fallback <= 0 {
		fallback = 10
	}
	if window <= 0 {
		window = time.Minute
	}
	return &RateLimiter{
		buckets:  make(map[string]*rateBucket),
		limitFn:  limitFn,
		fallback: fallback,
		window:   window,
		msg:      "请求过于频繁，请稍后再试",
	}
}

// NewLoginRateLimiter 登录 / 刷新场景：按 IP 分桶 + 登录专用文案。
func NewLoginRateLimiter(limitFn func() int, fallback int, window time.Duration) *RateLimiter {
	return NewRateLimiter(limitFn, fallback, window).
		WithMessage("登录尝试过于频繁，请稍后再试")
}

// WithKey 覆盖分桶键（如把业务参数也算进 key，避免单一 IP 下的枚举刷量）
func (l *RateLimiter) WithKey(fn func(c *gin.Context) string) *RateLimiter {
	if fn != nil {
		l.keyFn = fn
	}
	return l
}

// WithMessage 覆盖超限提示文案
func (l *RateLimiter) WithMessage(msg string) *RateLimiter {
	if msg != "" {
		l.msg = msg
	}
	return l
}

// limit 当前生效的上限：limitFn 优先，缺失或非法（<=0）时回落 fallback。
func (l *RateLimiter) limit() int {
	if l.limitFn != nil {
		if n := l.limitFn(); n > 0 {
			return n
		}
	}
	return l.fallback
}

// allow 判断该 key 本次是否放行，并维护计数（窗口过期后自动重置）。
func (l *RateLimiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	b, ok := l.buckets[key]
	if !ok || now.After(b.resetAt) {
		l.buckets[key] = &rateBucket{count: 1, resetAt: now.Add(l.window)}
		return true
	}
	if b.count >= l.limit() {
		return false
	}
	b.count++
	return true
}

// Handler 返回 gin 中间件：超限返回 HTTP 200 + {code:429}，与现有鉴权约定（非 HTTP 状态）一致。
func (l *RateLimiter) Handler() gin.HandlerFunc {
	return func(c *gin.Context) {
		key := c.ClientIP()
		if l.keyFn != nil {
			key = l.keyFn(c)
		}
		if !l.allow(key) {
			c.AbortWithStatusJSON(http.StatusOK, gin.H{
				"code": 429,
				"msg":  l.msg,
			})
			return
		}
		c.Next()
	}
}

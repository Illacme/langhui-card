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
	// lastSweep 上次清理过期桶的时间；sweepEvery 清理间隔（取窗口量级）。
	// 见 allow 里的说明——没有这个的话 buckets 会随唯一 key 数量单调增长。
	lastSweep  time.Time
	sweepEvery time.Duration
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
		buckets:    make(map[string]*rateBucket),
		limitFn:    limitFn,
		fallback:   fallback,
		window:     window,
		msg:        "请求过于频繁，请稍后再试",
		lastSweep:  time.Now(),
		sweepEvery: window,
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
//
// 顺带负责清理过期桶。这是必须的：桶只在「同一个 key 再次访问」时才会被覆盖成
// 新的计数，也就是说一个 IP 只要不再来，它的过期桶就永远留在 map 里。
// 公网服务每天会遇到成百上千个新 IP，长期运行下这张表会单调增长——
// 典型的「不报错、只在 OOM 那天被发现」的内存缓慢泄漏。
// 清理用惰性触发（而不是后台 goroutine）：不需要为限流器额外管理生命周期，
// 代价是每 sweepEvery 才做一次 O(n) 扫描，与请求处理共享同一把锁。
func (l *RateLimiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	if now.After(l.lastSweep.Add(l.sweepEvery)) {
		l.sweep(now)
		l.lastSweep = now
	}
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

// sweep 删除已过期的桶（调用方须持有锁）。
func (l *RateLimiter) sweep(now time.Time) {
	for k, b := range l.buckets {
		if now.After(b.resetAt) {
			delete(l.buckets, k)
		}
	}
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

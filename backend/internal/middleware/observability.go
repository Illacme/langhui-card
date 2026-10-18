package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
)

// ─────────────────────────────────────────────────────────────
// 可观测性三件套（零第三方依赖）：
//   1. RequestID：注入 / 透传 X-Request-Id，串联访问日志与排障；
//   2. AccessLog：每个请求一行结构化日志（方法 / 路径 / 状态 / 耗时 / IP）；
//   3. Metrics：内存计数器 + Prometheus 文本格式 /metrics（默认关闭，
//      由 METRICS_ENABLED=1 显式开启，避免公网暴露流量规模）。
// 日志走标准 log（12-factor），生产可被 journald / 日志采集直接收集。
// ─────────────────────────────────────────────────────────────

// RequestIDHeader 请求 ID 头（入站缺失则生成，出源与响应头均带回，便于串联排查）。
const RequestIDHeader = "X-Request-Id"

// RequestIDKey 请求 ID 在 gin.Context 中的键。
const RequestIDKey = "requestId"

// RequestID 注入请求 ID：优先沿用调用方传来的 X-Request-Id（网关 / 上游透传），
// 否则生成 8 字节随机十六进制串；同时写入响应头，便于客户端报障时引用。
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		rid := c.GetHeader(RequestIDHeader)
		if rid == "" {
			rid = newRequestID()
		}
		c.Set(RequestIDKey, rid)
		c.Writer.Header().Set(RequestIDHeader, rid)
		c.Next()
	}
}

// RequestIDOf 取出当前请求的 ID（未挂 RequestID 中间件时返回空串）。
func RequestIDOf(c *gin.Context) string {
	if v, ok := c.Get(RequestIDKey); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func newRequestID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		// 熵耗尽属极端情况，退化为纳秒时间戳
		return strconv.FormatInt(time.Now().UnixNano(), 16)
	}
	return hex.EncodeToString(b)
}

// AccessLog 访问日志：请求结束后输出一行，含状态码与耗时。
// 5xx 以 [ACCESS-5XX] 前缀标出，便于日志告警规则直接匹配。
// 注意：panic 会抛穿本中间件（c.Next() 不返回），此类 500 由链上的
// gin.Recovery 记录（含栈信息），不重复输出访问日志行。
func AccessLog() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		status := c.Writer.Status()
		latency := time.Since(start).Round(time.Millisecond)
		line := fmt.Sprintf("%s %s %d %s req=%s ip=%s",
			c.Request.Method, c.Request.URL.Path, status, latency, RequestIDOf(c), c.ClientIP())
		if status >= 500 {
			log.Printf("[ACCESS-5XX] %s", line)
			return
		}
		log.Printf("[ACCESS] %s", line)
	}
}

// Metrics 轻量进程内指标收集器（无第三方依赖，单实例口径）。
// 多实例部署时各实例只反映自身流量，聚合由 Prometheus 侧完成。
type Metrics struct {
	mu       sync.Mutex
	counters map[string]int64 // key: "METHOD|statusClass"（如 "GET|2xx"）
	total    atomic.Int64
	started  time.Time
	// Enabled 运行时开关（由系统设置 metrics_enabled 驱动，见 cmd/server）。
	// 关闭时不计数、GET /metrics 返回 404——避免在公网暴露流量规模。
	// nil 表示不接入系统设置（始终启用）。
	Enabled func() bool
}

// enabled 当前是否采集指标；未接入开关时恒为 true。
func (m *Metrics) enabled() bool {
	return m.Enabled == nil || m.Enabled()
}

// NewMetrics 创建指标收集器。
func NewMetrics() *Metrics {
	return &Metrics{counters: make(map[string]int64), started: time.Now()}
}

// Middleware 计数中间件：按「方法 × 状态类（2xx/4xx/5xx…）」累计请求数。
func (m *Metrics) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		if !m.enabled() {
			return
		}
		class := fmt.Sprintf("%dxx", c.Writer.Status()/100)
		key := c.Request.Method + "|" + class
		m.mu.Lock()
		m.counters[key]++
		m.mu.Unlock()
		m.total.Add(1)
	}
}

// Handler 输出 Prometheus 文本格式（text/plain; version=0.0.4）指标。
func (m *Metrics) Handler() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 指标默认关闭：开关未开时 /metrics 视为不存在，避免公网探测到流量规模
		if !m.enabled() {
			c.Status(http.StatusNotFound)
			return
		}
		m.mu.Lock()
		keys := make([]string, 0, len(m.counters))
		for k := range m.counters {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var b strings.Builder
		b.WriteString("# HELP langhui_http_requests_total HTTP requests total, grouped by method and status class\n")
		b.WriteString("# TYPE langhui_http_requests_total counter\n")
		for _, k := range keys {
			method, class, _ := strings.Cut(k, "|")
			fmt.Fprintf(&b, "langhui_http_requests_total{method=%q,status=%q} %d\n", method, class, m.counters[k])
		}
		total := m.total.Load()
		uptime := int64(time.Since(m.started).Seconds())
		m.mu.Unlock()

		b.WriteString("# HELP langhui_http_requests_received_total All received HTTP requests\n")
		b.WriteString("# TYPE langhui_http_requests_received_total counter\n")
		fmt.Fprintf(&b, "langhui_http_requests_received_total %d\n", total)
		b.WriteString("# HELP langhui_uptime_seconds Process uptime in seconds\n")
		b.WriteString("# TYPE langhui_uptime_seconds gauge\n")
		fmt.Fprintf(&b, "langhui_uptime_seconds %d\n", uptime)

		c.Data(http.StatusOK, "text/plain; version=0.0.4", []byte(b.String()))
	}
}

package middleware

import (
	"bytes"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// testEngine 装配一个最小引擎：RequestID + AccessLog + 计数器 + 指标端点，
// 以及若干用于制造状态码的路由（含一个 panic 路由验证 Recovery 不受影响）。
func testEngine() (*gin.Engine, *Metrics) {
	gin.SetMode(gin.TestMode)
	m := NewMetrics()
	r := gin.New()
	r.Use(gin.Recovery(), RequestID(), m.Middleware())
	r.GET("/ok", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
	r.GET("/missing", func(c *gin.Context) { c.JSON(http.StatusNotFound, gin.H{"code": 404}) })
	r.GET("/boom", func(c *gin.Context) { panic("boom") })
	r.GET("/metrics", m.Handler())
	return r, m
}

// TestMetricsEnabledGate 指标开关接线验证：系统设置 metrics_enabled 通过 Metrics.Enabled
// 接入运行期开关——关闭时 /metrics 必须视为不存在（404），否则等于把流量规模暴露到公网。
func TestMetricsEnabledGate(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// 1) 未接入开关（Enabled=nil）→ 保持原有「始终启用」行为
	m := NewMetrics()
	r := gin.New()
	r.GET("/metrics", m.Handler())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/metrics", nil))
	require.Equal(t, http.StatusOK, w.Code, "未接入开关时应照常输出指标")

	// 2) 开关关闭 → 404
	off := NewMetrics()
	off.Enabled = func() bool { return false }
	rOff := gin.New()
	rOff.GET("/metrics", off.Handler())
	wOff := httptest.NewRecorder()
	rOff.ServeHTTP(wOff, httptest.NewRequest("GET", "/metrics", nil))
	require.Equal(t, http.StatusNotFound, wOff.Code, "开关关闭时 /metrics 应视为不存在")

	// 3) 开关打开 → 200，且计数中间件开始累计
	on := NewMetrics()
	on.Enabled = func() bool { return true }
	rOn := gin.New()
	rOn.Use(on.Middleware())
	rOn.GET("/ok", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
	rOn.GET("/metrics", on.Handler())
	rOn.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/ok", nil))
	wOn := httptest.NewRecorder()
	rOn.ServeHTTP(wOn, httptest.NewRequest("GET", "/metrics", nil))
	require.Equal(t, http.StatusOK, wOn.Code)
	// 注意断言的是**渲染后的 Prometheus 文本**（method="GET",status="2xx"），
	// 不是内部计数键（"GET|2xx"）——两者格式不同，别按内部键断言。
	require.Contains(t, wOn.Body.String(), `status="2xx"`, "开关打开后应能统计到刚发生的请求")
}

func TestRequestIDGeneratedAndEchoed(t *testing.T) {
	r, _ := testEngine()

	// 未带请求头 → 服务端生成并回写响应头
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/ok", nil))
	rid := w.Header().Get(RequestIDHeader)
	require.NotEmpty(t, rid, "应生成 X-Request-Id")
	require.Len(t, rid, 16, "8 字节十六进制 = 16 字符")

	// 带请求头 → 原样透传（网关 / 上游串联）
	w2 := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/ok", nil)
	req.Header.Set(RequestIDHeader, "trace-from-gw")
	r.ServeHTTP(w2, req)
	require.Equal(t, "trace-from-gw", w2.Header().Get(RequestIDHeader))
}

func TestMetricsCountersAndFormat(t *testing.T) {
	r, _ := testEngine()

	for i := 0; i < 3; i++ {
		r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/ok", nil))
	}
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/missing", nil))

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/metrics", nil))
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "text/plain; version=0.0.4", w.Header().Get("Content-Type"))

	body := w.Body.String()
	require.Contains(t, body, `langhui_http_requests_total{method="GET",status="2xx"} 3`)
	require.Contains(t, body, `langhui_http_requests_total{method="GET",status="4xx"} 1`)
	require.Contains(t, body, "langhui_uptime_seconds ")
	require.Contains(t, body, "# TYPE langhui_http_requests_total counter")

	// /metrics 自身这次请求也会被计数（4xx 不变，2xx +1 → 4）
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, httptest.NewRequest("GET", "/metrics", nil))
	require.Contains(t, w2.Body.String(), `langhui_http_requests_total{method="GET",status="2xx"} 4`)
}

func TestAccessLogOutput(t *testing.T) {
	// 捕获标准 log 输出，验证 AccessLog 行格式与 5xx 前缀。
	// 注意：AccessLog 在 c.Next() 之后记录，panic 会抛穿它——panic 的 500
	// 由 gin.Recovery 负责记录（栈信息），AccessLog 只覆盖正常返回的请求。
	gin.SetMode(gin.TestMode)
	var buf bytes.Buffer
	oldOut, oldFlags := log.Writer(), log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	defer func() { log.SetOutput(oldOut); log.SetFlags(oldFlags) }()

	r := gin.New()
	r.Use(gin.Recovery(), RequestID(), AccessLog())
	r.GET("/ok", func(c *gin.Context) { c.Status(http.StatusOK) })
	r.GET("/oops", func(c *gin.Context) { c.Status(http.StatusInternalServerError) })

	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/ok", nil))
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/oops", nil))

	out := buf.String()
	require.Contains(t, out, "[ACCESS] GET /ok 200")
	require.Contains(t, out, "req=", "访问日志应带请求 ID")
	require.Contains(t, out, "[ACCESS-5XX] GET /oops 500", "5xx 应带专用前缀便于告警匹配")
}

func TestPanicRecoveredByChain(t *testing.T) {
	// panic 不应逃逸出中间件链：Recovery 捕获后返回 500
	r, _ := testEngine()
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/boom", nil))
	require.Equal(t, http.StatusInternalServerError, w.Code)
}

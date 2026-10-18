package handler

import (
	"net/http"
	"testing"

	"langhui/internal/config"
	"langhui/internal/setting"

	"github.com/stretchr/testify/require"
)

// TestSettingsAPI 覆盖系统设置接口：平台超管读取/更新、类型校验、未知 key 拒绝、公开接口白名单。
func TestSettingsAPI(t *testing.T) {
	r := setupSQLiteEngine(t)
	c := newTestClient(r, t)
	adminToken := loginAdmin(t, c)

	// 1) 平台超管读取全部可配置项：含分组与默认值
	code, body := c.do("GET", "/api/admin/settings", nil, adminToken)
	require.Equal(t, http.StatusOK, code)
	require.EqualValues(t, 0, body["code"])
	list, ok := body["data"].([]interface{})
	require.True(t, ok, "data 应为设置项数组")
	require.NotEmpty(t, list)

	// 找到 brand_name 与 upload_max_mb 的默认值（应保持 registry 缺省）
	var brandDef, uploadDef string
	for _, it := range list {
		m := it.(map[string]interface{})
		if m["key"] == "brand_name" {
			brandDef = m["value"].(string)
		}
		if m["key"] == "upload_max_mb" {
			uploadDef = m["value"].(string)
		}
	}
	require.Equal(t, "智能名片", brandDef)
	require.Equal(t, "10", uploadDef)

	// 2) 批量更新：改品牌名 / 上传上限 / 指标开关 → 成功
	_, body = c.do("PUT", "/api/admin/settings", map[string]string{
		"brand_name":      "朗汇名片",
		"upload_max_mb":   "20",
		"metrics_enabled": "true",
	}, adminToken)
	require.EqualValues(t, 0, body["code"])

	// 3) 再次读取：值已更新（即时生效，无需重启）
	_, body = c.do("GET", "/api/admin/settings", nil, adminToken)
	list = body["data"].([]interface{})
	var brandNew, uploadNew, metricsNew string
	for _, it := range list {
		m := it.(map[string]interface{})
		switch m["key"] {
		case "brand_name":
			brandNew = m["value"].(string)
		case "upload_max_mb":
			uploadNew = m["value"].(string)
		case "metrics_enabled":
			metricsNew = m["value"].(string)
		}
	}
	require.Equal(t, "朗汇名片", brandNew)
	require.Equal(t, "20", uploadNew)
	require.Equal(t, "true", metricsNew)

	// 4) 类型校验：upload_max_mb 传非整数 → code:1
	_, body = c.do("PUT", "/api/admin/settings", map[string]string{
		"upload_max_mb": "abc",
	}, adminToken)
	require.EqualValues(t, 1, body["code"])

	// 5) 未知 key → code:1
	_, body = c.do("PUT", "/api/admin/settings", map[string]string{
		"not_a_real_key": "x",
	}, adminToken)
	require.EqualValues(t, 1, body["code"])

	// 6) 企业管理员（非平台超管）访问 → 403（PlatformAdminMiddleware 兜底）
	//    用开通的乙公司管理员登录后被拒；这里直接以无平台权限的 Bearer 访问验证中间件：
	//    用员工 token（非 admin）访问应被 authenticate 拦为 401/403，均非 0。
	empToken := loginEmployee(t, c) // 见下辅助：建员工并登录
	_, body = c.do("GET", "/api/admin/settings", nil, empToken)
	require.NotEqual(t, 0, body["code"])

	// 7) 公开接口：仅含非敏感项（brand_name），不含 wx_appid
	code, body = c.do("GET", "/api/public/settings", nil, "")
	require.Equal(t, http.StatusOK, code)
	require.EqualValues(t, 0, body["code"])
	pub := body["data"].(map[string]interface{})
	require.Equal(t, "朗汇名片", pub["brand_name"])
	_, hasAppID := pub["wx_appid"]
	require.False(t, hasAppID, "wx_appid 为敏感项，不应出现在公开接口")
}

// TestWXAppIDSettingOverridesEnv wx_appid 接线验证。
// 历史缺陷：该项曾在「系统设置」界面可见可存，但代码里**从不读取**（死配置，改了没用）。
// 本测试把语义钉死：设置非空 → 以设置为准；设置为空 → 回退环境变量 WX_APPID。
// （不能写成「设置项存在就覆盖」：seed 会把它写成空串，那样会让只配环境变量的部署凭空失效。）
func TestWXAppIDSettingOverridesEnv(t *testing.T) {
	r := setupSQLiteEngine(t)
	c := newTestClient(r, t)
	adminToken := loginAdmin(t, c)

	cfg := config.Config{WXAppID: "env-appid"}
	// 1) seed 缺省为空串 → 回退环境变量
	require.Equal(t, "env-appid", resolveWXAppID(cfg))

	// 2) 后台设置 AppID → 覆盖环境变量，且无需重启
	_, body := c.do("PUT", "/api/admin/settings", map[string]string{"wx_appid": "ui-appid"}, adminToken)
	require.EqualValues(t, 0, body["code"])
	require.Equal(t, "ui-appid", resolveWXAppID(cfg))

	// 3) 清空设置 → 重新回退环境变量
	_, body = c.do("PUT", "/api/admin/settings", map[string]string{"wx_appid": ""}, adminToken)
	require.EqualValues(t, 0, body["code"])
	require.Equal(t, "env-appid", resolveWXAppID(cfg))
}

// TestMetricsEnabledSettingFallsBackToEnv metrics_enabled 接线验证（同为曾经的死配置项）。
// 缺省空串 =「未设置」→ 回退环境变量 METRICS_ENABLED；界面显式设置后以界面为准。
func TestMetricsEnabledSettingFallsBackToEnv(t *testing.T) {
	r := setupSQLiteEngine(t)
	c := newTestClient(r, t)
	adminToken := loginAdmin(t, c)

	// 1) 未设置：空串视同缺失，回退传入的（环境变量）默认值
	require.False(t, setting.GetBool("metrics_enabled", false))
	require.True(t, setting.GetBool("metrics_enabled", true))

	// 2) 界面显式开启 → 环境变量为 false 也应以界面为准
	_, body := c.do("PUT", "/api/admin/settings", map[string]string{"metrics_enabled": "true"}, adminToken)
	require.EqualValues(t, 0, body["code"])
	require.True(t, setting.GetBool("metrics_enabled", false))

	// 3) 界面显式关闭 → 环境变量为 true 也应以界面为准
	_, body = c.do("PUT", "/api/admin/settings", map[string]string{"metrics_enabled": "false"}, adminToken)
	require.EqualValues(t, 0, body["code"])
	require.False(t, setting.GetBool("metrics_enabled", true))
}

// loginEmployee 建一个员工并登录，返回其 Bearer 令牌（用于越权断言）。
func loginEmployee(t *testing.T, c *testClient) string {
	t.Helper()
	_, body := c.do("POST", "/api/admin/employees", map[string]interface{}{
		"name": "设置测试员工", "phone": "13900000999", "email": "s@a.com", "title": "测试",
	}, c.cookies["access_token"])
	require.EqualValues(t, 0, body["code"])
	initPwd := body["initialPassword"].(string)
	_, body = c.do("POST", "/api/employee/login", map[string]interface{}{
		"phone": "13900000999", "password": initPwd,
	}, "")
	require.EqualValues(t, 0, body["code"])
	return body["token"].(string)
}

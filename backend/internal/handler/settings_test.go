package handler

import (
	"encoding/json"
	"fmt"
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

// TestIndependentLockMakesCompaniesReadOnly independent_lock 接线验证（同为曾经的死配置项）。
// 语义：开启后企业管理页整体只读——列表 / 管理员列表照常可查，
// 新建企业、编辑企业、启停企业、新建企业管理员一律 403。
func TestIndependentLockMakesCompaniesReadOnly(t *testing.T) {
	r := setupSQLiteEngine(t)
	c := newTestClient(r, t)
	loginAdmin(t, c) // 企管接口走 Cookie 鉴权：登录即写入 jar

	// 1) 未锁定：可正常新建企业
	_, body := c.doWithCookie("POST", "/api/admin/companies", map[string]interface{}{"name": "甲公司"})
	require.EqualValues(t, 0, body["code"])
	cid := fmt.Sprint(body["data"].(map[string]interface{})["id"])

	// 2) 开启独立版锁定
	_, body = c.doWithCookie("PUT", "/api/admin/settings", map[string]string{"independent_lock": "true"})
	require.EqualValues(t, 0, body["code"])

	// 3) 四类写操作全部被拒（403），且给出明确原因
	_, body = c.doWithCookie("POST", "/api/admin/companies", map[string]interface{}{"name": "乙公司"})
	require.EqualValues(t, 403, body["code"])
	require.Contains(t, body["msg"], "只读")

	_, body = c.doWithCookie("PUT", "/api/admin/companies/"+cid, map[string]interface{}{"name": "改名"})
	require.EqualValues(t, 403, body["code"])

	_, body = c.doWithCookie("POST", "/api/admin/companies/"+cid+"/status",
		map[string]interface{}{"status": "disabled"})
	require.EqualValues(t, 403, body["code"])

	_, body = c.doWithCookie("POST", "/api/admin/companies/"+cid+"/admins",
		map[string]interface{}{"username": "boss2", "password": "pwd12345"})
	require.EqualValues(t, 403, body["code"])

	// 4) 只读接口仍然可用（列表 / 管理员列表）
	_, body = c.doWithCookie("GET", "/api/admin/companies", nil)
	require.EqualValues(t, 0, body["code"])
	_, body = c.doWithCookie("GET", "/api/admin/companies/"+cid+"/admins", nil)
	require.EqualValues(t, 0, body["code"])

	// 5) /admin/me 下发锁定状态，供前端把企管页切为只读
	_, body = c.doWithCookie("GET", "/api/admin/me", nil)
	require.EqualValues(t, 0, body["code"])
	require.Equal(t, true, body["data"].(map[string]interface{})["independentLock"])

	// 6) 关闭锁定后写操作立刻恢复（运行期开关，无需重启）
	_, body = c.doWithCookie("PUT", "/api/admin/settings", map[string]string{"independent_lock": "false"})
	require.EqualValues(t, 0, body["code"])
	_, body = c.doWithCookie("POST", "/api/admin/companies", map[string]interface{}{"name": "丙公司"})
	require.EqualValues(t, 0, body["code"])
}

// TestSecretSettingsNeverLeakRealValue 敏感项（AppSecret / 存储 SecretKey / DB 连接串 / JWT 密钥）
// 的真实值永不出现在接口响应里——后台是给运营用的，响应体里的明文密钥等于泄漏。
func TestSecretSettingsNeverLeakRealValue(t *testing.T) {
	r := setupSQLiteEngine(t)
	c := newTestClient(r, t)
	adminToken := loginAdmin(t, c)

	const secret = "real-app-secret-9f3a"
	_, body := c.do("PUT", "/api/admin/settings", map[string]string{"wx_appsecret": secret}, adminToken)
	require.EqualValues(t, 0, body["code"])

	_, body = c.do("GET", "/api/admin/settings", nil, adminToken)
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	require.NotContains(t, string(raw), secret, "敏感项真实值绝不能出现在接口响应里")

	var got map[string]interface{}
	for _, it := range body["data"].([]interface{}) {
		m := it.(map[string]interface{})
		if m["key"] == "wx_appsecret" {
			got = m
		}
	}
	require.NotNil(t, got)
	require.Equal(t, setting.Mask, got["value"])
	require.Equal(t, true, got["secret"])
	require.Equal(t, true, got["configured"])

	// 掩码提交要被拒绝：否则用户一点保存就会把 "******" 当成真值写进库
	_, body = c.do("PUT", "/api/admin/settings", map[string]string{"wx_appsecret": setting.Mask}, adminToken)
	require.EqualValues(t, 1, body["code"])

	// 真实值仍然被代码读到（回退链：设置 → 环境变量）
	cfg := config.Config{WXAppSecret: "env-secret"}
	require.Equal(t, secret, resolveWXAppSecret(cfg))
	_, body = c.do("PUT", "/api/admin/settings", map[string]string{"wx_appsecret": ""}, adminToken)
	require.EqualValues(t, 0, body["code"])
	require.Equal(t, "env-secret", resolveWXAppSecret(cfg))
}

// TestReadOnlySettingsCannotBeWrittenFromUI 只读项（DB 连接串 / JWT 密钥 / Cookie 安全）
// 必须在界面可见（管理员得知道当前配了什么），但不可改——改错会把「改回来的入口」一起弄丢。
func TestReadOnlySettingsCannotBeWrittenFromUI(t *testing.T) {
	// 只读项的值来自环境变量（测试装配里的 cfg.DBDSN 是空的），先把它设上，
	// 才能验证「界面如实展示已配置」而不是显示一片空白。
	t.Setenv("DB_DSN", "sqlite://test-only")
	r := setupSQLiteEngine(t)
	c := newTestClient(r, t)
	adminToken := loginAdmin(t, c)

	// 1) 只读项照常出现在列表里，且带 readOnly / envVar 元数据
	_, body := c.do("GET", "/api/admin/settings", nil, adminToken)
	meta := map[string]map[string]interface{}{}
	for _, it := range body["data"].([]interface{}) {
		m := it.(map[string]interface{})
		meta[m["key"].(string)] = m
	}
	for _, k := range []string{"db_dsn", "jwt_key", "cookie_secure", "cookie_domain"} {
		require.Contains(t, meta, k)
		require.Equal(t, true, meta[k]["readOnly"], k)
		require.NotEmpty(t, meta[k]["envVar"], k)
	}
	require.Equal(t, setting.Mask, meta["db_dsn"]["value"])

	// 2) 试图从界面改写 → 拒绝，并提示改法（只说「不允许」等于没说）
	_, body = c.do("PUT", "/api/admin/settings", map[string]string{"db_dsn": "root@tcp(bad)"}, adminToken)
	require.EqualValues(t, 1, body["code"])
	require.Contains(t, body["msg"], "DB_DSN")

	_, body = c.do("PUT", "/api/admin/settings", map[string]string{"jwt_key": "x"}, adminToken)
	require.EqualValues(t, 1, body["code"])

	// 3) 只读项不落库：库里没有 db_dsn 行（否则会出现「界面没改、库里有值」的误解）
	require.Equal(t, "", setting.GetString("db_dsn", ""), "只读项不应被 seed 进 sys_setting")
}

// TestStorageSettingsOverlayEnv 云存储 / CDN 凭据的覆盖语义：
// 界面设置非空 → 覆盖环境变量；留空 → 回退环境变量（不是「设了空串就把它清成空」）。
func TestStorageSettingsOverlayEnv(t *testing.T) {
	r := setupSQLiteEngine(t)
	c := newTestClient(r, t)
	adminToken := loginAdmin(t, c)

	base := func() config.Config {
		return config.Config{
			StorageDriver:  "local",
			OssEndpoint:    "oss-env.aliyuncs.com",
			OssBucket:      "env-bucket",
			CDNAccessKeyID: "env-cdn-id",
			CDNEnabled:     false,
		}
	}

	// 1) 界面全空 → 原样保留环境变量值
	cfg := base()
	cfg.OverlayFromSettings()
	require.Equal(t, "local", cfg.StorageDriver)
	require.Equal(t, "oss-env.aliyuncs.com", cfg.OssEndpoint)
	require.Equal(t, "env-cdn-id", cfg.CDNAccessKeyID)
	require.False(t, cfg.CDNEnabled)

	// 2) 界面填了 → 覆盖（MinIO 与 OSS 共用同一组界面字段，按驱动二选一）
	_, body := c.do("PUT", "/api/admin/settings", map[string]string{
		"storage_driver":   "oss",
		"storage_endpoint": "oss-cn-hangzhou.aliyuncs.com",
		"storage_bucket":   "ui-bucket",
		"cdn_enabled":      "true",
	}, adminToken)
	require.EqualValues(t, 0, body["code"])

	cfg = base()
	cfg.OverlayFromSettings()
	require.Equal(t, "oss", cfg.StorageDriver)
	require.Equal(t, "oss-cn-hangzhou.aliyuncs.com", cfg.OssEndpoint)
	require.Equal(t, "ui-bucket", cfg.OssBucket)
	require.True(t, cfg.CDNEnabled)

	// 3) 清空 → 回到环境变量
	_, body = c.do("PUT", "/api/admin/settings", map[string]string{
		"storage_driver":   "",
		"storage_endpoint": "",
		"storage_bucket":   "",
		"cdn_enabled":      "",
	}, adminToken)
	require.EqualValues(t, 0, body["code"])

	cfg = base()
	cfg.OverlayFromSettings()
	require.Equal(t, "local", cfg.StorageDriver)
	require.Equal(t, "oss-env.aliyuncs.com", cfg.OssEndpoint)
	require.False(t, cfg.CDNEnabled)
}

// TestRestartKeysReportedOnSave 云存储 / CDN 客户端在启动时构造，改完必须重启才生效。
// 接口要点名告知，否则管理员会认为「保存成功但没生效」是 bug。
func TestRestartKeysReportedOnSave(t *testing.T) {
	r := setupSQLiteEngine(t)
	c := newTestClient(r, t)
	adminToken := loginAdmin(t, c)

	// 常规项：不提示重启
	_, body := c.do("PUT", "/api/admin/settings", map[string]string{"brand_name": "甲"}, adminToken)
	require.EqualValues(t, 0, body["code"])
	require.Empty(t, body["data"].(map[string]interface{})["restartKeys"])

	// 需重启项：点名返回
	_, body = c.do("PUT", "/api/admin/settings", map[string]string{"storage_driver": "oss"}, adminToken)
	require.EqualValues(t, 0, body["code"])
	keys := body["data"].(map[string]interface{})["restartKeys"].([]interface{})
	require.Contains(t, keys, "storage_driver")
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

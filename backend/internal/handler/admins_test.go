package handler

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestPlatformAdminAccounts 平台超管「账号管理」接口：
// 列表 / 新增 / 改用户名 / 重置密码 / 删除，以及三条安全护栏。
//
// 背景：初始超管由 SeedAdmin 按 ADMIN_USER 一次性 seed，此前没有改用户名的入口，
// 账号名等于写死在首次启动那一刻——本组接口补上这个缺口。
func TestPlatformAdminAccounts(t *testing.T) {
	r := setupSQLiteEngine(t)
	c := newTestClient(r, t)
	loginAdmin(t, c)
	// 登录校验用独立 client：登录会写 cookie jar，用同一个会把主 client 的身份顶掉
	probe := newTestClient(r, t)

	// 1) 初始只有 seed 出来的那一个平台超管
	_, body := c.doWithCookie("GET", "/api/admin/platform-admins", nil)
	require.EqualValues(t, 0, body["code"])
	list := body["data"].([]interface{})
	require.Len(t, list, 1, "初始应只有一个平台超管")
	selfID := list[0].(map[string]interface{})["id"]

	// 2) 新增平台超管
	_, body = c.doWithCookie("POST", "/api/admin/platform-admins", map[string]interface{}{
		"username": "ops2", "password": "pwd123456",
	})
	require.EqualValues(t, 0, body["code"])
	secondID := body["data"].(map[string]interface{})["id"]

	// 3) 用户名重复 → 拒绝（全表唯一，含企业管理员）
	_, body = c.doWithCookie("POST", "/api/admin/platform-admins", map[string]interface{}{
		"username": "ops2", "password": "pwd123456",
	})
	require.EqualValues(t, 1, body["code"])
	require.Contains(t, body["msg"], "已存在")

	// 4) 改用户名（改自己）→ 新名可登录，且回传 self 供前端刷新顶栏
	_, body = c.doWithCookie("PUT", "/api/admin/platform-admins/"+fmt.Sprint(selfID),
		map[string]interface{}{"username": "superadmin"})
	require.EqualValues(t, 0, body["code"])
	require.Equal(t, true, body["self"], "改自己的用户名时应回传 self=true")
	_, body = probe.do("POST", "/api/admin/login", map[string]interface{}{
		"username": "superadmin", "password": "admin123456",
	}, "")
	require.EqualValues(t, 0, body["code"], "改名后应能用新用户名登录")

	// 5) 改名撞上已存在的用户名 → 拒绝
	_, body = c.doWithCookie("PUT", "/api/admin/platform-admins/"+fmt.Sprint(selfID),
		map[string]interface{}{"username": "ops2"})
	require.EqualValues(t, 1, body["code"])

	// 6) 重置他人密码 → 新密码可登录（安全戳自增会踢掉对方会话，此处只验凭据生效）
	_, body = c.doWithCookie("POST", "/api/admin/platform-admins/"+fmt.Sprint(secondID)+"/password",
		map[string]interface{}{"password": "newpass123"})
	require.EqualValues(t, 0, body["code"])
	require.NotEqual(t, true, body["self"], "重置他人密码不应标记 self")
	_, body = probe.do("POST", "/api/admin/login", map[string]interface{}{
		"username": "ops2", "password": "newpass123",
	}, "")
	require.EqualValues(t, 0, body["code"], "重置后应能用新密码登录")

	// 7) 护栏一：不能删当前登录的账号
	_, body = c.doWithCookie("DELETE", "/api/admin/platform-admins/"+fmt.Sprint(selfID), nil)
	require.EqualValues(t, 1, body["code"])
	require.Contains(t, body["msg"], "当前登录")

	// 8) 可以删同级账号
	_, body = c.doWithCookie("DELETE", "/api/admin/platform-admins/"+fmt.Sprint(secondID), nil)
	require.EqualValues(t, 0, body["code"])
	_, body = c.doWithCookie("GET", "/api/admin/platform-admins", nil)
	require.Len(t, body["data"].([]interface{}), 1, "删除后应只剩一个")

	// 9) 越权：企业管理员不能碰平台超管账号（PlatformAdminMiddleware 兜底）
	_, body = c.doWithCookie("POST", "/api/admin/companies", map[string]interface{}{
		"name": "乙公司", "adminUsername": "boss2", "adminPassword": "pwd123456",
	})
	require.EqualValues(t, 0, body["code"])
	compAdmin := newTestClient(r, t)
	_, body = compAdmin.do("POST", "/api/admin/login", map[string]interface{}{
		"username": "boss2", "password": "pwd123456",
	}, "")
	require.EqualValues(t, 0, body["code"])
	_, body = compAdmin.doWithCookie("GET", "/api/admin/platform-admins", nil)
	require.NotEqual(t, 0, body["code"], "企业管理员不应看到平台超管账号列表")

	// 10) 不得借本接口改到企业管理员头上（id 存在但 company_id≠0 → 按「账号不存在」拒绝）
	_, body = compAdmin.doWithCookie("GET", "/api/admin/me", nil)
	require.EqualValues(t, 0, body["code"])
	bossID := body["data"].(map[string]interface{})["adminId"]
	_, body = c.doWithCookie("PUT", "/api/admin/platform-admins/"+fmt.Sprint(bossID),
		map[string]interface{}{"username": "hacked"})
	require.EqualValues(t, 1, body["code"], "不应能改企业管理员的用户名")
}

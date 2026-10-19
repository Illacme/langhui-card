package handler

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

// 管理端列表分页的契约测试（2026-10-19）：
// 这些列表（员工 / 企业 / 申请 / 超管账号）都是随运营时间单调增长的，
// 拉全表在千级就会拖垮响应体与内存，所以分页必须落在**服务端**——
// 前端拿到全量再切片只是把问题藏起来（网络照样传了全部数据）。
//
// 信封约定见 handler/pagination.go：{ code:0, data:[...], total, page, size }。

// paged 取出分页回包的三个元信息 + 当前页数组，避免每个断言都写一遍类型转换。
func paged(t *testing.T, body map[string]interface{}) (list []interface{}, total, page, size int) {
	t.Helper()
	require.EqualValues(t, 0, body["code"])
	list, _ = body["data"].([]interface{})
	total = int(body["total"].(float64))
	page = int(body["page"].(float64))
	size = int(body["size"].(float64))
	return list, total, page, size
}

// seedEmployees 批量建员工（走真实接口，保证与生产同一条写入路径）。
func seedEmployees(t *testing.T, c *testClient, token string, n int) {
	t.Helper()
	for i := 1; i <= n; i++ {
		_, body := c.do("POST", "/api/admin/employees", map[string]interface{}{
			"name":  fmt.Sprintf("分页员工%02d", i),
			"phone": fmt.Sprintf("139%08d", i),
			"title": "测试岗",
		}, token)
		require.EqualValues(t, 0, body["code"])
	}
}

// TestEmployeeListPaged 员工列表分页：默认 20 / 指定 size / 越界页码夹到末页 /
// size 超上限截断（防止一次拉全量）。
func TestEmployeeListPaged(t *testing.T) {
	r := setupSQLiteEngine(t)
	c := newTestClient(r, t)
	tok := loginAdmin(t, c)
	seedEmployees(t, c, tok, 25)

	// 1) 不带参数：默认每页 20，total 仍是全量 25
	_, body := c.do("GET", "/api/admin/employees", nil, tok)
	list, total, page, size := paged(t, body)
	require.Len(t, list, 20)
	require.Equal(t, 25, total)
	require.Equal(t, 1, page)
	require.Equal(t, defaultPageSize, size)

	// 2) 指定 size：第 3 页只剩 5 条
	_, body = c.do("GET", "/api/admin/employees?page=3&size=10", nil, tok)
	list, total, page, _ = paged(t, body)
	require.Len(t, list, 5)
	require.Equal(t, 25, total)
	require.Equal(t, 3, page)

	// 3) 越界页码夹到末页：删完一页数据后停在原页码不该看到空列表
	_, body = c.do("GET", "/api/admin/employees?page=99&size=10", nil, tok)
	list, _, page, _ = paged(t, body)
	require.Equal(t, 3, page, "越界页码应夹到末页")
	require.Len(t, list, 5)

	// 4) size 上限保护：请求 1000 条也只给 100
	_, body = c.do("GET", "/api/admin/employees?size=1000", nil, tok)
	_, _, _, size = paged(t, body)
	require.Equal(t, maxPageSize, size)

	// 5) 非法参数回落缺省（page=0 / size=abc）
	_, body = c.do("GET", "/api/admin/employees?page=0&size=abc", nil, tok)
	list, _, page, size = paged(t, body)
	require.Equal(t, 1, page)
	require.Equal(t, defaultPageSize, size)
	require.Len(t, list, 20)
}

// TestEmployeeListKeywordAndStatus 关键字与状态筛选必须落在服务端：
// 否则「前端只筛当前页」会出现「搜得到但翻页后没了」的错觉。
func TestEmployeeListKeywordAndStatus(t *testing.T) {
	r := setupSQLiteEngine(t)
	c := newTestClient(r, t)
	tok := loginAdmin(t, c)
	seedEmployees(t, c, tok, 12)

	// 关键字命中姓名
	_, body := c.do("GET", "/api/admin/employees?keyword=分页员工03", nil, tok)
	list, total, _, _ := paged(t, body)
	require.Equal(t, 1, total)
	require.Equal(t, "分页员工03", list[0].(map[string]interface{})["name"])

	// 关键字命中手机（与 PC 后台搜索框的匹配范围一致）
	_, body = c.do("GET", "/api/admin/employees?keyword=13900000007", nil, tok)
	_, total, _, _ = paged(t, body)
	require.Equal(t, 1, total)

	// 关键字无命中 → 空列表而不是报错
	_, body = c.do("GET", "/api/admin/employees?keyword=不存在的人", nil, tok)
	list, total, _, _ = paged(t, body)
	require.Equal(t, 0, total)
	require.Empty(t, list)

	// 状态筛选：禁用一人后 disabled 只剩 1 条、enabled 为 11 条
	_, body = c.do("GET", "/api/admin/employees?keyword=分页员工05", nil, tok)
	list, _, _, _ = paged(t, body)
	staffID := fmt.Sprint(list[0].(map[string]interface{})["staffId"])
	_, body = c.do("POST", "/api/admin/employees/"+staffID+"/disable", nil, tok)
	require.EqualValues(t, 0, body["code"])

	_, body = c.do("GET", "/api/admin/employees?status=disabled", nil, tok)
	_, total, _, _ = paged(t, body)
	require.Equal(t, 1, total)
	_, body = c.do("GET", "/api/admin/employees?status=enabled", nil, tok)
	_, total, _, _ = paged(t, body)
	require.Equal(t, 11, total)

	// staffIds 精确取（名片修改申请页回显员工原值用）：有界集合，不再分页
	_, body = c.do("GET", "/api/admin/employees?staffIds=1,2", nil, tok)
	list, total, _, _ = paged(t, body)
	require.Equal(t, 2, total)
	require.Len(t, list, 2)
}

// TestCompanyListPaged 企业列表分页 + 关键字筛选；顺带钉死「员工/管理员数」
// 在分页后仍然正确（曾经是逐企业 COUNT 的 N+1 写法，改成批量 group by 后语义不变）。
func TestCompanyListPaged(t *testing.T) {
	r := setupSQLiteEngine(t)
	c := newTestClient(r, t)
	tok := loginAdmin(t, c)

	for i := 1; i <= 12; i++ {
		_, body := c.do("POST", "/api/admin/companies", map[string]interface{}{
			"name": fmt.Sprintf("分页企业%02d", i),
		}, tok)
		require.EqualValues(t, 0, body["code"])
	}

	// 朗汇（seed 的 1 号租户）+ 新建 12 家 = 13 家
	_, body := c.do("GET", "/api/admin/companies?size=5", nil, tok)
	list, total, page, size := paged(t, body)
	require.Equal(t, 13, total)
	require.Equal(t, 1, page)
	require.Equal(t, 5, size)
	require.Len(t, list, 5)

	// 末页 3 条
	_, body = c.do("GET", "/api/admin/companies?page=3&size=5", nil, tok)
	list, _, page, _ = paged(t, body)
	require.Equal(t, 3, page)
	require.Len(t, list, 3)

	// 关键字：只命中一家，且员工/管理员计数仍是数字（批量统计没漏字段）
	_, body = c.do("GET", "/api/admin/companies?keyword=分页企业07", nil, tok)
	list, total, _, _ = paged(t, body)
	require.Equal(t, 1, total)
	m := list[0].(map[string]interface{})
	require.Equal(t, "分页企业07", m["name"])
	require.Contains(t, m, "employeeCount")
	require.Contains(t, m, "adminCount")

	// 状态筛选：全部新建企业默认 enabled
	_, body = c.do("GET", "/api/admin/companies?status=enabled", nil, tok)
	_, total, _, _ = paged(t, body)
	require.Equal(t, 13, total)
	_, body = c.do("GET", "/api/admin/companies?status=disabled", nil, tok)
	_, total, _, _ = paged(t, body)
	require.Equal(t, 0, total)
}

// TestPlatformAdminAndApplyListPaged 平台超管列表与两类申请列表同样分页。
func TestPlatformAdminAndApplyListPaged(t *testing.T) {
	r := setupSQLiteEngine(t)
	c := newTestClient(r, t)
	tok := loginAdmin(t, c)

	// 平台超管：初始 1 个（admin）+ 新建 2 个 = 3
	for _, u := range []string{"paged-boss-1", "paged-boss-2"} {
		_, body := c.do("POST", "/api/admin/platform-admins", map[string]interface{}{
			"username": u, "password": "pwd12345",
		}, tok)
		require.EqualValues(t, 0, body["code"])
	}
	_, body := c.do("GET", "/api/admin/platform-admins?size=2", nil, tok)
	list, total, page, _ := paged(t, body)
	require.Equal(t, 3, total)
	require.Equal(t, 1, page)
	require.Len(t, list, 2)
	_, body = c.do("GET", "/api/admin/platform-admins?page=2&size=2", nil, tok)
	list, _, page, _ = paged(t, body)
	require.Equal(t, 2, page)
	require.Len(t, list, 1)

	// 开通申请：公开接口提交 3 条 → 后台分页读
	for i := 1; i <= 3; i++ {
		_, body := c.do("POST", "/api/apply/open", map[string]interface{}{
			"name": fmt.Sprintf("分页申请人%d", i), "phone": fmt.Sprintf("1380000000%d", i),
		}, "")
		require.EqualValues(t, 0, body["code"])
	}
	_, body = c.do("GET", "/api/admin/apply/open?size=2", nil, tok)
	list, total, _, _ = paged(t, body)
	require.Equal(t, 3, total)
	require.Len(t, list, 2)

	// 状态筛选：均为待审核
	_, body = c.do("GET", "/api/admin/apply/open?status=pending", nil, tok)
	_, total, _, _ = paged(t, body)
	require.Equal(t, 3, total)

	// 关键字落在服务端：搜姓名命中 1 条（前端若只筛当前页，翻页后就会「搜不到」）
	_, body = c.do("GET", "/api/admin/apply/open?keyword=分页申请人2", nil, tok)
	list, total, _, _ = paged(t, body)
	require.Equal(t, 1, total)
	require.Equal(t, "分页申请人2", list[0].(map[string]interface{})["name"])

	// 关键字也能命中手机
	_, body = c.do("GET", "/api/admin/apply/open?keyword=13800000003", nil, tok)
	_, total, _, _ = paged(t, body)
	require.Equal(t, 1, total)

	// 名片修改申请同样是分页信封（当前无数据：空数组而不是 null）
	_, body = c.do("GET", "/api/admin/apply/modify", nil, tok)
	list, total, _, _ = paged(t, body)
	require.Equal(t, 0, total)
	require.NotNil(t, list)
}

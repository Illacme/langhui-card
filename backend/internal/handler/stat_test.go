package handler

import (
	"bytes"
	"encoding/csv"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
	"time"

	"weichuan/internal/auth"
	"weichuan/internal/config"
	"weichuan/internal/model"
	"weichuan/internal/storage"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// ===== 统计模块（埋点进水口 + 日期维度 + 导出）专项测试 =====
//
// 覆盖本轮改造的四个面：
//  1. 进水口：/api/stat/track 限流（按 IP）+ staffId 存在校验 + 事件白名单；
//  2. 查询：/api/admin/stats 与 /api/employee/me/stat 的 from/to 日期范围 + 按天趋势；
//  3. 导出：/api/admin/stats/export 的 BOM / 转义 / 公式注入 / 租户隔离；
//  4. 明细：/api/admin/stats/detail 的倒序分页 / 三类筛选 / 租户隔离 / 坏参数拒收。
//
// setupSQLiteEngine 只回引擎，而统计测试需要直写**任意日期**的埋点行
// （track 接口只能写今天），故这里自建一套并回传 db。

// setupStatEngine 建测试引擎并回传 DB 句柄（含 1 号朗汇 + 2 号乙公司两个租户）。
func setupStatEngine(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()

	tmpDB, err := os.CreateTemp("", "weichuan-stat-*.db")
	require.NoError(t, err)
	dbPath := tmpDB.Name()
	require.NoError(t, tmpDB.Close())
	t.Cleanup(func() { os.Remove(dbPath) })

	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(model.All()...))
	require.NoError(t, db.Create(&model.Company{ID: 1, Name: "廊坊市朗汇机电设备有限公司", ShortName: "朗汇机电", Status: "enabled"}).Error)
	require.NoError(t, db.Create(&model.Company{ID: 2, Name: "测试乙公司", ShortName: "乙公司", Status: "enabled"}).Error)

	uploadDir, err := os.MkdirTemp("", "weichuan-upload-")
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(uploadDir) })

	cfg := config.Config{
		UploadDir:     uploadDir,
		UploadBaseURL: "http://127.0.0.1:8080/uploads",
		StorageDriver: "local",
		StoragePublic: true,
		JWTKey:        "test-jwt-key",
		AdminUser:     "admin",
		AdminPass:     "admin123456",
	}
	up := &storage.LocalUploader{
		Dir:      cfg.UploadDir,
		BaseURL:  cfg.UploadBaseURL,
		MaxBytes: 10 * 1024 * 1024,
		AllowExt: storage.DefaultAllowExt,
	}
	return buildEngine(t, db, cfg, up, nil), db
}

// mkStatEmployee 直建一个员工（绕过申请流，测试只关心统计口径）。
func mkStatEmployee(t *testing.T, db *gorm.DB, companyID int64, name, title, phone string) int64 {
	t.Helper()
	e := model.Employee{CompanyID: companyID, Name: name, Title: title, Phone: phone, Status: "enabled"}
	require.NoError(t, db.Create(&e).Error)
	return e.StaffID
}

// mkStatEvent 直写一条指定日期的埋点（track 接口只能写今天）。
func mkStatEvent(t *testing.T, db *gorm.DB, staffID int64, eventType, date string) {
	t.Helper()
	require.NoError(t, db.Create(&model.StatEvent{
		StaffID: staffID, EventType: eventType, StatDate: date,
	}).Error)
}

// exportCSVRecords 打导出接口并解析成 CSV 记录（校验 BOM 后交给标准解析器）。
func exportCSVRecords(t *testing.T, r *gin.Engine, token, query string) [][]string {
	t.Helper()
	req := httptest.NewRequest("GET", "/api/admin/stats/export"+query, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, 200, w.Code)
	require.Contains(t, w.Header().Get("Content-Type"), "text/csv")
	require.Contains(t, w.Header().Get("Content-Disposition"), "attachment")
	raw := w.Body.Bytes()
	require.True(t, bytes.HasPrefix(raw, []byte("\xEF\xBB\xBF")), "CSV 必须带 BOM，否则 Excel 打开中文乱码")
	recs, err := csv.NewReader(bytes.NewReader(raw[3:])).ReadAll()
	require.NoError(t, err)
	return recs
}

// dayOffset 取相对今天的日期串（YYYY-MM-DD）。
func dayOffset(days int) string {
	return time.Now().AddDate(0, 0, days).Format("2006-01-02")
}

// ===== 1. 进水口 =====

// TestTrackRejectsUnknownStaff：不存在的 staffId 不得进库——
// 否则脚本就能给统计表灌幽灵访问量，而按员工汇总照单全收。
func TestTrackRejectsUnknownStaff(t *testing.T) {
	r, db := setupStatEngine(t)
	c := newTestClient(r, t)

	_, body := c.do("POST", "/api/stat/track", map[string]interface{}{
		"staffId": 99999, "eventType": "visit",
	}, "")
	require.EqualValues(t, 1, body["code"])
	require.Equal(t, "员工不存在", body["msg"])

	var n int64
	require.NoError(t, db.Model(&model.StatEvent{}).Count(&n).Error)
	require.Zero(t, n, "被拒的上报不应留下任何行")
}

// TestTrackRejectsUnknownEventType：白名单之外的事件类型不得进库——
// 查询侧只认 visit/call/save，脏值会让总量与分类悄悄对不上。
func TestTrackRejectsUnknownEventType(t *testing.T) {
	r, db := setupStatEngine(t)
	staffID := mkStatEmployee(t, db, 1, "白名单", "测试", "13800000001")
	c := newTestClient(r, t)

	_, body := c.do("POST", "/api/stat/track", map[string]interface{}{
		"staffId": staffID, "eventType": "hack",
	}, "")
	require.EqualValues(t, 1, body["code"])

	var n int64
	require.NoError(t, db.Model(&model.StatEvent{}).Count(&n).Error)
	require.Zero(t, n)
}

// TestTrackRateLimitedByIP：超限请求回 429 且不落库。
// 阈值压到 3（statTrackLimitFn 间接层）——注意必须在**建引擎之前**覆盖：
// 限流器在 New(db) 构造时就把 limitFn 收下了，事后再改变量捕到的是旧函数。
// 测完还原，避免影响同包其他测试。
func TestTrackRateLimitedByIP(t *testing.T) {
	old := statTrackLimitFn
	statTrackLimitFn = func() int { return 3 }
	t.Cleanup(func() { statTrackLimitFn = old })

	r, db := setupStatEngine(t)
	staffID := mkStatEmployee(t, db, 1, "限流", "测试", "13800000002")

	c := newTestClient(r, t)
	for i := 0; i < 3; i++ {
		_, body := c.do("POST", "/api/stat/track", map[string]interface{}{
			"staffId": staffID, "eventType": "visit",
		}, "")
		require.EqualValues(t, 0, body["code"])
	}
	code, body := c.do("POST", "/api/stat/track", map[string]interface{}{
		"staffId": staffID, "eventType": "visit",
	}, "")
	require.Equal(t, 200, code, "限流约定是 HTTP 200 + 信封码，不是 429 状态码")
	require.EqualValues(t, 429, body["code"])

	var n int64
	require.NoError(t, db.Model(&model.StatEvent{}).Count(&n).Error)
	require.EqualValues(t, 3, n, "被限流挡下的请求不应落库")
}

// ===== 2. 日期维度与趋势 =====

// TestAdminStatsDateRangeAndTrend：from/to 只放行范围内事件；趋势按天升序；
// 零数据员工仍补 0；坏参数回 code:1 而不是静默忽略。
func TestAdminStatsDateRangeAndTrend(t *testing.T) {
	r, db := setupStatEngine(t)
	s1 := mkStatEmployee(t, db, 1, "张三", "工程师", "13800000010")
	s2 := mkStatEmployee(t, db, 1, "李四", "销售", "13800000011")
	mkStatEmployee(t, db, 1, "零数据", "实习生", "13800000012") // 全程无事件

	d0, d1, d2 := dayOffset(0), dayOffset(-1), dayOffset(-2)
	mkStatEvent(t, db, s1, "visit", d2)
	mkStatEvent(t, db, s1, "visit", d1)
	mkStatEvent(t, db, s1, "visit", d0)
	mkStatEvent(t, db, s1, "call", d0)
	mkStatEvent(t, db, s2, "visit", d2)
	mkStatEvent(t, db, s2, "save", d0)

	c := newTestClient(r, t)
	admin := loginAdmin(t, c)

	// 不带范围 = 全部时间（向后兼容）：visit4 / call1 / save1
	_, body := c.do("GET", "/api/admin/stats", nil, admin)
	require.EqualValues(t, 0, body["code"])
	stats := body["data"].(map[string]interface{})
	require.EqualValues(t, 4, stats["summary"].(map[string]interface{})["visit"])
	require.EqualValues(t, 1, stats["summary"].(map[string]interface{})["call"])
	require.EqualValues(t, 1, stats["summary"].(map[string]interface{})["save"])
	// 趋势三天齐全且升序
	trend := stats["trend"].([]interface{})
	require.Len(t, trend, 3)
	for i, want := range []string{d2, d1, d0} {
		require.Equal(t, want, trend[i].(map[string]interface{})["date"])
	}
	// 零数据员工出现在列表里（计数 0）
	var zeroFound bool
	for _, it := range stats["byStaff"].([]interface{}) {
		m := it.(map[string]interface{})
		if m["name"] == "零数据" {
			zeroFound = true
			require.EqualValues(t, 0, m["visit"])
		}
	}
	require.True(t, zeroFound, "零数据员工也应出现在统计列表")

	// 范围 [d1, d0]：排除 d2 → visit2（s1 两天 + s2 无）/ call1 / save1
	_, body = c.do("GET", "/api/admin/stats?from="+d1+"&to="+d0, nil, admin)
	require.EqualValues(t, 0, body["code"])
	stats = body["data"].(map[string]interface{})
	require.EqualValues(t, 2, stats["summary"].(map[string]interface{})["visit"])
	require.EqualValues(t, 1, stats["summary"].(map[string]interface{})["call"])
	require.EqualValues(t, 1, stats["summary"].(map[string]interface{})["save"])
	trend = stats["trend"].([]interface{})
	require.Len(t, trend, 2, "范围外的日期不应出现在趋势里")
	require.Equal(t, d1, trend[0].(map[string]interface{})["date"])
	require.Equal(t, d0, trend[1].(map[string]interface{})["date"])

	// 坏参数：from > to / 不存在的日期 / 错格式 —— 都必须 code:1
	for _, q := range []string{"?from=" + d0 + "&to=" + d1, "?from=2026-02-31", "?to=2026-13-01", "?from=20260101"} {
		_, body = c.do("GET", "/api/admin/stats"+q, nil, admin)
		require.EqualValues(t, 1, body["code"], "坏日期参数应被拒绝: %s", q)
	}
}

// TestEmployeeStatDateRange：员工端统计同样支持 from/to，且看不到别人的数。
func TestEmployeeStatDateRange(t *testing.T) {
	r, db := setupStatEngine(t)
	s1 := mkStatEmployee(t, db, 1, "王五", "工程师", "13800000020")
	s2 := mkStatEmployee(t, db, 1, "赵六", "销售", "13800000021")
	hash, err := auth.HashPassword("emp-pass-1")
	require.NoError(t, err)
	require.NoError(t, db.Model(&model.Employee{}).Where("staff_id = ?", s1).Update("password_hash", hash).Error)

	d0, d1 := dayOffset(0), dayOffset(-1)
	mkStatEvent(t, db, s1, "visit", d1)
	mkStatEvent(t, db, s1, "visit", d1)
	mkStatEvent(t, db, s1, "call", d0)
	mkStatEvent(t, db, s2, "visit", d0) // 别人的事件不能算到王五头上

	c := newTestClient(r, t)
	_, body := c.do("POST", "/api/employee/login", map[string]interface{}{
		"phone": "13800000020", "password": "emp-pass-1",
	}, "")
	require.EqualValues(t, 0, body["code"])
	empToken := body["token"].(string)

	// 全部：visit2 / call1 / save0
	_, body = c.do("GET", "/api/employee/me/stat", nil, empToken)
	require.EqualValues(t, 0, body["code"])
	data := body["data"].(map[string]interface{})
	require.EqualValues(t, 2, data["visit"])
	require.EqualValues(t, 1, data["call"])
	require.EqualValues(t, 0, data["save"])

	// 只要今天：visit0 / call1
	_, body = c.do("GET", "/api/employee/me/stat?from="+d0+"&to="+d0, nil, empToken)
	require.EqualValues(t, 0, body["code"])
	data = body["data"].(map[string]interface{})
	require.EqualValues(t, 0, data["visit"])
	require.EqualValues(t, 1, data["call"])

	// 坏日期参数 → code:1
	_, body = c.do("GET", "/api/employee/me/stat?from=2026-02-31", nil, empToken)
	require.EqualValues(t, 1, body["code"])
}

// ===== 3. CSV 导出 =====

// TestStatsExportCSV：BOM / 表头 / 公式注入 / 逗号引号转义 / 文件名。
func TestStatsExportCSV(t *testing.T) {
	r, db := setupStatEngine(t)
	// 姓名带公式注入前缀、职位带逗号与引号——Excel 打开时的两个经典坑
	s1 := mkStatEmployee(t, db, 1, "=cmd|'/c calc'!A1", "销售, 高级\"顾问\"", "13800000030")
	mkStatEvent(t, db, s1, "visit", dayOffset(0))

	c := newTestClient(r, t)
	admin := loginAdmin(t, c)
	recs := exportCSVRecords(t, r, admin, "")

	require.Len(t, recs, 2)
	require.Equal(t, []string{"编号", "姓名", "职位", "访问", "拨号", "保存"}, recs[0])
	// 公式注入被 ' 前缀挡下；逗号/引号经 csv 包裹后原样读回
	require.Equal(t, "'=cmd|'/c calc'!A1", recs[1][1])
	require.Equal(t, "销售, 高级\"顾问\"", recs[1][2])
	require.Equal(t, "1", recs[1][3])
	require.Equal(t, "0", recs[1][4])
	require.Equal(t, "0", recs[1][5])
}

// ===== 4. 明细细列表 =====

// TestStatsDetailPaginationAndFilters：倒序 + 分页信封 + 三类筛选各自生效且可组合；
// stat_date 必须是 YYYY-MM-DD（CAST 归一，MySQL DATE 列直接扫会带 T00:00:00Z）；
// createdAt 形如 "YYYY-MM-DD HH:mm:ss"（服务端格式化，前端不再换算时区）。
func TestStatsDetailPaginationAndFilters(t *testing.T) {
	r, db := setupStatEngine(t)
	s1 := mkStatEmployee(t, db, 1, "张三", "工程师", "13800000050")
	s2 := mkStatEmployee(t, db, 1, "李四", "销售", "13800000051")

	d0, d1 := dayOffset(0), dayOffset(-1)
	// 按 id 递增顺序建：最后一条是 s2 的 save，倒序后应排最前
	mkStatEvent(t, db, s1, "visit", d1)
	mkStatEvent(t, db, s1, "visit", d0)
	mkStatEvent(t, db, s1, "call", d0)
	mkStatEvent(t, db, s2, "save", d0)

	c := newTestClient(r, t)
	admin := loginAdmin(t, c)

	// 缺省参数：全部 4 条，倒序（最新在前），分页元信息齐全
	_, body := c.do("GET", "/api/admin/stats/detail", nil, admin)
	require.EqualValues(t, 0, body["code"])
	require.EqualValues(t, 4, body["total"])
	require.EqualValues(t, 1, body["page"])
	require.EqualValues(t, 20, body["size"], "size 缺省 20（全站约定）")
	list := body["data"].([]interface{})
	require.Len(t, list, 4)
	first := list[0].(map[string]interface{})
	require.EqualValues(t, 4, first["id"], "倒序：最新事件在最前")
	require.Equal(t, "save", first["eventType"])
	require.Equal(t, "李四", first["name"], "姓名由 JOIN 带出")
	require.Equal(t, d0, first["statDate"], "stat_date 必须是 YYYY-MM-DD，不能带 T00:00:00Z")
	require.Regexp(t, `^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$`, first["createdAt"])

	// 分页：size=2&page=2 → 第 2 页恰好是 id 2、3（与第 1 页不重叠）
	_, body = c.do("GET", "/api/admin/stats/detail?size=2&page=2", nil, admin)
	require.EqualValues(t, 0, body["code"])
	require.EqualValues(t, 4, body["total"])
	require.EqualValues(t, 2, body["page"])
	require.EqualValues(t, 2, body["size"])
	list = body["data"].([]interface{})
	require.Len(t, list, 2)
	require.EqualValues(t, 2, list[0].(map[string]interface{})["id"])

	// staffId 筛选：只有 s1 的 3 条
	_, body = c.do("GET", "/api/admin/stats/detail?staffId="+strconv.FormatInt(s1, 10), nil, admin)
	require.EqualValues(t, 0, body["code"])
	require.EqualValues(t, 3, body["total"])
	for _, it := range body["data"].([]interface{}) {
		require.EqualValues(t, s1, it.(map[string]interface{})["staffId"])
	}

	// eventType 筛选：visit 2 条 / call 1 条 / save 1 条
	for et, want := range map[string]int{"visit": 2, "call": 1, "save": 1} {
		_, body = c.do("GET", "/api/admin/stats/detail?eventType="+et, nil, admin)
		require.EqualValues(t, 0, body["code"])
		require.EqualValues(t, want, body["total"], "eventType=%s", et)
	}

	// 日期范围：只要 d0 → 3 条（d1 的 visit 被排除）
	_, body = c.do("GET", "/api/admin/stats/detail?from="+d0+"&to="+d0, nil, admin)
	require.EqualValues(t, 0, body["code"])
	require.EqualValues(t, 3, body["total"])

	// 组合筛选：s1 + visit + d0 → 1 条
	_, body = c.do("GET", "/api/admin/stats/detail?staffId="+strconv.FormatInt(s1, 10)+
		"&eventType=visit&from="+d0+"&to="+d0, nil, admin)
	require.EqualValues(t, 0, body["code"])
	require.EqualValues(t, 1, body["total"])
}

// TestStatsDetailTenantScope：企业管理员只能下钻自家事件；
// 拿别家员工 id 来筛是空列表——不是报错、更不是漏出数据。
func TestStatsDetailTenantScope(t *testing.T) {
	r, db := setupStatEngine(t)
	s1 := mkStatEmployee(t, db, 1, "朗汇员工", "工程师", "13800000060")
	s2 := mkStatEmployee(t, db, 2, "乙公司员工", "销售", "13800000061")
	d := dayOffset(0)
	mkStatEvent(t, db, s1, "visit", d)
	mkStatEvent(t, db, s2, "visit", d)
	mkStatEvent(t, db, s2, "call", d)

	hash, err := auth.HashPassword("comp2pass")
	require.NoError(t, err)
	require.NoError(t, db.Create(&model.Admin{
		CompanyID: 2, Username: "comp2", PasswordHash: hash,
	}).Error)

	plat := newTestClient(r, t)
	platAdmin := loginAdmin(t, plat)

	comp2 := newTestClient(r, t)
	_, body := comp2.do("POST", "/api/admin/login", map[string]interface{}{
		"username": "comp2", "password": "comp2pass",
	}, "")
	require.EqualValues(t, 0, body["code"])
	comp2Token := comp2.cookies["access_token"]
	require.NotEmpty(t, comp2Token)

	// 平台超管：两家 3 条全见
	_, body = plat.do("GET", "/api/admin/stats/detail", nil, platAdmin)
	require.EqualValues(t, 0, body["code"])
	require.EqualValues(t, 3, body["total"])

	// 乙公司管理员：只有自家 2 条
	_, body = comp2.do("GET", "/api/admin/stats/detail", nil, comp2Token)
	require.EqualValues(t, 0, body["code"])
	require.EqualValues(t, 2, body["total"])
	for _, it := range body["data"].([]interface{}) {
		require.Equal(t, "乙公司员工", it.(map[string]interface{})["name"])
	}

	// 拿朗汇员工的 id 来筛：空列表（隔离由员工子查询保证，不靠 JOIN）
	_, body = comp2.do("GET", "/api/admin/stats/detail?staffId="+strconv.FormatInt(s1, 10), nil, comp2Token)
	require.EqualValues(t, 0, body["code"])
	require.EqualValues(t, 0, body["total"])
	require.Len(t, body["data"].([]interface{}), 0)
}

// TestStatsDetailDeletedStaffScope：员工从库里删掉后，其历史事件在明细与汇总中
// 一致地消失。这条测试钉住的是「下钻可对账」约束——明细是汇总的展开，
// 若明细多出行数，运营拿着汇总数逐条找就对不上（当前产品无删员工功能，
// 直改库或未来加删除时都不能破坏这个约束）。
func TestStatsDetailDeletedStaffScope(t *testing.T) {
	r, db := setupStatEngine(t)
	s1 := mkStatEmployee(t, db, 1, "在职", "工程师", "13800000070")
	s2 := mkStatEmployee(t, db, 1, "离职同事", "工程师", "13800000071")
	d := dayOffset(0)
	mkStatEvent(t, db, s1, "visit", d)
	mkStatEvent(t, db, s2, "visit", d)
	mkStatEvent(t, db, s2, "call", d)
	require.NoError(t, db.Delete(&model.Employee{}, s2).Error)

	c := newTestClient(r, t)
	admin := loginAdmin(t, c)

	// 明细：只剩在职员工的 1 条
	_, body := c.do("GET", "/api/admin/stats/detail", nil, admin)
	require.EqualValues(t, 0, body["code"])
	require.EqualValues(t, 1, body["total"])
	require.Equal(t, "在职", body["data"].([]interface{})[0].(map[string]interface{})["name"])

	// 汇总：访问同样只剩 1（visit 原为 2，离职同事那份不计入）
	_, body = c.do("GET", "/api/admin/stats", nil, admin)
	require.EqualValues(t, 0, body["code"])
	require.EqualValues(t, 1, body["data"].(map[string]interface{})["summary"].(map[string]interface{})["visit"])
}

// TestStatsDetailBadParams：空结果信封完整（data 是空数组不是 null）；
// 坏参数一律 code:1——静默忽略会让「筛了却没生效」比不筛更难排查。
func TestStatsDetailBadParams(t *testing.T) {
	r, _ := setupStatEngine(t)
	c := newTestClient(r, t)
	admin := loginAdmin(t, c)

	_, body := c.do("GET", "/api/admin/stats/detail", nil, admin)
	require.EqualValues(t, 0, body["code"])
	require.EqualValues(t, 0, body["total"])
	require.NotNil(t, body["data"], "空结果也要给数组，前端省一层判空")
	require.Len(t, body["data"].([]interface{}), 0)

	for _, q := range []string{
		"?from=2026-02-31",          // 不存在的日期
		"?to=2026-13-01",            // 不存在的月份
		"?from=20260101",            // 错格式
		"?eventType=hack",           // 白名单外的事件类型
		"?staffId=abc",              // 非数字
		"?staffId=0",                // 非正数
		"?staffId=-1",               // 负数
	} {
		_, body = c.do("GET", "/api/admin/stats/detail"+q, nil, admin)
		require.EqualValues(t, 1, body["code"], "坏参数应被拒绝: %s", q)
	}
}

// TestStatsExportTenantScope：企业管理员只能导出自家员工——
// 统计接口按租户隔离，导出是同一份数据的另一个出口，不能成为越权口子。
func TestStatsExportTenantScope(t *testing.T) {
	r, db := setupStatEngine(t)
	s1 := mkStatEmployee(t, db, 1, "朗汇员工", "工程师", "13800000040")
	s2 := mkStatEmployee(t, db, 2, "乙公司员工", "销售", "13800000041")
	d := dayOffset(0)
	mkStatEvent(t, db, s1, "visit", d)
	mkStatEvent(t, db, s2, "visit", d)
	mkStatEvent(t, db, s2, "call", d)

	// 2 号租户的企业管理员直建入库（本测试关注的是隔离，不是开户流程；
	// 开户流程本身由 TestMultiTenantIsolation 覆盖）
	hash, err := auth.HashPassword("comp2pass")
	require.NoError(t, err)
	require.NoError(t, db.Create(&model.Admin{
		CompanyID: 2, Username: "comp2", PasswordHash: hash,
	}).Error)

	plat := newTestClient(r, t)
	platAdmin := loginAdmin(t, plat)

	comp2 := newTestClient(r, t)
	_, body := comp2.do("POST", "/api/admin/login", map[string]interface{}{
		"username": "comp2", "password": "comp2pass",
	}, "")
	require.EqualValues(t, 0, body["code"])
	comp2Token := comp2.cookies["access_token"]
	require.NotEmpty(t, comp2Token)

	// 平台超管：两家员工都在
	recs := exportCSVRecords(t, r, platAdmin, "")
	require.Len(t, recs, 3, "平台超管应看到两个租户的员工")
	names := recs[1][1] + "," + recs[2][1]
	require.Contains(t, names, "朗汇员工")
	require.Contains(t, names, "乙公司员工")

	// 乙公司管理员：只有自家
	recs = exportCSVRecords(t, r, comp2Token, "")
	require.Len(t, recs, 2, "企业管理员只应看到自家员工")
	require.Equal(t, "乙公司员工", recs[1][1])

	// 接口层同样隔离：乙公司管理员查 /api/admin/stats 只汇总自家
	_, body = comp2.do("GET", "/api/admin/stats", nil, comp2Token)
	require.EqualValues(t, 0, body["code"])
	byStaff := body["data"].(map[string]interface{})["byStaff"].([]interface{})
	require.Len(t, byStaff, 1)
	require.Equal(t, "乙公司员工", byStaff[0].(map[string]interface{})["name"])
	require.EqualValues(t, 1, byStaff[0].(map[string]interface{})["visit"])
	require.EqualValues(t, 1, byStaff[0].(map[string]interface{})["call"])
	require.EqualValues(t, 1, body["data"].(map[string]interface{})["summary"].(map[string]interface{})["visit"])
}

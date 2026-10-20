// Package handler 接口集成测试（白盒 HTTP 层）。
//
// 设计说明：
//  1. 通过 handler.New(db) + 各 Register* 函数，用 httptest 把完整 Gin 路由跑起来，
//     覆盖「登录鉴权 / 审核流 / 员工管理 / 企业内容 / 上传存储」等全部 /api/* 接口。
//  2. 默认测试不依赖外部 MySQL：注入一个临时文件的 SQLite 库（纯 Go 驱动 glebarez/sqlite，
//     免 cgo），并用 GORM AutoMigrate 按模型建表。SQLite 把模型里的 JSON 列按 TEXT 存储，
//     与 MySQL JSON 列在读写字符串语义上等价，足以验证 handler 业务逻辑。
//  3. 生产契约一致性：设置 TEST_MYSQL_DSN 后，TestAPIFlowMySQL 会连真实 MySQL、应用
//     migrations/*.sql，再跑与 SQLite 完全相同的断言（runAPIFlow），用于 CI 中的 parity 校验。
//  4. 鉴权中间件约定：未登录/无权限返回的是 HTTP 200 + {code:401|403}，而不是 HTTP 状态
//     码，因此本测试统一断言 body["code"]，不依赖状态码。
//  5. 存储增强的「成功分支」用 fake Presigner/Signer/CDNRefresher 覆盖（见 TestStorageEnhanceSuccess）；
//     本地驱动不支持预签名/签名的「回退分支 code:1」在 runAPIFlow 中覆盖。
//
// 运行：
//
//	cd backend && go test ./internal/handler/ -v
//	TEST_MYSQL_DSN='root:root@tcp(127.0.0.1:3306)/langhui_test?charset=utf8mb4&parseTime=true' go test ./internal/handler/ -run TestAPIFlowMySQL -v
package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"langhui/internal/config"
	"langhui/internal/model"
	"langhui/internal/setting"
	"langhui/internal/storage"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// testClient 对引擎发起一次 HTTP 调用并解析 JSON 响应体。
// 带 cookie jar：累积服务端 Set-Cookie（浏览器式），并记录最近一次响应的 Set-Cookie 供断言。
type testClient struct {
	r              *gin.Engine
	t              *testing.T
	cookies        map[string]string // cookie jar
	lastSetCookies []*http.Cookie    // 最近一次响应的 Set-Cookie
}

func newTestClient(r *gin.Engine, t *testing.T) *testClient {
	return &testClient{r: r, t: t, cookies: map[string]string{}}
}

// do  Bearer 头鉴权调用（token 为空则不带头，也不带 cookie）。
func (c *testClient) do(method, path string, body interface{}, token string) (int, map[string]interface{}) {
	return c.doRaw(method, path, body, token, "")
}

// doWithCookie 携带 jar 中的 cookie（模拟浏览器自动携带），不发送 Bearer——用于验证 Cookie 鉴权路径。
func (c *testClient) doWithCookie(method, path string, body interface{}) (int, map[string]interface{}) {
	var pairs []string
	for k, v := range c.cookies {
		pairs = append(pairs, k+"="+v)
	}
	return c.doRaw(method, path, body, "", strings.Join(pairs, "; "))
}

func (c *testClient) doRaw(method, path string, body interface{}, token, cookieHeader string) (int, map[string]interface{}) {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		require.NoError(c.t, err)
		reader = bytes.NewReader(b)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if cookieHeader != "" {
		req.Header.Set("Cookie", cookieHeader)
	}
	w := httptest.NewRecorder()
	c.r.ServeHTTP(w, req)

	// 累积 cookie：MaxAge<0 表示删除（浏览器行为）
	c.lastSetCookies = w.Result().Cookies()
	for _, ck := range c.lastSetCookies {
		if ck.MaxAge < 0 {
			delete(c.cookies, ck.Name)
		} else {
			c.cookies[ck.Name] = ck.Value
		}
	}

	var m map[string]interface{}
	if w.Body.Len() > 0 {
		require.NoError(c.t, json.Unmarshal(w.Body.Bytes(), &m))
	}
	return w.Code, m
}

// buildEngine 用给定的 db / 配置 / 存储实现装配与生产 main.go 等价的路由。
// 把「建库」与「装配」分离，便于 SQLite（AutoMigrate）与 MySQL（migrations SQL）复用同一套断言。
func buildEngine(t *testing.T, db *gorm.DB, cfg config.Config, up storage.Uploader, refresher storage.CDNRefresher) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	t.Setenv("JWT_KEY", cfg.JWTKey)

	SeedAdmin(db, cfg) // 首启 seed 初始超管

	// 系统设置：seed 缺省值 + 载入内存缓存（测试依赖公开/后台设置接口）
	require.NoError(t, setting.Init(db))

	r := New(db)
	RegisterAuth(r, db, cfg)
	RegisterApply(r, db)
	RegisterContent(r, db)
	RegisterCompanies(r, db)
	RegisterAdmins(r, db)    // 平台超管账号管理（列表 / 新增 / 改用户名 / 重置密码 / 删除）
	RegisterAuditLogs(r, db) // 操作审计日志（只读查询）

	// 静态托管需要目录存在
	require.NoError(t, os.MkdirAll(cfg.UploadDir, 0755))
	r.Static("/uploads", cfg.UploadDir)
	RegisterUpload(r, db, up, refresher)
	RegisterStorage(r, db, up, refresher, cfg)
	RegisterAssets(r, db, up) // 常用素材库（内嵌 Lucide 图标 + 通用封面/LOGO）
	RegisterWechat(r, cfg, up)
	RegisterSettings(r, db, cfg) // 系统设置：GET/PUT /api/admin/settings + GET /api/public/settings

	return r
}

// setupSQLiteEngine 创建临时 SQLite 库 + AutoMigrate + 本地存储，返回引擎。
func setupSQLiteEngine(t *testing.T) *gin.Engine {
	t.Helper()

	tmpDB, err := os.CreateTemp("", "langhui-test-*.db")
	require.NoError(t, err)
	dbPath := tmpDB.Name()
	require.NoError(t, tmpDB.Close())
	t.Cleanup(func() { os.Remove(dbPath) })

	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(model.All()...))
	// 1 号租户（朗汇）：公开接口缺省落点；存量行为不随多租户改造而变
	require.NoError(t, db.Create(&model.Company{ID: 1, Name: "廊坊市朗汇机电设备有限公司", ShortName: "朗汇机电", Status: "enabled"}).Error)

	uploadDir, err := os.MkdirTemp("", "langhui-upload-")
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
	var refresher storage.CDNRefresher
	return buildEngine(t, db, cfg, up, refresher)
}

// loginAdmin 用 seed 的超管登录并返回 access 令牌值。
// PC 后台改为 Cookie 鉴权后，token 不再出现在响应体，而从 Set-Cookie 的 access_token 取。
func loginAdmin(t *testing.T, c *testClient) string {
	_, body := c.do("POST", "/api/admin/login", map[string]interface{}{
		"username": "admin", "password": "admin123456",
	}, "")
	require.EqualValues(t, 0, body["code"])
	tok := c.cookies["access_token"]
	require.NotEmpty(t, tok)
	return tok
}

// runAPIFlow 是 SQLite 与 MySQL parity 共用的核心断言集合。
func runAPIFlow(t *testing.T, c *testClient, adminToken string) {
	r := c.r

	// ===== 1. 公开接口（无需登录） =====
	t.Run("公开接口", func(t *testing.T) {
		code, body := c.do("GET", "/api/achievements", nil, "")
		require.Equal(t, 200, code)
		require.EqualValues(t, 0, body["code"])

		// 尚不存在的员工 → code:1
		_, body = c.do("GET", "/api/card?staffId=1", nil, "")
		require.EqualValues(t, 1, body["code"])

		// 提交开通申请 → code:0
		_, body = c.do("POST", "/api/apply/open", map[string]interface{}{
			"name": "张三", "phone": "13800000001", "email": "z@a.com", "title": "工程师",
		}, "")
		require.EqualValues(t, 0, body["code"])

		// 埋点上报 → code:0
		_, body = c.do("POST", "/api/stat/track", map[string]interface{}{
			"staffId": 1, "eventType": "visit",
		}, "")
		require.EqualValues(t, 0, body["code"])

		// 参数缺失的埋点 → code:1
		_, body = c.do("POST", "/api/stat/track", map[string]interface{}{"staffId": 1}, "")
		require.EqualValues(t, 1, body["code"])
	})

	// ===== 2. 超管登录与鉴权 =====
	t.Run("超管登录与鉴权", func(t *testing.T) {
		_, body := c.do("POST", "/api/admin/login", map[string]interface{}{
			"username": "admin", "password": "admin123456",
		}, "")
		require.EqualValues(t, 0, body["code"])
		require.Empty(t, body["token"])                // token 不再入响应体
		require.NotEmpty(t, c.cookies["access_token"]) // 改为下发 httpOnly Cookie

		// 错误密码 → code:1
		_, body = c.do("POST", "/api/admin/login", map[string]interface{}{
			"username": "admin", "password": "wrong",
		}, "")
		require.EqualValues(t, 1, body["code"])

		// 缺 token → code:401（HTTP 仍为 200）
		code, body := c.do("GET", "/api/admin/employees", nil, "")
		require.Equal(t, 200, code)
		require.EqualValues(t, 401, body["code"])

		// 非法 token → code:401
		_, body = c.do("GET", "/api/admin/employees", nil, "not.a.valid.token")
		require.EqualValues(t, 401, body["code"])
	})

	// ===== 3. 开通申请审核流（通过→自动建员工） =====
	var openApplyID int64
	t.Run("开通申请审核流", func(t *testing.T) {
		_, body := c.do("GET", "/api/admin/apply/open", nil, adminToken)
		require.EqualValues(t, 0, body["code"])
		list := body["data"].([]interface{})
		require.GreaterOrEqual(t, len(list), 1)
		openApplyID = int64(list[0].(map[string]interface{})["id"].(float64))

		_, body = c.do("POST", fmt.Sprintf("/api/admin/apply/open/%d/approve", openApplyID), nil, adminToken)
		require.EqualValues(t, 0, body["code"])
		require.NotEmpty(t, body["initialPassword"])
		require.NotEmpty(t, body["staffId"])

		// 重复审核已处理的申请 → code:1
		_, body = c.do("POST", fmt.Sprintf("/api/admin/apply/open/%d/approve", openApplyID), nil, adminToken)
		require.EqualValues(t, 1, body["code"])

		// 驳回接口对已是 approved 的记录同样返回 code:1
		_, body = c.do("POST", fmt.Sprintf("/api/admin/apply/open/%d/reject", openApplyID),
			map[string]interface{}{"reason": "x"}, adminToken)
		require.EqualValues(t, 1, body["code"])
	})

	// ===== 4. 员工管理 + 员工端自服务 =====
	t.Run("员工管理与员工端", func(t *testing.T) {
		_, body := c.do("POST", "/api/admin/employees", map[string]interface{}{
			"name": "李四", "phone": "13900000002", "email": "l@a.com", "title": "销售",
		}, adminToken)
		require.EqualValues(t, 0, body["code"])
		empID := int64(body["staffId"].(float64))
		require.NotEmpty(t, body["initialPassword"])

		// 创建即落库职位（曾疑「创建员工不保存职位」）：列表回读断言
		_, body = c.do("GET", "/api/admin/employees", nil, adminToken)
		require.EqualValues(t, 0, body["code"])
		var created bool
		for _, it := range body["data"].([]interface{}) {
			m := it.(map[string]interface{})
			if int64(m["staffId"].(float64)) != empID {
				continue
			}
			created = true
			require.Equal(t, "销售", m["title"])
			require.Equal(t, "l@a.com", m["email"])
		}
		require.True(t, created, "新建员工应出现在列表中")

		_, body = c.do("GET", "/api/admin/employees", nil, adminToken)
		require.EqualValues(t, 0, body["code"])

		// 超管编辑员工资料（即时生效，不经员工端审核流）——含座机、手机 2 字段
		_, body = c.do("PUT", fmt.Sprintf("/api/admin/employees/%d", empID), map[string]interface{}{
			"name": "李四四", "title": "高级销售", "phone": "13900000002",
			"phone2": "13900000022", "landline": "021-12345678", "email": "l4@a.com", "avatar": "http://x/a.png",
		}, adminToken)
		require.EqualValues(t, 0, body["code"])

		_, body = c.do("GET", "/api/admin/employees", nil, adminToken)
		require.EqualValues(t, 0, body["code"])
		var found bool
		for _, it := range body["data"].([]interface{}) {
			m := it.(map[string]interface{})
			if int64(m["staffId"].(float64)) != empID {
				continue
			}
			found = true
			require.Equal(t, "李四四", m["name"])
			require.Equal(t, "高级销售", m["title"])
			require.Equal(t, "13900000022", m["phone2"])
			require.Equal(t, "021-12345678", m["landline"])
			require.Equal(t, "l4@a.com", m["email"])
		}
		require.True(t, found, "编辑后的员工应仍在列表中")

		// 手机 2 也要能被搜索命中（服务端关键字范围含 phone2）
		_, body = c.do("GET", "/api/admin/employees?keyword=13900000022", nil, adminToken)
		require.EqualValues(t, 0, body["code"])
		var hit2 bool
		for _, it := range body["data"].([]interface{}) {
			m := it.(map[string]interface{})
			if int64(m["staffId"].(float64)) == empID {
				hit2 = true
			}
		}
		require.True(t, hit2, "按手机 2 搜索应能查到该员工")

		// 编辑不存在的员工 / 姓名为空 → code:1
		_, body = c.do("PUT", "/api/admin/employees/999999", map[string]interface{}{
			"name": "x", "phone": "1",
		}, adminToken)
		require.EqualValues(t, 1, body["code"])
		_, body = c.do("PUT", fmt.Sprintf("/api/admin/employees/%d", empID), map[string]interface{}{
			"name": "", "phone": "13900000002",
		}, adminToken)
		require.EqualValues(t, 1, body["code"])

		// 重置密码 → 返回新初始密码（覆盖创建时的初始密码，登录须用此值）
		_, body = c.do("POST", fmt.Sprintf("/api/admin/employees/%d/reset-password", empID), nil, adminToken)
		require.EqualValues(t, 0, body["code"])
		require.NotEmpty(t, body["initialPassword"])
		resetPwd := body["initialPassword"].(string)

		// 禁用 / 启用
		_, body = c.do("POST", fmt.Sprintf("/api/admin/employees/%d/disable", empID), nil, adminToken)
		require.EqualValues(t, 0, body["code"])
		_, body = c.do("POST", fmt.Sprintf("/api/admin/employees/%d/enable", empID), nil, adminToken)
		require.EqualValues(t, 0, body["code"])

		// 全局名片统计：汇总 + 按员工（前面公开接口已上报过一次 visit）
		_, body = c.do("GET", "/api/admin/stats", nil, adminToken)
		require.EqualValues(t, 0, body["code"])
		stats := body["data"].(map[string]interface{})
		summary := stats["summary"].(map[string]interface{})
		require.GreaterOrEqual(t, int(summary["visit"].(float64)), 1)
		byStaff := stats["byStaff"].([]interface{})
		require.GreaterOrEqual(t, len(byStaff), 1)
		// 按员工列表须含 staffId/name 与三个计数，且总数与汇总一致
		var sumVisit int64
		for _, it := range byStaff {
			m := it.(map[string]interface{})
			require.Contains(t, m, "staffId")
			require.Contains(t, m, "name")
			sumVisit += int64(m["visit"].(float64))
		}
		require.EqualValues(t, summary["visit"], sumVisit)

		// 员工登录（用重置后的密码）
		_, body = c.do("POST", "/api/employee/login", map[string]interface{}{
			"phone": "13900000002", "password": resetPwd,
		}, "")
		require.EqualValues(t, 0, body["code"])
		empToken := body["token"].(string)

		// 员工本人名片 + 统计
		_, body = c.do("GET", "/api/employee/me", nil, empToken)
		require.EqualValues(t, 0, body["code"])
		_, body = c.do("GET", "/api/employee/me/stat", nil, empToken)
		require.EqualValues(t, 0, body["code"])

		// 权限越界：员工 token 调超管接口 → code:403（读与写都拦）
		_, body = c.do("GET", "/api/admin/employees", nil, empToken)
		require.EqualValues(t, 403, body["code"])
		_, body = c.do("PUT", fmt.Sprintf("/api/admin/employees/%d", empID), map[string]interface{}{
			"name": "越权改", "phone": "000",
		}, empToken)
		require.EqualValues(t, 403, body["code"])

		// 员工提交名片修改申请（含座机与手机 2，审核通过后应落到员工资料上）
		_, body = c.do("PUT", "/api/employee/me", map[string]interface{}{
			"title": "高级销售", "landline": "021-87654321", "phone2": "13900000222",
		}, empToken)
		require.EqualValues(t, 0, body["code"])

		// 有待审申请时 /api/employee/me 根级返回 pendingModify=true（编辑页「审核中」标签用）
		_, body = c.do("GET", "/api/employee/me", nil, empToken)
		require.EqualValues(t, 0, body["code"])
		require.Equal(t, true, body["pendingModify"])

		// 超管审核名片修改
		_, body = c.do("GET", "/api/admin/apply/modify", nil, adminToken)
		require.EqualValues(t, 0, body["code"])
		mods := body["data"].([]interface{})
		require.GreaterOrEqual(t, len(mods), 1)
		modID := int64(mods[0].(map[string]interface{})["id"].(float64))
		_, body = c.do("POST", fmt.Sprintf("/api/admin/apply/modify/%d/approve", modID), nil, adminToken)
		require.EqualValues(t, 0, body["code"])

		// 审核处理后 pendingModify 回落 false
		_, body = c.do("GET", "/api/employee/me", nil, empToken)
		require.EqualValues(t, 0, body["code"])
		require.Equal(t, false, body["pendingModify"])

		// 审核通过后快照生效：title / landline / phone2 都写到员工资料
		_, body = c.do("GET", "/api/admin/employees", nil, adminToken)
		require.EqualValues(t, 0, body["code"])
		for _, it := range body["data"].([]interface{}) {
			m := it.(map[string]interface{})
			if int64(m["staffId"].(float64)) != empID {
				continue
			}
			require.Equal(t, "高级销售", m["title"])
			require.Equal(t, "021-87654321", m["landline"])
			require.Equal(t, "13900000222", m["phone2"])
		}
	})

	// ===== 5. 分类与项目强校验（防止孤立数据 / 锁定保护） =====
	t.Run("分类与项目强校验", func(t *testing.T) {
		_, body := c.do("POST", "/api/admin/categories", map[string]interface{}{
			"name": "测试分类", "icon": "ic", "sort": 10, "isLocked": false,
		}, adminToken)
		require.EqualValues(t, 0, body["code"])
		catID := int64(body["data"].(map[string]interface{})["id"].(float64))

		// 在该分类下建项目
		_, body = c.do("POST", "/api/admin/projects", map[string]interface{}{
			"categoryId": catID, "name": "案例A", "summary": "s", "detail": "d",
			"album": []string{"u1"}, "isHomeFeatured": false, "enabled": true, "sort": 1,
		}, adminToken)
		require.EqualValues(t, 0, body["code"])

		// 公开列表接口回填 categoryName（跳转页文档 §4：免前端二次查询）
		_, pubBody := c.do("GET", fmt.Sprintf("/api/projects?categoryId=%d", catID), nil, "")
		require.EqualValues(t, 0, pubBody["code"])
		pubList := pubBody["data"].([]interface{})
		require.NotEmpty(t, pubList)
		pubItem := pubList[0].(map[string]interface{})
		require.Equal(t, "测试分类", pubItem["categoryName"])
		pubID := int64(pubItem["id"].(float64))

		// 公开详情接口含 categoryName + updatedAt（跳转页文档 §2.2）
		_, pubBody = c.do("GET", fmt.Sprintf("/api/projects/%d", pubID), nil, "")
		require.EqualValues(t, 0, pubBody["code"])
		pubDetail := pubBody["data"].(map[string]interface{})
		require.Equal(t, "测试分类", pubDetail["categoryName"])
		require.Contains(t, pubDetail, "updatedAt")
		require.NotEmpty(t, pubDetail["updatedAt"])

		// 分类下有项目 → 删除失败 code:1
		_, body = c.do("DELETE", fmt.Sprintf("/api/admin/categories/%d", catID), nil, adminToken)
		require.EqualValues(t, 1, body["code"])
		require.Contains(t, body["msg"], "项目")

		// 建一个锁定分类 → 删除失败 code:1（锁定保护）
		_, body = c.do("POST", "/api/admin/categories", map[string]interface{}{
			"name": "锁定分类", "isLocked": true,
		}, adminToken)
		lockID := int64(body["data"].(map[string]interface{})["id"].(float64))
		_, body = c.do("DELETE", fmt.Sprintf("/api/admin/categories/%d", lockID), nil, adminToken)
		require.EqualValues(t, 1, body["code"])
		require.Contains(t, body["msg"], "锁定")

		// 清理：先删项目，再删分类（此时无项目且非锁定）→ 成功
		_, body = c.do("GET", fmt.Sprintf("/api/admin/projects?categoryId=%d", catID), nil, adminToken)
		proj := body["data"].([]interface{})[0].(map[string]interface{})
		projID := int64(proj["id"].(float64))
		_, body = c.do("DELETE", fmt.Sprintf("/api/admin/projects/%d", projID), nil, adminToken)
		require.EqualValues(t, 0, body["code"])

		_, body = c.do("DELETE", fmt.Sprintf("/api/admin/categories/%d", catID), nil, adminToken)
		require.EqualValues(t, 0, body["code"])
	})

	// ===== 6. 业务服务 CRUD =====
	t.Run("业务服务 CRUD", func(t *testing.T) {
		_, body := c.do("POST", "/api/admin/services", map[string]interface{}{
			"title": "机电维保", "content": "<p>test</p>", "sort": 1,
		}, adminToken)
		require.EqualValues(t, 0, body["code"])
		sid := int64(body["data"].(map[string]interface{})["id"].(float64))

		_, body = c.do("PUT", fmt.Sprintf("/api/admin/services/%d", sid), map[string]interface{}{
			"title": "机电维保2", "content": "<p>x</p>", "sort": 2,
		}, adminToken)
		require.EqualValues(t, 0, body["code"])

		// 公开详情含 updatedAt（跳转页文档 §3.2）
		_, body = c.do("GET", fmt.Sprintf("/api/services/%d", sid), nil, "")
		require.EqualValues(t, 0, body["code"])
		require.Contains(t, body["data"].(map[string]interface{}), "updatedAt")

		_, body = c.do("DELETE", fmt.Sprintf("/api/admin/services/%d", sid), nil, adminToken)
		require.EqualValues(t, 0, body["code"])
	})

	// ===== 7. 企业信息与首页布局 =====
	t.Run("企业内容与布局", func(t *testing.T) {
		_, body := c.do("PUT", "/api/admin/company-info", map[string]interface{}{
			"fullName": "朗汇机电", "address": "上海", "tel": "021-1", "logo": "l.png",
		}, adminToken)
		require.EqualValues(t, 0, body["code"])
		_, body = c.do("GET", "/api/admin/company-info", nil, adminToken)
		require.EqualValues(t, 0, body["code"])

		_, body = c.do("PUT", "/api/admin/company-profile", map[string]interface{}{
			"introText": "简介", "videoSource": "channels", "videoChannelId": "sph_abc",
			"videoFeedId": "feed123", "videoUrl": "", "videoFallbackUrl": false, "cover": "c.png",
		}, adminToken)
		require.EqualValues(t, 0, body["code"])
		_, body = c.do("GET", "/api/admin/company-profile", nil, adminToken)
		require.EqualValues(t, 0, body["code"])

		_, body = c.do("PUT", "/api/admin/home-layout", map[string]interface{}{
			"order": []string{"service", "achievement"},
		}, adminToken)
		require.EqualValues(t, 0, body["code"])
		_, body = c.do("GET", "/api/admin/home-layout", nil, adminToken)
		require.EqualValues(t, 0, body["code"])

		// 首页背景：写入 → 后台读回 → 公开接口下发
		bgURL := "http://127.0.0.1:8080/uploads/library/bg-deepspace.jpg"
		_, body = c.do("PUT", "/api/admin/home-layout", map[string]interface{}{
			"order": []string{"service", "achievement"}, "bgImage": bgURL, "bgMask": 40,
		}, adminToken)
		require.EqualValues(t, 0, body["code"])
		_, body = c.do("GET", "/api/admin/home-layout", nil, adminToken)
		require.EqualValues(t, 0, body["code"])
		hl := body["data"].(map[string]interface{})
		require.Equal(t, bgURL, hl["bgImage"])
		require.EqualValues(t, 40, hl["bgMask"])

		// 公开接口根级下发（小程序按 order/bgImage/bgMask 读取）
		_, body = c.do("GET", "/api/home-layout", nil, "")
		require.EqualValues(t, 0, body["code"])
		require.Equal(t, bgURL, body["bgImage"])
		require.EqualValues(t, 40, body["bgMask"])
		require.EqualValues(t, []interface{}{"service", "achievement"}, body["order"])

		// 只提交 order（不带 bgImage）时背景保持——指针语义区分「未提供」与「清空」
		_, body = c.do("PUT", "/api/admin/home-layout", map[string]interface{}{
			"order": []string{"achievement", "service"},
		}, adminToken)
		require.EqualValues(t, 0, body["code"])
		_, body = c.do("GET", "/api/admin/home-layout", nil, adminToken)
		require.Equal(t, bgURL, body["data"].(map[string]interface{})["bgImage"], "未提供 bgImage 不应清空")

		// 遮罩浓度钳制到 0~100
		_, body = c.do("PUT", "/api/admin/home-layout", map[string]interface{}{
			"order": []string{"achievement", "service"}, "bgMask": 999,
		}, adminToken)
		require.EqualValues(t, 0, body["code"])
		_, body = c.do("GET", "/api/admin/home-layout", nil, adminToken)
		require.EqualValues(t, 100, body["data"].(map[string]interface{})["bgMask"])

		// 显式传空串即清除背景（恢复灰底布局）
		_, body = c.do("PUT", "/api/admin/home-layout", map[string]interface{}{
			"order": []string{"achievement", "service"}, "bgImage": "", "bgMask": 0,
		}, adminToken)
		require.EqualValues(t, 0, body["code"])
		_, body = c.do("GET", "/api/admin/home-layout", nil, adminToken)
		require.Equal(t, "", body["data"].(map[string]interface{})["bgImage"])
		_, body = c.do("GET", "/api/home-layout", nil, "")
		require.Equal(t, "", body["bgImage"], "清空后公开接口应为空串，小程序据此不铺背景")
	})

	// ===== 8. 文件上传与存储增强（本地驱动回退分支） =====
	t.Run("上传与存储增强", func(t *testing.T) {
		// 构造 multipart/form-data 上传
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		part, err := mw.CreateFormFile("file", "a.png")
		require.NoError(t, err)
		// 写入合法 PNG 文件头（魔法字节），满足上传内容嗅探校验
		_, _ = part.Write([]byte("\x89PNG\r\n\x1a\n"))
		require.NoError(t, mw.Close())

		req := httptest.NewRequest("POST", "/api/admin/upload", &buf)
		req.Header.Set("Content-Type", mw.FormDataContentType())
		req.Header.Set("Authorization", "Bearer "+adminToken)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		require.Equal(t, 200, w.Code)
		var up map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &up))
		require.EqualValues(t, 0, up["code"])
		urls := up["data"].(map[string]interface{})["urls"].([]interface{})
		require.NotEmpty(t, urls)

		// 本地驱动不支持预签名 → code:1（前端回退普通上传）
		_, body := c.do("GET", "/api/admin/upload/presign?ext=png", nil, adminToken)
		require.EqualValues(t, 1, body["code"])

		// CDN 未配置 → code:1
		_, body = c.do("POST", "/api/admin/cdn/refresh", map[string]interface{}{"urls": []string{"x"}}, adminToken)
		require.EqualValues(t, 1, body["code"])

		// 私有签名：本地驱动无 Signer 能力 → code:1
		_, body = c.do("POST", "/api/admin/storage/sign", map[string]interface{}{"urls": []string{"x"}}, adminToken)
		require.EqualValues(t, 1, body["code"])

		// 小程序码未配置 WX_APPID → code:1
		_, body = c.do("GET", "/api/wechat/wxacode?staffId=1", nil, "")
		require.EqualValues(t, 1, body["code"])
	})

	// ===== 9. 超管修改自身密码（同时验证令牌吊销） =====
	t.Run("超管改密与令牌吊销", func(t *testing.T) {
		oldToken := adminToken
		_, body := c.do("PUT", "/api/admin/password", map[string]interface{}{
			"old": "admin123456", "new": "admin123456",
		}, adminToken)
		require.EqualValues(t, 0, body["code"])

		// 改密后旧令牌立即失效（安全戳自增）→ 401
		_, body = c.do("GET", "/api/admin/employees", nil, oldToken)
		require.EqualValues(t, 401, body["code"])

		// 重新登录拿到新令牌（密码未变）
		adminToken = loginAdmin(t, c)

		// 原密码错误 → code:1（用新令牌才能走到密码校验逻辑）
		_, body = c.do("PUT", "/api/admin/password", map[string]interface{}{
			"old": "wrong", "new": "admin123456",
		}, adminToken)
		require.EqualValues(t, 1, body["code"])
	})
}

// TestAPIFlow 默认用 SQLite 跑完整接口流程（无需外部 MySQL）。
func TestAPIFlow(t *testing.T) {
	r := setupSQLiteEngine(t)
	c := newTestClient(r, t)
	adminToken := loginAdmin(t, c)
	runAPIFlow(t, c, adminToken)
}

// TestCompanyProfileVideoSource 钉死「视频多来源」的语义。
// 这几条一旦松动，小程序端就会出现「明明配了却播不出来」的疑难问题——
// 排查时根本看不出当前走的是哪一路，所以必须在服务端把档位收敛成显式枚举。
func TestCompanyProfileVideoSource(t *testing.T) {
	r := setupSQLiteEngine(t)
	c := newTestClient(r, t)
	adminToken := loginAdmin(t, c)

	put := func(m map[string]interface{}) map[string]interface{} {
		_, body := c.do("PUT", "/api/admin/company-profile", m, adminToken)
		return body
	}
	get := func() map[string]interface{} {
		_, body := c.do("GET", "/api/admin/company-profile", nil, adminToken)
		require.EqualValues(t, 0, body["code"])
		return body["data"].(map[string]interface{})
	}

	// 1) 档位是显式枚举，不在枚举内一律拒绝
	require.EqualValues(t, 1, put(map[string]interface{}{"videoSource": "bilibili"})["code"])

	// 2) 腾讯视频档未接线：插件必须在 app.json 静态声明，统一声明会让未通过插件授权的
	//    客户小程序直接报「插件未授权」，故服务端也拦，防前端置灰被绕过
	require.EqualValues(t, 1, put(map[string]interface{}{"videoSource": "tencent", "videoVid": "v001"})["code"])

	// 3) 视频号档：channelId 只作留档，真正定位视频的是 feedId；兜底开关独立
	require.EqualValues(t, 0, put(map[string]interface{}{
		"videoSource": "channels", "videoChannelId": "sph_abc", "videoFeedId": "feed_1",
		"videoFeedToken": "tok_1",
		"videoUrl": "https://cdn.example.com/fallback.mp4", "videoFallbackUrl": true,
	})["code"])
	d := get()
	require.Equal(t, "channels", d["videoSource"])
	require.Equal(t, "sph_abc", d["videoChannelId"])
	require.Equal(t, "feed_1", d["videoFeedId"])
	require.Equal(t, "tok_1", d["videoFeedToken"])
	require.EqualValues(t, true, d["videoFallbackUrl"])

	// 3.1) 只填 feed-token 也必须能保存：平台模式下客户视频号与我们小程序不同主体，
	//      feed-id 官方明确「仅同主体生效」，跨主体只能靠 feed-token
	require.EqualValues(t, 0, put(map[string]interface{}{
		"videoSource": "channels", "videoFeedId": "", "videoFeedToken": "tok_cross",
	})["code"])
	d = get()
	require.Equal(t, "", d["videoFeedId"])
	require.Equal(t, "tok_cross", d["videoFeedToken"])

	// 4) 外部直链档：videoUrl 是主源而非兜底
	require.EqualValues(t, 0, put(map[string]interface{}{
		"videoSource": "url", "videoUrl": "https://cdn.example.com/a.mp4",
	})["code"])
	d = get()
	require.Equal(t, "url", d["videoSource"])
	require.Equal(t, "https://cdn.example.com/a.mp4", d["videoUrl"])

	// 5) 空串归一为 none：该列后加，老客户端 / 脚本可能不带该字段
	require.EqualValues(t, 0, put(map[string]interface{}{"videoSource": ""})["code"])
	require.Equal(t, "none", get()["videoSource"])
}

// TestAPIFlowMySQL 生产契约 parity：连真实 MySQL、应用 migrations/*.sql，再跑同一套断言。
// 本地无 TEST_MYSQL_DSN 时跳过；CI 中用 MySQL service 容器提供 DSN。
func TestAPIFlowMySQL(t *testing.T) {
	dsn := os.Getenv("TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("set TEST_MYSQL_DSN (e.g. root:root@tcp(127.0.0.1:3306)/langhui_test?charset=utf8mb4&parseTime=true) to run MySQL parity")
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	applyMigrations(t, db)

	uploadDir, err := os.MkdirTemp("", "langhui-upload-")
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
	var refresher storage.CDNRefresher
	r := buildEngine(t, db, cfg, up, refresher)
	c := newTestClient(r, t)
	adminToken := loginAdmin(t, c)
	runAPIFlow(t, c, adminToken)
}

// TestMySQLMigrations 只验证「连得上 MySQL + migrations 能建表」，不跑业务断言。
// 用于在 CI 中把「连接 / 建表失败」与「业务逻辑在 MySQL 下不等价」区分开。
func TestMySQLMigrations(t *testing.T) {
	dsn := os.Getenv("TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("set TEST_MYSQL_DSN to run MySQL migrations check")
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.Exec("SELECT 1").Error)

	applyMigrations(t, db)

	// 校验 migrations 确实建出了关键表
	for _, tbl := range []string{"employee", "achievement_category", "home_layout", "project_case", "admin", "open_apply", "card_modify_apply"} {
		var cnt int64
		require.NoError(t, db.Raw(
			"SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = ?", tbl,
		).Scan(&cnt).Error)
		require.EqualValues(t, 1, cnt, "table %s should exist after migrations", tbl)
	}
}

// applyMigrations 读取并顺序执行 migrations 下的建表 SQL（按 ";" 切分，逐条 Exec）。
// 用 runtime.Caller 定位本测试文件所在目录，再回溯到 migrations/，
// 避免依赖 go test 的 CWD（不同调用方式下 CWD 可能不同）。
// 按文件名排序执行全部 *.sql（不再硬编码清单）——新增迁移忘记登记时，
// MySQL parity 会静默缺列并在业务断言处才炸，难以定位；glob 从机制上杜绝漏登记。
func applyMigrations(t *testing.T, db *gorm.DB) {
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok, "runtime.Caller failed")
	migrationsDir := filepath.Join(filepath.Dir(thisFile), "..", "..", "migrations")
	matches, err := filepath.Glob(filepath.Join(migrationsDir, "*.sql"))
	require.NoError(t, err)
	require.NotEmpty(t, matches)
	sort.Strings(matches)
	for _, path := range matches {
		raw, err := os.ReadFile(path)
		require.NoError(t, err)
		for _, stmt := range strings.Split(string(raw), ";") {
			s := strings.TrimSpace(stmt)
			if s == "" {
				continue
			}
			err := db.Exec(s).Error
			if err != nil {
				// 幂等：建表用 IF NOT EXISTS，但 0001 的分类种子是固定主键 INSERT，
				// 重复执行会撞 duplicate entry；ALTER 在列已存在时报
				// "Duplicate column" / 1060。这里忽略「已存在」类错误，使 parity
				// 用例可重复跑（CI 中会被多个用例依次应用）。
				msg := err.Error()
				if strings.Contains(msg, "Duplicate entry") ||
					strings.Contains(msg, "already exists") ||
					strings.Contains(msg, "Duplicate column") ||
					strings.Contains(msg, "1060") {
					continue
				}
				require.NoError(t, err)
			}
		}
	}
}

// ---- 存储增强「成功分支」测试：用 fake 实现覆盖预签名 / 私有签名 / CDN 刷新 ----

// fakeUploader 同时实现 Uploader + Presigner + Signer，用于验证成功路径。
type fakeUploader struct{}

func (fakeUploader) Save(*multipart.FileHeader) (string, error)       { return "http://up/x.png", nil }
func (fakeUploader) SaveBytes(string, []byte, string) (string, error) { return "http://up/x.png", nil }
func (fakeUploader) PresignPut(key, _ string, _ time.Duration) (string, string, error) {
	return "http://put/" + key, "http://final/" + key, nil
}
func (fakeUploader) SignGetURL(key string, _ time.Duration) (string, error) {
	return "http://signed/" + key, nil
}

// fakeRefresher 记录是否被调用。
type fakeRefresher struct{ called bool }

func (f *fakeRefresher) Refresh([]string) error {
	f.called = true
	return nil
}

// TestStorageEnhanceSuccess 验证存储增强在对象存储驱动可用时的成功分支。
func TestStorageEnhanceSuccess(t *testing.T) {
	uploadDir, err := os.MkdirTemp("", "langhui-upload-")
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(uploadDir) })

	cfg := config.Config{
		UploadDir:     uploadDir,
		UploadBaseURL: "http://final", // sign 处理按此前缀反推 key
		StorageDriver: "minio",
		StoragePublic: true,
		JWTKey:        "test-jwt-key",
		AdminUser:     "admin",
		AdminPass:     "admin123456",
	}
	up := fakeUploader{}
	ref := &fakeRefresher{}
	r := buildEngine(t, newSeedDB(t), cfg, up, ref)
	c := newTestClient(r, t)
	adminToken := loginAdmin(t, c)

	// 预签名成功：返回 putUrl / url / key
	_, body := c.do("GET", "/api/admin/upload/presign?ext=png", nil, adminToken)
	require.EqualValues(t, 0, body["code"])
	data := body["data"].(map[string]interface{})
	require.NotEmpty(t, data["putUrl"])
	require.NotEmpty(t, data["url"])
	require.NotEmpty(t, data["key"])

	// 私有签名成功：返回 signed 映射（key 由 BaseURL 前缀反推）
	_, body = c.do("POST", "/api/admin/storage/sign",
		map[string]interface{}{"urls": []string{"http://final/abc.jpg"}}, adminToken)
	require.EqualValues(t, 0, body["code"])
	signed := body["data"].(map[string]interface{})["signed"].(map[string]interface{})
	require.Equal(t, "http://signed/abc.jpg", signed["http://final/abc.jpg"])

	// CDN 刷新成功：refresher 被调用且无错误
	_, body = c.do("POST", "/api/admin/cdn/refresh",
		map[string]interface{}{"urls": []string{"http://final/a.jpg"}}, adminToken)
	require.EqualValues(t, 0, body["code"])
	require.True(t, ref.called)
}

// TestTokenRefresh 验证 refresh 端点（Body 流，小程序端）与令牌吊销：
//  1. 用 refresh 令牌换取新 access，且新 access 可用；
//  2. 垃圾 refresh / 把 access 当 refresh 均被拒绝（类型与签名校验）；
//  3. 员工改密后旧令牌立即失效（安全戳机制）。
//
// 注：PC 超管的 Cookie 流刷新由 TestCookieAuth 覆盖。
func TestTokenRefresh(t *testing.T) {
	r := setupSQLiteEngine(t)
	c := newTestClient(r, t)
	adminToken := loginAdmin(t, c)

	// 建员工并登录，拿到双令牌（员工端 Body 流：token/refreshToken 在响应体）
	_, body := c.do("POST", "/api/admin/employees", map[string]interface{}{
		"name": "王五", "phone": "13700000003", "email": "w@a.com", "title": "经理",
	}, adminToken)
	require.EqualValues(t, 0, body["code"])
	empID := int64(body["staffId"].(float64))
	initPwd := body["initialPassword"].(string)

	_, body = c.do("POST", "/api/employee/login", map[string]interface{}{
		"phone": "13700000003", "password": initPwd,
	}, "")
	require.EqualValues(t, 0, body["code"])
	access := body["token"].(string)
	refresh := body["refreshToken"].(string)
	require.NotEmpty(t, access)
	require.NotEmpty(t, refresh)
	require.EqualValues(t, 900, body["expiresIn"])

	// 用 refresh 换新的 access → 新 access 可访问员工接口
	_, body = c.do("POST", "/api/refresh", map[string]interface{}{"refreshToken": refresh}, "")
	require.EqualValues(t, 0, body["code"])
	newAccess := body["token"].(string)
	require.NotEmpty(t, newAccess)
	_, body = c.do("GET", "/api/employee/me", nil, newAccess)
	require.EqualValues(t, 0, body["code"])

	// 垃圾 refresh → 401
	_, body = c.do("POST", "/api/refresh", map[string]interface{}{"refreshToken": "not-a-token"}, "")
	require.EqualValues(t, 401, body["code"])

	// 把 access 当 refresh 用（类型不符）→ 401
	_, body = c.do("POST", "/api/refresh", map[string]interface{}{"refreshToken": access}, "")
	require.EqualValues(t, 401, body["code"])

	// 员工改密（用初始密码）→ 旧令牌立即失效
	_, body = c.do("PUT", "/api/employee/password", map[string]interface{}{
		"old": initPwd, "new": "newpass1",
	}, access)
	require.EqualValues(t, 0, body["code"])
	_, body = c.do("GET", "/api/employee/me", nil, access)
	require.EqualValues(t, 401, body["code"])

	_ = empID
}

// TestCookieAuth 验证 PC 后台的 httpOnly Cookie 鉴权路径：
//  1. 超管登录下发 access/refresh Cookie（HttpOnly + SameSite=Lax），且 body 不含 token；
//  2. 不带 Bearer、仅带 Cookie 即可访问受保护接口（浏览器自动携带的场景）；
//  3. /api/refresh 走 Cookie 流轮换新 access Cookie；
//  4. /api/logout 清除 Cookie，之后同 jar 访问 → 401。
func TestCookieAuth(t *testing.T) {
	r := setupSQLiteEngine(t)
	c := newTestClient(r, t)

	// 1. 登录：下发 Cookie，body 无 token
	_, body := c.do("POST", "/api/admin/login", map[string]interface{}{
		"username": "admin", "password": "admin123456",
	}, "")
	require.EqualValues(t, 0, body["code"])
	require.Empty(t, body["token"])
	var sawAccess bool
	for _, ck := range c.lastSetCookies {
		if ck.Name == "access_token" {
			sawAccess = true
			require.True(t, ck.HttpOnly, "access_token 必须 HttpOnly")
			require.Equal(t, http.SameSiteLaxMode, ck.SameSite, "SameSite=Lax 缓解 CSRF")
		}
		if ck.Name == "refresh_token" {
			require.True(t, ck.HttpOnly, "refresh_token 必须 HttpOnly")
		}
	}
	require.True(t, sawAccess, "应下发 access_token Cookie")

	// 2. 仅带 Cookie（无 Bearer）访问受保护接口 → 成功
	code, body := c.doWithCookie("GET", "/api/admin/employees", nil)
	require.Equal(t, 200, code)
	require.EqualValues(t, 0, body["code"])

	// 3. /api/refresh Cookie 流：轮换出新 access Cookie（不回传 token）
	_, body = c.doWithCookie("POST", "/api/refresh", nil)
	require.EqualValues(t, 0, body["code"])
	require.Empty(t, body["token"])
	_, body = c.doWithCookie("GET", "/api/admin/employees", nil)
	require.EqualValues(t, 0, body["code"])

	// 4. 登出：清 Cookie，之后同 jar 访问 → 401
	_, body = c.doWithCookie("POST", "/api/logout", nil)
	require.EqualValues(t, 0, body["code"])
	_, ok := c.cookies["access_token"]
	require.False(t, ok, "登出后 access_token 应从 jar 删除")
	_, body = c.doWithCookie("GET", "/api/admin/employees", nil)
	require.EqualValues(t, 401, body["code"])
}

// TestSingleDeviceRevocation 验证单设备精确吊销（会话黑名单，A3）：
//  1. 同一账号登录两次 = 两个独立会话（sid 不同），两个令牌均可用；
//  2. 设备 A 登出（携带其 Bearer）→ 只有 A 的 access 失效；
//  3. A 的 refresh 也失效（登出终结整条令牌链，刷新不换 sid）；
//  4. 设备 B 完全不受影响（与 token_version 账号级吊销的区别所在）。
func TestSingleDeviceRevocation(t *testing.T) {
	r := setupSQLiteEngine(t)
	c := newTestClient(r, t)
	adminToken := loginAdmin(t, c)

	// 建员工（超管创建后需用初始密码登录）
	_, body := c.do("POST", "/api/admin/employees", map[string]interface{}{
		"name": "赵六", "phone": "13600000004", "email": "z@a.com", "title": "技术",
	}, adminToken)
	require.EqualValues(t, 0, body["code"])
	initPwd := body["initialPassword"].(string)

	login := func() (access, refresh string) {
		_, body := c.do("POST", "/api/employee/login", map[string]interface{}{
			"phone": "13600000004", "password": initPwd,
		}, "")
		require.EqualValues(t, 0, body["code"])
		return body["token"].(string), body["refreshToken"].(string)
	}

	// 设备 A / 设备 B 分别登录（两个会话）
	accessA, refreshA := login()
	accessB, refreshB := login()
	require.NotEqual(t, accessA, accessB, "两次登录应签发不同令牌")

	// 1. 两个令牌均可用
	_, body = c.do("GET", "/api/employee/me", nil, accessA)
	require.EqualValues(t, 0, body["code"])
	_, body = c.do("GET", "/api/employee/me", nil, accessB)
	require.EqualValues(t, 0, body["code"])

	// 2. 设备 A 登出 → 只吊销 A 的会话
	_, body = c.do("POST", "/api/logout", nil, accessA)
	require.EqualValues(t, 0, body["code"])
	_, body = c.do("GET", "/api/employee/me", nil, accessA)
	require.EqualValues(t, 401, body["code"], "登出设备的 access 应立即失效")

	// 3. A 的 refresh 同样失效（会话已终结，不能靠刷新绕过登出）
	_, body = c.do("POST", "/api/refresh", map[string]interface{}{"refreshToken": refreshA}, "")
	require.EqualValues(t, 401, body["code"], "登出设备的 refresh 应立即失效")

	// 4. 设备 B 不受影响
	_, body = c.do("GET", "/api/employee/me", nil, accessB)
	require.EqualValues(t, 0, body["code"])

	// B 的 refresh 仍可正常换发（其 sid 未被吊销），且换出的新 access 可用
	_, body = c.do("POST", "/api/refresh", map[string]interface{}{"refreshToken": refreshB}, "")
	require.EqualValues(t, 0, body["code"])
	newAccessB := body["token"].(string)
	require.NotEmpty(t, newAccessB)
	_, body = c.do("GET", "/api/employee/me", nil, newAccessB)
	require.EqualValues(t, 0, body["code"])
}

// newSeedDB 建一个 SQLite 库并 seed 超管，供成功路径测试装配路由。
func newSeedDB(t *testing.T) *gorm.DB {
	tmpDB, err := os.CreateTemp("", "langhui-test-*.db")
	require.NoError(t, err)
	dbPath := tmpDB.Name()
	require.NoError(t, tmpDB.Close())
	t.Cleanup(func() { os.Remove(dbPath) })
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(model.All()...))
	require.NoError(t, db.Create(&model.Company{ID: 1, Name: "廊坊市朗汇机电设备有限公司", ShortName: "朗汇机电", Status: "enabled"}).Error)
	return db
}

// TestHealthzOK 正常 DB 下 /healthz 应返回 200 且声明 db=up。
// 这是 docker-compose backend healthcheck 与 alert_check.sh R1 的依赖：
// 仅当 DB 可达才认为就绪，DB 故障时编排/告警才能识别"进程在但不可用"。
func TestHealthzOK(t *testing.T) {
	db := newSeedDB(t)
	r := New(db)
	code, body := newTestClient(r, t).do("GET", "/healthz", nil, "")
	require.Equal(t, http.StatusOK, code)
	require.Equal(t, "ok", body["status"])
	require.Equal(t, "up", body["db"])
}

// TestHealthzDBDown DB 不可达时 /healthz 应返回 503 且声明 db=down，
// 而不是伪装成健康的 200（旧实现的问题：DB 挂了仍返回 200）。
func TestHealthzDBDown(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close()) // 关闭底层连接，使后续 Exec 失败

	r := New(db)
	code, body := newTestClient(r, t).do("GET", "/healthz", nil, "")
	require.Equal(t, http.StatusServiceUnavailable, code)
	require.Equal(t, "degraded", body["status"])
	require.Equal(t, "down", body["db"])
}

// ===== 多租户第一期：数据隔离 + 两级后台 =====
//
// 覆盖点：
//  1. 平台超管登录 → isPlatform=true、companyId=0；可开通企业（一并建企业管理员）；
//  2. 企业管理员登录 → companyId=企业、isPlatform=false；只能看/改本企业数据；
//  3. 公开内容接口缺省落 1 号租户（朗汇），?companyId= 可切；企业停用即拒；
//  4. 越权防护：企业管理员访问企业管理接口 403、改不到别家分类/员工；
//  5. 申请流按租户归属：默认落朗汇，显式 companyId 落指定企业，审核后员工落到同企业。
func TestMultiTenantIsolation(t *testing.T) {
	r := setupSQLiteEngine(t)
	plat := newTestClient(r, t)

	// ===== 1. 平台超管登录与身份 =====
	_, body := plat.do("POST", "/api/admin/login", map[string]interface{}{
		"username": "admin", "password": "admin123456",
	}, "")
	require.EqualValues(t, 0, body["code"])
	require.EqualValues(t, true, body["isPlatform"])
	require.EqualValues(t, 0, body["companyId"])

	_, body = plat.doWithCookie("GET", "/api/admin/me", nil)
	require.EqualValues(t, 0, body["code"])
	require.EqualValues(t, true, body["data"].(map[string]interface{})["isPlatform"])

	// 平台超管在朗汇（缺省目标租户）建一个分类
	_, body = plat.doWithCookie("POST", "/api/admin/categories", map[string]interface{}{"name": "朗汇分类A"})
	require.EqualValues(t, 0, body["code"])
	langhuiCatID := int64(body["data"].(map[string]interface{})["id"].(float64))
	require.NotZero(t, langhuiCatID)

	// ===== 2. 开通企业 2（一并建企业管理员；简称种子公司信息 + 侧边栏品牌名） =====
	_, body = plat.doWithCookie("POST", "/api/admin/companies", map[string]interface{}{
		"name": "测试乙公司", "shortName": "乙公司简称", "adminUsername": "comp2", "adminPassword": "comp2pass",
	})
	require.EqualValues(t, 0, body["code"])
	comp2ID := int64(body["data"].(map[string]interface{})["id"].(float64))
	require.NotZero(t, comp2ID)
	require.Equal(t, "乙公司简称", body["data"].(map[string]interface{})["shortName"], "创建响应应回显企业简称")

	_, body = plat.doWithCookie("GET", "/api/admin/companies", nil)
	require.EqualValues(t, 0, body["code"])
	comps := body["data"].([]interface{})
	require.Len(t, comps, 2, "应有朗汇 + 乙公司两个租户")
	for _, item := range comps {
		m := item.(map[string]interface{})
		if int64(m["id"].(float64)) == comp2ID {
			require.Equal(t, "乙公司简称", m["shortName"], "企业列表应返回简称")
		}
	}

	// ===== 3. 企业管理员登录与隔离 =====
	comp2 := newTestClient(r, t)
	_, body = comp2.do("POST", "/api/admin/login", map[string]interface{}{
		"username": "comp2", "password": "comp2pass",
	}, "")
	require.EqualValues(t, 0, body["code"])
	require.EqualValues(t, false, body["isPlatform"])
	require.EqualValues(t, comp2ID, body["companyId"])
	require.Equal(t, "测试乙公司", body["companyName"])
	require.Equal(t, "乙公司简称", body["companyShortName"], "登录响应应带企业简称（侧边栏品牌名）")

	// /api/admin/me 同样返回简称（页面刷新 hydration 用）
	_, body = comp2.doWithCookie("GET", "/api/admin/me", nil)
	require.EqualValues(t, 0, body["code"])
	require.Equal(t, "乙公司简称", body["data"].(map[string]interface{})["companyShortName"])

	// 开通时填写的简称种子为公司信息简称的初始值（企业管理员登录后在此基础上完善）
	_, body = comp2.doWithCookie("GET", "/api/admin/company-info", nil)
	require.EqualValues(t, 0, body["code"])
	require.Equal(t, "乙公司简称", body["data"].(map[string]interface{})["shortName"])

	// 企业管理员看不到朗汇的分类（空列表），也看不到企业管理入口
	_, body = comp2.doWithCookie("GET", "/api/admin/categories", nil)
	require.EqualValues(t, 0, body["code"])
	require.Len(t, body["data"].([]interface{}), 0, "企业管理员不应看到别家分类")

	_, body = comp2.doWithCookie("GET", "/api/admin/companies", nil)
	require.EqualValues(t, 403, body["code"], "企业管理员不得访问企业管理接口")

	// 企业管理员在自己的租户里建分类 → 仅自己可见
	_, body = comp2.doWithCookie("POST", "/api/admin/categories", map[string]interface{}{"name": "乙分类"})
	require.EqualValues(t, 0, body["code"])
	_, body = comp2.doWithCookie("GET", "/api/admin/categories", nil)
	list := body["data"].([]interface{})
	require.Len(t, list, 1)
	require.Equal(t, "乙分类", list[0].(map[string]interface{})["name"])

	// 平台超管用 ?companyId=2 能看到乙公司的分类
	_, body = plat.doWithCookie("GET", "/api/admin/categories?companyId=2", nil)
	require.EqualValues(t, 0, body["code"])
	require.Len(t, body["data"].([]interface{}), 1)

	// ===== 4. 公开内容接口：缺省朗汇，?companyId= 可切 =====
	_, body = plat.do("GET", "/api/achievements", nil, "")
	require.EqualValues(t, 0, body["code"])
	cats := body["categories"].([]interface{})
	require.Len(t, cats, 1)
	require.Equal(t, "朗汇分类A", cats[0].(map[string]interface{})["name"])

	_, body = plat.do("GET", "/api/achievements?companyId=2", nil, "")
	require.EqualValues(t, 0, body["code"])
	cats = body["categories"].([]interface{})
	require.Len(t, cats, 1)
	require.Equal(t, "乙分类", cats[0].(map[string]interface{})["name"])

	// 不存在的租户 → 明确拒绝
	_, body = plat.do("GET", "/api/achievements?companyId=99", nil, "")
	require.EqualValues(t, 1, body["code"])

	// 企业公开信息（小程序申请页展示申请目标用）：缺省朗汇，?companyId= 可切
	_, body = plat.do("GET", "/api/company", nil, "")
	require.EqualValues(t, 0, body["code"])
	require.Equal(t, "廊坊市朗汇机电设备有限公司", body["data"].(map[string]interface{})["name"])
	_, body = plat.do("GET", "/api/company?companyId=2", nil, "")
	require.EqualValues(t, 0, body["code"])
	require.Equal(t, "测试乙公司", body["data"].(map[string]interface{})["name"])
	_, body = plat.do("GET", "/api/company?companyId=99", nil, "")
	require.EqualValues(t, 1, body["code"])

	// ===== 5. 越权修改防护 =====
	// 企业管理员改不到朗汇的分类
	_, body = comp2.doWithCookie("PUT", "/api/admin/categories/"+fmt.Sprint(langhuiCatID), map[string]interface{}{"name": "被篡改"})
	require.EqualValues(t, 1, body["code"])
	_, body = plat.doWithCookie("GET", "/api/admin/categories", nil)
	cats = body["data"].([]interface{})
	require.Equal(t, "朗汇分类A", cats[0].(map[string]interface{})["name"], "别家分类不应被改")

	// 平台超管手动建一个朗汇员工；企业管理员禁不掉他
	_, body = plat.doWithCookie("POST", "/api/admin/employees", map[string]interface{}{
		"name": "朗汇员工", "phone": "13900000001", "title": "工程师",
	})
	require.EqualValues(t, 0, body["code"])
	langhuiStaff := int64(body["staffId"].(float64))

	_, body = comp2.doWithCookie("GET", "/api/admin/employees", nil)
	require.Len(t, body["data"].([]interface{}), 0, "企业管理员不应看到别家员工")

	_, body = comp2.doWithCookie("POST", "/api/admin/employees/"+fmt.Sprint(langhuiStaff)+"/disable", nil)
	require.EqualValues(t, 0, body["code"]) // 接口不报错，但按租户条件为空操作
	_, body = plat.doWithCookie("GET", "/api/admin/employees", nil)
	emps := body["data"].([]interface{})
	require.Equal(t, "enabled", emps[0].(map[string]interface{})["status"], "越权禁用不应生效")

	// 平台超管 ?companyId=1 只看朗汇员工
	_, body = plat.doWithCookie("GET", "/api/admin/employees?companyId=1", nil)
	require.Len(t, body["data"].([]interface{}), 1)

	// ===== 6. 申请流按租户归属 =====
	// 默认申请 → 落朗汇；企业管理员看不到
	_, body = plat.do("POST", "/api/apply/open", map[string]interface{}{
		"name": "默认申请人", "phone": "13800000002", "title": "销售",
	}, "")
	require.EqualValues(t, 0, body["code"])
	_, body = comp2.doWithCookie("GET", "/api/admin/apply/open", nil)
	require.Len(t, body["data"].([]interface{}), 0)

	// 显式 companyId=2 的申请 → 企业管理员可见并可审核
	_, body = plat.do("POST", "/api/apply/open", map[string]interface{}{
		"name": "乙公司申请人", "phone": "13800000003", "phone2": "13800000033", "title": "销售", "companyId": comp2ID,
	}, "")
	require.EqualValues(t, 0, body["code"])
	_, body = comp2.doWithCookie("GET", "/api/admin/apply/open", nil)
	applies := body["data"].([]interface{})
	require.Len(t, applies, 1)
	applyID := int64(applies[0].(map[string]interface{})["id"].(float64))

	_, body = comp2.doWithCookie("POST", "/api/admin/apply/open/"+fmt.Sprint(applyID)+"/approve", nil)
	require.EqualValues(t, 0, body["code"])
	newStaff := int64(body["staffId"].(float64))
	require.NotZero(t, newStaff)

	// 审核出的员工落在乙公司：企业管理员列表可见，朗汇列表不可见
	_, body = comp2.doWithCookie("GET", "/api/admin/employees", nil)
	require.Len(t, body["data"].([]interface{}), 1)
	_, body = plat.doWithCookie("GET", "/api/admin/employees?companyId=1", nil)
	require.Len(t, body["data"].([]interface{}), 1, "朗汇仍只有手动建的那个员工")

	// 公开名片按员工归属企业取公司信息（乙公司的 CompanyInfo 为空但可读）；
	// 同时校验申请里填的第二个手机号已随审核落到员工资料（名片联系表第二行要用）
	_, body = plat.do("GET", "/api/card?staffId="+fmt.Sprint(newStaff), nil, "")
	require.EqualValues(t, 0, body["code"])
	require.EqualValues(t, comp2ID, body["company"].(map[string]interface{})["companyId"])
	require.Equal(t, "13800000033", body["employee"].(map[string]interface{})["phone2"])

	// ===== 7. 企业停用：管理员不能登录 =====
	_, body = plat.doWithCookie("POST", "/api/admin/companies/"+fmt.Sprint(comp2ID)+"/status", map[string]interface{}{"status": "disabled"})
	require.EqualValues(t, 0, body["code"])

	_, body = comp2.do("POST", "/api/admin/login", map[string]interface{}{
		"username": "comp2", "password": "comp2pass",
	}, "")
	require.EqualValues(t, 2, body["code"], "企业停用后管理员不能登录")

	// 停用企业的公开内容也被拒
	_, body = plat.do("GET", "/api/achievements?companyId=2", nil, "")
	require.EqualValues(t, 1, body["code"])

	// 恢复启用 → 可再登录
	_, body = plat.doWithCookie("POST", "/api/admin/companies/"+fmt.Sprint(comp2ID)+"/status", map[string]interface{}{"status": "enabled"})
	require.EqualValues(t, 0, body["code"])
	_, body = comp2.do("POST", "/api/admin/login", map[string]interface{}{
		"username": "comp2", "password": "comp2pass",
	}, "")
	require.EqualValues(t, 0, body["code"])

	// ===== 8. 编辑企业（名称/简称）与简称解耦 =====
	_, body = plat.doWithCookie("PUT", "/api/admin/companies/"+fmt.Sprint(comp2ID), map[string]interface{}{
		"name": "测试乙公司改名后", "shortName": "乙司",
	})
	require.EqualValues(t, 0, body["code"])

	_, body = plat.doWithCookie("GET", "/api/admin/companies", nil)
	require.EqualValues(t, 0, body["code"])
	for _, item := range body["data"].([]interface{}) {
		m := item.(map[string]interface{})
		if int64(m["id"].(float64)) == comp2ID {
			require.Equal(t, "测试乙公司改名后", m["name"])
			require.Equal(t, "乙司", m["shortName"], "编辑后列表应返回新简称")
		}
	}

	// 公司信息里的简称不随编辑变化（两个简称相互独立：公司信息简称仅小程序展示）
	_, body = comp2.doWithCookie("GET", "/api/admin/company-info", nil)
	require.EqualValues(t, 0, body["code"])
	require.Equal(t, "乙公司简称", body["data"].(map[string]interface{})["shortName"], "编辑企业不应回写公司信息简称")

	// 企业管理员重新登录：侧边栏品牌名取改后的简称
	_, body = comp2.do("POST", "/api/admin/login", map[string]interface{}{
		"username": "comp2", "password": "comp2pass",
	}, "")
	require.EqualValues(t, 0, body["code"])
	require.Equal(t, "乙司", body["companyShortName"])
}

// TestCompanyShortNameDefault 企业简称缺省值：企业管理里未设置简称（留空/纯空白）时，
// company.short_name 回落「智能名片」（侧边栏品牌名）；公司信息简称不跟随该缺省值
// （留空 → 小程序顶栏展示企业全称），两个简称相互独立。
func TestCompanyShortNameDefault(t *testing.T) {
	r := setupSQLiteEngine(t)
	plat := newTestClient(r, t)

	_, body := plat.do("POST", "/api/admin/login", map[string]interface{}{
		"username": "admin", "password": "admin123456",
	}, "")
	require.EqualValues(t, 0, body["code"])

	// 不填简称开通 → company.short_name 缺省「智能名片」
	_, body = plat.doWithCookie("POST", "/api/admin/companies", map[string]interface{}{
		"name": "缺省简称公司", "adminUsername": "defcomp", "adminPassword": "defpass",
	})
	require.EqualValues(t, 0, body["code"])
	compID := int64(body["data"].(map[string]interface{})["id"].(float64))
	require.Equal(t, "智能名片", body["data"].(map[string]interface{})["shortName"], "未填简称应回落缺省值")

	// 企业管理员登录：companyShortName 取缺省值（侧边栏「智能名片 · 管理后台」）
	defc := newTestClient(r, t)
	_, body = defc.do("POST", "/api/admin/login", map[string]interface{}{
		"username": "defcomp", "password": "defpass",
	}, "")
	require.EqualValues(t, 0, body["code"])
	require.Equal(t, "智能名片", body["companyShortName"])

	// 公司信息简称不跟随缺省值：留空 → 小程序展示企业全称
	_, body = defc.doWithCookie("GET", "/api/admin/company-info", nil)
	require.EqualValues(t, 0, body["code"])
	require.Equal(t, "", body["data"].(map[string]interface{})["shortName"])

	// 编辑时清空（纯空白）简称 → 同样回落缺省值
	_, body = plat.doWithCookie("PUT", "/api/admin/companies/"+fmt.Sprint(compID), map[string]interface{}{
		"name": "缺省简称公司", "shortName": "   ",
	})
	require.EqualValues(t, 0, body["code"])
	_, body = plat.doWithCookie("GET", "/api/admin/companies", nil)
	require.EqualValues(t, 0, body["code"])
	for _, item := range body["data"].([]interface{}) {
		m := item.(map[string]interface{})
		if int64(m["id"].(float64)) == compID {
			require.Equal(t, "智能名片", m["shortName"], "清空简称后应回落缺省值")
		}
	}
}

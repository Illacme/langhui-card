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
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"langhui/internal/config"
	"langhui/internal/model"
	"langhui/internal/storage"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// testClient 对引擎发起一次 HTTP 调用并解析 JSON 响应体。
type testClient struct {
	r *gin.Engine
	t *testing.T
}

func (c *testClient) do(method, path string, body interface{}, token string) (int, map[string]interface{}) {
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
	w := httptest.NewRecorder()
	c.r.ServeHTTP(w, req)

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

	r := New(db)
	RegisterAuth(r, db)
	RegisterApply(r, db)
	RegisterContent(r, db)

	// 静态托管需要目录存在
	require.NoError(t, os.MkdirAll(cfg.UploadDir, 0755))
	r.Static("/uploads", cfg.UploadDir)
	RegisterUpload(r, db, up, refresher)
	RegisterStorage(r, db, up, refresher, cfg)
	RegisterWechat(r, cfg, up)

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
	require.NoError(t, db.AutoMigrate(
		&model.Employee{}, &model.CompanyInfo{}, &model.CompanyProfile{},
		&model.AchievementCategory{}, &model.ProjectCase{}, &model.BizService{},
		&model.HomeLayout{}, &model.StatEvent{}, &model.Admin{},
		&model.OpenApply{}, &model.CardModifyApply{},
	))

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

// loginAdmin 用 seed 的超管登录并返回 token。
func loginAdmin(t *testing.T, c *testClient) string {
	_, body := c.do("POST", "/api/admin/login", map[string]interface{}{
		"username": "admin", "password": "admin123456",
	}, "")
	require.EqualValues(t, 0, body["code"])
	tok, _ := body["token"].(string)
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
		require.NotEmpty(t, body["token"])

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

		_, body = c.do("GET", "/api/admin/employees", nil, adminToken)
		require.EqualValues(t, 0, body["code"])

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

		// 权限越界：员工 token 调超管接口 → code:403
		_, body = c.do("GET", "/api/admin/employees", nil, empToken)
		require.EqualValues(t, 403, body["code"])

		// 员工提交名片修改申请
		_, body = c.do("PUT", "/api/employee/me", map[string]interface{}{"title": "高级销售"}, empToken)
		require.EqualValues(t, 0, body["code"])

		// 超管审核名片修改
		_, body = c.do("GET", "/api/admin/apply/modify", nil, adminToken)
		require.EqualValues(t, 0, body["code"])
		mods := body["data"].([]interface{})
		require.GreaterOrEqual(t, len(mods), 1)
		modID := int64(mods[0].(map[string]interface{})["id"].(float64))
		_, body = c.do("POST", fmt.Sprintf("/api/admin/apply/modify/%d/approve", modID), nil, adminToken)
		require.EqualValues(t, 0, body["code"])
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
			"introText": "简介", "videoChannelId": "gh_abc", "backupMp4Url": "", "cover": "c.png",
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
	c := &testClient{r: r, t: t}
	adminToken := loginAdmin(t, c)
	runAPIFlow(t, c, adminToken)
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
	c := &testClient{r: r, t: t}
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
func applyMigrations(t *testing.T, db *gorm.DB) {
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok, "runtime.Caller failed")
	migrationsDir := filepath.Join(filepath.Dir(thisFile), "..", "..", "migrations")
	for _, f := range []string{"0001_init.sql", "0002_apply.sql", "0003_soft_delete.sql", "0004_token_version.sql"} {
		raw, err := os.ReadFile(filepath.Join(migrationsDir, f))
		require.NoError(t, err)
		for _, stmt := range strings.Split(string(raw), ";") {
			s := strings.TrimSpace(stmt)
			if s == "" {
				continue
			}
			err := db.Exec(s).Error
			if err != nil {
				// 幂等：建表用 IF NOT EXISTS，但 0001 的分类种子是固定主键 INSERT，
				// 重复执行会撞 duplicate entry；0003 的 ALTER 在列已存在时报
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
	c := &testClient{r: r, t: t}
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

// TestTokenRefresh 验证 refresh 端点与令牌吊销：
//  1. 用 refresh 令牌换取新 access，且新 access 可用；
//  2. 垃圾 refresh / 把 access 当 refresh 均被拒绝（类型与签名校验）；
//  3. 员工改密后旧令牌立即失效（安全戳机制）。
func TestTokenRefresh(t *testing.T) {
	r := setupSQLiteEngine(t)
	c := &testClient{r: r, t: t}
	adminToken := loginAdmin(t, c)

	// 超管登录拿到双令牌
	_, body := c.do("POST", "/api/admin/login", map[string]interface{}{
		"username": "admin", "password": "admin123456",
	}, "")
	require.EqualValues(t, 0, body["code"])
	access := body["token"].(string)
	refresh := body["refreshToken"].(string)
	require.NotEmpty(t, access)
	require.NotEmpty(t, refresh)
	require.EqualValues(t, 900, body["expiresIn"])

	// 用 refresh 换新的 access → 新 access 可访问受保护接口
	_, body = c.do("POST", "/api/refresh", map[string]interface{}{"refreshToken": refresh}, "")
	require.EqualValues(t, 0, body["code"])
	newAccess := body["token"].(string)
	require.NotEmpty(t, newAccess)
	_, body = c.do("GET", "/api/admin/employees", nil, newAccess)
	require.EqualValues(t, 0, body["code"])

	// 垃圾 refresh → 401
	_, body = c.do("POST", "/api/refresh", map[string]interface{}{"refreshToken": "not-a-token"}, "")
	require.EqualValues(t, 401, body["code"])

	// 把 access 当 refresh 用（类型不符）→ 401
	_, body = c.do("POST", "/api/refresh", map[string]interface{}{"refreshToken": access}, "")
	require.EqualValues(t, 401, body["code"])

	// 员工改密触发吊销：旧令牌失效
	_, body = c.do("POST", "/api/admin/employees", map[string]interface{}{
		"name": "王五", "phone": "13700000003", "email": "w@a.com", "title": "经理",
	}, adminToken)
	require.EqualValues(t, 0, body["code"])
	empID := int64(body["staffId"].(float64))
	initPwd := body["initialPassword"].(string)

	_, body = c.do("POST", "/api/employee/login", map[string]interface{}{
		"phone": "13700000003", "password": initPwd,
	}, "")
	require.EqualValues(t, 0, body["code"])
	empToken := body["token"].(string)
	require.NotEmpty(t, empToken)

	// 员工改密（用初始密码）→ 旧令牌立即失效
	_, body = c.do("PUT", "/api/employee/password", map[string]interface{}{
		"old": initPwd, "new": "newpass1",
	}, empToken)
	require.EqualValues(t, 0, body["code"])
	_, body = c.do("GET", "/api/employee/me", nil, empToken)
	require.EqualValues(t, 401, body["code"])

	_ = empID
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
	require.NoError(t, db.AutoMigrate(
		&model.Employee{}, &model.CompanyInfo{}, &model.CompanyProfile{},
		&model.AchievementCategory{}, &model.ProjectCase{}, &model.BizService{},
		&model.HomeLayout{}, &model.StatEvent{}, &model.Admin{},
		&model.OpenApply{}, &model.CardModifyApply{},
	))
	return db
}

package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"weichuan/internal/auth"
	"weichuan/internal/dbopen"
	"weichuan/internal/model"
	"weichuan/internal/repository"
)

// 鉴权中间件是后台所有写接口的门。这里把它的每一条拒绝路径都钉一遍：
// 漏掉任何一条，都是「本该 401 的请求被放行」。

const testKey = "middleware-test-key-0123456789abcdef"

func newAuthDB(t *testing.T) *gorm.DB {
	t.Helper()
	t.Setenv("JWT_KEY", testKey)
	db, _, err := dbopen.Open(filepath.Join(t.TempDir(), "authmw.db"))
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Admin{}, &model.Employee{}, &model.RevokedSession{}))

	// 平台超管 id=1（vid=2）；企业管理员 id=2 归属租户 5；员工 1001 归属租户 5
	a := model.Admin{Username: "root", TokenVersion: 2, CompanyID: 0}
	require.NoError(t, db.Create(&a).Error)
	require.NoError(t, db.Create(&model.Admin{Username: "tenant-admin", TokenVersion: 1, CompanyID: 5}).Error)
	require.NoError(t, db.Create(&model.Employee{StaffID: 1001, Name: "张伟", CompanyID: 5, TokenVersion: 3}).Error)
	require.EqualValues(t, 1, a.ID)
	return db
}

// hit 用给定中间件跑一次请求，返回 HTTP 码与响应信封里的 code
func hit(t *testing.T, db *gorm.DB, mw gin.HandlerFunc, req *http.Request) (int, float64, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(mw)
	r.GET("/probe", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "ok",
			"staffId": c.GetInt64("staffId"), "role": c.GetString("role"), "companyId": c.GetInt64("companyId")})
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var body struct {
		Code float64 `json:"code"`
		Msg  string  `json:"msg"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	return w.Code, body.Code, body.Msg
}

func bearer(token string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/probe", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	return req
}

// ---- 放行 ----

func TestAuthMiddlewarePassesAndInjectsContext(t *testing.T) {
	db := newAuthDB(t)
	access, _, _, err := auth.IssueTokens(1, "admin", 2, 0) // vid 与库中一致
	require.NoError(t, err)

	code, envCode, _ := hit(t, db, AuthMiddleware(db, "admin"), bearer(access))
	require.Equal(t, http.StatusOK, code)
	require.EqualValues(t, 0, envCode)
}

// 不指定角色时登录即可（员工也能过），但上下文必须带上租户，
// 否则业务层做不了「只能改本企业数据」的隔离。
func TestAuthMiddlewareAnyRoleInjectsTenant(t *testing.T) {
	db := newAuthDB(t)
	access, _, _, err := auth.IssueTokens(1001, "employee", 3, 5)
	require.NoError(t, err)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(AuthMiddleware(db, ""))
	r.GET("/probe", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"code": 0, "companyId": c.GetInt64("companyId"), "role": c.GetString("role")})
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, bearer(access))
	require.Contains(t, w.Body.String(), `"companyId":5`)
	require.Contains(t, w.Body.String(), `"role":"employee"`)
}

// ---- 拒绝 ----

func TestAuthMiddlewareRejectsMissingToken(t *testing.T) {
	db := newAuthDB(t)
	_, envCode, msg := hit(t, db, AuthMiddleware(db, "admin"), httptest.NewRequest(http.MethodGet, "/probe", nil))
	require.EqualValues(t, 401, envCode)
	require.Contains(t, msg, "未登录")
}

func TestAuthMiddlewareRejectsInvalidToken(t *testing.T) {
	db := newAuthDB(t)
	_, envCode, msg := hit(t, db, AuthMiddleware(db, "admin"), bearer("garbage.token.here"))
	require.EqualValues(t, 401, envCode)
	require.Contains(t, msg, "登录失效")
}

// refresh 令牌不能当访问令牌用：它寿命长、且语义上只该出现在 /api/refresh。
func TestAuthMiddlewareRejectsRefreshToken(t *testing.T) {
	db := newAuthDB(t)
	_, refresh, _, err := auth.IssueTokens(1, "admin", 2, 0)
	require.NoError(t, err)

	_, envCode, msg := hit(t, db, AuthMiddleware(db, "admin"), bearer(refresh))
	require.EqualValues(t, 401, envCode)
	require.Contains(t, msg, "令牌类型错误")
}

// 改密 / 禁用后 token_version 自增，旧令牌必须立即失效（账号级吊销）
func TestAuthMiddlewareRejectsStaleTokenVersion(t *testing.T) {
	db := newAuthDB(t)
	access, _, _, err := auth.IssueTokens(1, "admin", 2, 0)
	require.NoError(t, err)
	require.NoError(t, db.Model(&model.Admin{}).Where("id = 1").Update("token_version", 3).Error)

	_, envCode, msg := hit(t, db, AuthMiddleware(db, "admin"), bearer(access))
	require.EqualValues(t, 401, envCode)
	require.Contains(t, msg, "登录已失效")
}

// 登出把 sid 写黑名单后，该会话的令牌必须失效（单设备精确吊销）
func TestAuthMiddlewareRejectsRevokedSession(t *testing.T) {
	db := newAuthDB(t)
	access, _, _, err := auth.IssueTokens(1, "admin", 2, 0)
	require.NoError(t, err)
	claims, err := auth.ParseToken(access)
	require.NoError(t, err)
	require.NoError(t, repository.RevokeSession(db, claims.SID, "admin", 1, time.Now().Add(time.Hour)))

	_, envCode, msg := hit(t, db, AuthMiddleware(db, "admin"), bearer(access))
	require.EqualValues(t, 401, envCode)
	require.Contains(t, msg, "登录已失效")
}

func TestAuthMiddlewareRejectsWrongRole(t *testing.T) {
	db := newAuthDB(t)
	access, _, _, err := auth.IssueTokens(1001, "employee", 3, 5)
	require.NoError(t, err)

	_, envCode, msg := hit(t, db, AuthMiddleware(db, "admin"), bearer(access))
	require.EqualValues(t, 403, envCode)
	require.Contains(t, msg, "无权限")
}

// 企业管理员（role=admin 但 company_id≠0）不得进平台级接口
func TestPlatformAdminMiddlewareRejectsTenantAdmin(t *testing.T) {
	db := newAuthDB(t)
	access, _, _, err := auth.IssueTokens(2, "admin", 1, 5)
	require.NoError(t, err)

	_, envCode, msg := hit(t, db, PlatformAdminMiddleware(db), bearer(access))
	require.EqualValues(t, 403, envCode)
	require.Contains(t, msg, "仅平台超级管理员")

	plat, _, _, err := auth.IssueTokens(1, "admin", 2, 0)
	require.NoError(t, err)
	_, envCode, _ = hit(t, db, PlatformAdminMiddleware(db), bearer(plat))
	require.EqualValues(t, 0, envCode)
}

// ---- 令牌来源 ----

func TestExtractTokenPrefersBearerThenCookie(t *testing.T) {
	gin.SetMode(gin.TestMode)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Request.Header.Set("Authorization", "Bearer header-token")
	c.Request.Header.Set("Cookie", "access_token=cookie-token")
	require.Equal(t, "header-token", ExtractToken(c), "Authorization 头优先")

	c2, _ := gin.CreateTestContext(httptest.NewRecorder())
	c2.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c2.Request.Header.Set("Cookie", "access_token=cookie-token")
	require.Equal(t, "cookie-token", ExtractToken(c2))

	// 非 Bearer 前缀的头不算（避免把 Basic/OAuth 其它 scheme 当 JWT 解）
	c3, _ := gin.CreateTestContext(httptest.NewRecorder())
	c3.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c3.Request.Header.Set("Authorization", "Basic abc")
	require.Empty(t, ExtractToken(c3))

	c4, _ := gin.CreateTestContext(httptest.NewRecorder())
	c4.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	require.Empty(t, ExtractToken(c4))
}

// Cookie 通道也要能放行：PC 后台正是走 httpOnly Cookie（JS 读不到，防 XSS 窃取）
func TestAuthMiddlewareAcceptsCookie(t *testing.T) {
	db := newAuthDB(t)
	access, _, _, err := auth.IssueTokens(1, "admin", 2, 0)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/probe", nil)
	req.Header.Set("Cookie", "access_token="+access)
	_, envCode, _ := hit(t, db, AuthMiddleware(db, "admin"), req)
	require.EqualValues(t, 0, envCode)
}

// ---- TokenVersion ----

func TestTokenVersionByRole(t *testing.T) {
	db := newAuthDB(t)
	v, err := TokenVersion(db, "admin", 1)
	require.NoError(t, err)
	require.Equal(t, 2, v)

	v, err = TokenVersion(db, "employee", 1001)
	require.NoError(t, err)
	require.Equal(t, 3, v)

	// 用户不存在必须报错：取不到版本时中间件按「失效」处理，
	// 若这里返回 0 而令牌 vid 也是 0，就等于放行了一个已不存在的用户
	_, err = TokenVersion(db, "admin", 999)
	require.Error(t, err)
}

func TestCurrentCompanyIDAndPlatformAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set("role", "admin")
	c.Set("companyId", int64(0))
	require.True(t, IsPlatformAdmin(c))
	require.EqualValues(t, 0, CurrentCompanyID(c))

	c.Set("companyId", int64(5))
	require.False(t, IsPlatformAdmin(c))
	require.EqualValues(t, 5, CurrentCompanyID(c))
}

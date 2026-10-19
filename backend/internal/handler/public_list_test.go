package handler

import (
	"fmt"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"langhui/internal/model"
	"langhui/internal/repository"

	"github.com/glebarez/sqlite"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// C 端内容型列表的容量收口（2026-10-19）：
//
// 这三个接口（/api/projects、/api/services、/api/achievements 的 featured）
// 端上是「一次全量展示」、没有分页 UI，所以不做分页，但要防住两件事：
//  1. payload 无上限膨胀——案例 detail / 服务 content 都是富文本，单条可达几十 KB，
//     列表页只画封面 + 标题 + 摘要，详情由 /xxx/:id 单独取，不该跟着列表走；
//  2. 条数无天花板——内容随运营时间增长，必须有一个夹断点。
//
// 本文件把这两条钉死，同时守住两个「别改坏了」的底线：详情接口仍返回正文、
// 列表的软删除与分类名回填在列裁剪后仍然生效。

// setupPublicEngine 建临时库 + 造量级数据（案例、服务均超过各自上限），返回引擎与库。
func setupPublicEngine(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	tmpDB, err := os.CreateTemp("", "langhui-public-*.db")
	require.NoError(t, err)
	dbPath := tmpDB.Name()
	require.NoError(t, tmpDB.Close())
	t.Cleanup(func() { os.Remove(dbPath) })

	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.Company{}, &model.AchievementCategory{}, &model.ProjectCase{}, &model.BizService{},
	))
	// 1 号租户（朗汇）：公开接口缺省落点
	require.NoError(t, db.Create(&model.Company{ID: 1, Name: "朗汇机电", Status: "enabled"}).Error)
	require.NoError(t, db.Create(&model.AchievementCategory{ID: 1, CompanyID: 1, Name: "机房工程"}).Error)

	// 富文本正文：故意造大，列表若带着它，响应体会明显不一样
	longDetail := strings.Repeat("<p>工程正文内容</p>", 200)
	total := repository.MaxProjectsList + 50 // 超过上限，验证会被夹断
	for i := 1; i <= total; i++ {
		require.NoError(t, db.Create(&model.ProjectCase{
			CompanyID:      1,
			CategoryID:     1,
			Name:           fmt.Sprintf("案例%d", i),
			Summary:        fmt.Sprintf("摘要%d", i),
			Detail:         longDetail,
			Cover:          fmt.Sprintf("/uploads/cover%d.png", i),
			IsHomeFeatured: i <= repository.MaxFeaturedList+5, // 超过精选上限
			Enabled:        true,
			Sort:           i,
		}).Error)
	}
	for i := 1; i <= repository.MaxServicesList+10; i++ {
		require.NoError(t, db.Create(&model.BizService{
			CompanyID: 1,
			Title:     fmt.Sprintf("服务%d", i),
			Icon:      fmt.Sprintf("/uploads/icon%d.png", i),
			Content:   longDetail,
			Sort:      i,
		}).Error)
	}
	return New(db), db
}

func TestPublicListCapacity(t *testing.T) {
	r, db := setupPublicEngine(t)
	c := newTestClient(r, t)

	// 1) 案例列表：条数夹到上限，且不带富文本正文
	_, body := c.do("GET", "/api/projects", nil, "")
	require.Equal(t, float64(0), body["code"])
	list := body["data"].([]interface{})
	require.Len(t, list, repository.MaxProjectsList, "案例列表应被夹到上限")
	for _, it := range list {
		m := it.(map[string]interface{})
		require.Empty(t, m["detail"], "列表不应携带富文本 detail（详情由 /api/projects/:id 单独取）")
		require.NotEmpty(t, m["name"])
		require.NotEmpty(t, m["cover"], "列表需要的封面字段不能丢")
		// 列裁剪后分类名回填仍要生效（依赖 select 里保留 category_id）
		require.Equal(t, "机房工程", m["categoryName"], "裁剪列不能破坏分类名回填")
	}

	// 2) ?limit= 生效且越界夹到上限
	_, body = c.do("GET", "/api/projects?limit=5", nil, "")
	require.Len(t, body["data"].([]interface{}), 5)
	_, body = c.do("GET", "/api/projects?limit=99999", nil, "")
	require.Len(t, body["data"].([]interface{}), repository.MaxProjectsList, "超限应夹到上限而非全量")
	_, body = c.do("GET", "/api/projects?limit=0", nil, "")
	require.Len(t, body["data"].([]interface{}), repository.MaxProjectsList, "非法值应回落默认量级")

	// 3) 服务列表：同样夹上限 + 不带富文本正文
	_, body = c.do("GET", "/api/services", nil, "")
	svcs := body["data"].([]interface{})
	require.Len(t, svcs, repository.MaxServicesList)
	for _, it := range svcs {
		m := it.(map[string]interface{})
		require.Empty(t, m["content"], "列表不应携带服务正文 content")
		require.NotEmpty(t, m["title"])
		require.NotEmpty(t, m["icon"])
	}

	// 4) 首页精选：夹到首页画得下的量级，且不带正文
	_, body = c.do("GET", "/api/achievements", nil, "")
	feat := body["featured"].([]interface{})
	require.Len(t, feat, repository.MaxFeaturedList)
	for _, it := range feat {
		require.Empty(t, it.(map[string]interface{})["detail"], "首页精选不应携带富文本")
	}
	require.NotEmpty(t, body["categories"].([]interface{}), "分类列表不受精选上限影响")

	// 5) 详情接口仍返回正文：裁剪只作用于列表，不能误伤详情
	var first model.ProjectCase
	require.NoError(t, db.Order("sort asc").First(&first).Error)
	_, body = c.do("GET", fmt.Sprintf("/api/projects/%d", first.ID), nil, "")
	require.Equal(t, float64(0), body["code"])
	require.NotEmpty(t, body["data"].(map[string]interface{})["detail"], "详情必须返回富文本正文")

	// 6) 软删除仍然生效：列裁剪不能把 deleted_at IS NULL 条件弄丢
	require.NoError(t, db.Delete(&first).Error)
	_, body = c.do("GET", "/api/projects?limit=5", nil, "")
	for _, it := range body["data"].([]interface{}) {
		require.NotEqual(t, float64(first.ID), it.(map[string]interface{})["id"], "已删除案例不应出现在列表里")
	}
}

// 缓存头只给成功响应：错误响应被缓存会把故障延长到 TTL 之后。
func TestPublicListCacheHeader(t *testing.T) {
	r, _ := setupPublicEngine(t)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/api/projects", nil))
	require.Contains(t, w.Header().Get("Cache-Control"), "max-age=60")

	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/api/services", nil))
	require.Contains(t, w.Header().Get("Cache-Control"), "max-age=60")

	// 企业不存在 → 走失败分支，不该带缓存头
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/api/projects?companyId=9999", nil))
	require.Empty(t, w.Header().Get("Cache-Control"), "错误响应不应被缓存")
}

package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestAssetLibrary 常用素材库：清单 / 公开预览图 / 采用（复制进上传存储）/
// 未登录拒绝 / 未知素材 / 路径穿越防护
func TestAssetLibrary(t *testing.T) {
	r := setupSQLiteEngine(t)
	plat := newTestClient(r, t)

	_, body := plat.do("POST", "/api/admin/login", map[string]interface{}{
		"username": "admin", "password": "admin123456",
	}, "")
	require.EqualValues(t, 0, body["code"])

	// 1. 未登录拿不到清单（走 Cookie 鉴权，匿名请求 401）
	anon := newTestClient(r, t)
	_, body = anon.do("GET", "/api/admin/assets/library", nil, "")
	require.EqualValues(t, 401, body["code"])

	// 2. 清单（默认第一页）：五个分类 + 分页元信息
	_, body = plat.doWithCookie("GET", "/api/admin/assets/library", nil)
	require.EqualValues(t, 0, body["code"])
	data := body["data"].(map[string]interface{})
	require.Len(t, data["categories"].([]interface{}), 5, "图标/封面/LOGO/头像/背景 五个分类")
	items := data["items"].([]interface{})
	total := int(data["total"].(float64))
	require.GreaterOrEqual(t, total, 200, "素材应覆盖五大类两百项以上")
	require.EqualValues(t, 1, data["page"])
	require.EqualValues(t, 24, data["size"], "缺省每页 24 条")
	require.EqualValues(t, 24, len(items), "第一页按 size 截断")
	pages := int(data["pages"].(float64))
	require.GreaterOrEqual(t, pages, 8, "两百项素材应有多页")

	// 2.1 翻页：第 2 页换一批素材，且不与第 1 页重复
	_, body = plat.doWithCookie("GET", "/api/admin/assets/library?page=2", nil)
	require.EqualValues(t, 0, body["code"])
	page2 := body["data"].(map[string]interface{})
	require.EqualValues(t, 2, page2["page"])
	require.EqualValues(t, total, page2["total"], "total 为过滤后的总条数，不随页码变化")
	firstIDs := map[string]bool{}
	for _, it := range items {
		firstIDs[it.(map[string]interface{})["id"].(string)] = true
	}
	dup := 0
	for _, it := range page2["items"].([]interface{}) {
		if firstIDs[it.(map[string]interface{})["id"].(string)] {
			dup++
		}
	}
	require.Zero(t, dup, "第 2 页不应与第 1 页重复")

	// 2.2 size 生效（且上限 60 生效：请求 999 只回落 60）
	_, body = plat.doWithCookie("GET", "/api/admin/assets/library?size=5", nil)
	require.EqualValues(t, 5, len(body["data"].(map[string]interface{})["items"].([]interface{})))
	_, body = plat.doWithCookie("GET", "/api/admin/assets/library?size=999", nil)
	require.EqualValues(t, 60, body["data"].(map[string]interface{})["size"])

	// 2.3 非法页码不报错：page=0 → 1；超界页码被夹到末页
	_, body = plat.doWithCookie("GET", "/api/admin/assets/library?page=0", nil)
	require.EqualValues(t, 0, body["code"])
	require.EqualValues(t, 1, body["data"].(map[string]interface{})["page"])
	_, body = plat.doWithCookie("GET", "/api/admin/assets/library?page=9999", nil)
	require.EqualValues(t, pages, body["data"].(map[string]interface{})["page"], "超界页码夹到末页")
	require.NotEmpty(t, body["data"].(map[string]interface{})["items"], "末页仍应有内容")

	// 2.4 分类过滤：只返回背景（竖幅 750×1334，含深浅两色系），total 随之收窄
	_, body = plat.doWithCookie("GET", "/api/admin/assets/library?type=background", nil)
	require.EqualValues(t, 0, body["code"])
	bgData := body["data"].(map[string]interface{})
	bgItems := bgData["items"].([]interface{})
	bgTotal := int(bgData["total"].(float64))
	require.GreaterOrEqual(t, bgTotal, 30, "背景分类应覆盖三十款")
	require.Less(t, bgTotal, total, "分类过滤后总数应小于全量")
	for _, it := range bgItems {
		m := it.(map[string]interface{})
		require.Equal(t, "background", m["category"])
		require.EqualValues(t, 750, m["width"])
		require.EqualValues(t, 1334, m["height"])
	}

	// 2.5 关键字过滤：命中名称或标签，且翻页前重置到第 1 页
	_, body = plat.doWithCookie("GET", "/api/admin/assets/library?keyword=电梯", nil)
	require.EqualValues(t, 0, body["code"])
	kwData := body["data"].(map[string]interface{})
	kwItems := kwData["items"].([]interface{})
	require.EqualValues(t, 1, kwData["total"])
	require.Equal(t, "icon-arrow-up-down", kwItems[0].(map[string]interface{})["id"])
	// 英文标签同样命中（大小写不敏感）
	_, body = plat.doWithCookie("GET", "/api/admin/assets/library?keyword=ELEVATOR", nil)
	require.EqualValues(t, 1, body["data"].(map[string]interface{})["total"])
	// 背景分类 + 关键字组合过滤：分类内再按关键字收窄
	_, body = plat.doWithCookie("GET", "/api/admin/assets/library?type=background&keyword=金", nil)
	require.EqualValues(t, 0, body["code"])
	combo := body["data"].(map[string]interface{})
	require.GreaterOrEqual(t, int(combo["total"].(float64)), 1, "背景内应有命中「金」的素材")
	require.LessOrEqual(t, int(combo["total"].(float64)), bgTotal, "组合过滤不应超出分类总数")
	for _, it := range combo["items"].([]interface{}) {
		require.Equal(t, "background", it.(map[string]interface{})["category"])
	}
	// 无命中：空列表且 total=0，不应报错
	_, body = plat.doWithCookie("GET", "/api/admin/assets/library?keyword=zzz-not-exist", nil)
	require.EqualValues(t, 0, body["code"])
	require.EqualValues(t, 0, body["data"].(map[string]interface{})["total"])
	require.Empty(t, body["data"].(map[string]interface{})["items"])

	// 3. 采用图标：URL 落进上传存储（/uploads/library/...），静态托管可取到真实 PNG
	_, body = plat.doWithCookie("POST", "/api/admin/assets/adopt", map[string]interface{}{"id": "icon-house"})
	require.EqualValues(t, 0, body["code"])
	adopted := body["data"].(map[string]interface{})["url"].(string)
	require.True(t, strings.HasSuffix(adopted, "/uploads/library/icon-house.png"), "采用 URL: %s", adopted)

	req := httptest.NewRequest("GET", "/uploads/library/icon-house.png", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "image/png", w.Header().Get("Content-Type"))
	require.Greater(t, w.Body.Len(), 500, "采用的素材应真实落盘")

	// 4. 预览图：公开可取（无登录），带长缓存 + immutable
	req = httptest.NewRequest("GET", "/api/assets/file/icons/icon-house.png", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "image/png", w.Header().Get("Content-Type"))
	require.Contains(t, w.Header().Get("Cache-Control"), "immutable")
	require.Greater(t, w.Body.Len(), 500)

	// 5. 封面缩略图同样公开可取（JPEG）
	req = httptest.NewRequest("GET", "/api/assets/file/thumbs/covers/cover-skyline.jpg", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "image/jpeg", w.Header().Get("Content-Type"))

	// 5.1 首页背景：竖幅缩略图公开可取，采用后落地为 JPEG（与手动上传同构）
	req = httptest.NewRequest("GET", "/api/assets/file/thumbs/backgrounds/bg-deepspace.jpg", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "image/jpeg", w.Header().Get("Content-Type"))

	_, body = plat.doWithCookie("POST", "/api/admin/assets/adopt", map[string]interface{}{"id": "bg-deepspace"})
	require.EqualValues(t, 0, body["code"])
	bgURL := body["data"].(map[string]interface{})["url"].(string)
	require.True(t, strings.HasSuffix(bgURL, "/uploads/library/bg-deepspace.jpg"), "背景采用 URL: %s", bgURL)

	// 6. 异常分支：未知素材 / 空参数 / 路径穿越 / 不存在文件
	_, body = plat.doWithCookie("POST", "/api/admin/assets/adopt", map[string]interface{}{"id": "not-exist"})
	require.EqualValues(t, 1, body["code"])
	_, body = plat.doWithCookie("POST", "/api/admin/assets/adopt", map[string]interface{}{})
	require.EqualValues(t, 1, body["code"])
	for _, p := range []string{
		"/api/assets/file/",
		"/api/assets/file/thumbs/covers/nope.jpg",
		"/api/assets/file/..%2f..%2fgo.mod",
		"/api/assets/file/icons/../../manifest.json",
	} {
		req = httptest.NewRequest("GET", p, nil)
		w = httptest.NewRecorder()
		r.ServeHTTP(w, req)
		require.NotEqual(t, http.StatusOK, w.Code, p)
	}
}

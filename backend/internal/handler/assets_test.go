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

	// 2. 清单：图标/封面/LOGO/头像 四个分类 + 数十项素材
	_, body = plat.doWithCookie("GET", "/api/admin/assets/library", nil)
	require.EqualValues(t, 0, body["code"])
	data := body["data"].(map[string]interface{})
	require.Len(t, data["categories"].([]interface{}), 4, "图标/封面/LOGO/头像 四个分类")
	items := data["items"].([]interface{})
	require.GreaterOrEqual(t, len(items), 60, "素材应覆盖四大类数十项")

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

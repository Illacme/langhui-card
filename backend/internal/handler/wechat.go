package handler

import (
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"langhui/internal/config"
	"langhui/internal/storage"
	"langhui/internal/wechat"

	"github.com/gin-gonic/gin"
)

// 小程序码内存缓存：避免每次请求都重新调微信 + 重新写对象存储。
// expAt 表示「二维码内容」的过期时间（到达后重新调微信生成）；私有桶下每次返回前会单独签发短期 GET URL。
var (
	wxacodeMu    sync.Mutex
	wxacodeCache = map[int64]wxacodeEntry{}
)

type wxacodeEntry struct {
	key       string    // 对象键，如 wxacode/12.jpg
	publicURL string    // 公开形态 URL（BaseURL/key），公开桶或本地私有兜底用
	expAt     time.Time // 二维码内容过期时间（按 cfg.WxacodeTTL）
}

// RegisterWechat 小程序码接口：按 staffId 生成（写入当前存储驱动）小程序码，返回可访问 URL
// 海报合成放在小程序端 canvas；后端只负责产出小程序码图片，存到 local/minio/oss 后返回访问 URL。
// 私有桶（STORAGE_PUBLIC=false）下，返回的是限时签名 GET URL，避免把对象设为公开读。
func RegisterWechat(r *gin.Engine, cfg config.Config, up storage.Uploader) {
	if cfg.WXAppID == "" || cfg.WXAppSecret == "" {
		r.GET("/api/wechat/wxacode", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "未配置 WX_APPID / WX_APPSECRET"})
		})
		return
	}
	wc := wechat.Config{AppID: cfg.WXAppID, AppSecret: cfg.WXAppSecret}

	r.GET("/api/wechat/wxacode", func(c *gin.Context) {
		staffID, _ := strconv.ParseInt(c.Query("staffId"), 10, 64)
		if staffID <= 0 {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "缺少 staffId"})
			return
		}
		page := c.Query("page")
		if page == "" {
			page = "pages/index/index"
		}
		scene := "staffId=" + strconv.FormatInt(staffID, 10)

		// 1) 命中缓存且二维码内容未过期：直接取（私有桶下当场签发短期 URL）
		wxacodeMu.Lock()
		e, ok := wxacodeCache[staffID]
		wxacodeMu.Unlock()
		if ok && time.Now().Before(e.expAt) {
			url, err := resolveWxacodeURL(e, cfg, up)
			if err != nil {
				c.JSON(http.StatusOK, gin.H{"code": 1, "msg": err.Error()})
				return
			}
			c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"url": url}})
			return
		}

		// 2) 重新生成：调微信 -> 写入当前存储驱动 -> 缓存
		b, err := wechat.GetWXACodeUnlimit(wc, scene, page)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": err.Error()})
			return
		}
		key := fmt.Sprintf("wxacode/%d.jpg", staffID)
		publicURL, err := up.SaveBytes(key, b, "image/jpeg")
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": err.Error()})
			return
		}
		e = wxacodeEntry{key: key, publicURL: publicURL, expAt: time.Now().Add(cfg.WxacodeTTL)}
		wxacodeMu.Lock()
		wxacodeCache[staffID] = e
		wxacodeMu.Unlock()

		url, err := resolveWxacodeURL(e, cfg, up)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"url": url}})
	})
}

// resolveWxacodeURL 公开桶返回公开 URL；私有桶（且存储支持签名）返回限时 GET URL（每次重签，1 小时有效）。
// 本地驱动无 Signer：静态路由已托管，回退公开 URL。
func resolveWxacodeURL(e wxacodeEntry, cfg config.Config, up storage.Uploader) (string, error) {
	if cfg.StoragePublic {
		return e.publicURL, nil
	}
	if s, ok := up.(storage.Signer); ok {
		return s.SignGetURL(e.key, time.Hour)
	}
	return e.publicURL, nil
}

package handler

import (
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"langhui/internal/config"
	"langhui/internal/middleware"
	"langhui/internal/storage"

	"github.com/gin-gonic/gin"
)

// RegisterStorage 注册存储增强接口：直传预签名 / 私有桶限时签名 / CDN 刷新 / 本地私有文件流
func RegisterStorage(r *gin.Engine, up storage.Uploader, refresher storage.CDNRefresher, cfg config.Config) {
	// 直传预签名（超管 / 员工）：客户端拿 putUrl 后直接 PUT 到对象存储
	r.GET("/api/admin/upload/presign", middleware.AuthMiddleware("admin"), presignHandler(up))
	r.GET("/api/employee/upload/presign", middleware.AuthMiddleware("employee"), presignHandler(up))
	// 私有桶：已存 URL → 限时签名 URL（超管 / 员工）
	r.POST("/api/admin/storage/sign", middleware.AuthMiddleware("admin"), signHandler(up, cfg))
	r.POST("/api/employee/storage/sign", middleware.AuthMiddleware("employee"), signHandler(up, cfg))
	// CDN 刷新（超管）
	r.POST("/api/admin/cdn/refresh", middleware.AuthMiddleware("admin"), cdnRefreshHandler(refresher))
	// 本地私有文件流（仅本地驱动 + 私有模式：文件不通过 /uploads 静态公开，需鉴权后读取）
	if local, ok := up.(*storage.LocalUploader); ok && !cfg.StoragePublic {
		r.GET("/api/storage/file", middleware.AuthMiddleware("admin"), localFileHandler(local))
		r.GET("/api/employee/storage/file", middleware.AuthMiddleware("employee"), localFileHandler(local))
	}
}

// presignHandler 返回预签名 PUT URL + 最终可访问 URL；本地驱动不支持时返回 code=1（前端回退普通上传）
func presignHandler(up storage.Uploader) gin.HandlerFunc {
	return func(c *gin.Context) {
		p, ok := up.(storage.Presigner)
		if !ok {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "当前存储模式不支持直传预签名，已回退普通上传"})
			return
		}
		ext := strings.TrimPrefix(c.Query("ext"), ".")
		contentType := c.Query("contentType")
		key := storage.RandName(ext)
		putURL, finalURL, err := p.PresignPut(key, contentType, 15*time.Minute)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"putUrl": putURL, "url": finalURL, "key": key}})
	}
}

// signHandler 私有桶场景：把已存的公网 URL 换成限时签名 URL（key 由 BaseURL 前缀反推）
func signHandler(up storage.Uploader, cfg config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		s, ok := up.(storage.Signer)
		if !ok {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "当前存储模式不支持限时签名访问（仅对象存储私有桶支持）"})
			return
		}
		var req struct {
			Urls []string `json:"urls"`
		}
		if c.ShouldBindJSON(&req) != nil || len(req.Urls) == 0 {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "参数错误"})
			return
		}
		base := strings.TrimRight(cfg.UploadBaseURL, "/") + "/"
		out := make(map[string]string, len(req.Urls))
		for _, u := range req.Urls {
			key := strings.TrimPrefix(u, base)
			if key == "" || key == u {
				c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "无法解析对象 key: " + u})
				return
			}
			signed, err := s.SignGetURL(key, 30*time.Minute)
			if err != nil {
				c.JSON(http.StatusOK, gin.H{"code": 1, "msg": err.Error()})
				return
			}
			out[u] = signed
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"signed": out}})
	}
}

// cdnRefreshHandler 触发 CDN 缓存刷新（覆盖写后使旧缓存失效）
func cdnRefreshHandler(rf storage.CDNRefresher) gin.HandlerFunc {
	return func(c *gin.Context) {
		if rf == nil {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "CDN 刷新未配置（需设置 CDN_ENABLED=1 与 CDN_ACCESS_KEY_*）"})
			return
		}
		var req struct {
			Urls []string `json:"urls"`
		}
		if c.ShouldBindJSON(&req) != nil || len(req.Urls) == 0 {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "参数错误"})
			return
		}
		if err := rf.Refresh(req.Urls); err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "ok"})
	}
}

// localFileHandler 本地私有文件流：鉴权后按 key 读取，防目录穿越
func localFileHandler(l *storage.LocalUploader) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := c.Query("key")
		if key == "" || strings.Contains(key, "..") {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "参数错误"})
			return
		}
		fp := filepath.Join(l.Dir, key)
		abs, err := filepath.Abs(fp)
		if err != nil || !strings.HasPrefix(abs, filepath.Clean(l.Dir)) {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "非法路径"})
			return
		}
		c.File(fp)
	}
}

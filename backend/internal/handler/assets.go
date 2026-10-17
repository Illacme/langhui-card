package handler

import (
	"net/http"
	"path"
	"strings"

	"langhui/internal/assets"
	"langhui/internal/middleware"
	"langhui/internal/storage"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// RegisterAssets 注册「常用开源素材库」接口：
//
//	GET  /api/assets/file/*path    素材预览图。公开 + 长缓存：素材是通用图形、
//	                                不含租户数据；<img> 标签也无法带 Authorization 头，
//	                                公开可避免任何部署形态（同源/跨域/CDN）下的预览问题。
//	GET  /api/admin/assets/library 素材清单（管理员登录态）
//	POST /api/admin/assets/adopt   采用素材：把内嵌素材复制进上传存储，返回与普通
//	                                上传完全同构的 URL（管理员登录态）
//
// 采用（adopt）而非直接引用素材库 URL 的原因：产物落进上传存储后，
// URL / CDN / 下游消费（小程序 <image>）与普通上传零差异，不引入新的依赖域。
func RegisterAssets(r *gin.Engine, db *gorm.DB, up storage.Uploader) {
	r.GET("/api/assets/file/*path", assetFileHandler)
	r.GET("/api/admin/assets/library", middleware.AuthMiddleware(db, "admin"), libraryListHandler)
	r.POST("/api/admin/assets/adopt", middleware.AuthMiddleware(db, "admin"), assetAdoptHandler(up))
}

// assetFileHandler 提供素材预览图（选择器缩略图 + 原图）。
func assetFileHandler(c *gin.Context) {
	rel := strings.TrimPrefix(c.Param("path"), "/")
	// 双保险：显式拒绝穿越路径（assets.ReadFile 与 io/fs 语义本身也会拒）
	if rel == "" || path.IsAbs(rel) || strings.Contains(rel, "..") {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": "not found"})
		return
	}
	data, err := assets.ReadFile(rel)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": "not found"})
		return
	}
	// 素材不可变：长缓存 + immutable，CDN 边缘可长期持有
	c.Header("Cache-Control", "public, max-age=31536000, immutable")
	c.Data(http.StatusOK, assets.ContentType(rel), data)
}

// libraryListHandler 返回素材清单（分类 + 全部素材项）。
func libraryListHandler(c *gin.Context) {
	m, err := assets.Load()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "素材库加载失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{
		"version":    m.Version,
		"categories": m.Categories,
		"items":      m.Items,
	}})
}

// assetAdoptHandler 采用素材：复制进上传存储并返回 URL。
func assetAdoptHandler(up storage.Uploader) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			ID string `json:"id"`
		}
		if c.ShouldBindJSON(&req) != nil || req.ID == "" {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "参数错误"})
			return
		}
		item, err := assets.Find(req.ID)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "素材不存在"})
			return
		}
		data, err := assets.ReadFile(item.File)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "素材读取失败"})
			return
		}
		// 采用路径含素材 ID：重复采用同一素材为幂等覆盖（内容不变，还省存储）
		rel := "library/" + item.ID + item.File[strings.LastIndex(item.File, "."):]
		url, err := up.SaveBytes(rel, data, assets.ContentType(item.File))
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "采用失败：" + err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{
			"url": url,
			"item": gin.H{
				"id": item.ID, "name": item.Name, "category": item.Category,
			},
		}})
	}
}

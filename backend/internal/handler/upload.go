package handler

import (
	"log"
	"net/http"

	"langhui/internal/middleware"
	"langhui/internal/storage"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// RegisterUpload 注册文件上传接口：超管(/api/admin/upload) 与 员工(/api/employee/upload)
// 返回可访问 URL，填回封面/相册/头像/LOGO/员工头像等。
// 存储实现通过 storage.Uploader 注入：本地磁盘（默认）/ MinIO / OSS 可互换，业务代码不变。
// refresher 可选：上传后 best-effort 触发 CDN 刷新（覆盖写使边缘旧缓存失效）。
func RegisterUpload(r *gin.Engine, db *gorm.DB, up storage.Uploader, refresher storage.CDNRefresher) {
	h := uploadHandler(up, refresher)
	r.POST("/api/admin/upload", middleware.AuthMiddleware(db, "admin"), h)
	r.POST("/api/employee/upload", middleware.AuthMiddleware(db, "employee"), h)
}

func uploadHandler(up storage.Uploader, refresher storage.CDNRefresher) gin.HandlerFunc {
	return func(c *gin.Context) {
		form, err := c.MultipartForm()
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "参数错误"})
			return
		}
		files := form.File["file"]
		if len(files) == 0 {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "未选择文件"})
			return
		}
		urls := make([]string, 0, len(files))
		for _, fh := range files {
			u, err := up.Save(fh)
			if err != nil {
				c.JSON(http.StatusOK, gin.H{"code": 1, "msg": err.Error()})
				return
			}
			urls = append(urls, u)
		}
		// 上传后触发 CDN 刷新（覆盖写场景使旧缓存失效），失败仅记录，不影响本次上传结果
		if refresher != nil {
			if err := refresher.Refresh(urls); err != nil {
				log.Printf("cdn refresh after upload failed: %v", err)
			}
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"urls": urls}})
	}
}

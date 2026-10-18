package handler

import (
	"net/http"
	"sort"

	"langhui/internal/middleware"
	"langhui/internal/setting"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// RegisterSettings 注册系统设置接口：
//   - GET  /api/public/settings  公开：仅返回非敏感项（小程序 / 前端引导用）
//   - GET  /api/admin/settings   平台超管读取全部可配置项（按分组）
//   - PUT  /api/admin/settings   平台超管批量更新配置（按类型校验，即时生效）
//
// 密钥类（云存储 / 微信 AppSecret / JWT / DB）始终留环境变量，不进本表（见 internal/setting）。
func RegisterSettings(r *gin.Engine, db *gorm.DB) {
	r.GET("/api/public/settings", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": setting.Public()})
	})

	plat := middleware.PlatformAdminMiddleware(db)
	r.GET("/api/admin/settings", plat, func(c *gin.Context) {
		items := setting.All()
		sort.Slice(items, func(i, j int) bool {
			if items[i].Group != items[j].Group {
				return items[i].Group < items[j].Group
			}
			return items[i].Key < items[j].Key
		})
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": items})
	})

	r.PUT("/api/admin/settings", plat, func(c *gin.Context) {
		var req map[string]string
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "参数错误"})
			return
		}
		if len(req) == 0 {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "无更新项"})
			return
		}
		// 逐条校验 + 写入；任何一项失败立即返回提示（Set 内部已即时刷新内存）
		for k, v := range req {
			if err := setting.Upsert(k, v); err != nil {
				c.JSON(http.StatusOK, gin.H{"code": 1, "msg": err.Error()})
				return
			}
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "保存成功"})
	})
}

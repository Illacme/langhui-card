package handler

import (
	"net/http"
	"strconv"

	"langhui/internal/middleware"
	"langhui/internal/model"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// RegisterAuditLogs 平台超管查询操作审计日志（GET /api/admin/audit-logs）。
//
// 只提供**只读查询**：审计记录一旦可改就等于没有审计，所以本组不提供任何写接口。
// 支持按 action / actor / result 过滤与按时间倒序分页，默认最近 20 条。
func RegisterAuditLogs(r *gin.Engine, db *gorm.DB) {
	plat := middleware.PlatformAdminMiddleware(db)

	r.GET("/api/admin/audit-logs", plat, func(c *gin.Context) {
		page := atoiDefault(c.Query("page"), 1)
		size := atoiDefault(c.Query("size"), 20)
		if size > 100 {
			size = 100 // 上限保护：避免一次拉全量把内存打满
		}
		if page < 1 {
			page = 1
		}

		q := db.Model(&model.AuditLog{})
		if v := c.Query("action"); v != "" {
			q = q.Where("action = ?", v)
		}
		if v := c.Query("actorType"); v != "" {
			q = q.Where("actor_type = ?", v)
		}
		if v := c.Query("result"); v != "" {
			q = q.Where("result = ?", v)
		}
		if v := c.Query("actorId"); v != "" {
			q = q.Where("actor_id = ?", v)
		}

		var total int64
		q.Count(&total)

		var rows []model.AuditLog
		q.Order("id desc").Offset((page - 1) * size).Limit(size).Find(&rows)

		c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{
			"total": total, "page": page, "size": size, "list": rows,
		}})
	})
}

// atoiDefault 解析分页参数，非法回落缺省值（避免手写 strconv 的重复分支）。
func atoiDefault(s string, def int) int {
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return def
	}
	return n
}

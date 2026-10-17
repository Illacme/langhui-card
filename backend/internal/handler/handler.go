package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"langhui/internal/repository"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// New 构建 Gin 引擎并注册首页相关路由。
// 注意：用 gin.New() + 显式 Recovery，而非 gin.Default()——Default 自带的 Logger
// 会与 middleware.AccessLog 重复输出；访问日志统一由调用方（main / devserver）
// 挂 RequestID + AccessLog 中间件，格式见 middleware/observability.go。
func New(db *gorm.DB) *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())

	// 健康检查（就绪探针）：容器编排 / 负载均衡探活用。
	// 注意：这里必须探 DB 连通性，而不是无条件返回 200。若只做存活探测，
	// DB 故障（连接断开 / 实例宕机）时 /healthz 仍为 200，docker-compose 的
	// backend healthcheck（依赖本端点）与 alert_check.sh 的 R1 探活都会误判为
	// "可用"，实际所有 /api 请求已 5xx。改为 DB 不可达时返回 503，让编排的
	// depends_on/service_healthy 与外部探活告警能识别"进程在但不可用"。
	r.GET("/healthz", func(c *gin.Context) {
		if err := db.Exec("SELECT 1").Error; err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status": "degraded",
				"db":     "down",
				"error":  err.Error(),
			})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok", "db": "up"})
	})

	// 聚合名片（员工 + 企业 + 简介）
	r.GET("/api/card", func(c *gin.Context) {
		staffID, _ := strconv.ParseInt(c.Query("staffId"), 10, 64)
		e, ci, cp, err := repository.GetCard(db, staffID)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "员工不存在"})
			return
		}
		if e.Status == "disabled" {
			c.JSON(http.StatusOK, gin.H{"code": 2, "msg": "该员工名片已停用"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "employee": e, "company": ci, "profile": cp})
	})

	// 经典业绩：分类 + 精选项目
	r.GET("/api/achievements", func(c *gin.Context) {
		cats, featured, err := repository.GetAchievements(db)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "查询失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "categories": cats, "featured": featured})
	})

	// 项目列表（按分类）
	r.GET("/api/projects", func(c *gin.Context) {
		cid, _ := strconv.ParseInt(c.Query("categoryId"), 10, 64)
		ps, err := repository.ListProjects(db, cid)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "查询失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": ps})
	})

	// 项目详情（解析 album JSON）
	r.GET("/api/projects/:id", func(c *gin.Context) {
		id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
		p, err := repository.GetProject(db, id)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "案例不存在或已下架"})
			return
		}
		var album []string
		if p.Album != "" {
			_ = json.Unmarshal([]byte(p.Album), &album)
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{
			"id":           p.ID,
			"name":         p.Name,
			"summary":      p.Summary,
			"cover":        p.Cover,
			"detail":       p.Detail,
			"album":        album,
			"categoryId":   p.CategoryID,
			"categoryName": p.CategoryName, // 由 repository 回填，前端免二次查询
			"createdAt":    p.CreatedAt,
			"updatedAt":    p.UpdatedAt,
		}})
	})

	// 业务服务列表
	r.GET("/api/services", func(c *gin.Context) {
		s, err := repository.ListServices(db)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "查询失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": s})
	})

	// 业务服务详情
	r.GET("/api/services/:id", func(c *gin.Context) {
		id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
		s, err := repository.GetService(db, id)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "服务不存在"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": s})
	})

	// 首页布局顺序
	r.GET("/api/home-layout", func(c *gin.Context) {
		order := []string{"achievement", "service"}
		if l, err := repository.GetHomeLayout(db); err == nil && l.Order != "" {
			_ = json.Unmarshal([]byte(l.Order), &order)
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "order": order})
	})

	// 埋点上报
	r.POST("/api/stat/track", func(c *gin.Context) {
		var req struct {
			StaffID   int64  `json:"staffId"`
			EventType string `json:"eventType"`
		}
		if err := c.ShouldBindJSON(&req); err != nil || req.EventType == "" {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "参数错误"})
			return
		}
		if err := repository.TrackStat(db, req.StaffID, req.EventType); err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "上报失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "ok"})
	})

	return r
}

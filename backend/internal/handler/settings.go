package handler

import (
	"net/http"
	"sort"
	"strconv"
	"strings"

	"langhui/internal/audit"
	"langhui/internal/config"
	"langhui/internal/middleware"
	"langhui/internal/setting"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// RegisterSettings 注册系统设置接口：
//   - GET  /api/public/settings  公开：仅返回非敏感项（小程序 / 前端引导用）
//   - GET  /api/admin/settings   平台超管读取全部可配置项（按分组）
//   - PUT  /api/admin/settings   平台超管批量更新配置（按类型校验）
//
// 三档生效方式（详见 internal/setting 包注释）：
//  1. 常规项保存即时生效（内存缓存即时回写）；
//  2. Restart 项（云存储 / CDN / 上传基址）保存后需重启进程；
//  3. ReadOnly 项（DB 连接串 / JWT 密钥 / Cookie）只掩码展示当前值，PUT 会被拒绝。
//
// cfg 用于注入「环境变量类配置」的当前实际值——这些项即使环境变量没设也有进程内缺省，
// 只有拿 cfg 才能如实告诉管理员「现在到底配了什么」，而不是显示一片空白。
func RegisterSettings(r *gin.Engine, db *gorm.DB, cfg config.Config) {
	setting.SetEnvSnapshot(map[string]string{
		"db_dsn":        cfg.DBDSN,
		"jwt_key":       cfg.JWTKey,
		"cookie_secure": strconv.FormatBool(cfg.CookieSecure),
		"cookie_domain": cfg.CookieDomain,
	})

	r.GET("/api/public/settings", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": setting.Public()})
	})

	plat := middleware.PlatformAdminMiddleware(db)
	r.GET("/api/admin/settings", plat, func(c *gin.Context) {
		items := setting.All()
		sort.SliceStable(items, func(i, j int) bool {
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
		restartKeys := make([]string, 0)
		for k, v := range req {
			if err := setting.Upsert(k, v); err != nil {
				// 失败也记一笔：非法提交（含试图改只读项 / 提交掩码）本身就是需要追溯的行为
				audit.Record(db, c, audit.Entry{
					Action: audit.ActionSettingUpdate, TargetType: "setting", TargetID: k,
					Result: audit.ResultFail, Detail: map[string]any{"keys": keysOf(req), "error": err.Error()},
				})
				c.JSON(http.StatusOK, gin.H{"code": 1, "msg": err.Error()})
				return
			}
			if setting.IsRestart(k) {
				restartKeys = append(restartKeys, k)
			}
		}
		sort.Strings(restartKeys)
		// 审计：记录改了哪些项，敏感项（密钥）只记「已修改」不落明文
		audit.Record(db, c, audit.Entry{
			Action: audit.ActionSettingUpdate, TargetType: "setting",
			Detail: audit.DetailKeys(req, setting.IsSecret),
		})
		msg := "保存成功"
		if len(restartKeys) > 0 {
			// 明确告知哪些项不会立刻生效，避免「改了没反应」的误解
			msg = "保存成功；其中 " + strconv.Itoa(len(restartKeys)) + " 项需重启服务后生效：" +
				strings.Join(restartKeys, "、")
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": msg, "data": gin.H{"restartKeys": restartKeys}})
	})
}

// keysOf 取提交里的设置项 key 列表（审计用：失败时不记录值，只记动过哪些项）。
func keysOf(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

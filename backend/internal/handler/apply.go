package handler

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"strconv"

	"weichuan/internal/audit"
	"weichuan/internal/auth"
	"weichuan/internal/middleware"
	"weichuan/internal/model"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// RegisterApply 注册写操作：开通申请 / 名片修改审核 / 员工管理 / 统计。
// 多租户：所有 /api/admin 端点按 tenantScope 收窄（企业管理员只见本企业）；
// 公开申请端点按请求体 companyId 归属租户（缺省 1 号租户朗汇）。
func RegisterApply(r *gin.Engine, db *gorm.DB) {
	// 员工自助提交开通申请（公开）
	r.POST("/api/apply/open", func(c *gin.Context) {
		var req model.OpenApply
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "参数错误"})
			return
		}
		// 租户归属：客户端可传 companyId（第二期按企业分发用）；缺省落朗汇。
		// 以服务端白名单为准——只接受已存在且启用的租户，防越权申请到别家。
		if req.CompanyID <= 0 {
			req.CompanyID = defaultCompanyID
		}
		if !companyEnabled(db, req.CompanyID) {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "申请目标企业不存在或已停用"})
			return
		}
		req.Status = "pending"
		if err := db.Create(&req).Error; err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "提交失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "已提交，等待审核"})
	})

	// 员工本人名片信息（需登录）。
	// 响应根级附带 pendingModify：是否有待审的名片修改申请——编辑名片页据此
	// 展示「审核中」标签（需求 B9：审核期对外仍显示旧信息，仅 approve 时写回）。
	// 放根级而非塞进 data，是不动既有 data=Employee 契约（小程序 edit/me 两页在用）。
	r.GET("/api/employee/me", middleware.AuthMiddleware(db, "employee"), func(c *gin.Context) {
		staffID := c.GetInt64("staffId")
		var e model.Employee
		if err := db.First(&e, "staff_id = ?", staffID).Error; err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "未找到"})
			return
		}
		var pending int64
		db.Model(&model.CardModifyApply{}).Where("staff_id = ? AND status = ?", staffID, "pending").Count(&pending)
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": e, "pendingModify": pending > 0})
	})

	// 员工提交名片修改（待审，对外仍显示旧信息）
	r.PUT("/api/employee/me", middleware.AuthMiddleware(db, "employee"), func(c *gin.Context) {
		staffID := c.GetInt64("staffId")
		var patch struct {
			Avatar   string `json:"avatar"`
			Name     string `json:"name"`
			Title    string `json:"title"`
			Phone    string `json:"phone"`
			Phone2   string `json:"phone2"`
			Landline string `json:"landline"`
			Email    string `json:"email"`
		}
		if err := c.ShouldBindJSON(&patch); err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "参数错误"})
			return
		}
		var me model.Employee
		if db.First(&me, "staff_id = ?", staffID).Error != nil {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "未找到"})
			return
		}
		snap, _ := json.Marshal(patch)
		apply := model.CardModifyApply{CompanyID: me.CompanyID, StaffID: staffID, Snapshot: string(snap), Status: "pending"}
		if err := db.Create(&apply).Error; err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "提交失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "已提交，等待审核"})
	})

	// 员工本人统计（?from=&to= 日期范围，YYYY-MM-DD 闭区间，均可缺省=全部时间）。
	// 一次 SUM(CASE) 拿三类计数，而不是三次 Count：三条 SQL 三条错误路径，
	// 而这里要的只是「范围内三个数」。
	r.GET("/api/employee/me/stat", middleware.AuthMiddleware(db, "employee"), func(c *gin.Context) {
		staffID := c.GetInt64("staffId")
		from, to, ok := statDateRange(c)
		if !ok {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "日期参数错误"})
			return
		}
		var row struct {
			Visit int64 `gorm:"column:visit"`
			Call  int64 `gorm:"column:call"`
			Save  int64 `gorm:"column:save"`
		}
		q := db.Model(&model.StatEvent{}).Where("staff_id = ?", staffID).
			Select("SUM(CASE WHEN event_type = 'visit' THEN 1 ELSE 0 END) AS `visit`, " +
				"SUM(CASE WHEN event_type = 'call' THEN 1 ELSE 0 END) AS `call`, " +
				"SUM(CASE WHEN event_type = 'save' THEN 1 ELSE 0 END) AS `save`")
		if from != "" {
			q = q.Where("stat_date >= ?", from)
		}
		if to != "" {
			q = q.Where("stat_date <= ?", to)
		}
		// 零行时 Scan 不回错、row 保持零值——「没有事件」与「计数为 0」同形，正是要的
		if err := q.Scan(&row).Error; err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "查询失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"visit": row.Visit, "call": row.Call, "save": row.Save}})
	})

	// 员工修改本人密码
	r.PUT("/api/employee/password", middleware.AuthMiddleware(db, "employee"), func(c *gin.Context) {
		staffID := c.GetInt64("staffId")
		var req struct {
			Old string `json:"old"`
			New string `json:"new"`
		}
		if c.ShouldBindJSON(&req) != nil || req.New == "" {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "参数错误"})
			return
		}
		var e model.Employee
		db.First(&e, "staff_id = ?", staffID)
		if !auth.CheckPassword(e.PasswordHash, req.Old) {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "原密码错误"})
			return
		}
		h, _ := auth.HashPassword(req.New)
		// 改密同时自增安全戳 → 旧令牌立即失效，需重新登录
		db.Model(&e).Updates(map[string]interface{}{"password_hash": h, "token_version": e.TokenVersion + 1})
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "ok"})
	})

	// ===== 超管端（多租户：按 tenantScope 收窄） =====

	// inScope 判断某租户 id 是否在当前用户可见范围内（平台超管恒真）。
	inScope := func(c *gin.Context, cid int64) bool {
		scid, all := tenantScope(c)
		return all || scid == cid
	}

	// 开通申请列表（按租户）：分页 + 状态筛选（pending/approved/rejected，空=全部）
	// + 关键字（姓名/手机/邮箱/昵称）——关键字必须落在服务端，否则前端「只筛当前页」
	// 会让运营以为搜到的数据翻页后丢了。
	r.GET("/api/admin/apply/open", middleware.AuthMiddleware(db, "admin"), func(c *gin.Context) {
		page, size := pageParams(c)
		q := db.Model(&model.OpenApply{}).Scopes(scopeCond(c))
		if cond, args := keywordCond(c.Query("keyword"), "name", "phone", "email", "nickname"); cond != "" {
			q = q.Where(cond, args...)
		}
		if cond, v := statusCond(c.Query("status"), "pending", "approved", "rejected"); cond != "" {
			q = q.Where(cond, v)
		}
		var list []model.OpenApply
		total, page := pagedQuery(q, "created_at desc", &list, page, size)
		pagedOK(c, list, total, page, size)
	})

	// 开通申请通过 → 自动建员工账号（员工落到申请所属租户）
	r.POST("/api/admin/apply/open/:id/approve", middleware.AuthMiddleware(db, "admin"), func(c *gin.Context) {
		id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
		var a model.OpenApply
		if db.First(&a, "id = ?", id).Error != nil {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "未找到"})
			return
		}
		if !inScope(c, a.CompanyID) {
			c.JSON(http.StatusOK, gin.H{"code": 403, "msg": "无权操作其他企业的申请"})
			return
		}
		if a.Status != "pending" {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "已处理"})
			return
		}
		pwd, err := genPassword()
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": err.Error()})
			return
		}
		hash, err := auth.HashPassword(pwd)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "密码加密失败"})
			return
		}
		emp := model.Employee{CompanyID: a.CompanyID, Name: a.Name, Phone: a.Phone, Phone2: a.Phone2, Email: a.Email, Title: a.Title, Landline: a.Landline, Avatar: a.Avatar, OpenID: a.OpenID, Nickname: a.Nickname, PasswordHash: hash, Status: "enabled"}
		db.Create(&emp)
		a.Status = "approved"
		db.Save(&a)
		c.JSON(http.StatusOK, gin.H{"code": 0, "staffId": emp.StaffID, "initialPassword": pwd})
	})

	// 开通申请驳回（按租户）
	r.POST("/api/admin/apply/open/:id/reject", middleware.AuthMiddleware(db, "admin"), func(c *gin.Context) {
		id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
		var body struct {
			Reason string `json:"reason"`
		}
		c.ShouldBindJSON(&body)
		q := db.Model(&model.OpenApply{}).Where("id = ? AND status = ?", id, "pending").Scopes(scopeCond(c))
		if q.Updates(map[string]interface{}{"status": "rejected", "reject_reason": body.Reason}).RowsAffected == 0 {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "未找到或已处理"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "ok"})
	})

	// 名片修改申请列表（按租户：company_id 冗余自员工归属）：分页 + 状态筛选。
	// 关键字按**员工姓名**匹配（子查询）：申请行本身只有 staff_id + 变更快照，
	// 直接对快照做 LIKE 在 MySQL 的 JSON 列上行为不一致（SQLite 存的是 TEXT），
	// 而运营搜索的实际意图就是「找某个人的申请」。
	r.GET("/api/admin/apply/modify", middleware.AuthMiddleware(db, "admin"), func(c *gin.Context) {
		page, size := pageParams(c)
		q := db.Model(&model.CardModifyApply{}).Scopes(scopeCond(c))
		if like := likeKeyword(c.Query("keyword")); like != "" {
			sub := db.Model(&model.Employee{}).Select("staff_id").Where("name LIKE ?", like)
			q = q.Where("staff_id IN (?)", sub)
		}
		if cond, v := statusCond(c.Query("status"), "pending", "approved", "rejected"); cond != "" {
			q = q.Where(cond, v)
		}
		var list []model.CardModifyApply
		total, page := pagedQuery(q, "created_at desc", &list, page, size)
		pagedOK(c, list, total, page, size)
	})

	// 名片修改通过 → 应用快照到员工（按租户校验）
	r.POST("/api/admin/apply/modify/:id/approve", middleware.AuthMiddleware(db, "admin"), func(c *gin.Context) {
		id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
		var a model.CardModifyApply
		if db.First(&a, "id = ?", id).Error != nil {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "未找到"})
			return
		}
		var e model.Employee
		if db.First(&e, "staff_id = ?", a.StaffID).Error != nil {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "员工不存在"})
			return
		}
		if !inScope(c, e.CompanyID) {
			c.JSON(http.StatusOK, gin.H{"code": 403, "msg": "无权操作其他企业的申请"})
			return
		}
		if a.Status != "pending" {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "已处理"})
			return
		}
		var patch map[string]string
		_ = json.Unmarshal([]byte(a.Snapshot), &patch)
		updates := map[string]interface{}{}
		if v, ok := patch["avatar"]; ok {
			updates["avatar"] = v
		}
		if v, ok := patch["name"]; ok {
			updates["name"] = v
		}
		if v, ok := patch["title"]; ok {
			updates["title"] = v
		}
		if v, ok := patch["phone"]; ok {
			updates["phone"] = v
		}
		if v, ok := patch["phone2"]; ok {
			updates["phone2"] = v
		}
		if v, ok := patch["landline"]; ok {
			updates["landline"] = v
		}
		if v, ok := patch["email"]; ok {
			updates["email"] = v
		}
		db.Model(&e).Updates(updates)
		a.Status = "approved"
		db.Save(&a)
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "ok"})
	})

	// 名片修改驳回（按租户）
	r.POST("/api/admin/apply/modify/:id/reject", middleware.AuthMiddleware(db, "admin"), func(c *gin.Context) {
		id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
		var body struct {
			Reason string `json:"reason"`
		}
		c.ShouldBindJSON(&body)
		q := db.Model(&model.CardModifyApply{}).Where("id = ? AND status = ?", id, "pending").Scopes(scopeCond(c))
		if q.Updates(map[string]interface{}{"status": "rejected", "reject_reason": body.Reason}).RowsAffected == 0 {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "未找到或已处理"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "ok"})
	})

	// 超管手动新增员工（自动生成账号 + 初始密码）。
	// 租户归属：企业管理员 → 本企业；平台超管 → ?companyId=（缺省朗汇）。
	r.POST("/api/admin/employees", middleware.AuthMiddleware(db, "admin"), func(c *gin.Context) {
		var req model.Employee
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "参数错误"})
			return
		}
		cid, _ := tenantScope(c)
		if cid == 0 {
			cid = targetCompanyID(c)
		}
		if !companyEnabled(db, cid) {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "目标企业不存在或已停用"})
			return
		}
		pwd, err := genPassword()
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": err.Error()})
			return
		}
		hash, err := auth.HashPassword(pwd)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "密码加密失败"})
			return
		}
		emp := model.Employee{CompanyID: cid, Name: req.Name, Phone: req.Phone, Phone2: req.Phone2, Email: req.Email, Title: req.Title, Landline: req.Landline, Avatar: req.Avatar, PasswordHash: hash, Status: "enabled"}
		db.Create(&emp)
		c.JSON(http.StatusOK, gin.H{"code": 0, "staffId": emp.StaffID, "initialPassword": pwd})
	})

	// 超管编辑员工资料（即时生效，不经过员工端审核流）。
	// 只开放白名单字段：姓名 / 职位 / 手机 / 手机 2 / 座机 / 邮箱 / 头像。
	// 状态、密码各有专门端点（disable/enable/reset-password），此处不混写，避免绕过安全戳逻辑。
	// 租户条件进 Where：企业管理员编辑不到别家员工。
	r.PUT("/api/admin/employees/:id", middleware.AuthMiddleware(db, "admin"), func(c *gin.Context) {
		id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
		var req struct {
			Name     string `json:"name"`
			Title    string `json:"title"`
			Phone    string `json:"phone"`
			Phone2   string `json:"phone2"`
			Landline string `json:"landline"`
			Email    string `json:"email"`
			Avatar   string `json:"avatar"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "参数错误"})
			return
		}
		if req.Name == "" || req.Phone == "" {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "姓名与手机为必填"})
			return
		}
		if db.Model(&model.Employee{}).Where("staff_id = ?", id).Scopes(scopeCond(c)).
			Updates(map[string]interface{}{
				"name": req.Name, "title": req.Title, "phone": req.Phone, "phone2": req.Phone2,
				"landline": req.Landline, "email": req.Email, "avatar": req.Avatar,
			}).RowsAffected == 0 {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "未找到"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "ok"})
	})

	// 员工列表（按租户）
	//
	// 分页 + 关键字（姓名/手机/手机 2/职位/座机/昵称/openid）+ 状态筛选。
	// 关键字覆盖的字段与 PC 后台搜索框的匹配范围保持一致——否则「搜得到但翻页后
	// 又没了」（前端只筛当前页）会让运营以为数据丢了。
	//
	// 另有 ?staffIds=1,2,3：按 id 精确取少数几条，供「名片修改申请」页回显
	// 员工原值（替代原先拉全量员工建映射的做法），上限 200 条。
	r.GET("/api/admin/employees", middleware.AuthMiddleware(db, "admin"), func(c *gin.Context) {
		page, size := pageParams(c)
		q := db.Model(&model.Employee{}).Scopes(scopeCond(c))
		if cond, args := keywordCond(c.Query("keyword"),
			"name", "phone", "phone2", "title", "landline", "nickname", "openid"); cond != "" {
			q = q.Where(cond, args...)
		}
		if cond, v := statusCond(c.Query("status"), "enabled", "disabled"); cond != "" {
			q = q.Where(cond, v)
		}
		if ids := intListParam(c.Query("staffIds"), 200); len(ids) > 0 {
			// 精确取：条数由调用方给的 id 数决定，不再分页（本身已是有界集合）
			var list []model.Employee
			q.Where("staff_id IN ?", ids).Order("staff_id asc").Find(&list)
			if size = len(list); size == 0 {
				size = defaultPageSize
			}
			pagedOK(c, list, int64(len(list)), 1, size)
			return
		}
		var list []model.Employee
		total, page := pagedQuery(q, "staff_id asc", &list, page, size)
		pagedOK(c, list, total, page, size)
	})

	// 启用 / 禁用（禁用同时自增安全戳 → 该员工在线会话立即失效；按租户）
	r.POST("/api/admin/employees/:id/disable", middleware.AuthMiddleware(db, "admin"), func(c *gin.Context) {
		id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
		// RowsAffected 必须查：从前不查，禁用一个不存在的 / 别家租户的员工也回 ok，
		// 管理员无从察觉操作压根没生效（更新的 Error 同样不能丢）
		res := db.Model(&model.Employee{}).Where("staff_id = ?", id).Scopes(scopeCond(c)).
			Updates(map[string]interface{}{"status": "disabled", "token_version": gorm.Expr("token_version + 1")})
		if res.Error != nil {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": res.Error.Error()})
			return
		}
		if res.RowsAffected == 0 {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "未找到"})
			return
		}
		// 启停员工是特权动作：从前这里不记审计，改自己密码反倒记了，日志里看不出谁动了谁
		audit.Record(db, c, audit.Entry{
			Action: audit.ActionEmployeeStatus, TargetType: "employee", TargetID: strconv.FormatInt(id, 10),
			Detail: map[string]any{"status": "disabled"},
		})
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "ok"})
	})
	r.POST("/api/admin/employees/:id/enable", middleware.AuthMiddleware(db, "admin"), func(c *gin.Context) {
		id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
		res := db.Model(&model.Employee{}).Where("staff_id = ?", id).Scopes(scopeCond(c)).
			Updates(map[string]interface{}{"status": "enabled", "token_version": gorm.Expr("token_version + 1")})
		if res.Error != nil {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": res.Error.Error()})
			return
		}
		if res.RowsAffected == 0 {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "未找到"})
			return
		}
		audit.Record(db, c, audit.Entry{
			Action: audit.ActionEmployeeStatus, TargetType: "employee", TargetID: strconv.FormatInt(id, 10),
			Detail: map[string]any{"status": "enabled"},
		})
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "ok"})
	})

	// 重置密码（返回新初始密码；按租户）
	r.POST("/api/admin/employees/:id/reset-password", middleware.AuthMiddleware(db, "admin"), func(c *gin.Context) {
		id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
		pwd, err := genPassword()
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": err.Error()})
			return
		}
		hash, err := auth.HashPassword(pwd)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "密码加密失败"})
			return
		}
		// 重置密码同时自增安全戳 → 该员工所有旧会话立即失效
		res := db.Model(&model.Employee{}).Where("staff_id = ?", id).Scopes(scopeCond(c)).
			Updates(map[string]interface{}{"password_hash": hash, "token_version": gorm.Expr("token_version + 1")})
		if res.Error != nil {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": res.Error.Error()})
			return
		}
		if res.RowsAffected == 0 {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "未找到"})
			return
		}
		// 重置他人密码是特权动作，必须留痕（从前只记「改自己密码」，这条漏了）
		audit.Record(db, c, audit.Entry{
			Action: audit.ActionEmployeeReset, TargetType: "employee", TargetID: strconv.FormatInt(id, 10),
		})
		c.JSON(http.StatusOK, gin.H{"code": 0, "initialPassword": pwd})
	})

	// 超管修改自己密码
	r.PUT("/api/admin/password", middleware.AuthMiddleware(db, "admin"), func(c *gin.Context) {
		adminID := c.GetInt64("staffId")
		var req struct {
			Old string `json:"old"`
			New string `json:"new"`
		}
		if c.ShouldBindJSON(&req) != nil || req.New == "" {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "参数错误"})
			return
		}
		var a model.Admin
		db.First(&a, "id = ?", adminID)
		if !auth.CheckPassword(a.PasswordHash, req.Old) {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "原密码错误"})
			return
		}
		h, _ := auth.HashPassword(req.New)
		// 改密同时自增安全戳 → 旧令牌立即失效，需重新登录
		db.Model(&a).Updates(map[string]interface{}{"password_hash": h, "token_version": a.TokenVersion + 1})
		// 审计只记「谁改了自己的密码」，绝不记新旧密码
		audit.Record(db, c, audit.Entry{
			Action: audit.ActionPassword, TargetType: "admin", TargetID: strconv.FormatInt(adminID, 10),
			Detail: map[string]any{"username": a.Username},
		})
		c.JSON(http.StatusOK, gin.H{"code": 0, "msg": "ok"})
	})

	// 全局名片统计（超管）：汇总 + 按员工分组 + 按天趋势。
	// 与 /api/employee/me/stat（仅本人）互补，供 PC 后台「数据统计」页使用。
	// 实现要点：
	//   1. 用 SUM(CASE WHEN ...) 而非 SUM(布尔表达式)——后者是 MySQL 方言，
	//      SQLite 不认，MySQL parity 与 SQLite 会一边挂一边绿。
	//   2. 分组结果与员工表合并：零数据的员工也要出现在列表里（补 0），
	//      否则「有员工但没访问」在后台看不出来。
	//   3. 别名必须加反引号：call 是 MySQL 8 保留字（存储过程关键字），
	//      裸 AS call 直接 1064（SQLite 不保留所以潜伏——2026-10-21 parity 首跑抓出）。
	//      反引号在 SQLite 同样合法（MySQL 兼容引用符），两边都能跑。
	//   4. ?from=&to= 日期范围（YYYY-MM-DD 闭区间，可缺省）：stat_date 有索引，
	//      字符串字典序即时间序，范围查询直接走索引；缺省即「全部时间」。
	//   5. summary 后五个 + topProjects/topServices 是「内容意向」：事件带
	//      target_id（migrations/0021），按 target 聚合 JOIN 对象名，回答
	//      「访客对什么内容感兴趣」——统计对企业客户的核心价值。
	r.GET("/api/admin/stats", middleware.AuthMiddleware(db, "admin"), func(c *gin.Context) {
		from, to, ok := statDateRange(c)
		if !ok {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "日期参数错误"})
			return
		}
		rows, err := statRowsByStaff(c, db, from, to)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "查询失败"})
			return
		}
		trend, err := statTrend(c, db, from, to)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "查询失败"})
			return
		}
		topProjects, err := statTopTargets(c, db, from, to, "project_view", "project_cases", "name", statTopLimit)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "查询失败"})
			return
		}
		topServices, err := statTopTargets(c, db, from, to, "service_view", "biz_services", "title", statTopLimit)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "查询失败"})
			return
		}
		sourceDist, err := statSourceDist(c, db, from, to)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "查询失败"})
			return
		}
		fissionTop, err := statFissionTop(c, db, from, to, statFissionLimit)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "查询失败"})
			return
		}
		list := make([]gin.H, 0, len(rows))
		var sumVisit, sumCall, sumSave int64
		var sumProjectView, sumServiceView, sumVideoPlay, sumShare, sumPosterSave int64
		for _, r := range rows {
			sumVisit += r.Visit
			sumCall += r.Call
			sumSave += r.Save
			sumProjectView += r.ProjectView
			sumServiceView += r.ServiceView
			sumVideoPlay += r.VideoPlay
			sumShare += r.Share
			sumPosterSave += r.PosterSave
			list = append(list, gin.H{
				"staffId": r.StaffID,
				"name":    r.Name,
				"title":   r.Title,
				"avatar":  r.Avatar,
				"visit":   r.Visit,
				"call":    r.Call,
				"save":    r.Save,
				// 意向计数随行返回（CSV 同源），PC 表格不展示——列宽装不下
				"projectView": r.ProjectView,
				"serviceView": r.ServiceView,
				"videoPlay":   r.VideoPlay,
				"share":       r.Share,
				"posterSave":  r.PosterSave,
			})
		}
		trendList := make([]gin.H, 0, len(trend))
		for _, t := range trend {
			trendList = append(trendList, gin.H{
				// 八项全给：前端图例可点选，勾到哪个指标都要有线
				"date": t.Day, "visit": t.Visit, "call": t.Call, "save": t.Save,
				"projectView": t.ProjectView, "serviceView": t.ServiceView,
				"videoPlay": t.VideoPlay, "share": t.Share, "posterSave": t.PosterSave,
			})
		}
		topList := func(rows []statTopRow) []gin.H {
			out := make([]gin.H, 0, len(rows))
			for _, r := range rows {
				out = append(out, gin.H{"targetId": r.TargetID, "name": r.Name, "count": r.Count})
			}
			return out
		}
		// 来源分布行 → 响应：{source, count}
		topListSource := func(rows []statSourceRow) []gin.H {
			out := make([]gin.H, 0, len(rows))
			for _, r := range rows {
				out = append(out, gin.H{"source": r.Source, "count": r.Count})
			}
			return out
		}
		// 裂变榜行 → 响应：{staffId, name, count}
		topListFission := func(rows []statFissionRow) []gin.H {
			out := make([]gin.H, 0, len(rows))
			for _, r := range rows {
				out = append(out, gin.H{"staffId": r.StaffID, "name": r.Name, "count": r.Count})
			}
			return out
		}
		c.JSON(http.StatusOK, gin.H{
			"code": 0,
			"data": gin.H{
				"summary": gin.H{
					"visit": sumVisit, "call": sumCall, "save": sumSave,
					"projectView": sumProjectView, "serviceView": sumServiceView,
					"videoPlay": sumVideoPlay, "share": sumShare, "posterSave": sumPosterSave,
				},
				"byStaff":     list,
				"trend":       trendList,
				"topProjects": topList(topProjects),
				"topServices": topList(topServices),
				// 流量来源分布：visit 事件按 source 分组（scan/share_card/search/poster/unknown/other）
				"sourceDist": topListSource(sourceDist),
				// 裂变榜：按 staff_id 聚合「source=share_card 的 visit」（谁的名片被转发带来的新访客多）
				"fissionTop": topListFission(fissionTop),
			},
		})
	})
}

// genPassword 生成 8 位随机密码（字母+数字）。
//
// crypto/rand 取值失败**必须回传错误**：从前写成 `n, _ := rand.Int(...)`，
// 失败时 n 是 nil，下一行 n.Int64() 直接 panic 把进程打挂——
// 一次密码生成失败不该让整个服务挂掉，更不该给员工发一个可预测的密码。
// randInt 随机源的间接层：唯一目的是可测——crypto/rand 正常不会失败，
// 不注入就覆盖不到「失败时该怎么办」这条支线（从前忽略错误会直接 panic）。
var randInt = rand.Int

func genPassword() (string, error) {
	const chars = "abcdefghijkmnpqrstuvwxyz23456789"
	b := make([]byte, 8)
	for i := range b {
		n, err := randInt(rand.Reader, big.NewInt(int64(len(chars))))
		if err != nil {
			return "", fmt.Errorf("生成随机密码失败: %w", err)
		}
		b[i] = chars[n.Int64()]
	}
	return string(b), nil
}

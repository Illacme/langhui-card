package handler

import (
	"bytes"
	"encoding/csv"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"weichuan/internal/middleware"
	"weichuan/internal/model"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ===== 埋点统计（stat_events）的共享件 =====
// 数据流：小程序埋点（visit/call/save）→ POST /api/stat/track（公开，按 IP 限流）
// → stat_events 原始表 → /api/admin/stats（汇总 + 按员工 + 按天趋势，日期范围可调）
// → PC 后台「数据统计」页 / CSV 导出。

// statEventTypes 埋点类型白名单。
// 为什么不「非空即可」：event_type 是自由字符串列，而查询侧只认这三类做
// SUM(CASE WHEN)。脏值不报错、不影响别的事件，只会让「总量」与「分类」
// 悄悄对不上——典型的静默数据污染。新增事件类型在此登记，查询侧与端上同步跟上。
var statEventTypes = map[string]bool{
	"visit": true, // 访问名片（小程序首页，按页面实例去重）
	"call":  true, // 点击拨号
	"save":  true, // 点击保存
}

// statTrackLimitFn 埋点上报限流阈值（每 IP 每分钟事件数）。
//
// 取 60 的考量：一次真实到访最多产生个位数事件（访问 1 + 点击若干），
// 办公网 NAT 下整层人同时扫也远到不了这个量；对脚本刷量（每分钟上千次）
// 则是硬顶。故意不进设置界面——运营没有「放开伪造刷量」的诉求，
// 少一个可调项少一个误操作面。
//
// 抽成包级变量是为了测试能把阈值压低（同 randRead 的间接层手法）。
var statTrackLimitFn = func() int { return 60 }

// statTrackLimiter 埋点接口的限流器：按客户端 IP 分桶 + 数据库共享计数。
// 不按 staffId 分桶——扫号脚本正是靠换 ID 放大调用量，按 ID 分桶等于没限。
func statTrackLimiter(db *gorm.DB) *middleware.RateLimiter {
	return middleware.NewRateLimiter(statTrackLimitFn, 60, time.Minute).
		WithMessage("上报过于频繁，请稍后再试").
		WithCounter(dbRateCounter{db: db})
}

// statDateRange 解析统计查询的日期范围（YYYY-MM-DD，闭区间）。
// from/to 均可缺省：缺省表示不设下界 / 上界（「全部时间」口径，保持兼容）。
// ok=false 表示参数非法（格式错、或 2026-02-31 这类不存在的日期、或 from > to），
// 调用方应回 code:1——静默忽略坏参数会让「筛了却没生效」比不筛更难排查。
func statDateRange(c *gin.Context) (from, to string, ok bool) {
	from = c.Query("from")
	to = c.Query("to")
	if from != "" && !validStatDate(from) {
		return "", "", false
	}
	if to != "" && !validStatDate(to) {
		return "", "", false
	}
	// YYYY-MM-DD 的字典序即时间序，字符串比较即可（无需再解析一次）
	if from != "" && to != "" && from > to {
		return "", "", false
	}
	return from, to, true
}

// validStatDate 严格校验 YYYY-MM-DD。time.Parse 会挡掉 2026-02-31 与
// 2026-13-01——这类值若进了查询条件，在两种数据库上都是「查不到」而非报错，
// 表现出来就是「统计莫名少一块」。
func validStatDate(s string) bool {
	_, err := time.Parse("2006-01-02", s)
	return err == nil
}

// statStaffRow 按员工聚合的一行：员工资料 + 三个计数。
// 零数据员工也出现在结果里（计数 0）——「有员工但没访问」是运营要看到的事实。
type statStaffRow struct {
	StaffID int64
	Name    string
	Title   string
	Avatar  string
	Visit   int64
	Call    int64
	Save    int64
}

// statRowsByStaff 取「当前后台用户可见范围」内员工的统计聚合（按租户隔离）。
//
// 租户范围用子查询（staff_id IN (SELECT staff_id FROM employees ...)）而不是
// 先查员工再拼 IN 列表：员工数随运营增长，IN 列表会越来越长，而子查询在两种
// 方言上同形态。stat_events 表本身不带 company_id（埋点只有 staffId），
// 租户隔离正是通过这个子查询建立的。
func statRowsByStaff(c *gin.Context, db *gorm.DB, from, to string) ([]statStaffRow, error) {
	var emps []model.Employee
	if err := db.Scopes(scopeCond(c)).Order("staff_id asc").Find(&emps).Error; err != nil {
		return nil, err
	}
	agg := map[int64]statStaffRow{}
	if len(emps) > 0 {
		var rows []statStaffRow
		q := db.Model(&model.StatEvent{}).
			Where("staff_id IN (?)", db.Model(&model.Employee{}).Select("staff_id").Scopes(scopeCond(c))).
			Select("staff_id, " +
				"SUM(CASE WHEN event_type = 'visit' THEN 1 ELSE 0 END) AS `visit`, " +
				"SUM(CASE WHEN event_type = 'call' THEN 1 ELSE 0 END) AS `call`, " +
				"SUM(CASE WHEN event_type = 'save' THEN 1 ELSE 0 END) AS `save`").
			Group("staff_id")
		if from != "" {
			q = q.Where("stat_date >= ?", from)
		}
		if to != "" {
			q = q.Where("stat_date <= ?", to)
		}
		if err := q.Scan(&rows).Error; err != nil {
			return nil, err
		}
		for _, r := range rows {
			agg[r.StaffID] = r
		}
	}
	out := make([]statStaffRow, 0, len(emps))
	for _, e := range emps {
		r := agg[e.StaffID] // 查不到 = 该员工在范围内零事件，天然补 0
		out = append(out, statStaffRow{
			StaffID: e.StaffID, Name: e.Name, Title: e.Title, Avatar: e.Avatar,
			Visit: r.Visit, Call: r.Call, Save: r.Save,
		})
	}
	return out, nil
}

// statTrendRow 按天聚合的一行（升序返回）。
type statTrendRow struct {
	Day    string `gorm:"column:day"`
	Visit  int64  `gorm:"column:visit"`
	Call   int64  `gorm:"column:call"`
	Save   int64  `gorm:"column:save"`
}

// statDateExpr 把 stat_date 归一成 YYYY-MM-DD 字符串的 SQL 表达式。
//
// 为什么不直接 Select("stat_date")：MySQL 的 stat_date 是 DATE 列，读回来是
// time.Time，GORM 扫进 string 字段会格式化成 RFC3339（2026-10-23T00:00:00Z，
// parity 首跑实测抓到）；而 SQLite 侧模型是 string、存的就是 'YYYY-MM-DD' 原样。
// 同一个字段两种形状，前端按日期渲染就会一半带 T 一半不带。
// CAST(... AS CHAR) 在两边都得到 'YYYY-MM-DD'（MySQL 8.0.46 / SQLite 实测），
// 故用一个表达式通吃，不引方言分支。分组与排序仍走原始列（DATE/TEXT 排序等价）。
const statDateExpr = "CAST(stat_date AS CHAR)"

// statTrend 按天趋势（同一日期口径 + 同一租户范围）。
// 只回有事件的日期，无事件日期由前端在首尾之间补零——避免两边各写一套日期推导。
func statTrend(c *gin.Context, db *gorm.DB, from, to string) ([]statTrendRow, error) {
	var rows []statTrendRow
	q := db.Model(&model.StatEvent{}).
		Where("staff_id IN (?)", db.Model(&model.Employee{}).Select("staff_id").Scopes(scopeCond(c))).
		Select(statDateExpr+" AS `day`, "+
			"SUM(CASE WHEN event_type = 'visit' THEN 1 ELSE 0 END) AS `visit`, "+
			"SUM(CASE WHEN event_type = 'call' THEN 1 ELSE 0 END) AS `call`, "+
			"SUM(CASE WHEN event_type = 'save' THEN 1 ELSE 0 END) AS `save`").
		Group("stat_date").Order("stat_date asc")
	if from != "" {
		q = q.Where("stat_date >= ?", from)
	}
	if to != "" {
		q = q.Where("stat_date <= ?", to)
	}
	if err := q.Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// ===== 埋点明细列表 =====

// statEventRaw 明细查询的原始行（数据库形状，供 Scan）。
// stat_date 走 CAST 归一的理由同 statTrend（MySQL DATE 列扫进 string 会变 RFC3339）。
// employees.name 由 JOIN 带出。用普通（非 LEFT）JOIN 是故意的：WHERE 子查询
// 已把范围圈死在「当前存在的员工」上，员工不存在时其事件在明细与汇总中
// 一致地不计入——明细是汇总的下钻，两边必须能对账（汇总说 100、明细列出
// 105 条是最难排查的一类事故）。
type statEventRaw struct {
	ID        int64     `gorm:"column:id"`
	StaffID   int64     `gorm:"column:staff_id"`
	Name      string    `gorm:"column:name"`
	EventType string    `gorm:"column:event_type"`
	StatDate  string    `gorm:"column:stat_date"`
	CreatedAt time.Time `gorm:"column:created_at"`
}

// statDetailItem 明细回包一行。
// CreatedAt 是服务端格式化好的 "YYYY-MM-DD HH:mm:ss" 字符串：时区以服务端 TZ
// 为权威口径（stat_date 的「今天」同样按它落库，见 compose TZ 约定）。
// 前端若再按浏览器时区换算，跨时区使用时明细时间会与趋势图日期对不上。
type statDetailItem struct {
	ID        int64  `json:"id"`
	StaffID   int64  `json:"staffId"`
	Name      string `json:"name"`
	EventType string `json:"eventType"`
	StatDate  string `json:"statDate"`
	CreatedAt string `json:"createdAt"`
}

// RegisterStatsDetail 注册埋点明细列表（GET /api/admin/stats/detail，按租户隔离）。
//
// 与 /api/admin/stats（聚合）互补：汇回答「谁有多少」，明答「都有什么」——
// 排查「某个员工的访问量对不上」时必须能下钻到逐条事件，否则聚合数只是
// 一个无法验证的黑盒。分页信封沿用全站约定 {code:0,data,total,page,size}
// （pagination.go 是唯一权威实现）。
//
// 筛选参数均可缺省（缺省即不过滤）：
//   from/to   日期范围，复用 statDateRange 的严格校验（坏值 code:1，不静默忽略）
//   staffId   指定员工（别家员工的 id 查过来是空列表——租户隔离由子查询保证）
//   eventType 指定事件类型，白名单外拒收（同埋点进水口：脏值会让「筛了却没生效」）
func RegisterStatsDetail(r *gin.Engine, db *gorm.DB) {
	r.GET("/api/admin/stats/detail", middleware.AuthMiddleware(db, "admin"), func(c *gin.Context) {
		from, to, ok := statDateRange(c)
		if !ok {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "日期参数错误"})
			return
		}
		eventType := c.Query("eventType")
		if eventType != "" && !statEventTypes[eventType] {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "事件类型参数错误"})
			return
		}
		var staffID int64
		if v := c.Query("staffId"); v != "" {
			n, err := strconv.ParseInt(v, 10, 64)
			if err != nil || n <= 0 {
				c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "员工参数错误"})
				return
			}
			staffID = n
		}

		page, size := pageParams(c)
		q := db.Model(&model.StatEvent{}).
			// 租户隔离：stat_events 不带 company_id，仍靠员工子查询建立边界
			Where("stat_events.staff_id IN (?)",
				db.Model(&model.Employee{}).Select("staff_id").Scopes(scopeCond(c))).
			// JOIN 只为带出姓名（一页最多 100 行，比二次查 ID 列表省一轮往返）。
			// 隔离不依赖 JOIN——WHERE 子查询已圈死范围；员工不存在时事件在
			// 汇总里同样不计（byStaff 从员工表出发），两边口径一致才能对账
			Joins("JOIN employees ON employees.staff_id = stat_events.staff_id").
			Select("stat_events.id, stat_events.staff_id, employees.name, " +
				"stat_events.event_type, " + statDateExpr + " AS stat_date, stat_events.created_at")
		if from != "" {
			q = q.Where("stat_date >= ?", from)
		}
		if to != "" {
			q = q.Where("stat_date <= ?", to)
		}
		if staffID > 0 {
			q = q.Where("stat_events.staff_id = ?", staffID)
		}
		if eventType != "" {
			q = q.Where("event_type = ?", eventType)
		}

		var events []statEventRaw
		total, page := pagedQuery(q, "stat_events.id desc", &events, page, size)

		list := make([]statDetailItem, 0, len(events))
		for _, e := range events {
			list = append(list, statDetailItem{
				ID: e.ID, StaffID: e.StaffID, Name: e.Name,
				EventType: e.EventType, StatDate: e.StatDate,
				CreatedAt: e.CreatedAt.Format("2006-01-02 15:04:05"),
			})
		}
		pagedOK(c, list, total, page, size)
	})
}

// RegisterStatsExport 注册统计 CSV 导出（超管，按租户隔离）。
// 与 GET /api/admin/stats 同一套口径（scoped 员工 + stat_date 范围 + 零数据补 0），
// 只是把表格搬进 Excel：运营做月报、或客户要求「把数据给我自己看」的场景都要这个。
func RegisterStatsExport(r *gin.Engine, db *gorm.DB) {
	r.GET("/api/admin/stats/export", middleware.AuthMiddleware(db, "admin"), func(c *gin.Context) {
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
		body, err := statsCSV(rows)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"code": 1, "msg": "导出失败"})
			return
		}
		c.Header("Content-Type", "text/csv; charset=utf-8")
		c.Header("Content-Disposition", statsDisposition(from, to))
		c.Data(http.StatusOK, "text/csv; charset=utf-8", body)
	})
}

// statsCSV 生成 CSV 内容。
// BOM 不可省：没有它 Excel 双击打开时按本地代码页解析，中文全乱码——
// 而「导出来就是要用 Excel 打开」正是这个接口的存在理由。
func statsCSV(rows []statStaffRow) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString("\xEF\xBB\xBF")
	w := csv.NewWriter(&buf)
	if err := w.Write([]string{"编号", "姓名", "职位", "访问", "拨号", "保存"}); err != nil {
		return nil, err
	}
	for _, r := range rows {
		rec := []string{
			strconv.FormatInt(r.StaffID, 10),
			csvCell(r.Name),
			csvCell(r.Title),
			strconv.FormatInt(r.Visit, 10),
			strconv.FormatInt(r.Call, 10),
			strconv.FormatInt(r.Save, 10),
		}
		if err := w.Write(rec); err != nil {
			return nil, err
		}
	}
	w.Flush()
	// Write 的错要到 Flush 之后才在 Error() 上现形：不查就等于把
	// 「导出只剩半行」当成功交付，而下载方无从知道文件被截断。
	if err := w.Error(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// csvCell 防公式注入：以 = + - @ 或制表符 / 回车开头的单元格，Excel / WPS
// 打开时会当公式执行（姓名 / 职位是自由文本，建一个名叫
// =cmd|'/c calc'!A1 的员工就能在运营的电脑上跑东西）。加 ' 前缀是最通用的解法。
// 逗号 / 引号 / 换行的转义由 encoding/csv 负责（自动加引号包裹）。
func csvCell(s string) string {
	if s == "" {
		return s
	}
	switch s[0] {
	case '=', '+', '-', '@', '\t', '\r':
		return "'" + s
	}
	return s
}

// statsDisposition 下载文件名。中文名走 RFC 5987 的 filename*（百分号编码），
// 同时留一个 ASCII filename 兜底（不认 filename* 的老客户端）。
func statsDisposition(from, to string) string {
	name := "名片统计"
	switch {
	case from != "" && to != "":
		name += "_" + from + "_" + to
	case from != "":
		name += "_" + from + "_起"
	case to != "":
		name += "_至_" + to
	default:
		name += "_全部"
	}
	name += ".csv"
	return "attachment; filename=\"stats.csv\"; filename*=UTF-8''" + url.QueryEscape(name)
}

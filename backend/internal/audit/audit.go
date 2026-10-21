// Package audit 管理操作审计：把后台的写操作与登录失败记进 audit_log，
// 回答「谁在什么时候、对什么对象、做了什么、成功没有」。
//
// 约定（重要）：
//   - 只记**写操作**（含失败 / 被拒的尝试）与登录失败；只读查询不记，否则日志量会淹没有用信息。
//   - Detail 里**绝不能出现明文敏感值**（密钥 / 密码 / 连接串 / token）。
//     需要记录「改了什么」时，敏感项只写「已修改」——审计表一旦泄密，
//     就与它存在的初衷（安全追责）相悖。
//   - 写入失败只打日志、不影响主流程：审计是旁路，不能因为记不下来就让业务失败。
package audit

import (
	"encoding/json"
	"log"
	"time"

	"weichuan/internal/model"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// 动作常量：命名沿用「对象.动作」，便于按前缀聚合统计。
const (
	ActionSettingUpdate = "setting.update"

	ActionLoginFail = "login.fail"
	ActionLogin     = "login.success"
	ActionLogout    = "logout"
	ActionPassword  = "password.change"

	ActionAdminCreate = "admin.create"
	ActionAdminRename = "admin.rename"
	ActionAdminReset  = "admin.reset_password"
	ActionAdminDelete = "admin.delete"

	ActionCompanyCreate     = "company.create"
	ActionCompanyUpdate     = "company.update"
	ActionCompanyStatus     = "company.status"
	ActionCompanyAdminAdd   = "company.admin.create"
	ActionCompanyAdminReset = "company.admin.reset_password"

	ActionEmployeeCreate = "employee.create"
	ActionEmployeeUpdate = "employee.update"
	ActionEmployeeReset  = "employee.reset_password"
	ActionEmployeeStatus = "employee.status"

	ActionApplyApprove = "apply.approve"
	ActionApplyReject  = "apply.reject"
)

// ResultSuccess / ResultFail 操作结果。
const (
	ResultSuccess = "success"
	ResultFail    = "fail"
)

// ActorType 操作者类型。
const (
	ActorAdmin     = "admin"
	ActorEmployee  = "employee"
	ActorAnonymous = "anonymous"
)

// Sensitive 敏感值在审计详情里的占位符。
// Detail 里凡是需要表达「这个值改了但不记录内容」的地方一律用它。
const Sensitive = "******"

// Entry 一条审计记录（调用方按需填字段，未填的由 Record 从 gin 上下文补齐）。
type Entry struct {
	Action     string         // 见动作常量
	TargetType string         // setting / admin / company / employee ...
	TargetID   string         // 目标标识（字符串，兼容非数字主键）
	Detail     map[string]any // 变更摘要（敏感值用 Sensitive 占位）
	Result     string         // ResultSuccess / ResultFail；为空视同 success
	ActorName  string         // 可选：接口已知用户名时传入（登录失败时还没有身份）
}

// Record 写入一条审计日志：操作者身份优先从 gin 上下文取（中间件已注入），
// 取不到时降级为 anonymous（如登录接口本身失败）。
// 写入失败只打印 [WARN] 并继续——审计是旁路，不能因为它失败就让业务请求失败。
func Record(db *gorm.DB, c *gin.Context, e Entry) {
	if db == nil {
		return
	}
	actorType, actorID, companyID := ActorAnonymous, int64(0), int64(0)
	if c != nil {
		actorID = c.GetInt64("staffId")
		companyID = c.GetInt64("companyId")
		switch {
		case actorID == 0:
			actorType = ActorAnonymous
		case c.GetString("role") == "employee":
			actorType = ActorEmployee
		default:
			actorType = ActorAdmin
		}
	}
	result := e.Result
	if result == "" {
		result = ResultSuccess
	}
	// 操作人姓名：调用方没给（多数接口只知道自己的 id）时按 id 反查一次。
	// 不能省——日志里只有 actor_id 的话，管理员还得自己去 admin 表对号，等于没记。
	actorName := e.ActorName
	if actorName == "" && actorID > 0 {
		actorName = lookupName(db, actorType, actorID)
	}

	detail := ""
	if len(e.Detail) > 0 {
		if b, err := json.Marshal(e.Detail); err == nil {
			detail = string(b)
		}
	}

	ip := ""
	if c != nil {
		ip = c.ClientIP()
	}

	row := model.AuditLog{
		ActorType:  actorType,
		ActorID:    actorID,
		ActorName:  actorName,
		CompanyID:  companyID,
		Action:     e.Action,
		TargetType: e.TargetType,
		TargetID:   e.TargetID,
		Detail:     detail,
		IP:         ip,
		Result:     result,
		CreatedAt:  time.Now(),
	}
	if err := db.Create(&row).Error; err != nil {
		log.Printf("[WARN] audit record failed (action=%s): %v", e.Action, err)
	}
}

// lookupName 按操作人类型与 id 反查姓名（后台操作低频，多一次查询可接受）。
// 查不到时返回空串而不是报错：审计不能因为名字查不到就整条不记。
func lookupName(db *gorm.DB, actorType string, actorID int64) string {
	switch actorType {
	case ActorAdmin:
		var a model.Admin
		if err := db.Select("username").First(&a, "id = ?", actorID).Error; err == nil {
			return a.Username
		}
	case ActorEmployee:
		var e model.Employee
		if err := db.Select("name").First(&e, "staff_id = ?", actorID).Error; err == nil {
			return e.Name
		}
	}
	return ""
}

// DetailKeys 把「变更了哪些设置项」转成审计详情；敏感项只记「已修改」，不落明文。
// isSecret 由调用方传入（setting.IsSecret），避免本包反向依赖 setting 的内部实现。
func DetailKeys(values map[string]string, isSecret func(string) bool) map[string]any {
	out := make(map[string]any, len(values))
	for k, v := range values {
		if isSecret != nil && isSecret(k) {
			// 敏感项：只表达「改动了」，值的有无用 changed/cleared 区分
			if v == "" {
				out[k] = map[string]any{"changed": true, "cleared": true}
			} else {
				out[k] = Sensitive
			}
			continue
		}
		out[k] = v
	}
	return out
}

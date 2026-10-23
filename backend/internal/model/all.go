package model

// All 返回需要建表的全部业务模型，是建表清单的**唯一权威来源**。
//
// 历史上这份清单被复制了三份（cmd/devserver 的 devModels、handler_test 的两处
// AutoMigrate），新增模型时漏改任何一份都不报编译错，只在运行期报
// `no such table`——曾因漏加 RevokedSession，被中间件「查询出错按已吊销处理」
// 放大成「登录成功后所有请求 401」。现在三处都改为调用本函数。
//
// 不含 `setting.Setting`：sys_setting 由 `setting.Init` 自行 AutoMigrate
// （两个入口都在建表后调用它），且 setting 包依赖 model，反向引用会成环。
func All() []interface{} {
	return []interface{}{
		&Employee{}, &CompanyInfo{}, &CompanyProfile{},
		&AchievementCategory{}, &ProjectCase{}, &BizService{},
		&HomeLayout{}, &StatEvent{}, &Admin{},
		&OpenApply{}, &CardModifyApply{},
		&RevokedSession{}, &Company{},
		&AuditLog{}, // 操作审计日志（后台写操作与登录失败留痕）
		&RateLimitHit{}, // 限流计数（多副本共享，见该类型注释）
	}
}

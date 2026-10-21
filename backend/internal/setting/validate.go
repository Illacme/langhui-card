package setting

import (
	"fmt"
	"strconv"
	"strings"

	"weichuan/internal/model"

	"gorm.io/gorm"
)

// 业务校验（def.Validate）：类型校验之外的取值约束。
//
// 取舍：**只给「填错会造成大面积故障、或把管理员自己锁在系统外」的项加校验**，
// 不做普遍校验。原因：
//   - 普遍校验会让运营改个参数处处碰壁，而多数项填错了是可自愈的（改回来即可）；
//   - 校验本身也是代码，加得越多越容易与实际读取逻辑走偏。
//
// 反过来，下面这几项必须拦在入口：
//   - 上传上限填 0 → 所有上传被拒，且报错信息指向文件而不是配置，排查成本极高；
//   - 基址漏写 http:// → 小程序冷启动就打到错误地址，而「后台下发正确基址」只在
//     种子地址能连通一次之后才生效，等于永远自愈不了（见 README 的部署注意）；
//   - 存储驱动写错 → 重启后静默降级本地磁盘，运营以为在用对象存储；
//   - 锁定租户填了不存在 / 已停用的企业 → 小程序全部接口报「企业不存在或已停用」，
//     后台却看不到任何异常。

// intRange 整数上下限（含边界）。int 类型不允许空串，故不在此放行空值。
func intRange(min, max int) func(db *gorm.DB, value string) error {
	return func(_ *gorm.DB, value string) error {
		n, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil {
			return fmt.Errorf("必须为整数，收到 %q", value)
		}
		if n < min || n > max {
			return fmt.Errorf("取值需在 %d ~ %d 之间，收到 %d", min, max, n)
		}
		return nil
	}
}

// httpURLOrEmpty 允许留空（空 = 回退环境变量或内置缺省）；非空必须是 http(s) 完整地址。
func httpURLOrEmpty() func(db *gorm.DB, value string) error {
	return func(_ *gorm.DB, value string) error {
		s := strings.TrimSpace(value)
		if s == "" {
			return nil
		}
		if !strings.HasPrefix(s, "http://") && !strings.HasPrefix(s, "https://") {
			return fmt.Errorf("必须为 http:// 或 https:// 开头的完整地址，收到 %q", s)
		}
		return nil
	}
}

// oneOf 枚举校验；空串放行（表示「未在界面设置」，回退环境变量）。
func oneOf(allowed ...string) func(db *gorm.DB, value string) error {
	return func(_ *gorm.DB, value string) error {
		s := strings.TrimSpace(value)
		if s == "" {
			return nil
		}
		for _, a := range allowed {
			if s == a {
				return nil
			}
		}
		return fmt.Errorf("只能是 %s 之一，收到 %q", strings.Join(allowed, " / "), s)
	}
}

// companyEnabledOrZero 独立版锁定租户：0 / 空 = 平台模式（不锁定）；
// 非 0 必须是**存在且启用**的企业。
func companyEnabledOrZero(db *gorm.DB, value string) error {
	s := strings.TrimSpace(value)
	if s == "" {
		return nil
	}
	id, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return fmt.Errorf("必须为企业 ID（整数），收到 %q", value)
	}
	if id == 0 {
		return nil // 平台模式
	}
	// 未初始化（Store.db == nil）时无从查证，放行：此时也没有小程序在用这份配置
	if db == nil {
		return nil
	}
	var cnt int64
	if err := db.Model(&model.Company{}).
		Where("id = ? AND status = ?", id, "enabled").Count(&cnt).Error; err != nil {
		return fmt.Errorf("校验租户失败：%w", err)
	}
	if cnt == 0 {
		return fmt.Errorf("企业 %d 不存在或已停用，锁定后小程序将全部不可用", id)
	}
	return nil
}

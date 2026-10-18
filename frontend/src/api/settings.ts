import http from './request'

// 系统设置：把产品运行参数从「代码写死 / 环境变量」收敛为后台可维护的表数据。
// 与后端 sys_setting 表 + internal/setting 的 registry 一一对应。
// 密钥类（DB DSN / JWT / 云存储与 CDN 凭据 / 微信 AppSecret）不在此列，始终留环境变量。
export interface SettingItem {
  key: string
  group: string
  value: string
  // int | string | bool | string_slice（后端按此校验，前端据此渲染控件）
  type: string
  description: string
}

// 平台超管读取全部可配置项（GET /api/admin/settings）
export function getSettings() {
  return http.get<{ code: number; data?: SettingItem[] }>('/admin/settings')
}

// 平台超管批量更新（PUT /api/admin/settings）。
// 值统一为字符串（与后端 map[string]string 契约一致），由后端按 type 校验。
// 后端即时刷新内存缓存，无需重启即可生效。
export function updateSettings(values: Record<string, string>) {
  return http.put('/admin/settings', values).then(() => true)
}

// 公开配置（GET /api/public/settings）：仅返回非敏感项（品牌名 / 小程序基址 / 锁定租户），
// 无需登录——小程序冷启动与后台品牌展示都走这个接口。
export function getPublicSettings() {
  return http.get<{ code: number; data?: Record<string, string> }>('/public/settings')
}

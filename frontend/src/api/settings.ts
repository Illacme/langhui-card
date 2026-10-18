import http from './request'

// 系统设置：把产品运行参数从「代码写死 / 环境变量」收敛为后台可维护的表数据。
// 与后端 sys_setting 表 + internal/setting 的 registry 一一对应。
export interface SettingItem {
  key: string
  group: string
  value: string
  // int | string | bool | string_slice（后端按此校验，前端据此渲染控件）
  type: string
  description: string
  // --- 生效方式元数据（后端 registry 下发，前端据此渲染提示与控件形态）---
  /** 敏感项：value 恒为掩码 '******'，真实值不回传；留空提交表示「回退环境变量」 */
  secret: boolean
  /** 保存后需重启服务才生效（云存储 / CDN 客户端在启动时构造） */
  restart: boolean
  /** 只读项：只能改环境变量后重启，界面禁用编辑 */
  readOnly: boolean
  /** 对应环境变量名（未设置时回退它，界面也据此提示改法） */
  envVar: string
  /** 当前是否已有有效值（库值或环境变量） */
  configured: boolean
}

// 平台超管读取全部可配置项（GET /api/admin/settings）
export function getSettings() {
  return http.get<{ code: number; data?: SettingItem[] }>('/admin/settings')
}

// 平台超管批量更新（PUT /api/admin/settings）。
// 值统一为字符串（与后端 map[string]string 契约一致），由后端按 type 校验。
// 只提交变更项：敏感项不带入掩码，未修改的项不会误清成空值。
// 回包会点名「需重启才生效」的项（restartKeys），前端据此提示，避免「改了没反应」的误解。
export interface UpdateSettingsResp {
  code: number
  msg?: string
  data?: { restartKeys?: string[] }
}
export function updateSettings(values: Record<string, string>) {
  return http.put<UpdateSettingsResp>('/admin/settings', values)
}

// 公开配置（GET /api/public/settings）：仅返回非敏感项（品牌名 / 小程序基址 / 锁定租户），
// 无需登录——小程序冷启动与后台品牌展示都走这个接口。
export function getPublicSettings() {
  return http.get<{ code: number; data?: Record<string, string> }>('/public/settings')
}

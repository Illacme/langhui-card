import http from './request'

// 操作审计日志（只读查询）。
// 后台能改系统设置（含密钥）、重置他人密码、删除超管、启停企业，这些操作必须留痕，
// 否则出事无法回答「谁在什么时候改了什么」。
export interface AuditLog {
  id: number
  actorType: string // admin / employee / anonymous
  actorId: number
  actorName: string // 冗余用户名：账号删除后仍可追溯
  companyId: number
  action: string // setting.update / admin.reset_password ...
  targetType: string
  targetId: string
  detail: string // JSON 变更摘要（敏感值已脱敏为 ******）
  ip: string
  result: string // success / fail
  createdAt: string
}

export interface AuditLogPage {
  total: number
  page: number
  size: number
  list: AuditLog[]
}

// 查询审计日志（平台超管）。支持 action / actorType / actorId / result 过滤与分页。
export function listAuditLogs(params: {
  page?: number
  size?: number
  action?: string
  actorType?: string
  actorId?: string
  result?: string
}) {
  return http.get<{ code: number; data?: AuditLogPage }>('/admin/audit-logs', { params })
}

// 动作的中文名：界面给运营看，不能把 setting.update 这样的键直接摊开。
export const ACTION_LABELS: Record<string, string> = {
  'setting.update': '修改系统设置',
  'login.fail': '登录失败',
  'login.success': '登录成功',
  logout: '登出',
  'password.change': '修改自己的密码',
  'admin.create': '新增平台超管',
  'admin.rename': '改管理员用户名',
  'admin.reset_password': '重置管理员密码',
  'admin.delete': '删除平台超管',
  'company.create': '新建企业',
  'company.update': '编辑企业',
  'company.status': '启停企业',
  'company.admin.create': '新增企业管理员',
  'company.admin.reset_password': '重置企业管理员密码',
  'employee.create': '新增员工',
  'employee.update': '编辑员工',
  'employee.reset_password': '重置员工密码',
  'employee.status': '启停员工',
  'apply.approve': '通过申请',
  'apply.reject': '驳回申请',
}

// 审计日志只查不写：记录一旦可改就等于没有审计，因此不提供任何写接口。
export function actionLabel(action: string): string {
  return ACTION_LABELS[action] || action
}

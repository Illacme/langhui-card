import http from './request'

// 后台登录身份（多租户两级）：平台超管 companyId=0/isPlatform=true；
// 企业管理员 companyId=企业 id、companyName=企业名、companyShortName=企业简称
// （侧边栏「xxxx · 管理后台」品牌名，取自企业管理里设置的简称）。
export interface AdminProfile {
  adminId: number
  username: string
  companyId: number
  companyName: string
  companyShortName: string
  isPlatform: boolean
}

// 管理员登录（平台超管 / 企业管理员均可）：成功时服务端写入 httpOnly Cookie
// （access/refresh），响应体仅回显非敏感身份信息，令牌不下发到 JS。
export function adminLogin(username: string, password: string) {
  return http.post<{
    code: number
    expiresIn?: number
    companyId?: number
    companyName?: string
    companyShortName?: string
    isPlatform?: boolean
  }>('/admin/login', { username, password })
}

// 管理员登出：通知服务端清除 httpOnly Cookie
export function adminLogout() {
  return http.post('/logout').then(() => true)
}

// 超管修改自己密码（{ old, new }）；改密后 token_version 自增，旧会话立即失效，需重新登录
export function changeAdminPassword(oldPwd: string, newPwd: string) {
  return http.put('/admin/password', { old: oldPwd, new: newPwd }).then(() => true)
}

// 当前登录身份：令牌在 httpOnly Cookie 中 JS 不可读，页面刷新后据此恢复
// 「平台超管 / 企业管理员」身份与当前企业展示（GET /api/admin/me）。
export function getMe() {
  return http.get<{ code: number; data?: AdminProfile }>('/admin/me')
}

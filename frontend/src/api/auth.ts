import http from './request'

// 超管登录：成功时服务端写入 httpOnly Cookie（access/refresh），响应体仅返回 { code:0, expiresIn }，
// 令牌不下发到 JS。这里只负责触发登录并校验成功。
export function adminLogin(username: string, password: string) {
  return http.post('/admin/login', { username, password }).then(() => true)
}

// 超管登出：通知服务端清除 httpOnly Cookie
export function adminLogout() {
  return http.post('/logout').then(() => true)
}

// 超管修改自己密码（{ old, new }）；改密后 token_version 自增，旧会话立即失效，需重新登录
export function changeAdminPassword(oldPwd: string, newPwd: string) {
  return http.put('/admin/password', { old: oldPwd, new: newPwd }).then(() => true)
}

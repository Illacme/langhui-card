import http from './request'

// 超管登录 → 返回 access + refresh 令牌（响应字段在根：{ code:0, token, refreshToken, expiresIn }）
export function adminLogin(username: string, password: string) {
  return http
    .post('/admin/login', { username, password })
    .then((r: any) => ({
      token: r.token as string,
      refreshToken: r.refreshToken as string,
    }))
}

// 超管修改自己密码（{ old, new }）
export function changeAdminPassword(oldPwd: string, newPwd: string) {
  return http.put('/admin/password', { old: oldPwd, new: newPwd }).then(() => true)
}

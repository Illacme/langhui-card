import http from './request'

// 超管登录 → 返回 token（响应字段在根：{ code:0, token }）
export function adminLogin(username: string, password: string) {
  return http
    .post('/admin/login', { username, password })
    .then((r: any) => r.token as string)
}

// 超管修改自己密码（{ old, new }）
export function changeAdminPassword(oldPwd: string, newPwd: string) {
  return http.put('/admin/password', { old: oldPwd, new: newPwd }).then(() => true)
}

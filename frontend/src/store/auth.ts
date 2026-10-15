import { defineStore } from 'pinia'

// 超管登录态。PC 后台改用 httpOnly Cookie 鉴权后，令牌由浏览器持有、JS 不可读，
// 故这里只保存非敏感信息：用户名 + 登录标记，用于路由守卫与界面展示。
export const useAuthStore = defineStore('auth', {
  state: () => ({
    username: localStorage.getItem('admin_user') || '',
    loggedIn: localStorage.getItem('admin_loggedin') === '1',
  }),
  getters: {
    isLoggedIn: (s) => s.loggedIn,
  },
  actions: {
    // 登录成功（令牌已由服务端写入 httpOnly Cookie）：仅记录非敏感登录态
    setAuth(username: string) {
      this.username = username
      this.loggedIn = true
      localStorage.setItem('admin_user', username)
      localStorage.setItem('admin_loggedin', '1')
    },
    logout() {
      this.username = ''
      this.loggedIn = false
      localStorage.removeItem('admin_user')
      localStorage.removeItem('admin_loggedin')
    },
  },
})

import { defineStore } from 'pinia'

// 超管登录态：token / refreshToken 持久化到 localStorage（key 与 UploadField.vue / axios 拦截器保持一致）。
// refreshToken 用于 access 过期后静默换发新 access，避免频繁重登（见 api/request.ts）。
export const useAuthStore = defineStore('auth', {
  state: () => ({
    token: localStorage.getItem('token') || '',
    refreshToken: localStorage.getItem('refreshToken') || '',
    username: localStorage.getItem('admin_user') || '',
  }),
  getters: {
    isLoggedIn: (s) => !!s.token,
  },
  actions: {
    // refreshToken 可选：登录成功时一并写入；缺省仅更新 access（如静默刷新场景由 setToken 处理）
    setAuth(token: string, username: string, refreshToken?: string) {
      this.token = token
      this.username = username
      localStorage.setItem('token', token)
      localStorage.setItem('admin_user', username)
      if (refreshToken) {
        this.refreshToken = refreshToken
        localStorage.setItem('refreshToken', refreshToken)
      }
    },
    // 静默刷新后仅更新 access 令牌
    setToken(token: string) {
      this.token = token
      localStorage.setItem('token', token)
    },
    logout() {
      this.token = ''
      this.refreshToken = ''
      this.username = ''
      localStorage.removeItem('token')
      localStorage.removeItem('refreshToken')
      localStorage.removeItem('admin_user')
    },
  },
})

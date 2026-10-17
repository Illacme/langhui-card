import { defineStore } from 'pinia'
import type { AdminProfile } from '@/api/auth'

// 后台登录态。PC 后台改用 httpOnly Cookie 鉴权后，令牌由浏览器持有、JS 不可读，
// 故这里只保存非敏感信息：用户名 + 登录标记 + 租户身份，用于路由守卫与界面展示。
// 多租户两级身份：
//   - isPlatform=true（平台超级管理员）：可见全部租户，独享「企业管理」入口；
//   - isPlatform=false（企业管理员）：仅见本企业，界面展示当前企业名；
//     companyShortName 为企业简称（侧边栏「xxxx · 管理后台」品牌名，企业管理员处设置），
//     与公司信息里仅用于小程序展示的简称相互独立。
export const useAuthStore = defineStore('auth', {
  state: () => ({
    username: localStorage.getItem('admin_user') || '',
    loggedIn: localStorage.getItem('admin_loggedin') === '1',
    companyId: Number(localStorage.getItem('admin_company_id') || 0),
    companyName: localStorage.getItem('admin_company_name') || '',
    companyShortName: localStorage.getItem('admin_company_short_name') || '',
    isPlatform: localStorage.getItem('admin_is_platform') === '1',
    profileLoaded: false, // 本次会话是否已从 /api/admin/me 恢复过身份
  }),
  getters: {
    isLoggedIn: (s) => s.loggedIn,
  },
  actions: {
    // 登录成功（令牌已由服务端写入 httpOnly Cookie）：记录非敏感登录态 + 租户身份
    setAuth(username: string, profile?: Partial<AdminProfile>) {
      this.username = username
      this.loggedIn = true
      if (profile) {
        this.companyId = profile.companyId ?? 0
        this.companyName = profile.companyName ?? ''
        this.companyShortName = profile.companyShortName ?? ''
        this.isPlatform = profile.isPlatform ?? false
      }
      this.profileLoaded = !!profile
      this.persist()
    },
    // 页面刷新后用 /api/admin/me 恢复的身份（localStorage 可能过期，以服务端为准）
    hydrate(profile: AdminProfile) {
      this.username = profile.username
      this.loggedIn = true
      this.companyId = profile.companyId
      this.companyName = profile.companyName
      this.companyShortName = profile.companyShortName
      this.isPlatform = profile.isPlatform
      this.profileLoaded = true
      this.persist()
    },
    logout() {
      this.username = ''
      this.loggedIn = false
      this.companyId = 0
      this.companyName = ''
      this.companyShortName = ''
      this.isPlatform = false
      this.profileLoaded = false
      localStorage.removeItem('admin_user')
      localStorage.removeItem('admin_loggedin')
      localStorage.removeItem('admin_company_id')
      localStorage.removeItem('admin_company_name')
      localStorage.removeItem('admin_company_short_name')
      localStorage.removeItem('admin_is_platform')
    },
    persist() {
      localStorage.setItem('admin_user', this.username)
      localStorage.setItem('admin_loggedin', '1')
      localStorage.setItem('admin_company_id', String(this.companyId))
      localStorage.setItem('admin_company_name', this.companyName)
      localStorage.setItem('admin_company_short_name', this.companyShortName)
      localStorage.setItem('admin_is_platform', this.isPlatform ? '1' : '0')
    },
  },
})

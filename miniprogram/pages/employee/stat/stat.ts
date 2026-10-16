import { request } from '../../../utils/request'

// 员工数据统计页（B10）：总访问 / 拨号点击 / 保存点击。
// 从「我的」页 navigateTo 进入；onShow 拉取而非 onLoad——从后台返回时刷新。
// 401（令牌失效，request 封装已清 storage）时 redirect 回登录页，避免卡在空页。
Page({
  data: {
    stat: null as any,
    loading: true,
  },
  onShow() {
    this.load()
  },
  async load() {
    if (!wx.getStorageSync('empToken')) {
      wx.redirectTo({ url: '/pages/employee/login/login' })
      return
    }
    this.setData({ loading: true })
    const r = await request('GET', '/api/employee/me/stat')
    this.setData({ loading: false })
    if (r.code === 0 && r.data) {
      this.setData({ stat: r.data })
    } else if (r.code === 401) {
      wx.redirectTo({ url: '/pages/employee/login/login' })
    } else {
      wx.showToast({ title: r.msg || '加载失败', icon: 'none' })
    }
  },
})

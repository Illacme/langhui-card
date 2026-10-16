import { request } from '../../../utils/request'

Page({
  data: {
    phone: '',
    password: '',
  },
  onInput(e: any) {
    this.setData({ [e.currentTarget.dataset.field]: e.detail.value })
  },
  async onSubmit() {
    const { phone, password } = this.data
    if (!phone || !password) {
      wx.showToast({ title: '请输入手机号和密码', icon: 'none' })
      return
    }
    const r = await request('POST', '/api/employee/login', { phone, password })
    if (r.code === 0 && r.token) {
      wx.setStorageSync('empToken', r.token)
      if (r.refreshToken) wx.setStorageSync('empRefreshToken', r.refreshToken)
      // 登录后回「我的」tab（由它拉 /api/employee/me 拿资料并记 myStaffId）；
      // tabBar 页必须用 switchTab，redirectTo 打不开
      wx.switchTab({ url: '/pages/employee/me/me' })
    } else {
      wx.showToast({ title: r.msg || '登录失败', icon: 'none' })
    }
  },
})

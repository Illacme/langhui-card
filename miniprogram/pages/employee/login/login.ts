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
      wx.redirectTo({ url: '/pages/employee/edit/edit' })
    } else {
      wx.showToast({ title: r.msg || '登录失败', icon: 'none' })
    }
  },
})

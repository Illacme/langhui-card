import { request } from '../../../utils/request'
import { getOpenid } from '../../../utils/wechat'

Page({
  data: {
    phone: '',
    password: '',
    submitting: false,
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
    // 只静默采集 openid（wx.login 换取，无需弹窗）供后台核实身份；
    // 不再弹昵称授权框——昵称在申请开通时由用户显式授权填写，登录不重复索取。
    this.setData({ submitting: true })
    const openid = await getOpenid()
    const r = await request('POST', '/api/employee/login', { phone, password, openid })
    this.setData({ submitting: false })
    if (r.code === 0 && r.token) {
      wx.setStorageSync('empToken', r.token)
      if (r.refreshToken) wx.setStorageSync('empRefreshToken', r.refreshToken)
      // 登录页由「我的」页 navigateTo 打开：登录成功后返回上一页（即「我的」），
      // 其 onShow 会重新拉取员工资料并显示已登录态。兜底 reLaunch 到「我的」。
      wx.navigateBack({
        delta: 1,
        fail: () => wx.reLaunch({ url: '/pages/employee/me/me' }),
      })
    } else {
      wx.showToast({ title: r.msg || '登录失败', icon: 'none' })
    }
  },
})

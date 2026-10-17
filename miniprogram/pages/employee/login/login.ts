import { request } from '../../../utils/request'
import { getOpenid, getWechatNickname } from '../../../utils/wechat'

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
    // 采集微信身份（openid 静默 + 昵称授权弹窗）：
    // 两个函数在此同步触发，getWechatNickname 里的 wx.getUserProfile 才处于点击手势上下文、能弹出授权框。
    // 任一失败都降级为空串，不阻塞登录。
    this.setData({ submitting: true })
    const [openid, nickname] = await Promise.all([getOpenid(), getWechatNickname()])
    const r = await request('POST', '/api/employee/login', { phone, password, openid, nickname })
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

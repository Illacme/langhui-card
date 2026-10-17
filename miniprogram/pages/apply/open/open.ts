import { request, apiBaseUrl } from '../../../utils/request'
import { getOpenid, getWechatNickname } from '../../../utils/wechat'

// 访客自助提交开通申请（POST /api/apply/open）。
// 头像：默认读取用户微信头像（按钮 open-type=chooseAvatar 会直接调起微信头像选择器，
// 默认展示用户当前微信头像），同时支持从相册/拍照上传——两者都拿到本地临时文件后，
// 经专用公开端点 /api/apply/open/avatar 上传拿回永久 URL 再随表单提交。
Page({
  data: {
    form: { name: '', phone: '', title: '', landline: '', email: '', avatar: '' },
    saving: false,
    submitted: false,
  },
  onInput(e: any) {
    const field = e.currentTarget.dataset.field
    const form = this.data.form as any
    form[field] = e.detail.value
    this.setData({ form })
  },
  // 「使用微信头像」：open-type=chooseAvatar 回调拿到本地临时文件，直接上传
  onChooseAvatar(e: any) {
    const url = e.detail && e.detail.avatarUrl
    if (url) this.uploadAvatar(url)
  },
  // 「上传头像」：从相册/拍照选图后上传
  chooseImage() {
    const that = this
    wx.chooseMedia({
      count: 1,
      mediaType: ['image'],
      success(res: any) {
        const filePath = res.tempFiles[0].tempFilePath
        that.uploadAvatar(filePath)
      },
    })
  },
  // 头像上传：调公开端点（无需登录），返回永久 URL 写回表单
  uploadAvatar(filePath: string) {
    wx.showLoading({ title: '上传中' })
    wx.uploadFile({
      url: apiBaseUrl() + '/api/apply/open/avatar',
      filePath,
      name: 'file',
      success(r: any) {
        wx.hideLoading()
        let body: any = {}
        try {
          body = JSON.parse(r.data)
        } catch {}
        if (body.code === 0 && body.data && body.data.urls && body.data.urls.length) {
          const form = this.data.form as any
          form.avatar = body.data.urls[0]
          this.setData({ form })
          wx.showToast({ title: '头像已设置' })
        } else {
          wx.showToast({ title: body.msg || '上传失败', icon: 'none' })
        }
      },
      fail() {
        wx.hideLoading()
        wx.showToast({ title: '上传失败', icon: 'none' })
      },
    })
  },
  async onSubmit() {
    const { name, phone } = this.data.form
    if (!name || !phone) {
      wx.showToast({ title: '请填写姓名与手机', icon: 'none' })
      return
    }
    this.setData({ saving: true })
    // 采集微信身份（openid 静默 + 昵称授权弹窗），随申请提交供后台核实。
    // 两个函数在此同步触发，保证 getWechatNickname 的 wx.getUserProfile 处于点击手势上下文。
    // 任一失败都降级为空串，不阻塞提交。
    const [openid, nickname] = await Promise.all([getOpenid(), getWechatNickname()])
    try {
      const r = await request('POST', '/api/apply/open', { ...this.data.form, openid, nickname })
      if (r.code === 0) {
        this.setData({ submitted: true })
      } else {
        wx.showToast({ title: r.msg || '提交失败', icon: 'none' })
      }
    } finally {
      this.setData({ saving: false })
    }
  },
  backMe() {
    wx.navigateBack({
      delta: 1,
      fail: () => wx.reLaunch({ url: '/pages/employee/me/me' }),
    })
  },
})

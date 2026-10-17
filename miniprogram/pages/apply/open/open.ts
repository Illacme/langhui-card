import { request, apiBaseUrl } from '../../../utils/request'
import { getOpenid, getWechatNickname } from '../../../utils/wechat'

// 访客自助提交开通申请（POST /api/apply/open）。
// 头像：默认读取用户微信头像（按钮 open-type=chooseAvatar 会直接调起微信头像选择器，
// 默认展示用户当前微信头像），同时支持从相册/拍照上传——两者都拿到本地临时文件后，
// 经专用公开端点 /api/apply/open/avatar 上传拿回永久 URL 再随表单提交。
// 多租户：请求层按 utils/tenant.ts 自动带上 companyId（独立版锁定值 / 平台模式粘滞值），
// 申请即归属该企业，由该企业的管理员审核。
Page({
  data: {
    form: { name: '', phone: '', title: '', landline: '', email: '', avatar: '', nickname: '' },
    // 申请目标企业名（GET /api/company）：让访客知道在申请加入哪家企业
    companyName: '',
    saving: false,
    submitted: false,
  },
  onLoad() {
    this.loadCompany()
  },
  // 拉取申请目标企业名；失败（网络/企业停用）时隐藏该行，不阻断申请
  loadCompany() {
    request('GET', '/api/company')
      .then((r: any) => {
        if (r && r.code === 0 && r.data) {
          this.setData({ companyName: r.data.shortName || r.data.name || '' })
        }
      })
      .catch(() => {})
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
  // 「授权获取」微信昵称：getUserProfile 弹窗（需在点击手势内调用），成功后展示在「微信昵称」行，
  // 提交时随表单带上。用户可跳过——昵称仅作后台核实用途，不填也能提交。
  authorizeNickname() {
    wx.showLoading({ title: '获取中' })
    getWechatNickname().then((nickname) => {
      wx.hideLoading()
      if (nickname) {
        const form = this.data.form as any
        form.nickname = nickname
        this.setData({ form })
      } else {
        wx.showToast({ title: '未获取到昵称（可跳过）', icon: 'none' })
      }
    })
  },
  async onSubmit() {
    const { name, phone } = this.data.form
    if (!name || !phone) {
      wx.showToast({ title: '请填写姓名与手机', icon: 'none' })
      return
    }
    this.setData({ saving: true })
    // openid 静默采集（wx.login 换取，无需弹窗）；昵称由用户点「授权获取」显式授权后已在 form.nickname，
    // 此处只补 openid，不再触发昵称弹窗（避免提交时盲弹）。
    const openid = await getOpenid()
    try {
      const r = await request('POST', '/api/apply/open', { ...this.data.form, openid })
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

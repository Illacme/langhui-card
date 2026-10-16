import { request, apiBaseUrl } from '../../../utils/request'

Page({
  data: {
    form: { avatar: '', name: '', title: '', phone: '', landline: '', email: '' },
    loading: true,
  },
  onLoad() {
    this.loadMe()
  },
  async loadMe() {
    const r = await request('GET', '/api/employee/me')
    if (r.code === 401) {
      wx.redirectTo({ url: '/pages/employee/login/login' })
      return
    }
    if (r.data) {
      this.setData({ form: r.data, loading: false })
    } else {
      this.setData({ loading: false })
    }
  },
  onInput(e: any) {
    const field = e.currentTarget.dataset.field
    const form = this.data.form as any
    form[field] = e.detail.value
    this.setData({ form })
  },
  // 选图并上传头像（沿用 PC 端 UploadField 思路：调后端上传接口拿回 URL）
  chooseAvatar() {
    const that = this
    wx.chooseMedia({
      count: 1,
      mediaType: ['image'],
      success(res: any) {
        const filePath = res.tempFiles[0].tempFilePath
        wx.showLoading({ title: '上传中' })
        wx.uploadFile({
          url: apiBaseUrl() + '/api/employee/upload',
          filePath,
          name: 'file',
          header: { Authorization: 'Bearer ' + (wx.getStorageSync('empToken') || '') },
          success(r: any) {
            wx.hideLoading()
            const body = JSON.parse(r.data)
            if (body.code === 0 && body.data && body.data.urls && body.data.urls.length) {
              const form = that.data.form as any
              form.avatar = body.data.urls[0]
              that.setData({ form })
              wx.showToast({ title: '上传成功' })
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
    })
  },
  async onSubmit() {
    const r = await request('PUT', '/api/employee/me', this.data.form)
    if (r.code === 0) {
      wx.showToast({ title: '已提交，等待审核' })
    } else {
      wx.showToast({ title: r.msg || '提交失败', icon: 'none' })
    }
  },
})

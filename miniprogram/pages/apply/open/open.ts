import { request } from '../../../utils/request'

// 访客自助提交开通申请（POST /api/apply/open）。
// 本页刻意不做头像上传：/api/employee/upload 需员工 token，访客还没有账号，
// 放开匿名上传会成为滥用入口。头像由超管在 PC 后台补，或员工登录后自行上传。
Page({
  data: {
    form: { name: '', phone: '', title: '', email: '' },
    saving: false,
    submitted: false,
  },
  onInput(e: any) {
    const field = e.currentTarget.dataset.field
    const form = this.data.form as any
    form[field] = e.detail.value
    this.setData({ form })
  },
  async onSubmit() {
    const { name, phone } = this.data.form
    if (!name || !phone) {
      wx.showToast({ title: '请填写姓名与手机', icon: 'none' })
      return
    }
    this.setData({ saving: true })
    try {
      const r = await request('POST', '/api/apply/open', this.data.form)
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
    wx.switchTab({ url: '/pages/employee/me/me' })
  },
})

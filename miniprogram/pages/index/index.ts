import { request } from '../../utils/request'

Page({
  data: {
    staffId: 0,
    employee: null,
    company: null,
    profile: null,
    categories: [],
    featured: [],
    services: [],
    homeOrder: ['achievement', 'service'],
    disabled: false,
    notFound: false,
    loading: true,
  },
  onLoad(query: any) {
    let staffId = parseInt(query.staffId || '0', 10)
    // 从小程序码 scene 解析（形如 "staffId=12"）
    if (!staffId && query.scene) {
      const params = decodeURIComponent(query.scene)
      const m = /staffId=(\d+)/.exec(params)
      if (m) staffId = parseInt(m[1], 10)
    }
    // 仍无参数（直接打开 / 从「我的」tab 切回）：已登录员工用自己的编号，
    // 免去每次手输；未登录才弹窗输入（联调取数 / 演示用）
    if (!staffId) {
      const app = getApp() as any
      staffId = (app && app.globalData && app.globalData.myStaffId) || 0
    }
    if (staffId) {
      this.setData({ staffId })
      this.loadAll()
    } else {
      // 未携带 staffId（直接打开 / 无分享参数）：弹窗输入员工编号后进入（联调取数 / 演示用）
      this.setData({ loading: false })
      this.promptStaffId()
    }
  },
  // 输入员工编号进入名片（生产环境入口一般为分享链接 / 扫码，此处作为缺省兜底）
  promptStaffId() {
    const that = this
    wx.showModal({
      title: '输入员工编号',
      editable: true,
      placeholderText: '如 1',
      success(r: any) {
        if (!r.confirm) return
        const id = parseInt((r.content || '').trim(), 10)
        if (!id) {
          wx.showToast({ title: '编号无效', icon: 'none' })
          that.promptStaffId()
          return
        }
        that.setData({ staffId: id, loading: true })
        that.loadAll()
      },
    })
  },
  async loadAll() {
    const sid = this.data.staffId
    const [card, ach, svc, layout] = await Promise.all([
      request('GET', '/api/card?staffId=' + sid),
      request('GET', '/api/achievements'),
      request('GET', '/api/services'),
      request('GET', '/api/home-layout'),
    ])
    // 停用：后端返回 code=2（无 data）
    if (card.code === 2) {
      this.setData({ disabled: true, loading: false })
      return
    }
    // 不存在：后端返回 code=1
    if (card.code === 1 || !card.data || !card.data.employee) {
      this.setData({ notFound: true, loading: false })
      wx.showToast({ title: '员工不存在', icon: 'none' })
      return
    }
    this.setData({
      employee: card.data.employee,
      company: card.data.company,
      profile: card.data.profile,
      categories: ach.data ? ach.data.categories || [] : [],
      featured: ach.data ? ach.data.featured || [] : [],
      services: svc.data || [],
      homeOrder: layout.data && layout.data.order ? layout.data.order : ['achievement', 'service'],
      disabled: false,
      notFound: false,
      loading: false,
    })
    // 进入即记一次访问（与后端 /api/stat/track visit 对应）
    request('POST', '/api/stat/track', { staffId: sid, eventType: 'visit' })
  },
  onShareAppMessage() {
    return {
      title: this.data.company ? this.data.company.fullName : '朗汇机电',
      path: '/pages/index/index?staffId=' + this.data.staffId,
    }
  },
  goPoster() {
    wx.navigateTo({ url: '/pages/poster/poster?staffId=' + this.data.staffId })
  },
})

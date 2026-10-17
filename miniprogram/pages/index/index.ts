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
    error: false,
    // 自定义顶栏（navigationStyle=custom）：状态栏高度与整体高度，用于避让刘海/状态栏
    statusBarHeight: 20,
    navHeight: 64,
    // 左上角「我的」入口：已登录则显示头像 + 姓名，未登录显示「我的」
    meAvatar: '',
    meName: '',
    // 访问埋点去重标记：同一 staffId 在本页面实例内只上报一次。
    // onShow 语义（首页文档 §4）是「每次显示 +1」，但从「我的」switchTab 回名片页
    // 会再次触发 onShow——不去重的话切两个 tab 来回几下访问量就虚高。
    // 页面被销毁重建（重新进小程序）时 data 重置，会正常再计一次。
    lastVisitStaffId: 0,
  },
  onLoad(query: any) {
    // 自定义顶栏：先按设备状态栏高度算出避让高度，避免内容被刘海/状态栏遮挡
    let sb = 20
    try {
      const info = wx.getSystemInfoSync()
      if (info && info.statusBarHeight) sb = info.statusBarHeight
    } catch {}
    this.setData({ statusBarHeight: sb, navHeight: sb + 44 })

    let staffId = parseInt(query.staffId || '0', 10)
    // 从小程序码 scene 解析（形如 "staffId=12"）
    if (!staffId && query.scene) {
      const params = decodeURIComponent(query.scene)
      const m = /staffId=(\d+)/.exec(params)
      if (m) staffId = parseInt(m[1], 10)
    }
    // 仍无参数（直接打开 / 从「我的」页返回）：已登录员工用自己的编号，
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
  onShow() {
    // 埋点挂 onShow（首页文档 §4 明确要求）：名片每次显示记一次访问
    this.trackVisit()
    // 左上角「我的」入口跟随登录态刷新（登录 / 退出后回名片页时即时反映）
    this.loadMeInfo()
  },
  // 读取当前登录员工，用于在自定义顶栏的「我的」入口展示头像 + 姓名；
  // 无 token 时显示默认「我的」。失败（令牌失效）静默回退，不弹错。
  loadMeInfo() {
    const token = wx.getStorageSync('empToken')
    if (!token) {
      this.setData({ meAvatar: '', meName: '' })
      return
    }
    request('GET', '/api/employee/me')
      .then((r: any) => {
        if (r && r.code === 0 && r.data) {
          this.setData({ meAvatar: r.data.avatar || '', meName: r.data.name || '我的' })
        } else {
          this.setData({ meAvatar: '', meName: '' })
        }
      })
      .catch(() => this.setData({ meAvatar: '', meName: '' }))
  },
  // 访问埋点：带 lastVisitStaffId 去重（见 data 注释）。
  // loadAll 成功后也会调一次——onLoad 走弹窗输编号时，onShow 早于 staffId 就绪，
  // 只挂 onShow 会漏掉这条路径。
  trackVisit() {
    const sid = this.data.staffId
    if (!sid || sid === this.data.lastVisitStaffId) return
    this.setData({ lastVisitStaffId: sid })
    request('POST', '/api/stat/track', { staffId: sid, eventType: 'visit' })
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
        that.setData({ staffId: id })
        that.loadAll()
      },
    })
  },
  async loadAll() {
    const sid = this.data.staffId
    this.setData({ loading: true, error: false })
    try {
      const [card, ach, svc, layout] = await Promise.all([
        request('GET', '/api/card?staffId=' + sid),
        request('GET', '/api/achievements'),
        request('GET', '/api/services'),
        request('GET', '/api/home-layout'),
      ])
      // 停用：后端返回 code=2（无 employee）
      if (card.code === 2) {
        this.setData({ disabled: true, loading: false })
        return
      }
      // 不存在：后端返回 code=1（无 employee）。
      // 注意：/api/card 的 employee/company/profile 在响应根级（非 data 包裹），
      // /api/achievements 的 categories/featured、/api/home-layout 的 order 同样在根级，
      // 切勿误读成 card.data.* / ach.data.* / layout.data.*（此前 bug：卡片页恒走 notFound）。
      if (card.code === 1 || !card.employee) {
        this.setData({ notFound: true, loading: false })
        wx.showToast({ title: '员工不存在', icon: 'none' })
        return
      }
      this.setData({
        employee: card.employee,
        company: card.company,
        profile: card.profile,
        categories: ach.categories || [],
        featured: ach.featured || [],
        services: svc.data || [],
        homeOrder: layout.order && layout.order.length ? layout.order : ['achievement', 'service'],
        disabled: false,
        notFound: false,
        loading: false,
      })
      this.trackVisit()
    } catch {
      // 断网 / 接口异常：loading 必须落下来。此前 reject 直接冒泡出 async 函数，
      // 页面永远停在「加载中…」——这是真实缺陷，不是体验问题。
      this.setData({ loading: false, error: true })
    }
  },
  reload() {
    this.loadAll()
  },
  // 「案例进入」（首页文档 §2 名片区右上按钮）：业绩区在首页布局里 → 滚动过去（不离开名片页）；
  // 被布局隐藏 → 跳第一个分类的项目列表；一个分类都没有 → 提示
  onEnterCase() {
    const cats = this.data.categories || []
    if (!cats.length) {
      wx.showToast({ title: '暂无案例', icon: 'none' })
      return
    }
    const hasAchievement = (this.data.homeOrder || []).indexOf('achievement') >= 0
    if (hasAchievement) {
      wx.pageScrollTo({ selector: '#sec-achievement', duration: 300 })
      return
    }
    const url =
      '/pages/project/list/list?categoryId=' +
      cats[0].id +
      (this.data.staffId ? '&staffId=' + this.data.staffId : '')
    wx.navigateTo({ url })
  },
  onShareAppMessage() {
    const c = this.data.company
    return {
      title: (c && (c.shortName || c.fullName)) || '朗汇机电',
      path: '/pages/index/index?staffId=' + this.data.staffId,
    }
  },
  // 左上角「我的」入口：底部 tabBar 已移除，改由 navigateTo 进入「我的」页
  goMe() {
    wx.navigateTo({ url: '/pages/employee/me/me' })
  },
  goPoster() {
    wx.navigateTo({ url: '/pages/poster/poster?staffId=' + this.data.staffId })
  },
})

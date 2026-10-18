import { request, assetUrl } from '../../../utils/request'

// 员工「我的」页（tabBar）：未登录给登录 / 申请开通入口，已登录给名片与账号操作。
// 数据在 onShow 拉取而非 onLoad——从登录页 / 编辑页返回时需要刷新登录态与资料。
Page({
  data: {
    me: null as any,
    // 有待审名片修改时为 true：「编辑名片」单元格显示「审核中」
    pendingModify: false,
    pwdVisible: false,
    pwdSaving: false,
    pwd: { oldPwd: '', newPwd: '', confirmPwd: '' },
  },
  onShow() {
    this.loadMe()
  },
  async loadMe() {
    const token = wx.getStorageSync('empToken')
    if (!token) {
      this.setData({ me: null, pendingModify: false })
      return
    }
    const r = await request('GET', '/api/employee/me')
    if (r.code === 0 && r.data) {
      // 记到 globalData：「我的名片」switchTab 回名片页时按这个编号取数
      const app = getApp() as any
      app.globalData.myStaffId = r.data.staffId
      // 头像为后端相对路径，端上拼绝对（小程序 <image src> 不认相对）
      r.data.avatar = assetUrl(r.data.avatar)
      this.setData({ me: r.data, pendingModify: !!r.pendingModify })
    } else {
      // token 失效（拦截器已清 storage）：回到未登录态
      this.setData({ me: null, pendingModify: false })
    }
  },
  goLogin() {
    wx.navigateTo({ url: '/pages/employee/login/login' })
  },
  goApply() {
    wx.navigateTo({ url: '/pages/apply/open/open' })
  },
  // 返回名片首页：底部 tabBar 已移除，「我的」页由 navigateTo 进入，返回用 navigateBack；
  // 若栈内已无名片页（极端入口），兜底 reLaunch 回名片页
  goMyCard() {
    wx.navigateBack({
      delta: 1,
      fail: () => wx.reLaunch({ url: '/pages/index/index' }),
    })
  },
  // 生成海报（B9）：带本人编号进海报页，canvas 合成后存相册
  goPoster() {
    if (!this.data.me) return
    wx.navigateTo({ url: '/pages/poster/poster?staffId=' + this.data.me.staffId })
  },
  goEdit() {
    wx.navigateTo({ url: '/pages/employee/edit/edit' })
  },
  goStat() {
    wx.navigateTo({ url: '/pages/employee/stat/stat' })
  },
  openPwd() {
    this.setData({ pwdVisible: true, pwd: { oldPwd: '', newPwd: '', confirmPwd: '' } })
  },
  closePwd() {
    this.setData({ pwdVisible: false })
  },
  onPwdInput(e: any) {
    const field = e.currentTarget.dataset.field
    const pwd = this.data.pwd as any
    pwd[field] = e.detail.value
    this.setData({ pwd })
  },
  async submitPwd() {
    const { oldPwd, newPwd, confirmPwd } = this.data.pwd
    if (!oldPwd || !newPwd) {
      wx.showToast({ title: '请填写原密码与新密码', icon: 'none' })
      return
    }
    if (newPwd.length < 6) {
      wx.showToast({ title: '新密码至少 6 位', icon: 'none' })
      return
    }
    if (newPwd !== confirmPwd) {
      wx.showToast({ title: '两次输入的新密码不一致', icon: 'none' })
      return
    }
    this.setData({ pwdSaving: true })
    try {
      const r = await request('PUT', '/api/employee/password', { old: oldPwd, new: newPwd })
      if (r.code === 0) {
        wx.showToast({ title: '已修改，请重新登录' })
        this.setData({ pwdVisible: false })
        // 改密后安全戳自增，旧令牌已失效：直接清本地态回未登录
        setTimeout(() => this.logout(true), 800)
      } else {
        wx.showToast({ title: r.msg || '修改失败', icon: 'none' })
      }
    } finally {
      this.setData({ pwdSaving: false })
    }
  },
  // 退出登录：清令牌 + globalData.myStaffId，回到未登录态
  logout(silent = false) {
    const app = getApp() as any
    app.globalData.myStaffId = 0
    wx.removeStorageSync('empToken')
    wx.removeStorageSync('empRefreshToken')
    this.setData({ me: null, pendingModify: false, pwdVisible: false })
    if (!silent) wx.showToast({ title: '已退出登录' })
  },
})

import { request, assetUrl } from '../../utils/request'
import { track } from '../../utils/track'

Page({
  data: {
    staffId: 0,
    card: null as any,
    qrUrl: '',
    tempFilePath: '',
    // 加载 / 失败态：海报是「先取数据 → 再画 canvas → 再导出图片」三步串起来的，
    // 任一步挂了都必须落到失败态并给重试入口——否则用户只看到一块白板，
    // 既不知道在加载也不知道失败了，只能退出重进（此前 Promise.all 没有 catch，
    // 断网时 reject 直接冒泡出 async 函数，页面永远停在空白）。
    loading: true,
    error: false,
    errorText: '海报生成失败',
  },
  onLoad(query: any) {
    let staffId = parseInt(query.staffId || '0', 10)
    if (!staffId && query.scene) {
      const params = decodeURIComponent(query.scene)
      const m = /staffId=(\d+)/.exec(params)
      if (m) staffId = parseInt(m[1], 10)
    }
    this.setData({ staffId })
    this.prepare()
  },
  async prepare() {
    const sid = this.data.staffId
    // 重试时必须清掉上一张的 tempFilePath：否则导出失败后按钮还拿着旧图，
    // 用户点「保存/分享」会拿到上一版海报（数据可能已变），且不报错。
    this.setData({ loading: true, error: false, tempFilePath: '' })
    try {
      const [card, code] = await Promise.all([
        request('GET', '/api/card?staffId=' + sid),
        // from=poster：后端把该标记埋进小程序码 scene（staffId=12&from=poster），
        // 访客扫海报上的码打开名片时，scene 含 from=poster → 来源归为 poster，
        // 与「扫普通码」区分开（两者 scene 都是 scan 区间，只能靠这个标记区分）。
        request('GET', '/api/wechat/wxacode?staffId=' + sid + '&from=poster'),
      ])
      // /api/wechat/wxacode 成功返回 {code:0,data:{url}}（data.url 正确）；
      // /api/card 的 employee/company/profile 在响应根级（非 data 包裹），故此处直接存整个响应。
      const qrUrl = assetUrl(code.data ? code.data.url : '')
      // 头像为后端相对路径，canvas 的 img.src 需绝对 URL 才能加载
      const cardAny = card as any
      if (cardAny.employee) cardAny.employee.avatar = assetUrl(cardAny.employee.avatar)
      this.setData({ card, qrUrl })
      this.draw()
    } catch (e) {
      this.setData({ loading: false, error: true, errorText: '名片信息加载失败，请检查网络' })
    }
  },
  retry() {
    this.prepare()
  },
  draw() {
    const that = this
    wx.createSelectorQuery()
      .select('#poster')
      .fields({ node: true, size: true })
      .exec((res: any) => {
        if (!res || !res[0]) {
          // 取不到 canvas 节点：静默 return 会让页面永远停在「生成中」。
          that.setData({ loading: false, error: true, errorText: '海报生成失败' })
          return
        }
        const canvas: any = res[0].node
        const ctx = canvas.getContext('2d')
        const dpr = (wx.getSystemInfoSync() as any).pixelRatio || 2
        const W = 320
        const H = 460
        canvas.width = W * dpr
        canvas.height = H * dpr
        ctx.scale(dpr, dpr)

        ctx.fillStyle = '#ffffff'
        ctx.fillRect(0, 0, W, H)

        const emp = that.data.card && that.data.card.employee
        const comp = that.data.card && that.data.card.company
        ctx.fillStyle = '#222'
        ctx.font = 'bold 22px sans-serif'
        ctx.fillText(emp ? emp.name : '', 20, 50)
        ctx.font = '14px sans-serif'
        ctx.fillStyle = '#888'
        ctx.fillText(emp ? emp.title : '', 20, 74)
        if (comp) {
          ctx.fillStyle = '#444'
          ctx.fillText(comp.fullName, 20, 104)
        }
        if (comp) {
          ctx.fillStyle = '#aaa'
          ctx.font = '12px sans-serif'
          ctx.fillText(comp.address || '', 20, 126)
        }
        // 电话（需求 B8：海报含头像/姓名/职位/公司/电话/小程序码）
        if (emp && emp.phone) {
          ctx.fillStyle = '#444'
          ctx.font = '13px sans-serif'
          ctx.fillText('手机 ' + emp.phone, 20, 148)
        }

        // 头像
        that.loadImage(canvas, emp ? emp.avatar : '', (img: any) => {
          if (img) ctx.drawImage(img, 20, 165, 90, 90)
          // 小程序码
          that.loadImage(canvas, that.data.qrUrl, (qimg: any) => {
            if (qimg) ctx.drawImage(qimg, 190, 300, 110, 110)
            ctx.fillStyle = '#666'
            ctx.font = '12px sans-serif'
            ctx.fillText('长按识别名片', 190, 430)
            wx.canvasToTempFilePath({
              canvas,
              success: (r: any) => that.setData({ tempFilePath: r.tempFilePath, loading: false }),
              fail: () => {
                // 导出失败（画布为空 / 系统限制）：拿不到图片，后面保存与分享
                // 都无从下手，必须落到失败态让用户重试，不能只吞掉回调。
                that.setData({ loading: false, error: true, errorText: '海报导出失败' })
              },
            })
          })
        })
      })
  },
  loadImage(canvas: any, src: string, cb: (img: any) => void) {
    if (!src) {
      cb(null)
      return
    }
    const img = canvas.createImage()
    img.onload = () => cb(img)
    img.onerror = () => cb(null)
    img.src = src
  },
  // 「发送给好友」：海报裂变闭环的主路径（生成 → 分享 → 好友看海报 →
  // 扫海报上的码进来记为 poster 来源）。此前这一页只有「保存到相册」，
  // 海报能不能发出去全靠用户自己想起来去相册里找——闭环断在最关键的一步。
  share() {
    if (!this.data.tempFilePath) {
      wx.showToast({ title: '海报生成中…', icon: 'none' })
      return
    }
    // showShareImageMenu 基础库 2.14.3 起才支持（官方文档），老版本直接调用
    // 会抛 unknown api；先探测，低版本退回「先存相册再手动发」，
    // 不至于点了按钮毫无反应（用户会以为是 bug）。
    if (typeof wx.canIUse !== 'function' || !wx.canIUse('showShareImageMenu')) {
      wx.showModal({
        title: '提示',
        content: '当前微信版本不支持直接分享图片，请先保存到相册，再从相册发给好友',
        showCancel: false,
      })
      return
    }
    const that = this
    wx.showShareImageMenu({
      path: this.data.tempFilePath,
      success: () => {
        // 口径与首页转发（onShareAppMessage）一致：微信只回「接口调用成功」
        // （文档原文如此），弹窗打开即 success，不告知用户是否真的发出去——
        // 故按「发起分享」计数，与首页转发同一把尺子，不追求确认送达。
        track('share', that.data.staffId)
      },
      fail: () => {},
    })
  },
  save() {
    if (!this.data.tempFilePath) {
      wx.showToast({ title: '海报生成中…', icon: 'none' })
      return
    }
    wx.saveImageToPhotosAlbum({
      filePath: this.data.tempFilePath,
      success: () => {
        // 海报保存成功才计一次（poster_save）：用户愿意把海报存进相册，
        // 说明有线下发送的意图——比「打开过海报页」强得多的意向信号。
        // 保存失败（权限拒绝 / 写入失败）不计，那只是没做成，不是没意愿。
        track('poster_save', this.data.staffId)
        wx.showToast({ title: '已保存到相册' })
      },
      fail: (e: any) => {
        if (e.errMsg && e.errMsg.indexOf('auth') >= 0) {
          wx.showModal({ title: '提示', content: '需要相册权限，请在设置中开启', showCancel: false })
        } else {
          wx.showToast({ title: '保存失败', icon: 'none' })
        }
      },
    })
  },
  // 右上角「转发」：转发出去的是海报页本身——好友打开即看到同一张海报，
  // 可继续保存或直接转发，是裂变链路的下一段（此前这一页没有转发出口，
  // 从海报页点右上角转发拿不到任何分享配置）。
  onShareAppMessage() {
    const emp = this.data.card && this.data.card.employee
    const comp = this.data.card && this.data.card.company
    const name = (emp && emp.name) || ''
    // 与分享图片同口径：发起转发即计一次 share（targetId=0，名片级转发）
    track('share', this.data.staffId)
    return {
      title: (name ? name + '的名片' : '') || (comp && (comp.shortName || comp.fullName)) || '电子名片',
      path: '/pages/poster/poster?staffId=' + this.data.staffId,
    }
  },
})

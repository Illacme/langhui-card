import { request } from '../../utils/request'

Page({
  data: {
    staffId: 0,
    card: null as any,
    qrUrl: '',
    tempFilePath: '',
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
    const [card, code] = await Promise.all([
      request('GET', '/api/card?staffId=' + sid),
      request('GET', '/api/wechat/wxacode?staffId=' + sid),
    ])
    const qrUrl = code.data ? code.data.url : ''
    this.setData({ card: card.data, qrUrl })
    this.draw()
  },
  draw() {
    const that = this
    wx.createSelectorQuery()
      .select('#poster')
      .fields({ node: true, size: true })
      .exec((res: any) => {
        if (!res || !res[0]) return
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
              success: (r: any) => that.setData({ tempFilePath: r.tempFilePath }),
              fail: () => {},
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
  save() {
    if (!this.data.tempFilePath) {
      wx.showToast({ title: '海报生成中…', icon: 'none' })
      return
    }
    wx.saveImageToPhotosAlbum({
      filePath: this.data.tempFilePath,
      success: () => wx.showToast({ title: '已保存到相册' }),
      fail: (e: any) => {
        if (e.errMsg && e.errMsg.indexOf('auth') >= 0) {
          wx.showModal({ title: '提示', content: '需要相册权限，请在设置中开启', showCancel: false })
        } else {
          wx.showToast({ title: '保存失败', icon: 'none' })
        }
      },
    })
  },
})

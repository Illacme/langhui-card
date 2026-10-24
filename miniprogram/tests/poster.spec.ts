// 小程序「分享海报」页逻辑测试。
//
// 为什么先测这一页：它是小程序码扫码链路的落点——scene 参数解析错，
// 扫码进来拿不到 staffId，整页就是空的，而且这类问题只在真机扫码时暴露，
// 本地开发（直接带 query 跳转）永远复现不了。
//
// 做法：桩掉全局 Page 捕获页面选项，再手工造一个「页面实例」调用生命周期，
// 与真机上的调用形态一致（onLoad → prepare → draw）。
import { describe, it, expect, beforeEach, vi } from 'vitest'

const mocks = vi.hoisted(() => ({
  request: vi.fn(async (_method: string, _url: string) => ({})),
  assetUrl: vi.fn((u: string) => (u ? `https://cdn.example.com${u.startsWith('/') ? '' : '/'}${u}` : '')),
}))
vi.mock('../utils/request', () => mocks)

let captured: any = null

beforeEach(() => {
  captured = null
  vi.resetModules()
  vi.stubGlobal('Page', (opts: any) => {
    captured = opts
  })
  // 选择器查询返回空 → draw() 走「取不到节点」分支（jsdom 里没有真 canvas）
  vi.stubGlobal('wx', {
    createSelectorQuery: () => ({
      select: () => ({ fields: () => ({ exec: (cb: any) => cb([]) }) }),
    }),
    getSystemInfoSync: () => ({ pixelRatio: 2 }),
    showToast: vi.fn(),
    showModal: vi.fn(),
    // 保存相册：默认走成功回调（个别用例改成 fail 覆盖）
    saveImageToPhotosAlbum: vi.fn((o: any) => o.success({})),
    // 分享图片面板（基础库 2.14.3+）：默认「已支持 + 打开成功」，低版本用例改 canIUse
    canIUse: vi.fn(() => true),
    showShareImageMenu: vi.fn((o: any) => o.success({})),
    // canvas 导出图片：默认成功（导出失败用例改 fail）
    canvasToTempFilePath: vi.fn((o: any) => o.success({ tempFilePath: 'wxfile://tmp.png' })),
  })
  mocks.request.mockClear()
  mocks.assetUrl.mockClear()
})

async function loadPage() {
  await import('../pages/poster/poster')
  if (!captured) throw new Error('页面未调用 Page() 注册')
  return captured
}

// 造一个页面实例：data 用页面声明的初值，方法来自 Page 选项，setData 落回 data
function instance(opts: any) {
  const inst: any = {
    data: JSON.parse(JSON.stringify(opts.data || {})),
    setData(patch: any) {
      Object.assign(this.data, patch)
    },
  }
  for (const k of Object.keys(opts)) {
    if (typeof opts[k] === 'function') inst[k] = opts[k].bind(inst)
  }
  return inst
}

// 让两个接口都返回可用数据（card 在响应根级，wxacode 在 data.url）
function mockApis(card: any = { employee: { name: '张伟', avatar: '/uploads/a.png' } }, codeUrl = '/uploads/wxacode/42.jpg') {
  mocks.request.mockImplementation(async (method: string, url: string) => {
    if (url.includes('/api/wechat/wxacode')) return { code: 0, data: { url: codeUrl } }
    if (url.includes('/api/card')) return card
    return {}
  })
}

// 让 draw() 真的画一遍：造一个假 canvas 节点（jsdom 没有 2d canvas）。
// 图片加载用「给 src 赋值即同步触发 onload」的假 img 驱动，
// 最后一步导出交给 wx.canvasToTempFilePath 桩（默认成功）。
function stubCanvasNode() {
  const ctx: any = { scale: () => {}, fillRect: () => {}, fillText: () => {}, drawImage: () => {} }
  const canvas: any = {
    width: 0,
    height: 0,
    getContext: () => ctx,
    createImage() {
      const img: any = {}
      Object.defineProperty(img, 'src', {
        set() {
          if (img.onload) img.onload()
        },
        get() {
          return 'https://cdn.example.com/fake.png'
        },
      })
      return img
    },
  }
  ;(globalThis as any).wx.createSelectorQuery = vi.fn(() => ({
    select: () => ({ fields: () => ({ exec: (cb: any) => cb([{ node: canvas, width: 320, height: 460 }]) }) }),
  }))
  return canvas
}

describe('poster 页', () => {
  it('带 staffId 参数进入：解析出 staffId 并请求名片与小程序码', async () => {
    const opts = await loadPage()
    mockApis()
    const page = instance(opts)
    await page.onLoad({ staffId: '42' })
    await new Promise((r) => setTimeout(r, 0)) // 等 prepare 的 await 链

    expect(page.data.staffId).toBe(42)
    const urls = mocks.request.mock.calls.map((c) => c[1])
    expect(urls).toContain('/api/card?staffId=42')
    // 海报码必须带 from=poster：后端把它埋进 scene，扫这张码进来才能归到
    // poster 来源（普通码与海报码的微信 scene 同在扫码区间，只有这个标记能区分）。
    // 漏掉它就退化成「扫码」，海报渠道在来源分布里永远统计不到——静默失真。
    expect(urls).toContain('/api/wechat/wxacode?staffId=42&from=poster')
  })

  // 扫码进入：微信把参数塞进 scene（URL 编码），拿不到 staffId 就等于海报空白
  it('扫码进入（scene 参数）：解析出 staffId', async () => {
    const opts = await loadPage()
    mockApis()
    const page = instance(opts)
    await page.onLoad({ scene: encodeURIComponent('staffId=42') })
    await new Promise((r) => setTimeout(r, 0))

    expect(page.data.staffId).toBe(42)
  })

  it('scene 里混了其它参数也要能认出 staffId', async () => {
    const opts = await loadPage()
    mockApis()
    const page = instance(opts)
    await page.onLoad({ scene: encodeURIComponent('a=1&staffId=7&b=2') })
    await new Promise((r) => setTimeout(r, 0))
    expect(page.data.staffId).toBe(7)
  })

  it('既无 staffId 也无 scene：staffId 归 0 而不是 NaN/崩溃', async () => {
    const opts = await loadPage()
    mockApis()
    const page = instance(opts)
    await page.onLoad({})
    await new Promise((r) => setTimeout(r, 0))
    expect(page.data.staffId).toBe(0)
  })

  it('头像与小程序码都转成绝对 URL（canvas 的 img.src 必须绝对地址）', async () => {
    const opts = await loadPage()
    mockApis({ employee: { name: '张伟', avatar: '/uploads/a.png' } }, '/uploads/wxacode/42.jpg')
    const page = instance(opts)
    await page.onLoad({ staffId: '42' })
    await new Promise((r) => setTimeout(r, 0))

    expect(page.data.qrUrl).toBe('https://cdn.example.com/uploads/wxacode/42.jpg')
    expect(page.data.card.employee.avatar).toBe('https://cdn.example.com/uploads/a.png')
  })

  it('小程序码接口没有 data 时不崩（未配微信凭据时的降级路径）', async () => {
    const opts = await loadPage()
    mocks.request.mockImplementation(async (_m: string, url: string) => {
      if (url.includes('/api/wechat/wxacode')) return { code: 1, msg: '微信凭据未配置' }
      return { employee: { name: '张伟', avatar: '' } }
    })
    const page = instance(opts)
    await page.onLoad({ staffId: '42' })
    await new Promise((r) => setTimeout(r, 0))
    expect(page.data.qrUrl).toBe('')
    expect(page.data.card).toBeTruthy()
  })

  // ---- 海报保存埋点 ----

  // 保存成功才计 poster_save：用户愿意把海报存进相册，说明有线下发送的意图，
  // 比「打开过海报页」强得多的意向信号（后台意向指标卡之一）
  it('海报保存成功后上报 poster_save', async () => {
    const opts = await loadPage()
    mockApis()
    const page = instance(opts)
    await page.onLoad({ staffId: '42' })
    await new Promise((r) => setTimeout(r, 0))
    page.setData({ tempFilePath: 'wxfile://tmp.png' })
    mocks.request.mockClear()

    page.save()

    const tracks = mocks.request.mock.calls.filter((c) => c[1] === '/api/stat/track')
    expect(tracks).toHaveLength(1)
    expect(tracks[0][2]).toEqual({ staffId: 42, eventType: 'poster_save' })
  })

  // 保存失败（权限拒绝 / 写入失败）不计：没做成不等于没意愿，
  // 计了会让「保存海报数」虚高、运营按这个数评估线下发放量就偏了
  it('海报保存失败：不上报 poster_save', async () => {
    const opts = await loadPage()
    mockApis()
    const page = instance(opts)
    await page.onLoad({ staffId: '42' })
    await new Promise((r) => setTimeout(r, 0))
    page.setData({ tempFilePath: 'wxfile://tmp.png' })
    ;(globalThis as any).wx.saveImageToPhotosAlbum = vi.fn((o: any) =>
      o.fail({ errMsg: 'saveImageToPhotosAlbum:fail auth deny' })
    )
    mocks.request.mockClear()

    page.save()

    const tracks = mocks.request.mock.calls.filter((c) => c[1] === '/api/stat/track')
    expect(tracks).toHaveLength(0)
  })

  // 海报还没画完就点保存：连保存动作都没发生，更不该有埋点
  it('海报未生成时保存：直接提示，不上报', async () => {
    const opts = await loadPage()
    mockApis()
    const page = instance(opts)
    await page.onLoad({ staffId: '42' })
    await new Promise((r) => setTimeout(r, 0))
    mocks.request.mockClear()

    page.save()

    const tracks = mocks.request.mock.calls.filter((c) => c[1] === '/api/stat/track')
    expect(tracks).toHaveLength(0)
    expect((globalThis as any).wx.saveImageToPhotosAlbum).not.toHaveBeenCalled()
  })

  // ---- 加载 / 失败 / 重试 ----

  it('导出成功：加载态落下来并拿到临时图片', async () => {
    const opts = await loadPage()
    mockApis()
    stubCanvasNode()
    const page = instance(opts)
    await page.onLoad({ staffId: '42' })
    await new Promise((r) => setTimeout(r, 0))

    expect(page.data.loading).toBe(false)
    expect(page.data.error).toBe(false)
    expect(page.data.tempFilePath).toBe('wxfile://tmp.png')
  })

  // 断网 / 接口异常：loading 必须落下来并给重试入口。此前 Promise.all 没有 catch，
  // reject 直接冒泡出 async 函数 → 页面永远停在空白，用户只能退出重进
  it('名片接口失败：落到失败态而不是一直转圈', async () => {
    const opts = await loadPage()
    mocks.request.mockImplementation(async (_m: string, url: string) => {
      if (url.includes('/api/wechat/wxacode')) return { code: 0, data: { url: '/uploads/wxacode/42.jpg' } }
      throw new Error('network down')
    })
    stubCanvasNode()
    const page = instance(opts)
    await page.onLoad({ staffId: '42' })
    await new Promise((r) => setTimeout(r, 0))

    expect(page.data.loading).toBe(false)
    expect(page.data.error).toBe(true)
    expect(page.data.tempFilePath).toBe('')
  })

  // 取不到 canvas 节点（页面未渲染完 / 节点被销毁）也不能静默：
  // 否则按钮一直显示「生成中」，用户点了没反应也看不出原因
  it('取不到 canvas 节点：落到失败态而不是静默 return', async () => {
    const opts = await loadPage()
    mockApis()
    const page = instance(opts) // beforeEach 的查询桩返回空数组
    await page.onLoad({ staffId: '42' })
    await new Promise((r) => setTimeout(r, 0))

    expect(page.data.loading).toBe(false)
    expect(page.data.error).toBe(true)
  })

  it('canvas 导出失败：落到失败态并给出重试入口', async () => {
    const opts = await loadPage()
    mockApis()
    stubCanvasNode()
    ;(globalThis as any).wx.canvasToTempFilePath = vi.fn((o: any) => o.fail({ errMsg: 'canvasToTempFilePath:fail' }))
    const page = instance(opts)
    await page.onLoad({ staffId: '42' })
    await new Promise((r) => setTimeout(r, 0))

    expect(page.data.loading).toBe(false)
    expect(page.data.error).toBe(true)
    expect(page.data.tempFilePath).toBe('')
  })

  it('重试：重新请求并在成功前先清掉上一张海报', async () => {
    const opts = await loadPage()
    let n = 0
    mocks.request.mockImplementation(async (_m: string, url: string) => {
      if (url.includes('/api/wechat/wxacode')) return { code: 0, data: { url: '/uploads/wxacode/42.jpg' } }
      n++
      if (n === 1) throw new Error('network down')
      return { employee: { name: '张伟', avatar: '' } }
    })
    const page = instance(opts)
    await page.onLoad({ staffId: '42' })
    await new Promise((r) => setTimeout(r, 0))
    expect(page.data.error).toBe(true)

    stubCanvasNode() // 重试时才把 canvas 节点准备好（模拟第二次渲染到位）
    page.setData({ tempFilePath: 'wxfile://old.png' })
    const p = page.retry()
    // 重试必须先清掉旧图：否则导出失败后按钮仍拿着上一版海报，用户会拿到旧图且不报错
    expect(page.data.tempFilePath).toBe('')
    await p
    await new Promise((r) => setTimeout(r, 0))

    expect(page.data.error).toBe(false)
    expect(page.data.tempFilePath).toBe('wxfile://tmp.png')
    const cards = mocks.request.mock.calls.filter((c) => c[1] === '/api/card?staffId=42')
    expect(cards).toHaveLength(2) // 首次失败 + 重试
  })

  // ---- 分享（裂变闭环：生成 → 分享 → 好友看海报 → 扫码记为 poster 来源）----

  it('分享海报：打开分享面板并上报 share', async () => {
    const opts = await loadPage()
    mockApis()
    stubCanvasNode()
    const page = instance(opts)
    await page.onLoad({ staffId: '42' })
    await new Promise((r) => setTimeout(r, 0))
    mocks.request.mockClear()

    page.share()

    const menu = (globalThis as any).wx.showShareImageMenu
    expect(menu).toHaveBeenCalledTimes(1)
    expect(menu.mock.calls[0][0].path).toBe('wxfile://tmp.png')
    const tracks = mocks.request.mock.calls.filter((c) => c[1] === '/api/stat/track')
    expect(tracks).toHaveLength(1)
    // targetId=0：名片级转发，与首页转发同一把尺子（区别于案例/服务详情的转发）
    expect(tracks[0][2]).toEqual({ staffId: 42, eventType: 'share' })
  })

  it('海报未生成时点分享：不弹面板也不上报', async () => {
    const opts = await loadPage()
    mockApis()
    const page = instance(opts)
    await page.onLoad({ staffId: '42' })
    await new Promise((r) => setTimeout(r, 0))
    mocks.request.mockClear()

    page.share()

    expect((globalThis as any).wx.showShareImageMenu).not.toHaveBeenCalled()
    const tracks = mocks.request.mock.calls.filter((c) => c[1] === '/api/stat/track')
    expect(tracks).toHaveLength(0)
  })

  // 低版本基础库（<2.14.3）没有 showShareImageMenu：直接调用会抛 unknown api，
  // 页面表现为「点了没反应」。必须降级提示先存相册，而不是把按钮做成死键
  it('低版本微信（无分享图片 API）：降级提示而不是点了没反应', async () => {
    const opts = await loadPage()
    mockApis()
    stubCanvasNode()
    const page = instance(opts)
    await page.onLoad({ staffId: '42' })
    await new Promise((r) => setTimeout(r, 0))
    ;(globalThis as any).wx.canIUse = vi.fn(() => false)
    mocks.request.mockClear()

    page.share()

    expect((globalThis as any).wx.showShareImageMenu).not.toHaveBeenCalled()
    expect((globalThis as any).wx.showModal).toHaveBeenCalled()
    const tracks = mocks.request.mock.calls.filter((c) => c[1] === '/api/stat/track')
    expect(tracks).toHaveLength(0)
  })

  it('用户取消分享面板：不上报 share', async () => {
    const opts = await loadPage()
    mockApis()
    stubCanvasNode()
    const page = instance(opts)
    await page.onLoad({ staffId: '42' })
    await new Promise((r) => setTimeout(r, 0))
    ;(globalThis as any).wx.showShareImageMenu = vi.fn((o: any) => o.fail({ errMsg: 'showShareImageMenu:fail cancel' }))
    mocks.request.mockClear()

    page.share()

    const tracks = mocks.request.mock.calls.filter((c) => c[1] === '/api/stat/track')
    expect(tracks).toHaveLength(0)
  })

  // 右上角「转发」：转发出去的是海报页本身——好友打开即看到同一张海报并可继续
  // 保存/转发，是裂变链路的下一段（此前这一页没有转发出口）
  it('右上角转发：带 staffId 转发海报页并上报 share', async () => {
    const opts = await loadPage()
    mockApis()
    const page = instance(opts)
    await page.onLoad({ staffId: '42' })
    await new Promise((r) => setTimeout(r, 0))
    mocks.request.mockClear()

    const share = page.onShareAppMessage()

    expect(share.path).toBe('/pages/poster/poster?staffId=42')
    expect(share.title).toBe('张伟的名片')
    const tracks = mocks.request.mock.calls.filter((c) => c[1] === '/api/stat/track')
    expect(tracks).toHaveLength(1)
    expect(tracks[0][2]).toEqual({ staffId: 42, eventType: 'share' })
  })
})

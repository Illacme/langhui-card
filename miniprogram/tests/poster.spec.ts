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
  // 选择器查询返回空 → draw() 直接返回，不牵扯 canvas（jsdom 里没有真 canvas）
  vi.stubGlobal('wx', {
    createSelectorQuery: () => ({
      select: () => ({ fields: () => ({ exec: (cb: any) => cb([]) }) }),
    }),
    getSystemInfoSync: () => ({ pixelRatio: 2 }),
    showToast: vi.fn(),
    showModal: vi.fn(),
    // 保存相册：默认走成功回调（个别用例改成 fail 覆盖）
    saveImageToPhotosAlbum: vi.fn((o: any) => o.success({})),
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
})

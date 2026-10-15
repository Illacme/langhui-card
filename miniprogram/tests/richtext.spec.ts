// 小程序端富文本解析纯逻辑测试。
// 说明：这些是纯 TS 函数（无 wx / DOM 依赖），挂在 frontend 的 vitest 下统一运行，
// 避免为 miniprogram 再引入一套 Node 工具链（小程序包保持零依赖，见 miniprogram/README）。
import { describe, it, expect } from 'vitest'
import { parseRich, buildWxVideo, type RichSegment } from '../utils/richtext'

describe('parseRich', () => {
  it('空输入返回空数组', () => {
    expect(parseRich('')).toEqual([])
  })

  it('无视频号块时整体作为单个 html 片段', () => {
    const segs = parseRich('<p>hello</p>')
    expect(segs).toEqual<RichSegment[]>([{ type: 'html', content: '<p>hello</p>' }])
  })

  it('视频号块被拆成独立 video 片段，前后 html 保留', () => {
    const segs = parseRich('<p>a</p><wx-video channel-id="gh_1"></wx-video><p>b</p>')
    expect(segs).toEqual<RichSegment[]>([
      { type: 'html', content: '<p>a</p>' },
      { type: 'video', content: 'gh_1' },
      { type: 'html', content: '<p>b</p>' },
    ])
  })

  it('兼容自闭合写法且属性顺序无关', () => {
    const segs = parseRich('<wx-video data-x="1" channel-id="gh_2"/>')
    expect(segs).toEqual<RichSegment[]>([{ type: 'video', content: 'gh_2' }])
  })

  it('支持多个视频号块', () => {
    const segs = parseRich(
      'a<wx-video channel-id="gh_a"></wx-video>b<wx-video channel-id="gh_b"></wx-video>c'
    )
    expect(segs.map((s) => s.type)).toEqual(['html', 'video', 'html', 'video', 'html'])
    expect(segs.map((s) => s.content)).toEqual(['a', 'gh_a', 'b', 'gh_b', 'c'])
  })

  it('不含 channel-id 的 wx-video 不拆分为 video 片段', () => {
    const segs = parseRich('<wx-video></wx-video>')
    expect(segs).toEqual<RichSegment[]>([{ type: 'html', content: '<wx-video></wx-video>' }])
  })

  it('连续调用互不影响（全局正则 lastIndex 状态安全）', () => {
    const html = '<wx-video channel-id="gh_1"></wx-video>tail'
    expect(parseRich(html)).toEqual(parseRich(html))
    expect(parseRich(html).length).toBe(2)
  })
})

describe('buildWxVideo', () => {
  it('生成与小程序端约定的标准标记', () => {
    expect(buildWxVideo('gh_abc')).toBe('<wx-video channel-id="gh_abc"></wx-video>')
  })

  it('buildWxVideo 的产出可被 parseRich 还原', () => {
    const segs = parseRich(buildWxVideo('gh_roundtrip'))
    expect(segs).toEqual<RichSegment[]>([{ type: 'video', content: 'gh_roundtrip' }])
  })
})

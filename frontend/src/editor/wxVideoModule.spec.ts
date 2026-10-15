import { describe, it, expect, vi } from 'vitest'

// wxVideoModule 把 <wx-video channel-id> 块注册进 wangEditor，
// 其序列化结果（<wx-video channel-id="..."></wx-video>）必须被小程序端 rich-video 组件解析，
// 因此这里校验「仅注册一次」「序列化 / 反解析」契约。
// 注意：模块内有 registered 开关 + clearMocks，故把断言放在同一用例内。
const { registerModule } = vi.hoisted(() => ({
  registerModule: vi.fn(),
}))

vi.mock('@wangeditor/editor', () => ({
  Boot: { registerModule },
}))

import { registerWxVideoModule, WX_VIDEO_TYPE } from '@/editor/wxVideoModule'

describe('wxVideoModule 视频号块', () => {
  it('只注册一次，且序列化/反解析与小程序契约一致', () => {
    registerWxVideoModule()
    registerWxVideoModule() // 二次调用应为 no-op

    expect(registerModule).toHaveBeenCalledTimes(1)
    expect(WX_VIDEO_TYPE).toBe('wxVideo')

    const mod = registerModule.mock.calls[0][0]

    // 序列化：小程序端 parseRich 可解析
    const html = mod.elemsToHtml[0].elemToHtml({ channelId: 'gh_abc' })
    expect(html).toBe('<wx-video channel-id="gh_abc"></wx-video>')

    // 反解析：小程序格式的标签还原为块节点
    const dom = { getAttribute: (k: string) => (k === 'channel-id' ? 'gh_xyz' : null) }
    const node = mod.parseElemsHtml[0].parseElemHtml(dom)
    expect(node).toEqual({ type: WX_VIDEO_TYPE, channelId: 'gh_xyz', children: [{ text: '' }] })
  })
})

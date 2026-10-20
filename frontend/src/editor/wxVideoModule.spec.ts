import { describe, it, expect, vi } from 'vitest'

// wxVideoModule 把 <wx-video feed-id / channel-id> 块注册进 wangEditor，
// 其序列化结果必须被小程序端 rich-video 组件解析，因此这里校验「仅注册一次」
// 「序列化 / 反解析」契约，以及存量只有 channel-id 的内容不会在往返中丢失。
// 注意：模块内有 registered 开关 + clearMocks，故把断言放在同一用例内。
const { registerModule, insertNodes } = vi.hoisted(() => ({
  registerModule: vi.fn(),
  insertNodes: vi.fn(),
}))

vi.mock('@wangeditor/editor', () => ({
  Boot: { registerModule },
  SlateTransforms: { insertNodes },
}))

import { registerWxVideoModule, WX_VIDEO_TYPE } from '@/editor/wxVideoModule'

describe('wxVideoModule 视频号块', () => {
  it('只注册一次，且序列化/反解析与小程序契约一致', () => {
    registerWxVideoModule()
    registerWxVideoModule() // 二次调用应为 no-op

    expect(registerModule).toHaveBeenCalledTimes(1)
    expect(WX_VIDEO_TYPE).toBe('wxVideo')

    const mod = registerModule.mock.calls[0][0]

    // 新增：feedId + channelId 都写
    const html = mod.elemsToHtml[0].elemToHtml({ feedId: 'f_abc', channelId: 'sph_1' })
    expect(html).toBe('<wx-video feed-id="f_abc" channel-id="sph_1"></wx-video>')

    // 渲染节点必须把三个值写进 data-*：编辑弹窗靠它们回填表单、并按原值定位目标节点。
    // 刻意不含节点 id——wangEditor 的 slate 节点没有 id 属性（getElemsByType 返回的 id
    // 是 findKey 动态包装的），elem.id 恒为 undefined，靠它定位会静默失败误走新增分支。
    const vnode = mod.renderElems[0].renderElem({
      feedId: 'f_1',
      feedToken: 'tok_1',
      channelId: 'sph_1',
    })
    expect(vnode.data.dataset).toEqual({
      wEType: WX_VIDEO_TYPE,
      feedId: 'f_1',
      feedToken: 'tok_1',
      channelId: 'sph_1',
    })

    // 跨主体：只有 feed-token 时序列化出 feed-token 属性
    const htmlCross = mod.elemsToHtml[0].elemToHtml({ feedToken: 'tok_cross' })
    expect(htmlCross).toBe('<wx-video feed-token="tok_cross"></wx-video>')

    // 只填 feedId：不写出空的 channel-id 属性
    const html2 = mod.elemsToHtml[0].elemToHtml({ feedId: 'f_abc' })
    expect(html2).toBe('<wx-video feed-id="f_abc"></wx-video>')

    // 存量内容（只有 channel-id）往返不能丢：解析 → 序列化应还原原标签
    const dom1 = {
      getAttribute: (k: string) => (k === 'channel-id' ? 'sph_old' : null),
    }
    const node1 = mod.parseElemsHtml[0].parseElemHtml(dom1)
    expect(node1).toEqual({
      type: WX_VIDEO_TYPE,
      feedId: '',
      feedToken: '',
      channelId: 'sph_old',
      children: [{ text: '' }],
    })
    expect(mod.elemsToHtml[0].elemToHtml(node1)).toBe('<wx-video channel-id="sph_old"></wx-video>')

    // 新格式反解析：三个属性都读出来
    const dom2 = {
      getAttribute: (k: string) =>
        k === 'feed-id'
          ? 'f_xyz'
          : k === 'feed-token'
            ? 'tok_xyz'
            : k === 'channel-id'
              ? 'sph_xyz'
              : null,
    }
    const node2 = mod.parseElemsHtml[0].parseElemHtml(dom2)
    expect(node2).toEqual({
      type: WX_VIDEO_TYPE,
      feedId: 'f_xyz',
      feedToken: 'tok_xyz',
      channelId: 'sph_xyz',
      children: [{ text: '' }],
    })

    // ===== 末尾视频块：光标落点 + 序列化剥尾巴 =====
    // 视频号块是 void，位于文档末尾时光标无处落脚（用户反馈：点不到块后面、没法继续输入）。
    // 约定：normalizeNode 在它后面补一个空段落；这个空段落只是编辑器落点，
    // getHtml 时要剥掉，否则会存进数据库并在小程序详情页末尾多出一行空白。
    const editorPlugin = mod.editorPlugin
    const mkEditor = (children: any[]) => {
      // origNormalize 保留原始实现的引用：插件会覆盖 editor.normalizeNode
      const origNormalize = vi.fn()
      return {
        children,
      isVoid: (_elem?: any) => false,
      normalizeNode: origNormalize,
        origNormalize,
        getHtml: () =>
        children
          .map((n: any) =>
            n.type === WX_VIDEO_TYPE
              ? mod.elemsToHtml[0].elemToHtml(n)
              : n.children[0].text
                ? `<p>${n.children[0].text}</p>`
                : '<p><br></p>'
          )
          .join(''),
      }
    }
    const wxVideo = (feedId: string) => ({
      type: WX_VIDEO_TYPE,
      feedId,
      feedToken: '',
      channelId: '',
      children: [{ text: '' }],
    })
    const para = (text: string) => ({ type: 'paragraph', children: [{ text }] })

    // 视频块在末尾 → 补空段落，且不再往下走默认 normalize
    const e1 = mkEditor([para('前面'), wxVideo('f_1')])
    editorPlugin(e1)
    expect(e1.isVoid(wxVideo('f_1') as any)).toBe(true)
    e1.normalizeNode([e1.children[1], [1]])
    expect(insertNodes).toHaveBeenCalledTimes(1)
    const [ed, insertedNode, opt] = insertNodes.mock.calls[0] as any[]
    expect(ed).toBe(e1)
    expect(insertedNode).toEqual({ type: 'paragraph', children: [{ text: '' }] })
    expect(opt).toEqual({ at: [2] }) // 插在视频块之后
    expect(e1.origNormalize).not.toHaveBeenCalled() // 已被本层拦截

    // 视频块后面还有内容 → 不补，交给默认 normalize
    const e2 = mkEditor([wxVideo('f_1'), para('后面')])
    editorPlugin(e2)
    e2.normalizeNode([e2.children[0], [0]])
    expect(insertNodes).toHaveBeenCalledTimes(1) // 没有新增调用
    expect(e2.origNormalize).toHaveBeenCalledTimes(1)

    // 序列化：紧跟视频块的空段落被剥掉
    const e3 = mkEditor([para('前面'), wxVideo('f_1'), para('')])
    editorPlugin(e3)
    expect(e3.getHtml()).toBe('<p>前面</p><wx-video feed-id="f_1"></wx-video>')

    // 视频块后面有真内容 → 原样保留
    const e4 = mkEditor([para('前面'), wxVideo('f_1'), para('后面')])
    editorPlugin(e4)
    expect(e4.getHtml()).toBe('<p>前面</p><wx-video feed-id="f_1"></wx-video><p>后面</p>')

    // 用户自己敲出来的空行（前面不是视频块）不能被吃掉
    const e5 = mkEditor([para('前面'), para('')])
    editorPlugin(e5)
    expect(e5.getHtml()).toBe('<p>前面</p><p><br></p>')
  })
})

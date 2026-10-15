import { Boot, type IDomEditor, type IModuleConf, type SlateElement } from '@wangeditor/editor'

/**
 * 视频号块自定义元素：让 wangEditor 识别并序列化 <wx-video channel-id="gh_xxx"></wx-video>。
 * 该标签与小程序端 rich-video 组件约定一致（见《首页/跳转页组件与接口字段映射》的视频号块小节）。
 *
 * 必须在创建编辑器之前注册，且全局只注册一次。
 */
export const WX_VIDEO_TYPE = 'wxVideo'

let registered = false

export function registerWxVideoModule() {
  if (registered) return
  registered = true

  // 标记为 void（原子块，内部不可编辑）。
  // 泛型 <T extends IDomEditor> 与 Boot 的 PluginType 签名保持一致（否则 TS 推断不收敛）。
  const withWxVideo = <T extends IDomEditor>(editor: T): T => {
    const { isVoid } = editor
    editor.isVoid = (elem: SlateElement) => {
      const e = elem as any
      if (e.type === WX_VIDEO_TYPE) return true
      return isVoid(elem)
    }
    return editor
  }

  // 编辑区渲染：显示为一块绿色占位（视频号图标 + channel-id）。
  // renderElem 返回 DOM 元素（wangEditor 运行时可接受）；上游 IRenderElemConf 的
  // 类型签名按 snabbdom VNode 声明，且未从 @wangeditor/editor 根导出（内部来自
  // @wangeditor/core），故这里保持对象字面量原样，装配时统一断言收敛。
  const renderElemConf = {
    type: WX_VIDEO_TYPE,
    renderElem: (elem: any) => {
      const channelId = elem.channelId || ''
      const dom = document.createElement('div')
      dom.setAttribute('data-w-e-type', WX_VIDEO_TYPE)
      dom.setAttribute('contenteditable', 'false')
      dom.className = 'wx-video-block'
      dom.innerHTML = `<span class="wx-video-ic">▶ 视频号</span><span class="wx-video-id">${
        channelId || '未设置'
      }</span>`
      return dom
    },
  }

  // 序列化为 <wx-video channel-id="..."></wx-video>（小程序端可解析）
  const elemToHtmlConf = {
    type: WX_VIDEO_TYPE,
    elemToHtml: (elem: any) => {
      const channelId = elem.channelId || ''
      return `<wx-video channel-id="${channelId}"></wx-video>`
    },
  }

  // 解析：<wx-video channel-id="..."> 还原为元素节点
  const parseHtmlConf = {
    selector: 'wx-video',
    parseElemHtml: (dom: any) => {
      return {
        type: WX_VIDEO_TYPE,
        channelId: dom.getAttribute('channel-id') || '',
        children: [{ text: '' }],
      }
    },
  }

  // 装配：上游 conf 类型签名（snabbdom VNode）与运行时可接受的 DOM 返回值不一致，
  // 且未从根包导出，故逐项断言；运行时不变量（DOM 元素 / HTML 字符串 / 节点对象）不变。
  const module: Partial<IModuleConf> = {
    editorPlugin: withWxVideo,
    renderElems: [renderElemConf as any],
    elemsToHtml: [elemToHtmlConf as any],
    parseElemsHtml: [parseHtmlConf as any],
  }

  Boot.registerModule(module)
}

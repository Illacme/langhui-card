import { Boot, type IDomEditor, type IModuleConf, type SlateElement } from '@wangeditor/editor'
import { h } from 'snabbdom'

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
  // ⚠️ 关键修复：renderElem 必须返回 snabbdom VNode（与 wangEditor 内部同版本实例），
  // 此前返回原生 document.createElement 节点会导致视图层把它当 VNode 子节点处理，
  // 触发 `o2.forEach is not a function` + `Unable to find a host window element`，
  // 在 Vue dev 模式下被重抛从而冻结整个父组件的响应式更新。
  // 这里用 wangEditor 已依赖的 snabbdom（^3.1.0，与本项目安装版本一致）的 h 构造 VNode，
  // 并通过 props / dataset 适配 wangEditor 注册的 snabbdom 模块。
  const renderElemConf = {
    type: WX_VIDEO_TYPE,
    renderElem: (elem: any) => {
      const channelId = elem.channelId || ''
      return h(
        'div',
        {
          props: { contentEditable: false, className: 'wx-video-block' },
          dataset: { wEType: WX_VIDEO_TYPE },
        },
        [
          h('span', { props: { className: 'wx-video-ic' } }, '▶ 视频号'),
          h('span', { props: { className: 'wx-video-id' } }, channelId || '未设置'),
        ]
      )
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

  // 装配：上游 conf 类型签名（snabbdom VNode）与运行时可接受的返回值一致；
  // 仅 renderElem 的入参形态按 wangEditor 约定的对象字面量声明，装配时统一断言收敛。
  const module: Partial<IModuleConf> = {
    editorPlugin: withWxVideo,
    renderElems: [renderElemConf as any],
    elemsToHtml: [elemToHtmlConf as any],
    parseElemsHtml: [parseHtmlConf as any],
  }

  Boot.registerModule(module)
}

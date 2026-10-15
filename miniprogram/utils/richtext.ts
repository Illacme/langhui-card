// 富文本 + 视频号混排解析：把 <wx-video channel-id="gh_xxx"> 拆成独立片段
// 渲染策略（对应后端约定）：视频号片段用 <channel-video>，其余用 <rich-text>
export interface RichSegment {
  type: 'html' | 'video'
  content: string // html 片段 或 channel-id(视频号 username)
}

// 兼容自闭合 <wx-video channel-id="gh_x" /> 与成对 <wx-video ...></wx-video>，且属性顺序无关。
// 成对写法时闭合标签一并消费掉，避免 </wx-video> 漏进后续 html 片段被 rich-text 渲染。
const VIDEO_RE = /<wx-video\s+[^>]*?channel-id="([^"]+)"[^>]*?\/?>(\s*<\/wx-video>)?/g

export function parseRich(html: string): RichSegment[] {
  const segs: RichSegment[] = []
  if (!html) return segs
  let last = 0
  let m: RegExpExecArray | null
  while ((m = VIDEO_RE.exec(html)) !== null) {
    if (m.index > last) {
      segs.push({ type: 'html', content: html.slice(last, m.index) })
    }
    segs.push({ type: 'video', content: m[1] })
    last = VIDEO_RE.lastIndex
  }
  if (last < html.length) {
    segs.push({ type: 'html', content: html.slice(last) })
  }
  return segs
}

// 供 PC 管理端富文本编辑器插入视频号块时生成标准标记
export function buildWxVideo(channelId: string): string {
  return `<wx-video channel-id="${channelId}"></wx-video>`
}

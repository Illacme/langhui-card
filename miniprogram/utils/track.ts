// 埋点上报统一入口（后端 POST /api/stat/track：公开接口、按 IP 限流 60/分）。
//
// 事件类型与 targetId 约定必须与后端 internal/handler/stat.go 的 statEventTypes
// 保持一致（后端是权威白名单，端上多打一个不认的类型只会在统计表里留脏行）：
//
//   visit        访问名片（首页，页面实例级去重）   targetId=0
//   call         点击拨号                           targetId=0
//   save         点击保存（存通讯录）                targetId=0
//   project_view 浏览案例详情                       targetId=案例ID（后端强制存在+同租户）
//   service_view 浏览服务详情                       targetId=服务ID（同上）
//   video_play   播放公司视频（组件实例级去重）       targetId=0
//   share        转发                               targetId=0（名片）或案例/服务ID
//   poster_save  保存海报                           targetId=0
//
// 三条纪律：
// 1. 失败静默：埋点是旁路观察，不 toast、不 throw、不 await 阻塞渲染——
//    断网或撞限流（429）时丢就丢了，绝不能影响用户当前操作；
// 2. 白名单外类型就近丢弃：少一次注定被后端拒的无用请求；
// 3. targetId 为 0 时省略该字段：保持 visit/call/save 的老载荷形状不变
//    （后端缺省即 0），项目早期抓包的旧数据字段也保持一致。
import { request } from './request'

export type TrackEvent =
  | 'visit'
  | 'call'
  | 'save'
  | 'project_view'
  | 'service_view'
  | 'video_play'
  | 'share'
  | 'poster_save'

const TRACK_EVENTS: TrackEvent[] = [
  'visit',
  'call',
  'save',
  'project_view',
  'service_view',
  'video_play',
  'share',
  'poster_save',
]

// track 上报一条埋点。staffId 为 0（未携带员工编号的路径）时直接丢弃——
// 统计按员工聚合，没有归属的事件进了后台也只是对不上账的噪声。
// 注意 project_view / service_view 必须带目标 id：后端对这两类强制校验
// 「存在且与员工同租户」，漏带会回 code:1（页面无感，但统计里没这条）。
export function track(event: TrackEvent, staffId: number, targetId = 0): void {
  if (!staffId || staffId <= 0) return
  if (TRACK_EVENTS.indexOf(event) < 0) return
  const payload: { staffId: number; eventType: TrackEvent; targetId?: number } = {
    staffId,
    eventType: event,
  }
  if (targetId > 0) payload.targetId = targetId
  request('POST', '/api/stat/track', payload).catch(() => {
    // 断网 / 限流：静默丢弃（见文件头纪律 1）
  })
}

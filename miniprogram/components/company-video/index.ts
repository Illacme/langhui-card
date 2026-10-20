// 公司简介视频：按后端 videoSource 选档渲染，主源报错时（可选）回落到 MP4。
//
// 为什么不做单一来源：不同客户能接受的成本与资质条件不一样——
//   channels 微信视频号：零存储零流量、官方无资质要求
//   upload   云存储上传：落在客户自己的对象存储，直传不经我们的服务器
//   url      外部 MP4 直链：最灵活，流量与视听类目资质由客户自行承担
// 选档由后台显式完成（不是隐式自动降级链），出问题时能一眼看出当前走的是哪一路。
//
// 视频号档内还分两种凭证：
//   feed-id     同主体（视频号与小程序同主体），可自动播放，稳定且可从视频号助手直接复制
//   feed-token  跨主体专用——官方明确 feed-id「仅同主体时生效」，不同主体必须改用它，
//               代价是强制 autoplay=false。平台模式下客户的视频号与我们小程序必然不同主体，
//               走的就是这条。feed-id 优先：两个都填时只认 feed-id。
//
// 渲染档位：channels / channels-token 用 channel-video；mp4 用 video；none 回封面占位。
function pickMode(
  source: string,
  feedId: string,
  feedToken: string,
  videoUrl: string,
  fallback: boolean,
  failed: boolean
): string {
  const url = (videoUrl || '').trim()
  if (source === 'channels' && !failed) {
    if ((feedId || '').trim()) return 'channels'
    if ((feedToken || '').trim()) return 'channels-token'
  }
  // 云存储上传 / 外部直链本身就是 MP4，直接用 video 播放
  if ((source === 'upload' || source === 'url') && url) return 'mp4'
  // 兜底：只有视频号主源失败、后台开了开关、且真填了 MP4 时才顶上
  if (failed && fallback && url) return 'mp4'
  return 'none'
}

Component({
  options: { addGlobalClass: true },
  properties: {
    videoSource: String,
    // 同主体凭证：视频号视频 feedId（优先）
    feedId: String,
    // 跨主体凭证：feed-token（feedId 为空时启用）
    feedToken: String,
    // 腾讯视频 vid：预留字段。插件须在 app.json 静态声明，默认未声明故不渲染。
    vid: String,
    videoUrl: String,
    fallback: Boolean,
    cover: String,
  },
  data: {
    mode: 'none',
    failed: false,
  },
  observers: {
    // 任一入参变化都重算档位（后台改配置后刷新即可生效，无需重新进入页面）
    'videoSource, feedId, feedToken, videoUrl, fallback': function (
      videoSource: string,
      feedId: string,
      feedToken: string,
      videoUrl: string,
      fallback: boolean
    ) {
      this.setData({
        failed: false,
        mode: pickMode(videoSource || '', feedId || '', feedToken || '', videoUrl || '', !!fallback, false),
      })
    },
  },
  methods: {
    onVideoError() {
      const s: any = this.data
      // upload / url 两档本来就是 MP4，再回落没有意义；只处理视频号主源失败
      if ((s.videoSource || '') !== 'channels') return
      const next = pickMode(
        s.videoSource,
        s.feedId || '',
        s.feedToken || '',
        s.videoUrl || '',
        !!s.fallback,
        true
      )
      if (next !== s.mode) this.setData({ failed: true, mode: next })
    },
  },
})

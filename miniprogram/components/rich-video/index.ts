Component({
  options: { addGlobalClass: true },
  properties: {
    // 同主体：视频号视频 feedId（优先，可自动播放）
    feedId: String,
    // 跨主体：feed-token（feedId 为空时启用，强制不能自动播放）
    feedToken: String,
    // 视频号 ID（sph 开头）：前两者都缺失时的兼容回退，只展示视频号主页
    channelId: String,
  },
})

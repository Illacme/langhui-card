import { request } from '../../../utils/request'
import { parseRich } from '../../../utils/richtext'

Page({
  data: {
    id: 0,
    project: null,
    segments: [],
    loading: true,
  },
  onLoad(query: any) {
    this.setData({ id: parseInt(query.id || '0', 10) })
    this.load()
  },
  async load() {
    const r = await request('GET', '/api/projects/' + this.data.id)
    if (!r.data) {
      wx.showToast({ title: '案例不存在或已下架', icon: 'none' })
      this.setData({ loading: false })
      return
    }
    this.setData({
      project: r.data,
      segments: parseRich(r.data.detail || ''),
      loading: false,
    })
  },
  preview(e: any) {
    const album = (this.data.project && this.data.project.album) || []
    wx.previewImage({ urls: album, current: e.currentTarget.dataset.url })
  },
})

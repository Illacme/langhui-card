import { request } from '../../../utils/request'
import { parseRich } from '../../../utils/richtext'

Page({
  data: {
    id: 0,
    service: null,
    segments: [],
    loading: true,
  },
  onLoad(query: any) {
    this.setData({ id: parseInt(query.id || '0', 10) })
    this.load()
  },
  async load() {
    const r = await request('GET', '/api/services/' + this.data.id)
    if (!r.data) {
      wx.showToast({ title: '服务不存在', icon: 'none' })
      this.setData({ loading: false })
      return
    }
    this.setData({
      service: r.data,
      segments: parseRich(r.data.content || ''),
      loading: false,
    })
  },
})

import { request } from '../../../utils/request'
import { parseRich } from '../../../utils/richtext'
import { formatDateTime } from '../../../utils/datetime'

Page({
  data: {
    id: 0,
    staffId: 0,
    service: null,
    segments: [],
    updatedAtText: '',
    loading: true,
    error: false,
  },
  onLoad(query: any) {
    this.setData({
      id: parseInt(query.id || '0', 10),
      // 从首页透传（跳转页文档 §5）：分享时带回对应员工名片
      staffId: parseInt(query.staffId || '0', 10),
    })
    this.load()
  },
  async load() {
    this.setData({ loading: true, error: false })
    try {
      const r = await request('GET', '/api/services/' + this.data.id)
      if (!r.data) {
        wx.showToast({ title: '服务不存在', icon: 'none' })
        this.setData({ loading: false })
        return
      }
      this.setData({
        service: r.data,
        segments: parseRich(r.data.content || ''),
        updatedAtText: r.data.updatedAt ? formatDateTime(r.data.updatedAt) : '',
        loading: false,
      })
    } catch {
      this.setData({ loading: false, error: true })
    }
  },
  onShareAppMessage() {
    const s = this.data.service
    return {
      title: s ? s.title : '业务服务',
      path:
        '/pages/service/detail/detail?id=' +
        this.data.id +
        (this.data.staffId ? '&staffId=' + this.data.staffId : ''),
    }
  },
})

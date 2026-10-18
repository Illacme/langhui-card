import { request, assetUrl } from '../../../utils/request'
import { parseRich } from '../../../utils/richtext'
import { formatDateTime } from '../../../utils/datetime'
import { getCompanyId } from '../../../utils/tenant'

Page({
  data: {
    id: 0,
    staffId: 0,
    project: null,
    segments: [],
    updatedAtText: '',
    loading: true,
    error: false,
  },
  onLoad(query: any) {
    this.setData({
      id: parseInt(query.id || '0', 10),
      // 从首页 / 列表透传（跳转页文档 §5）：分享时带回对应员工名片
      staffId: parseInt(query.staffId || '0', 10),
    })
    this.load()
  },
  async load() {
    this.setData({ loading: true, error: false })
    try {
      const r = await request('GET', '/api/projects/' + this.data.id)
      if (!r.data) {
        wx.showToast({ title: '案例不存在或已下架', icon: 'none' })
        this.setData({ loading: false })
        return
      }
      // 封面与相册均为后端相对路径，端上拼绝对（<image> 与 previewImage 都需绝对 URL）
      r.data.cover = assetUrl(r.data.cover)
      if (Array.isArray(r.data.album)) {
        r.data.album = r.data.album.map((u: string) => assetUrl(u))
      }
      this.setData({
        project: r.data,
        segments: parseRich(r.data.detail || ''),
        // updatedAt 是后端 RFC3339，直接绑会把 T／时区后缀显示出来（同 PC 端 formatDateTime 口径）
        updatedAtText: r.data.updatedAt ? formatDateTime(r.data.updatedAt) : '',
        loading: false,
      })
    } catch {
      this.setData({ loading: false, error: true })
    }
  },
  // 分享带回 staffId 与 companyId：从分享入口进来的访客仍落在对应员工名片链、
  // 对应企业的内容上（不依赖本机粘滞）
  onShareAppMessage() {
    const p = this.data.project
    const cid = getCompanyId()
    return {
      title: p ? p.name : '项目案例',
      path:
        '/pages/project/detail/detail?id=' +
        this.data.id +
        (this.data.staffId ? '&staffId=' + this.data.staffId : '') +
        (cid > 0 ? '&companyId=' + cid : ''),
    }
  },
  preview(e: any) {
    const album = (this.data.project && this.data.project.album) || []
    wx.previewImage({ urls: album, current: e.currentTarget.dataset.url })
  },
})

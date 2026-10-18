import { request, assetUrl } from '../../../utils/request'

Page({
  data: {
    categoryId: 0,
    staffId: 0,
    list: [],
    loading: true,
    error: false,
  },
  onLoad(query: any) {
    this.setData({
      categoryId: parseInt(query.categoryId || '0', 10),
      // 从首页透传（跳转页文档 §5）：子页分享 / 返回名片用
      staffId: parseInt(query.staffId || '0', 10),
    })
    this.load()
  },
  async load() {
    this.setData({ loading: true, error: false })
    try {
      const r = await request('GET', '/api/projects', { categoryId: this.data.categoryId })
      const list = (r.data || []).map((p: any) => ({ ...p, cover: assetUrl(p.cover) }))
      this.setData({ list, loading: false })
      // 标题栏回显分类名（跳转页文档 §1.2）：列表接口已回填 categoryName，无需二次查询；
      // 分类被删导致空名时退化为固定标题
      const name = list.length ? list[0].categoryName : ''
      wx.setNavigationBarTitle({ title: name || '项目案例' })
    } catch {
      this.setData({ loading: false, error: true })
    }
  },
  onTap(e: any) {
    const url =
      '/pages/project/detail/detail?id=' +
      e.currentTarget.dataset.id +
      (this.data.staffId ? '&staffId=' + this.data.staffId : '')
    wx.navigateTo({ url })
  },
})

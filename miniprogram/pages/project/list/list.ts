import { request } from '../../../utils/request'

Page({
  data: {
    categoryId: 0,
    list: [],
  },
  onLoad(query: any) {
    this.setData({ categoryId: parseInt(query.categoryId || '0', 10) })
    this.load()
  },
  async load() {
    const r = await request('GET', '/api/projects', { categoryId: this.data.categoryId })
    this.setData({ list: r.data || [] })
  },
  onTap(e: any) {
    wx.navigateTo({ url: '/pages/project/detail/detail?id=' + e.currentTarget.dataset.id })
  },
})

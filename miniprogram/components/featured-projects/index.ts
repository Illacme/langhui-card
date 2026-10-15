Component({
  options: { addGlobalClass: true },
  properties: {
    featured: Array,
  },
  methods: {
    onTap(e: any) {
      wx.navigateTo({ url: '/pages/project/detail/detail?id=' + e.currentTarget.dataset.id })
    },
  },
})

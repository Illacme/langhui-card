Component({
  options: { addGlobalClass: true },
  properties: {
    categories: Array,
  },
  methods: {
    onTap(e: any) {
      wx.navigateTo({ url: '/pages/project/list/list?categoryId=' + e.currentTarget.dataset.id })
    },
  },
})

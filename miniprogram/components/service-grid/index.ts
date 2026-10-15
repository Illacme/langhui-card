Component({
  options: { addGlobalClass: true },
  properties: {
    services: Array,
  },
  methods: {
    onTap(e: any) {
      wx.navigateTo({ url: '/pages/service/detail/detail?id=' + e.currentTarget.dataset.id })
    },
  },
})

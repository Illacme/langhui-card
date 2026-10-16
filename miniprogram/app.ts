App({
  globalData: {
    // 后端接口基址；生产改为 https 域名（需在微信后台配置 request 合法域名）
    baseUrl: 'http://127.0.0.1:8080',
    // 当前登录员工的 staffId：供「我的 → 我的名片」用 switchTab 回名片页时回带。
    // tabBar 页之间只能 switchTab、不能带 query 参数，所以走 globalData 传递。
    myStaffId: 0,
  },
  onLaunch() {},
})

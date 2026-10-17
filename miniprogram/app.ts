import { bindCompanyFromScene } from './utils/tenant'

App({
  globalData: {
    // 后端接口基址；生产改为 https 域名（需在微信后台配置 request 合法域名）
    baseUrl: 'http://127.0.0.1:8080',
    // 当前登录员工的 staffId：供「我的 → 我的名片」navigateBack 回名片页时回带。
    myStaffId: 0,
  },
  // 冷启动即尝试绑定租户（平台模式）：访客扫企业小程序码进来时，
  // scene / query 携带 companyId，写入 Storage 粘滞（见 utils/tenant.ts）。
  // 企业独立版（PINNED_COMPANY_ID>0）在此函数内被忽略，全程锁定。
  onLaunch(options: any) {
    try {
      bindCompanyFromScene(
        options && (options.scene || (options.query && options.query.scene)),
        options && options.query && options.query.companyId
      )
    } catch {}
  },
})

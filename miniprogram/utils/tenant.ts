// 多租户（第二期：小程序按企业分发）。
//
// 两种部署形态：
//  1. 企业独立小程序版本（tenantConfig.pinnedCompanyId > 0）：
//     整个小程序锁定为该企业的独立版本——一切内容 / 申请 / 分享都不带、也不读
//     任何 companyId 来源（包括扫码 scene），不存在切换多租户的行为。
//     锁定值由后台「系统设置 → 小程序 → pinned_company_id」下发，
//     冷启动时写入下面的 tenantConfig（见 utils/config.ts）；如需强制内置，
//     也可直接把这里改成企业 id（在 PC 后台「企业管理」可查看）。
//  2. 平台模式（pinnedCompanyId = 0，默认）：
//     保留分发能力：访客扫企业专属小程序码（scene 携带 companyId=N）首次进入时
//     绑定该企业并写入 Storage 粘滞，后续打开 / 分享均落该企业；
//     无任何来源时不传 companyId，由后端缺省落 1 号租户（朗汇）。
//
// 注意：员工登录态与名片内容始终按员工归属企业走（token / staffId 维度），
// 本模块只决定「企业内容（业绩/服务/布局）与自助申请」落在哪个租户。

// 企业独立版配置：0=平台模式；非 0=锁定该企业（无任何切换行为）。
// 运行期由后台「系统设置 → 小程序 → pinned_company_id」下发覆盖（见 app.ts / utils/config.ts）；
// 缺省 0，后端未下发配置时保持平台模式。
export const tenantConfig = {
  pinnedCompanyId: 0,
}

// 平台模式下粘滞的租户（扫码 scene 绑定一次，之后本机所有入口都用它）
const STORAGE_KEY = 'bizCompanyId'

// 从 scene / query 字符串里解析 companyId。
// 兼容两种形态：小程序码 scene（"companyId=2&staffId=5"，可能整体 URL-encoded）
// 与页面 query（{ companyId: '2' }）。解析不到 / 非法返回 0。
export function parseCompanyId(raw: string | undefined | null): number {
  if (!raw) return 0
  let text = String(raw)
  try {
    text = decodeURIComponent(text)
  } catch {
    // 非法的 % 转义：按原串继续试
  }
  const m = /(?:^|[?&])companyId=(\d+)/.exec(text)
  if (!m) return 0
  const id = parseInt(m[1], 10)
  return id > 0 ? id : 0
}

// 是否企业独立版（锁定单租户，无切换）
export function isPinned(): boolean {
  return tenantConfig.pinnedCompanyId > 0
}

// 当前生效的租户；0 表示「不传，走后端缺省（朗汇）」。
// 独立版恒为 pinnedCompanyId；平台模式取 Storage 粘滞值。
export function getCompanyId(): number {
  if (isPinned()) return tenantConfig.pinnedCompanyId
  const v = wx.getStorageSync(STORAGE_KEY)
  const id = parseInt(v || '0', 10)
  return id > 0 ? id : 0
}

// 把扫码 / 分享场景里的 companyId 绑定到本机（平台模式，幂等）。
// scene 为小程序码场景串（"companyId=2&staffId=5"，可能整体 URL-encoded）；
// queryCompanyId 为页面 query 的裸值（如 "2" 或 2）。两者都没有则不绑定。
// 独立模式直接忽略——锁定租户不接受任何外部改写。
export function bindCompanyFromScene(scene: string | undefined | null, queryCompanyId?: string | number): void {
  if (isPinned()) return
  const fromScene = parseCompanyId(scene)
  let fromQuery = 0
  if (queryCompanyId !== undefined && queryCompanyId !== null && queryCompanyId !== '') {
    const n = parseInt(String(queryCompanyId), 10)
    if (n > 0) fromQuery = n
  }
  const cid = fromScene || fromQuery
  if (cid > 0) wx.setStorageSync(STORAGE_KEY, String(cid))
}

// 清掉粘滞绑定（联调 / 测试用：恢复「后端缺省朗汇」行为）。
// 独立模式下无意义（不存在绑定），但仍允许清 Storage 残留。
export function resetCompanyBinding(): void {
  wx.removeStorageSync(STORAGE_KEY)
}

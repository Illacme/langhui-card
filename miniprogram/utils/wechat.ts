import { request } from './request'

// 微信身份采集：openid（静默）+ 昵称（需用户授权）。
// 用于申请开通与员工登录时记录微信身份，供 PC 后台核实申请人/登录者。

// 获取微信 openid：wx.login 拿临时 code，后端经 jscode2session 换 openid。
// openid 在本小程序内稳定，缓存到 storage 避免重复换。
// 后端未配置 WX 凭据 / 换取失败时返回空串——不影响账号密码登录与申请（降级）。
export function getOpenid(): Promise<string> {
  return new Promise((resolve) => {
    const cached = wx.getStorageSync('wxOpenid')
    if (cached) {
      resolve(cached)
      return
    }
    wx.login({
      success(res: any) {
        if (!res.code) {
          resolve('')
          return
        }
        request('POST', '/api/wechat/openid', { code: res.code })
          .then((r: any) => {
            if (r && r.code === 0 && r.openid) {
              wx.setStorageSync('wxOpenid', r.openid)
              resolve(r.openid)
            } else {
              resolve('') // 未配置凭据等：降级为空
            }
          })
          .catch(() => resolve(''))
      },
      fail: () => resolve(''),
    })
  })
}

// 获取微信昵称（getUserProfile 授权弹窗）。
// 注意：必须在用户点击手势里同步调用 wx.getUserProfile 才会弹窗，
// 因此本函数被调用时就地触发（调用方应在 tap 处理里、任何 await 之前调用）。
// 用户拒绝 / 基础库不支持时返回空串，不阻塞主流程。
export function getWechatNickname(): Promise<string> {
  return new Promise((resolve) => {
    const wxAny = wx as any
    if (typeof wxAny.getUserProfile !== 'function') {
      resolve('')
      return
    }
    wxAny.getUserProfile({
      desc: '用于展示您的微信昵称，便于管理员核实身份',
      success(res: any) {
        resolve((res.userInfo && res.userInfo.nickName) || '')
      },
      fail: () => resolve(''),
    })
  })
}

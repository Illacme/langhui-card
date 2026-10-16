import http from './request'

// 员工名片小程序码（预览用）：按 staffId 生成，返回可访问 URL。
// 未配置 WX_APPID / WX_APPSECRET 时后端 code=1——这是预期内的降级路径
// （本地开发没配微信凭据），用 __silent 压掉全局错误提示，由调用方展示占位。
export function getWxacode(staffId: number) {
  return http
    .get('/wechat/wxacode', { params: { staffId }, __silent: true } as any)
    .then((r: any) => (r.data && r.data.url) || '')
    .catch(() => '')
}

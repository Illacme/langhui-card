// CDN 厂商的界面侧事实表。
//
// 与后端 storage 包同源（CDNProviders / CDNProviderNeedsResource / CDNProviderNeedsKeyPair），
// 加厂商或改某家的凭据形态时必须两边一起改——界面据此决定显示哪些输入框、
// 「CDN 刷新」页据此点名还缺哪几项。
export const CDN_PROVIDERS = ['aliyun', 'tencent', 'qiniu', 'cloudflare', 'cloudfront']

// 未选厂商时后端按 aliyun 处理（storage.normalizeCDNProvider），界面同步显示，不说「未设置」
export const CDN_DEFAULT_PROVIDER = 'aliyun'

// 中文名：设置页下拉里拼成「值（中文名）」。
// **长度是硬约束**：选项面板按最宽候选撑开，而每行选择框左侧到视口右边只有约 320px，
// 超过约 14 个全角字符就会翻到选择框左侧盖住标签列（详见 SettingRow 的 OPTION_LABELS 注释）。
// 所以这里只放品牌名，各家要填什么写在行内 description 与「CDN 刷新」页的提示里。
export const CDN_PROVIDER_LABELS: Record<string, string> = {
  aliyun: '阿里云',
  tencent: '腾讯云',
  qiniu: '七牛云',
  cloudflare: 'Cloudflare',
  cloudfront: 'AWS CloudFront',
}

// 需要「资源标识」的厂商：Cloudflare 填 Zone ID、CloudFront 填 Distribution ID。
// 其余三家按提交 URL 的域名自己定位加速域名，不需要这一项。
export const CDN_RESOURCE_PROVIDERS = ['cloudflare', 'cloudfront']

// 用不到 AccessKeyId 的厂商：Cloudflare 只认一个 API Token（填在 AccessKeySecret 位）
export const CDN_NO_AK_PROVIDERS = ['cloudflare']

export function cdnProviderLabel(v: string): string {
  const p = (v || '').trim()
  return CDN_PROVIDER_LABELS[p] || p
}

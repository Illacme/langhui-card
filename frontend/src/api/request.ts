import axios, { type AxiosInstance } from 'axios'
import { ElMessage } from 'element-plus'
import router from '@/router'

/**
 * 统一响应体约定（见《三端联调与接口总表》§2）：
 *   { code: 0, msg, data?: ... }   成功
 *   { code: 1, msg }               业务失败（参数错误 / 密码错误 / 文件不支持）
 *   { code: 2, msg }               特殊状态（如名片已停用 / 微信凭据未配置）
 * 鉴权失败：后端一律返回 HTTP 200 + { code: 401 | 403, msg }（见 middleware/auth.go），
 * 因此拦截器以 res.code 为准，而非 HTTP 状态码。
 */
export interface ApiResp {
  code: number
  msg?: string
  [key: string]: any
}

const http: AxiosInstance = axios.create({
  baseURL: import.meta.env.VITE_API_BASE || '/api',
  timeout: 15000,
})

// 请求拦截：自动注入 Bearer token（与 UploadField.vue 约定 localStorage.key='token' 一致）
http.interceptors.request.use((config) => {
  const token = localStorage.getItem('token')
  if (token) {
    config.headers.Authorization = 'Bearer ' + token
  }
  return config
})

// 响应拦截：统一处理 401/403/业务失败，并回传整包（api 函数按需取字段）
http.interceptors.response.use(
  (resp) => {
    const data = resp.data as ApiResp
    if (!data || typeof data !== 'object') return data

    // 401：未登录 / 登录失效 → 清 token 跳登录
    if (data.code === 401) {
      localStorage.removeItem('token')
      localStorage.removeItem('admin_user')
      ElMessage.error(data.msg || '登录失效，请重新登录')
      if (router.currentRoute.value.path !== '/login') {
        router.replace('/login')
      }
      return Promise.reject(new Error(data.msg || '未登录'))
    }
    // 403：角色不符（本后台仅超管，正常不应出现）
    if (data.code === 403) {
      ElMessage.warning(data.msg || '无权限')
      return Promise.reject(new Error(data.msg || '无权限'))
    }
    // 业务失败（code=1 及其他非 0）
    if (data.code !== 0) {
      ElMessage.error(data.msg || '请求失败')
      return Promise.reject(new Error(data.msg || '业务失败'))
    }
    // 成功：回传整包，由各 api 函数提取 token / data / staffId 等
    return data
  },
  (error) => {
    ElMessage.error('网络异常，请稍后重试')
    return Promise.reject(error)
  }
)

export default http

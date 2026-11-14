import axios, { type AxiosRequestConfig, type AxiosResponse, type InternalAxiosRequestConfig } from 'axios'
import { useAuthStore } from '../stores/auth'

const request = axios.create({
  // 空字符串 = 走相对路径，由 nginx 同源反代到 core（生产环境）
  baseURL: (import.meta.env.VITE_API_BASE as string) || '',
  timeout: 10000,
})

request.interceptors.request.use((config: InternalAxiosRequestConfig) => {
  const authStore = useAuthStore()
  if (authStore.accessToken) {
    config.headers.Authorization = `Bearer ${authStore.accessToken}`
  }
  return config
})

// 刷新接口自身也可能返回 401。若它也进入下面的自动刷新分支，就会无限递归，
// 所以这里显式跳过，并用 single-flight 锁合并并发刷新。
const REFRESH_PATH = '/api/v1/auth/refresh'
let refreshInFlight: Promise<void> | null = null

request.interceptors.response.use(
  (response: AxiosResponse) => response,
  async (error: any) => {
    const cfg = error.config
    const isRefreshCall = typeof cfg?.url === 'string' && cfg.url.includes(REFRESH_PATH)
    if (error.response?.status === 401 && cfg && !cfg._retry && !isRefreshCall) {
      cfg._retry = true
      try {
        if (!refreshInFlight) {
          refreshInFlight = useAuthStore().refreshAccessToken().finally(() => {
            refreshInFlight = null
          })
        }
        await refreshInFlight
        cfg.headers.Authorization = `Bearer ${useAuthStore().accessToken}`
        return request(cfg)
      } catch {
        useAuthStore().logout()
        return Promise.reject(error)
      }
    }
    const msg = error.response?.data?.error || '请求失败'
    // 通过全局事件提示，避免依赖具体 UI 库（AdminLayout 里监听渲染 snackbar）
    window.dispatchEvent(new CustomEvent('paily-api-error', { detail: msg }))
    return Promise.reject(error)
  }
)

export default request

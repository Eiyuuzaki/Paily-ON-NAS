import request from './request'
import type { AxiosResponse } from 'axios'

/** 单个来源的实际流量统计（来自 mihomo external-controller） */
export interface TrafficItem {
  source_id: string
  identifier: string
  priority: number
  upload: number
  download: number
  recorded_at: string
}

export interface TrafficStatsResponse {
  sources: TrafficItem[]
  total_upload: number
  total_download: number
  recorded_at: string
  /** 后端是否配置了 MIHOMO_API_URL。false 表示 reporter 根本没启动。 */
  mihomo_configured: boolean
}

export function getTrafficStats(): Promise<AxiosResponse<TrafficStatsResponse>> {
  return request.get<TrafficStatsResponse>('/api/v1/traffic/stats')
}

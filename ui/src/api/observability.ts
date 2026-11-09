import request from './request'
import type { AxiosResponse } from 'axios'

/** 失败原因分类（checker 判死原因，2026-11 上线） */
export interface FailReasonItem {
  reason: string
  count: number
  pct: number
}

export interface FailReasonsResponse {
  window_hours: number
  total_failures: number
  reasons: FailReasonItem[]
}

/** 每轮测活的池子健康度 */
export interface HealthRunItem {
  completed_at: string
  total: number
  alive: number
  alive_pct: number
  p50_latency_ms: number | null
}

export interface HealthTrendResponse {
  window_hours: number
  runs: HealthRunItem[]
}

export function getFailReasons(hours: number): Promise<AxiosResponse<FailReasonsResponse>> {
  return request.get<FailReasonsResponse>('/api/v1/observability/fail-reasons', { params: { hours } })
}

export function getHealthTrend(hours: number): Promise<AxiosResponse<HealthTrendResponse>> {
  return request.get<HealthTrendResponse>('/api/v1/observability/health-trend', { params: { hours } })
}

/** reason 英文码 → 中文显示名（未识别的码原样显示） */
export const REASON_LABELS: Record<string, string> = {
  timeout: '超时',
  conn_refused: '拒绝连接',
  dns: 'DNS 失败',
  tls_handshake: 'TLS 握手失败',
  dial_failed: '拨号失败',
  conn_reset: '连接重置',
  aborted: '探测中断',
  deep_timeout: '深测超时',
  semaphore_timeout: '队列超时',
  other: '其他',
  unknown: '未知(历史)',
}

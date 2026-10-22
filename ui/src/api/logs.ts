import request from './request'
import type { AxiosResponse } from 'axios'
import type { FetchLogListResponse, CheckLogListResponse } from '../types/api'

export function getFetchLogs(params?: Record<string, unknown>): Promise<AxiosResponse<FetchLogListResponse>> {
  return request.get<FetchLogListResponse>('/api/v1/logs/fetch', { params })
}

export function getCheckLogs(params?: Record<string, unknown>): Promise<AxiosResponse<CheckLogListResponse>> {
  return request.get<CheckLogListResponse>('/api/v1/logs/check', { params })
}

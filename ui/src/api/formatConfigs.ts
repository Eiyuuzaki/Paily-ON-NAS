import request from './request'
import type { AxiosResponse } from 'axios'
import type { FormatConfigEntry } from '../types/api'

export type FormatName = 'clash' | 'singbox' | 'base64'

export function getFormatConfig(format: FormatName): Promise<AxiosResponse<FormatConfigEntry>> {
  return request.get<FormatConfigEntry>(`/api/v1/format-configs/${format}`)
}

export function updateFormatConfig(format: FormatName, data: { config: Record<string, unknown> }): Promise<AxiosResponse<FormatConfigEntry>> {
  return request.put<FormatConfigEntry>(`/api/v1/format-configs/${format}`, data)
}

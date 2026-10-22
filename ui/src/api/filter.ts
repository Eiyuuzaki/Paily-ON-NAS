import request from './request'
import type { AxiosResponse } from 'axios'
import type { FilterPreviewRequest, FilterPreviewResponse } from '../types/api'

export function previewFilter(data: FilterPreviewRequest): Promise<AxiosResponse<FilterPreviewResponse>> {
  return request.post<FilterPreviewResponse>('/api/v1/filter/preview', data)
}

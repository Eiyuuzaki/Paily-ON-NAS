import request from './request'
import type { AxiosResponse } from 'axios'
import type {
  SourceListResponse,
  SourceDetail,
  NodeListResponse,
  SourceCreateRequest,
  SourceUpdateRequest,
} from '../types/api'

export function getSources(params?: Record<string, unknown>): Promise<AxiosResponse<SourceListResponse>> {
  return request.get<SourceListResponse>('/api/v1/sources', { params })
}

export function getSource(id: string): Promise<AxiosResponse<SourceDetail>> {
  return request.get<SourceDetail>(`/api/v1/sources/${id}`)
}

// POST /api/v1/sources uses admin JWT auth
export function createSource(data: SourceCreateRequest): Promise<AxiosResponse<SourceDetail>> {
  return request.post<SourceDetail>('/api/v1/sources', data)
}

export function updateSource(id: string, data: SourceUpdateRequest): Promise<AxiosResponse<SourceDetail>> {
  return request.put<SourceDetail>(`/api/v1/sources/${id}`, data)
}

export function deleteSource(id: string): Promise<AxiosResponse<void>> {
  return request.delete(`/api/v1/sources/${id}`)
}

export function searchSources(data: { expr: string; page?: number; limit?: number }): Promise<AxiosResponse<SourceListResponse>> {
  return request.post<SourceListResponse>('/api/v1/sources/search', data)
}

export function ignoreDeadSource(id: string, ignore_dead: boolean): Promise<AxiosResponse<void>> {
  return request.post(`/api/v1/sources/${id}/ignore-dead`, { ignore_dead })
}

export function getSourceNodes(id: string, params?: Record<string, unknown>): Promise<AxiosResponse<NodeListResponse>> {
  return request.get<NodeListResponse>(`/api/v1/sources/${id}/nodes`, { params })
}

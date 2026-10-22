import request from './request'
import type { AxiosResponse } from 'axios'
import type { NodeListResponse, NodeDetail, InitialCheckListResponse, DeepCheckListResponse } from '../types/api'

export function getNodes(params?: Record<string, unknown>): Promise<AxiosResponse<NodeListResponse>> {
  return request.get<NodeListResponse>('/api/v1/nodes', { params })
}

export function getNode(id: string): Promise<AxiosResponse<NodeDetail>> {
  return request.get<NodeDetail>(`/api/v1/nodes/${id}`)
}

export function searchNodes(data: { expr: string; page?: number; limit?: number }): Promise<AxiosResponse<NodeListResponse>> {
  return request.post<NodeListResponse>('/api/v1/nodes/search', data)
}

export function deleteNode(id: string): Promise<AxiosResponse<void>> {
  return request.delete(`/api/v1/nodes/${id}`)
}

export function getInitialChecks(id: string, params?: Record<string, unknown>): Promise<AxiosResponse<InitialCheckListResponse>> {
  return request.get<InitialCheckListResponse>(`/api/v1/nodes/${id}/checks/initial`, { params })
}

export function getDeepChecks(id: string, params?: Record<string, unknown>): Promise<AxiosResponse<DeepCheckListResponse>> {
  return request.get<DeepCheckListResponse>(`/api/v1/nodes/${id}/checks/deep`, { params })
}

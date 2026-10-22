import request from './request'
import type { AxiosResponse } from 'axios'
import type { SponsorListResponse, SponsorSummary, SponsorCreateRequest, SponsorUpdateRequest } from '../types/api'

export function getSponsors(params?: Record<string, unknown>): Promise<AxiosResponse<SponsorListResponse>> {
  return request.get<SponsorListResponse>('/api/v1/sponsors', { params })
}

export function createSponsor(data: SponsorCreateRequest): Promise<AxiosResponse<SponsorSummary>> {
  return request.post<SponsorSummary>('/api/v1/sponsors', data)
}

export function updateSponsor(id: string, data: SponsorUpdateRequest): Promise<AxiosResponse<SponsorSummary>> {
  return request.put<SponsorSummary>(`/api/v1/sponsors/${id}`, data)
}

export function deleteSponsor(id: string): Promise<AxiosResponse<void>> {
  return request.delete(`/api/v1/sponsors/${id}`)
}

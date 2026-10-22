import request from './request'
import type { AxiosResponse } from 'axios'

export interface StatsResponse {
    total_nodes: number
    alive_nodes: number
    total_sources: number
    dead_sources: number
}

export function getStats(): Promise<AxiosResponse<StatsResponse>> {
    return request.get<StatsResponse>('/api/v1/stats')
}

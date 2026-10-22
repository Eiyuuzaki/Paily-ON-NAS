import request from './request'
import type { AxiosResponse } from 'axios'
import type { CheckerListResponse } from '../types/api'

export function getCheckers(): Promise<AxiosResponse<CheckerListResponse>> {
    return request.get<CheckerListResponse>('/api/v1/checkers')
}

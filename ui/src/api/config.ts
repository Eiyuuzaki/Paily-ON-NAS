import request from './request'
import type { AxiosResponse } from 'axios'
import type { ConfigMap } from '../types/api'

export function getConfig(): Promise<AxiosResponse<ConfigMap>> {
  return request.get<ConfigMap>('/api/v1/config')
}

export function updateConfig(data: ConfigMap): Promise<AxiosResponse<ConfigMap>> {
  return request.put<ConfigMap>('/api/v1/config', data)
}

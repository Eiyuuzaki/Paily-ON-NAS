import request from './request'
import type { AxiosResponse } from 'axios'
import type { LoginResponse, RefreshResponse } from '../types/api'

export function login(password: string): Promise<AxiosResponse<LoginResponse>> {
  return request.post<LoginResponse>('/api/v1/auth/login', { password })
}

export function refreshToken(refresh_token: string): Promise<AxiosResponse<RefreshResponse>> {
  return request.post<RefreshResponse>('/api/v1/auth/refresh', { refresh_token })
}

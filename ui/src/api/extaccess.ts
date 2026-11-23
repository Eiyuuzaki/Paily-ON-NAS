import request from './request'

export interface ExtAccessInfo {
  token: string
  enabled: boolean
  public_host: string
  urls: { clash: string; singbox: string; base64: string }
  lan_port: number
}

export interface ExtTestResult {
  ok: boolean
  http_code: number
  latency_ms: number
  error?: string
}

export function getExtAccess(): Promise<{ data: { data: ExtAccessInfo } }> {
  return request.get('/api/v1/extaccess')
}

export function setExtToken(token: string): Promise<{ data: { data: { token: string } } }> {
  return request.put('/api/v1/extaccess/token', { token })
}

export function rotateExtToken(): Promise<{ data: { data: { token: string } } }> {
  return request.post('/api/v1/extaccess/token/rotate')
}

export function setExtHost(public_host: string): Promise<{ data: { data: { public_host: string } } }> {
  return request.put('/api/v1/extaccess/host', { public_host })
}

export function testExtAccess(): Promise<{ data: { data: ExtTestResult } }> {
  return request.post('/api/v1/extaccess/test')
}

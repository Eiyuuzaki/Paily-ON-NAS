import request from './request'

export interface CFAccount {
  id: string
  name: string
  role: string
  worker_url: string
  worker_name: string
  subdomain: string
  uuid: string
  repo_url: string
  enabled: boolean
  health: string
  last_check_at: string | null
  note: string
}

export interface CFModuleStatus {
  activated: boolean
  repo_url: string
}

export function getCFStatus(): Promise<{ data: CFModuleStatus }> {
  return request.get('/api/v1/cf/module')
}

export function activateCF(repoUrl: string): Promise<{ data: CFModuleStatus }> {
  return request.post('/api/v1/cf/module/activate', { repo_url: repoUrl })
}

export function deactivateCF(): Promise<{ data: { activated: boolean } }> {
  return request.post('/api/v1/cf/module/deactivate')
}

export function getCFAccounts(): Promise<{ data: { data: CFAccount[] } }> {
  return request.get('/api/v1/cf/accounts')
}

export function createCFAccount(data: Partial<CFAccount>): Promise<{ data: CFAccount }> {
  return request.post('/api/v1/cf/accounts', data)
}

export function updateCFAccount(id: string, data: Partial<CFAccount>): Promise<{ data: CFAccount }> {
  return request.put(`/api/v1/cf/accounts/${id}`, data)
}

export function deleteCFAccount(id: string): Promise<void> {
  return request.delete(`/api/v1/cf/accounts/${id}`)
}

export function checkCFAccount(id: string): Promise<{ data: { health: string; checked_at: string } }> {
  return request.post(`/api/v1/cf/accounts/${id}/check`)
}
export interface CFAccSummary {
  id: string
  name: string
  role: string
  worker_url: string
  health: string
  node_alive: number
  node_total: number
  last_check_at: string | null
}

export interface CFSummary {
  accounts: CFAccSummary[]
  cf_alive: number
  cf_total: number
  probes_today: number
  quota_daily: number
  quota_reset_hours: number
  ech_enabled: boolean
  egress_regions: { region: string; cnt: number }[]
}

export function getCFSummary(): Promise<{ data: CFSummary }> {
  return request.get('/api/v1/cf/summary')
}
export async function setCFToken(accountId: string, token: string) {
  return request.post("/api/v1/cf/accounts/" + accountId + "/token", { token })
}

export async function getCFTokenStatus(accountId: string) {
  return request.get("/api/v1/cf/accounts/" + accountId + "/token")
}
export interface CFSelfCheckResult {
  name: string
  ok: boolean
  ip?: string
  loc?: string
  colo?: string
  latency_ms: number
  detail?: string
}

export function runCFSelfCheck() {
  return request.post("/api/v1/cf/selfcheck", null, { timeout: 20000 })
}
export interface CFUsage {
  total_today: number
  quota_daily: number
  percent: number
  hourly: { hour: string; requests: number }[]
  account_tag: string
}

export function getAccountUsage(accountId: string) {
  return request.get("/api/v1/cf/accounts/" + accountId + "/usage")
}
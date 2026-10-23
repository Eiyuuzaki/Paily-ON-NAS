// Shared API response types matching spec.yaml

export interface PaginationInfo {
    page: number
    limit: number
    total: number
    total_pages: number
}

// ── Auth ──────────────────────────────────────────────────────────────────

export interface LoginResponse {
    token: string
    refresh_token: string
}

export interface RefreshResponse {
    token: string
}

// ── Source ────────────────────────────────────────────────────────────────

export type SourceType = 'subscribe' | 'node'
export type SourceStatus = 'active' | 'dead'

export interface SourceSummary {
    id: string
    type: SourceType
    identifier: string
    info: string
    status: SourceStatus
    ignore_dead?: boolean
    content?: string
    latest_fetch_node_count?: number
    alive_node_count?: number
    priority?: number
    upload?: number
    download?: number
    total?: number
    expire?: number
}

export interface SourceDetail extends SourceSummary {
    content: string
    created_at: string
    updated_at: string
}

export interface SourceListResponse {
    data: SourceSummary[]
    pagination: PaginationInfo
}

export interface SourceCreateRequest {
    type: SourceType
    content: string
    identifier?: string
    info?: string
}

export interface SourceUpdateRequest {
    identifier?: string
    info?: string
    content?: string
    priority?: number
}

// ── Node ─────────────────────────────────────────────────────────────────

export interface NodeSummary {
    id: string
    hash: string
    server: string
    protocol: string
    region: string
    alive: boolean
    score: number
    latest_latency_ms?: number
    latest_avg_latency_ms?: number
    latest_jitter_ms?: number
    sources?: SourceSummary[]
}

export interface DeepCheckRecord {
    id: number
    avg_latency_ms: number
    jitter_ms: number
    speed_kbps: number
    streaming_result: Record<string, boolean>
    metadata?: Record<string, unknown>
    checked_at: string
    checker_tag?: string
    check_run_id?: string
}

export interface InitialCheckRecord {
    id: number
    latency_ms: number
    checked_at: string
    checker_tag?: string
    check_run_id?: string
}

export interface NodeDetail extends NodeSummary {
    password?: string
    raw?: Record<string, unknown>
    latest_deep_check?: DeepCheckRecord
    tag_scores?: Record<string, number>
    latest_deep_check_by_tag?: Record<string, DeepCheckRecord>
    sources?: SourceSummary[]
    created_at: string
    updated_at: string
}

export interface NodeListResponse {
    data: NodeSummary[]
    pagination: PaginationInfo
}

export interface InitialCheckListResponse {
    data: InitialCheckRecord[]
    pagination: PaginationInfo
}

export interface DeepCheckListResponse {
    data: DeepCheckRecord[]
    pagination: PaginationInfo
}

// ── Checker ───────────────────────────────────────────────────────────────

export interface CheckerListResponse {
    checkers: string[]
}

// ── Sponsor ───────────────────────────────────────────────────────────────

export interface SponsorSummary {
    id: string
    name: string
    text: string
    filter_expr: string
    priority: number
    created_at?: string
    updated_at?: string
}

export interface SponsorListResponse {
    data: SponsorSummary[]
    pagination: PaginationInfo
}

export interface SponsorCreateRequest {
    name: string
    text: string
    filter_expr: string
    priority: number
}

export interface SponsorUpdateRequest {
    name?: string
    text?: string
    filter_expr?: string
    priority?: number
}

// ── Config ────────────────────────────────────────────────────────────────

export type ConfigMap = Record<string, string>

// ── Logs ──────────────────────────────────────────────────────────────────

export interface FetchLogEntry {
    id: number
    source_id: string
    success: boolean
    node_count?: number
    fetched_at: string
}

export interface CheckLogEntry {
    id: number
    node_id: string
    latency_ms: number
    checked_at: string
}

export interface FetchLogListResponse {
    data: FetchLogEntry[]
    pagination: PaginationInfo
}

export interface CheckLogListResponse {
    data: CheckLogEntry[]
    pagination: PaginationInfo
}

// ── Filter ────────────────────────────────────────────────────────────────

export type FilterTarget = 'node' | 'source'

export interface FilterPreviewRequest {
    expr: string
    target: FilterTarget
}

export interface FilterPreviewResponse {
    nodes?: NodeSummary[]
    sources?: SourceSummary[]
}

// ── FormatConfig ──────────────────────────────────────────────────────────

export interface FormatConfigEntry {
    format_name: string
    config: Record<string, unknown>
    updated_at?: string
}

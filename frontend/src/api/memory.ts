import { get, put, post, del } from '@/utils/request'

// Kinds mirror internal/types/memory.go. profile and preference make up the
// block injected on every turn; fact and task are pulled in only when the
// current question matches them.
export type MemoryKind = 'profile' | 'preference' | 'fact' | 'task'
export type MemoryStatus = 'active' | 'superseded' | 'archived'
export type MemoryOrigin = 'explicit' | 'extracted' | 'manual'

export interface MemoryItem {
  id: string
  kind: MemoryKind
  content: string
  topic: string
  importance: number
  origin: MemoryOrigin
  status: MemoryStatus
  source_session_id: string
  source_message_id: string
  valid_from: string
  invalid_at: string | null
  superseded_by: string
  last_used_at: string | null
  use_count: number
  created_at: string
  updated_at: string
}

// MemorySettings is already merged server-side, so the UI never has to combine
// a workspace switch with a personal one itself.
export interface MemorySettings {
  workspace_enabled: boolean
  user_enabled: boolean
  effective: boolean
  write_mode: string
  item_count: number
  max_items: number
}

export interface MemoryConfig {
  enabled: boolean
  write_mode: 'explicit_only' | 'auto'
  extract_model_id: string
  max_items: number
}

// ---------------------------------------------------------------------------
// Personal memory. Every endpoint operates on the caller's own memory space,
// which the server derives from the request principal, so none of these take
// an owner parameter.
// ---------------------------------------------------------------------------

export function getMemorySettings() {
  return get<{ success: boolean; data: MemorySettings }>('/api/v1/memory/settings')
}

export function updateMemoryEnabled(enabled: boolean) {
  return put<{ success: boolean; data: MemorySettings }>('/api/v1/memory/settings', { enabled })
}

export function listMemoryItems(params: { status?: MemoryStatus; limit?: number; offset?: number } = {}) {
  const query = new URLSearchParams()
  if (params.status) query.set('status', params.status)
  if (params.limit != null) query.set('limit', String(params.limit))
  if (params.offset != null) query.set('offset', String(params.offset))
  const suffix = query.toString() ? `?${query.toString()}` : ''
  return get<{ success: boolean; data: MemoryItem[]; total: number }>(`/api/v1/memory/items${suffix}`)
}

export function createMemoryItem(payload: { kind: MemoryKind; content: string; importance?: number }) {
  return post<{ success: boolean; data: MemoryItem }>('/api/v1/memory/items', payload)
}

export function updateMemoryItem(id: string, payload: { content: string; importance: number }) {
  return put<{ success: boolean; data: MemoryItem }>(
    `/api/v1/memory/items/${encodeURIComponent(id)}`,
    payload,
  )
}

export function deleteMemoryItem(id: string) {
  return del<{ success: boolean }>(`/api/v1/memory/items/${encodeURIComponent(id)}`)
}

export function clearMemoryItems() {
  return del<{ success: boolean; removed: number }>('/api/v1/memory/items')
}

export function exportMemoryItems() {
  return get<{ success: boolean; total: number; data: MemoryItem[] }>('/api/v1/memory/export')
}

// ---------------------------------------------------------------------------
// Workspace configuration, stored on the tenant like the other KV configs.
// ---------------------------------------------------------------------------

export function getTenantMemoryConfig() {
  return get<{ success: boolean; data: MemoryConfig }>('/api/v1/tenants/kv/memory-config')
}

export function updateTenantMemoryConfig(config: MemoryConfig) {
  return put<{ success: boolean; data: MemoryConfig }>('/api/v1/tenants/kv/memory-config', config)
}

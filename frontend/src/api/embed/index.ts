import { get, post, put, del } from '@/utils/request'

export interface EmbedChannel {
  id: string
  tenant_id: number
  agent_id: string
  name: string
  enabled: boolean
  allowed_origins: string[]
  welcome_message: string
  rate_limit_per_minute: number
  rate_limit_per_day?: number
  primary_color?: string
  page_title?: string
  header_title_mode?: HeaderTitleMode
  show_suggested_questions?: boolean
  widget_position?: WidgetPosition
  publish_token?: string
  created_at: string
  updated_at: string
}

export interface EmbedChannelPublicConfig {
  channel_id: string
  name: string
  display_title?: string
  knowledge_base_ids?: string[]
  agent_id: string
  agent_name?: string
  agent_avatar?: string
  welcome_message: string
  primary_color?: string
  page_title?: string
  header_title_mode?: HeaderTitleMode
  show_suggested_questions?: boolean
}

export type HeaderTitleMode = 'channel' | 'session'
export type WidgetPosition = 'bottom-right' | 'bottom-left' | 'top-right' | 'top-left'

export async function listEmbedChannels(agentId: string) {
  return get<{ success: boolean; data: EmbedChannel[] }>(`/api/v1/agents/${agentId}/embed-channels`)
}

export async function createEmbedChannel(agentId: string, data: Partial<EmbedChannel>) {
  return post<{ success: boolean; data: EmbedChannel }>(`/api/v1/agents/${agentId}/embed-channels`, data)
}

export async function updateEmbedChannel(channelId: string, data: Partial<EmbedChannel>) {
  return put<{ success: boolean; data: EmbedChannel }>(`/api/v1/embed-channels/${channelId}`, data)
}

export async function deleteEmbedChannel(channelId: string) {
  return del(`/api/v1/embed-channels/${channelId}`)
}

export async function rotateEmbedToken(channelId: string) {
  return post<{ success: boolean; data: EmbedChannel }>(`/api/v1/embed-channels/${channelId}/rotate-token`, {})
}

/** Short-lived session token for management UI preview (JWT auth, no publish token needed). */
export async function issueEmbedPreviewSession(channelId: string) {
  return post<{ success: boolean; data: { session_token: string; expires_in: number } }>(
    `/api/v1/embed-channels/${channelId}/preview-session`,
    {},
  )
}

export interface SuggestedQuestion {
  question: string
  source?: string
}

export async function getEmbedChunkById(channelId: string, token: string, chunkId: string) {
  return get<{ success: boolean; data: { content?: string } }>(
    `/api/v1/embed/${channelId}/chunks/${chunkId}`,
    { headers: { Authorization: `Embed ${token}` } },
  )
}

export async function getEmbedSuggestedQuestions(channelId: string, token: string, limit = 6) {
  return get<{ success: boolean; data: { questions: SuggestedQuestion[] } }>(
    `/api/v1/embed/${channelId}/suggested-questions?limit=${limit}`,
    { headers: { Authorization: `Embed ${token}` } },
  )
}

export async function getEmbedConfig(channelId: string, token: string) {
  return get<{ success: boolean; data: EmbedChannelPublicConfig }>(
    `/api/v1/embed/${channelId}/config`,
    { headers: { Authorization: `Embed ${token}` } },
  )
}

export async function createEmbedSession(channelId: string, token: string) {
  return post<{ success: boolean; data: { id: string } }>(
    `/api/v1/embed/${channelId}/sessions`,
    {},
    { headers: { Authorization: `Embed ${token}` } },
  )
}

export async function exchangeEmbedSession(channelId: string, publishToken: string) {
  return post<{ success: boolean; data: { session_token: string; expires_in: number } }>(
    `/api/v1/embed/${channelId}/exchange`,
    {},
    { headers: { Authorization: `Embed ${publishToken}` } },
  )
}

export async function getEmbedMessageList(
  channelId: string,
  token: string,
  sessionId: string,
  limit: number,
  beforeTime?: string,
) {
  const params = new URLSearchParams({ limit: String(limit) })
  if (beforeTime) {
    params.set('before_time', beforeTime)
  }
  return get<{ success: boolean; data: unknown[] }>(
    `/api/v1/embed/${channelId}/messages/${sessionId}/load?${params.toString()}`,
    { headers: { Authorization: `Embed ${token}` } },
  )
}

const EMBED_MSG_SOURCE = 'weknora-embed'
const EMBED_HOST_SOURCE = 'weknora-host'

// The exact parent origin, learned from the first trusted host message
// (trust-on-first-use). Once known, every inbound/outbound message is pinned to
// it so conversation content is never broadcast to an unexpected window.
let verifiedParentOrigin = ''

function referrerParentOrigin(): string {
  if (window.parent === window) return ''
  try {
    if (document.referrer) {
      return new URL(document.referrer).origin
    }
  } catch {
    // ignore malformed referrer
  }
  return ''
}

/** Best-known parent origin: verified handshake first, then referrer. */
function knownParentOrigin(): string {
  return verifiedParentOrigin || referrerParentOrigin()
}

function isTrustedParentMessage(event: MessageEvent): boolean {
  if (window.parent === window) return false
  if (event.source !== window.parent) return false
  if (!event.data || event.data.source !== EMBED_HOST_SOURCE) return false
  if (typeof event.origin !== 'string' || event.origin === 'null') return false
  const expected = knownParentOrigin()
  if (expected) {
    if (event.origin !== expected) return false
  } else {
    // First trusted handshake with no referrer hint: pin to this origin.
    verifiedParentOrigin = event.origin
  }
  return true
}

/**
 * Post a message to the host page.
 * `sensitive` payloads (conversation content) are dropped when the parent
 * origin is unknown rather than broadcast to '*'. Non-sensitive handshake
 * messages (bootstrap_request/ready) may fall back to '*' so token handoff can
 * still bootstrap when the referrer is stripped.
 */
function postToParent(payload: Record<string, unknown>, opts?: { sensitive?: boolean }) {
  if (window.parent === window) return
  const target = knownParentOrigin()
  if (!target) {
    if (opts?.sensitive) return
    window.parent.postMessage({ source: EMBED_MSG_SOURCE, ...payload }, '*')
    return
  }
  window.parent.postMessage({ source: EMBED_MSG_SOURCE, ...payload }, target)
}

/** Notify the parent page that the embed widget is ready. */
export function postEmbedReady(channelId: string) {
  postToParent({ type: 'ready', channel_id: channelId })
}

/** Request a publish token from the parent host page. */
export function postEmbedBootstrapRequest(channelId: string) {
  postToParent({ type: 'bootstrap_request', channel_id: channelId })
}

/** Notify the parent page when a user message is sent. */
export function postEmbedMessageSent(channelId: string, sessionId: string, query: string) {
  postToParent(
    {
      type: 'message_sent',
      channel_id: channelId,
      session_id: sessionId,
      query,
    },
    { sensitive: true },
  )
}

/** Notify the parent page when an assistant reply completes. */
export function postEmbedMessageReceived(channelId: string, sessionId: string, content: string) {
  postToParent(
    {
      type: 'message_received',
      channel_id: channelId,
      session_id: sessionId,
      content,
    },
    { sensitive: true },
  )
}

export function parseEmbedTokenFromLocation(): string {
  const queryToken = new URLSearchParams(window.location.search).get('token')
  if (queryToken) return queryToken

  const hash = window.location.hash.startsWith('#') ? window.location.hash.slice(1) : ''
  if (!hash) return ''
  return new URLSearchParams(hash).get('token') || ''
}

export function buildEmbedURL(channelId: string, token?: string) {
  const base = window.location.origin
  const path = `${base}/embed/${encodeURIComponent(channelId)}`
  if (!token) return path
  return `${path}#token=${encodeURIComponent(token)}`
}

/** Escape a value for safe interpolation inside an HTML double-quoted attribute. */
function escapeHtmlAttr(value: string): string {
  return String(value)
    .replace(/&/g, '&amp;')
    .replace(/"/g, '&quot;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
}

/** Validate that a base URL is a well-formed http(s) origin; fall back otherwise. */
function safeBaseUrl(raw?: string): string {
  const fallback = window.location.origin
  if (!raw) return fallback
  try {
    const u = new URL(raw, window.location.href)
    if (u.protocol !== 'http:' && u.protocol !== 'https:') return fallback
    return u.origin
  } catch {
    return fallback
  }
}

export function buildEmbedSnippet(channelId: string, token?: string) {
  // A bare iframe has no token-handoff host, so the snippet must carry the
  // publish token in the URL hash, otherwise the embed page cannot bootstrap.
  const url = escapeHtmlAttr(buildEmbedURL(channelId, token))
  return `<iframe src="${url}" style="width:400px;height:600px;border:none;border-radius:12px" allow="clipboard-write"></iframe>`
}

export function buildWidgetSnippet(
  channelId: string,
  token: string,
  opts?: { primaryColor?: string; title?: string; position?: WidgetPosition; baseUrl?: string },
) {
  const base = safeBaseUrl(opts?.baseUrl)
  const position = opts?.position || 'bottom-right'
  const attrs = [
    `src="${escapeHtmlAttr(`${base}/weknora-widget.js`)}"`,
    `data-channel="${escapeHtmlAttr(channelId)}"`,
    `data-token="${escapeHtmlAttr(token)}"`,
    `data-position="${escapeHtmlAttr(position)}"`,
  ]
  if (opts?.primaryColor) attrs.push(`data-primary-color="${escapeHtmlAttr(opts.primaryColor)}"`)
  if (opts?.title) attrs.push(`data-title="${escapeHtmlAttr(opts.title)}"`)
  return `<script ${attrs.join('\n        ')}></script>`
}

/** Listen for context injected by the parent page (embed host). */
export function onEmbedHostContext(handler: (payload: Record<string, unknown>) => void) {
  const listener = (e: MessageEvent) => {
    if (!isTrustedParentMessage(e) || e.data.type !== 'set_context') return
    handler(e.data.payload || {})
  }
  window.addEventListener('message', listener)
  return () => window.removeEventListener('message', listener)
}

/** Listen for a publish token provided by the parent host page. */
export function onEmbedHostToken(handler: (token: string, channelId?: string) => void) {
  const listener = (e: MessageEvent) => {
    if (!isTrustedParentMessage(e) || e.data.type !== 'provide_token') return
    const token = String(e.data.token || '').trim()
    if (!token) return
    handler(token, e.data.channel_id)
  }
  window.addEventListener('message', listener)
  return () => window.removeEventListener('message', listener)
}

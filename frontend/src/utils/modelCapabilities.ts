/**
 * Model capability manifest, served by GET /models/capabilities.
 *
 * The backend renders it from the same plugin descriptor the request path
 * uses, so a control this form offers is a control that will actually reach
 * the vendor. This replaces the provider heuristics that used to live in the
 * frontend and had to be kept aligned with the Go code by hand — a comment
 * asking future readers to do that is not a mechanism, and the two drifted.
 *
 * This module is deliberately free of I/O: fetching lives in the model API
 * client, so the reading helpers stay testable on their own.
 */

export type ValueKind = 'bool' | 'int' | 'float' | 'enum'
export type Widget = 'switch' | 'select' | 'number' | 'slider'

export interface EnumOption {
  value: string
  label_key?: string
  help_key?: string
}

export interface CapabilityValue {
  kind: ValueKind
  bool?: boolean
  num?: number
  str?: string
}

export interface FieldSchema {
  id: string
  kind: ValueKind
  widget: Widget
  label_key?: string
  help_key?: string
  options?: EnumOption[]
  min?: number
  max?: number
  default?: CapabilityValue
  /** The request field this control writes, shown as a hint in the editor. */
  wire_field?: string
  doc_url?: string
}

export interface GroupSchema {
  key: string
  fields: FieldSchema[]
}

export interface ModelCapabilities {
  vendor: string
  display_name?: string
  protocol: string
  protocols?: string[]
  groups: GroupSchema[]
  supports_thinking: boolean
  reasoning_replay?: string
  doc_url?: string
}

/** The neutral parameter ids the editor and the debug drawer refer to. */
export const PARAM_THINKING_MODE = 'thinking.mode'
export const PARAM_THINKING_EFFORT = 'thinking.effort'
export const PARAM_THINKING_BUDGET = 'thinking.budget'

/** Reported when a model has no thinking toggle at all. */
export const THINKING_CONTROL_NONE = 'none'

/**
 * The legacy override values a model may still have stored in
 * `extra_config.thinking_control`. The backend continues to honor them, so the
 * editor keeps offering them for deployments that pinned one.
 */
export const LEGACY_THINKING_CONTROLS = [
  'none',
  'chat_template_kwargs',
  'enable_thinking',
  'thinking_type',
] as const

/** Find one field in a manifest, across groups. */
export function findField(
  capabilities: ModelCapabilities | null,
  id: string,
): FieldSchema | undefined {
  if (!capabilities) return undefined
  for (const group of capabilities.groups ?? []) {
    const field = group.fields.find(f => f.id === id)
    if (field) return field
  }
  return undefined
}

/**
 * The request field carrying this model's thinking toggle, or 'none'.
 *
 * The value is a wire field such as 'enable_thinking' or 'thinking.type'
 * rather than a category name, because the field is what the old setting was
 * always standing in for.
 */
export function thinkingControlOf(capabilities: ModelCapabilities | null): string {
  return findField(capabilities, PARAM_THINKING_MODE)?.wire_field ?? THINKING_CONTROL_NONE
}

/**
 * Whether a model exposes a thinking toggle, honoring a stored legacy override
 * because the backend still does.
 */
export function supportsThinking(
  capabilities: ModelCapabilities | null,
  storedControl?: string,
): boolean {
  const stored = storedControl?.trim().toLowerCase()
  if (stored) return stored !== THINKING_CONTROL_NONE
  return capabilities?.supports_thinking ?? false
}

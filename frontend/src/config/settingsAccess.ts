export type SettingsRoleKey = 'viewer' | 'contributor' | 'admin' | 'owner'

/**
 * Workspace-scoped settings access policy.
 *
 * Keep this as the single frontend source of truth for both the complete
 * Settings navigation and any shortcuts that lead into it. Backend route
 * guards remain authoritative.
 */
export const SETTINGS_SECTION_MIN_ROLE: Record<string, SettingsRoleKey> = {
  general: 'viewer',
  ollama: 'admin',
  weknoracloud: 'admin',
  models: 'viewer',
  websearch: 'admin',
  chathistory: 'admin',
  // Workspace memory policy decides what every member's memory may do, and the
  // backend guards PUT /memory/tenant-settings with Admin. Showing the entry to
  // anyone else would only produce a 403 on save.
  memory: 'admin',
  // A person's own memory settings are theirs; the backend scopes them to the
  // request principal, so any member of the workspace may open this.
  'memory-personal': 'viewer',
  vectorstore: 'admin',
  parser: 'admin',
  storage: 'admin',
  mcp: 'admin',
  system: 'viewer',
  userprofile: 'viewer',
  tenant: 'viewer',
  members: 'viewer',
}

/**
 * A management-labelled avatar shortcut has a stricter threshold than the
 * corresponding read-only Settings page.
 */
export const SETTINGS_MANAGEMENT_SHORTCUT_MIN_ROLE = {
  members: 'owner',
  models: 'admin',
} as const satisfies Record<string, SettingsRoleKey>

export const SYSTEM_ADMIN_SETTINGS_SECTIONS = new Set([
  'system-global',
  'runtime-queues',
  'platform-api-keys',
  'system-audit-log',
])

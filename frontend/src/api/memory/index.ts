import { get, getDown, post, put, del } from "../../utils/request";

// Long-term memory API client.
//
// Note there is no space or user identifier anywhere in these signatures. The
// backend derives the memory space from the request principal and refuses to
// accept one from the client, so the UI cannot address anyone else's memory
// even by accident.

// A hierarchical slug ("preference/answer-style") travels as a path, so each
// segment is encoded separately; encoding the whole slug would escape the "/"
// and break routing.
function encodeSlugPath(slug: string): string {
  return slug.split("/").map(encodeURIComponent).join("/");
}

// The shared `get` helper takes an axios config, not a params object, so query
// parameters have to travel as `{ params }`. Passing them positionally looks
// right and silently sends no query string at all, which is a failure mode that
// only shows up as "the backend ignored my filter".
function query(params: Record<string, any>) {
  const clean: Record<string, any> = {};
  for (const [key, value] of Object.entries(params)) {
    if (value !== undefined && value !== null && value !== "") clean[key] = value;
  }
  return { params: clean };
}

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export const MEMORY_TYPES = [
  "profile",
  "preference",
  "project",
  "entity",
  "topic",
  "episode",
  "open_question",
] as const;

export type MemoryType = (typeof MEMORY_TYPES)[number];

export interface MemoryPreference {
  language?: string;
  verbosity?: string;
  tone?: string;
  format?: string;
  code_style?: string;
  avoid_topics?: string[];
}

export interface MemoryPage {
  id: string;
  space_id: string;
  slug: string;
  title: string;
  page_type: MemoryType;
  status: "active" | "archived" | "superseded";
  content: string;
  summary: string;
  structured: MemoryPreference;
  aliases: string[];
  in_links: string[];
  out_links: string[];
  folder_path: string[];
  strength: number;
  hit_count: number;
  confidence: number;
  pinned: boolean;
  superseded_by?: string;
  note_refs: string[];
  version: number;
  last_edit_source: string;
  last_seen_at?: string;
  created_at: string;
  updated_at: string;
}

export interface MemoryPageListResponse {
  pages: MemoryPage[];
  total: number;
  page: number;
  page_size: number;
}

export interface MemoryNote {
  id: string;
  space_id: string;
  note_type: MemoryType;
  statement: string;
  subject: string;
  scope: string;
  confidence: number;
  sensitivity: string;
  source: string;
  session_id: string;
  source_message_ids: string[];
  status: "pending" | "merged" | "rejected" | "expired";
  merged_page_id: string;
  created_at: string;
}

export interface MemoryNoteListResponse {
  notes: MemoryNote[];
  total: number;
  page: number;
  page_size: number;
}

export interface MemoryStats {
  total_pages: number;
  active_pages: number;
  archived_pages: number;
  pending_notes: number;
  total_anchors: number;
  by_type: Record<string, number>;
  last_updated_at?: string;
  anchored_kbs?: string[];
}

export interface MemoryCapability {
  available: boolean;
  reason?: string;
}

export interface MemorySpace {
  id: string;
  tenant_id: number;
  scope_type: string;
  owner_principal_type: string;
  owner_principal_id: string;
  display_name: string;
  status: string;
  created_at: string;
}

export interface MemorySpaceView {
  space: MemorySpace;
  stats: MemoryStats;
  capabilities: Record<string, MemoryCapability>;
}

export interface MemorySettingValue {
  value: any;
  // Which layer decided the effective value: deployment, tenant, agent, user,
  // space, or "default".
  source: string;
  // The widest layer that has pinned the value, when one has. The UI shows the
  // control read-only and explains why rather than letting the user change
  // something that would have no effect.
  locked_by?: string;
}

export interface MemorySettingDescriptor {
  key: string;
  group: string;
  kind: string;
  default: any;
  merge: string;
  levels: string[];
  allowed?: string[];
  min?: number;
  max?: number;
  hard_locked?: boolean;
}

export interface MemorySettingsView {
  values: Record<string, MemorySettingValue>;
  descriptors: MemorySettingDescriptor[];
  editable_level: string;
  editable: Record<string, boolean>;
  capabilities: Record<string, MemoryCapability>;
}

export interface MemorySettingsUpdateResponse {
  view: MemorySettingsView;
  // Adjustments the server made, such as a value clamped to its maximum. Shown
  // to the user so a silently different value never comes as a surprise.
  notes?: string[];
}

export interface MemoryGraphNode {
  id: string;
  kind: "memory" | "wiki";
  slug: string;
  title: string;
  type?: string;
  link_count: number;
  strength?: number;
  knowledge_base_id?: string;
}

export interface MemoryGraphEdge {
  source: string;
  target: string;
  kind: "link" | "anchor";
  relation?: string;
}

export interface MemoryGraphData {
  nodes: MemoryGraphNode[];
  edges: MemoryGraphEdge[];
  meta: {
    mode: string;
    total: number;
    returned: number;
    truncated: boolean;
    center?: string;
    depth?: number;
  };
}

export interface MemoryCoverageBucket {
  folder: string;
  total_pages: number;
  lit_pages: number;
  percent: number;
}

export interface MemoryCoverage {
  knowledge_base_id: string;
  total_pages: number;
  lit_pages: number;
  percent: number;
  state_counts: Record<string, number>;
  folders: MemoryCoverageBucket[];
}

export interface MemoryPageRevision {
  id: string;
  page_id: string;
  version: number;
  title: string;
  content: string;
  summary: string;
  edit_source: string;
  edited_at: string;
}

// ---------------------------------------------------------------------------
// Space and settings
// ---------------------------------------------------------------------------

export function getMemorySpace(): Promise<{ data: MemorySpaceView }> {
  return get("/api/v1/memory/space");
}

export function getMemorySettings(): Promise<{ data: MemorySettingsView }> {
  return get("/api/v1/memory/settings");
}

export function updateMemorySettings(
  settings: Record<string, any>,
): Promise<{ data: MemorySettingsUpdateResponse }> {
  return put("/api/v1/memory/settings", { settings });
}

export function getTenantMemorySettings(): Promise<{ data: MemorySettingsView }> {
  return get("/api/v1/memory/tenant-settings");
}

export function updateTenantMemorySettings(
  settings: Record<string, any>,
): Promise<{ data: MemorySettingsUpdateResponse }> {
  return put("/api/v1/memory/tenant-settings", { settings });
}

// ---------------------------------------------------------------------------
// Pages
// ---------------------------------------------------------------------------

export function listMemoryPages(params: {
  type?: string;
  status?: string;
  query?: string;
  page?: number;
  page_size?: number;
  sort_by?: string;
  sort_order?: string;
}): Promise<{ data: MemoryPageListResponse }> {
  return get("/api/v1/memory/pages", query(params));
}

export function getMemoryPage(slug: string) {
  return get(`/api/v1/memory/pages/${encodeSlugPath(slug)}`);
}

export function createMemoryPage(body: Partial<MemoryPage> & { page_type: string }) {
  return post("/api/v1/memory/pages", body);
}

export function updateMemoryPage(
  slug: string,
  body: Record<string, any>,
): Promise<{ data: MemoryPage }> {
  return put(`/api/v1/memory/pages/${encodeSlugPath(slug)}`, body);
}

export function deleteMemoryPage(slug: string) {
  return del(`/api/v1/memory/pages/${encodeSlugPath(slug)}`);
}

export function listMemoryRevisions(slug: string) {
  return get(`/api/v1/memory/revisions/${encodeSlugPath(slug)}`);
}

export function revertMemoryPage(
  slug: string,
  version: number,
  expectedVersion?: number,
): Promise<{ data: MemoryPage }> {
  // expected_version is what this client last saw, so a revert over an edit made
  // in the meantime is refused rather than silently winning.
  return post("/api/v1/memory/revert", {
    slug,
    version,
    expected_version: expectedVersion,
  });
}

// ---------------------------------------------------------------------------
// Notes
// ---------------------------------------------------------------------------

export function listMemoryNotes(params: {
  status?: string;
  type?: string;
  page?: number;
  page_size?: number;
}): Promise<{ data: MemoryNoteListResponse }> {
  return get("/api/v1/memory/notes", query(params));
}

export function promoteMemoryNote(id: string, body: Record<string, any> = {}) {
  return post(`/api/v1/memory/notes/${encodeURIComponent(id)}/promote`, body);
}

export function rejectMemoryNote(id: string) {
  return post(`/api/v1/memory/notes/${encodeURIComponent(id)}/reject`, {});
}

// ---------------------------------------------------------------------------
// Graph, stats, anchors
// ---------------------------------------------------------------------------

export function getMemoryGraph(params: {
  mode?: string;
  center?: string;
  depth?: number;
  types?: string;
  limit?: number;
}) {
  return get("/api/v1/memory/graph", query(params));
}

// ---------------------------------------------------------------------------
// Forget and export
// ---------------------------------------------------------------------------

export function forgetMemories(body: {
  scope: "slugs" | "type" | "all";
  slugs?: string[];
  types?: string[];
  purge_notes?: boolean;
}) {
  return post("/api/v1/memory/forget", body);
}

/** Downloads the export as a blob.
 *
 * Not a URL for the browser to navigate to: authentication is a Bearer header
 * plus X-Tenant-ID, both attached by the axios interceptor, and a plain
 * navigation carries neither — it just produced a 401 in a blank tab. Every
 * other download in this app goes through axios for the same reason.
 */
export async function exportMemories(): Promise<Blob> {
  const response = await getDown("/api/v1/memory/export");
  return response as unknown as Blob;
}

// ---------------------------------------------------------------------------
// Knowledge-base scoped
// ---------------------------------------------------------------------------

export function getMemoryCoverage(kbId: string) {
  return get(`/api/v1/knowledgebase/${encodeURIComponent(kbId)}/memory/coverage`);
}


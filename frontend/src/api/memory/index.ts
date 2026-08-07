import { get, post, put, del } from "../../utils/request";

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

export interface MemoryAnchor {
  id: string;
  space_id: string;
  memory_page_id: string;
  knowledge_base_id: string;
  target_kind: string;
  target_ref: string;
  relation: string;
  strength: number;
  hit_count: number;
  first_seen_at: string;
  last_seen_at: string;
  memory_page_slug?: string;
  memory_page_title?: string;
}

export interface MemoryOverlayNode {
  heat: number;
  state: "unlit" | "touched" | "familiar" | "mastered" | "flagged";
  anchor_count: number;
  memory_count: number;
  relations: string[];
  last_seen_at?: string;
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

export interface MemoryInsight {
  kind: string;
  target_ref: string;
  title?: string;
  content_length?: number;
  interactions: number;
  distinct_people: number;
  detail?: string;
}

export interface MemoryInsightsResponse {
  knowledge_base_id: string;
  k_anonymity: number;
  suppressed: number;
  insights: MemoryInsight[];
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

export function getMemorySpace() {
  return get("/api/v1/memory/space");
}

export function getMemorySettings() {
  return get("/api/v1/memory/settings");
}

export function updateMemorySettings(settings: Record<string, any>) {
  return put("/api/v1/memory/settings", { settings });
}

export function getTenantMemorySettings() {
  return get("/api/v1/memory/tenant-settings");
}

export function updateTenantMemorySettings(settings: Record<string, any>) {
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
}) {
  return get("/api/v1/memory/pages", params);
}

export function getMemoryPage(slug: string) {
  return get(`/api/v1/memory/pages/${encodeSlugPath(slug)}`);
}

export function createMemoryPage(body: Partial<MemoryPage> & { page_type: string }) {
  return post("/api/v1/memory/pages", body);
}

export function updateMemoryPage(slug: string, body: Record<string, any>) {
  return put(`/api/v1/memory/pages/${encodeSlugPath(slug)}`, body);
}

export function deleteMemoryPage(slug: string) {
  return del(`/api/v1/memory/pages/${encodeSlugPath(slug)}`);
}

export function searchMemoryPages(q: string, limit = 20) {
  return get("/api/v1/memory/search", { q, limit });
}

export function listMemoryRevisions(slug: string) {
  return get(`/api/v1/memory/revisions/${encodeSlugPath(slug)}`);
}

export function revertMemoryPage(slug: string, version: number) {
  return post("/api/v1/memory/revert", { slug, version });
}

// ---------------------------------------------------------------------------
// Notes
// ---------------------------------------------------------------------------

export function listMemoryNotes(params: {
  status?: string;
  type?: string;
  page?: number;
  page_size?: number;
}) {
  return get("/api/v1/memory/notes", params);
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
  return get("/api/v1/memory/graph", params);
}

export function getMemoryStats() {
  return get("/api/v1/memory/stats");
}

export function listMemoryAnchors(kbId?: string) {
  return get("/api/v1/memory/anchors", kbId ? { kb_id: kbId } : {});
}

export function addMemoryAnchor(body: {
  knowledge_base_id: string;
  target_ref: string;
  relation: string;
  target_kind?: string;
  memory_page_slug?: string;
}) {
  return post("/api/v1/memory/anchors", body);
}

export function deleteMemoryAnchor(id: string) {
  return del(`/api/v1/memory/anchors/${encodeURIComponent(id)}`);
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

export function exportMemoryUrl(): string {
  return "/api/v1/memory/export";
}

// ---------------------------------------------------------------------------
// Knowledge-base scoped
// ---------------------------------------------------------------------------

export function getMemoryCoverage(kbId: string) {
  return get(`/api/v1/knowledgebase/${encodeURIComponent(kbId)}/memory/coverage`);
}

export function getMemoryInsights(kbId: string) {
  return get(`/api/v1/knowledgebase/${encodeURIComponent(kbId)}/memory/insights`);
}

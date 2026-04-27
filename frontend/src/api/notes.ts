/**
 * 笔记 / 默认笔记本：复用文档型知识库 + manual knowledge API
 */
import i18n from '@/i18n'
import {
  listKnowledgeBases,
  createKnowledgeBase,
  listKnowledgeFiles,
} from '@/api/knowledge-base'
import { listModels, type ModelConfig } from '@/api/model'

export class NoModelConfiguredError extends Error {
  name = 'NoModelConfiguredError'
  constructor(message?: string) {
    super(message || i18n.global.t('notes.errors.noModel'))
  }
}

export type ManualNoteStatus = 'draft' | 'publish'

export interface ParsedNoteMetadata {
  content: string
  status: ManualNoteStatus
  updatedAt?: string
}

export function parseNoteMetadata(metadata: unknown): ParsedNoteMetadata | null {
  if (!metadata) return null
  try {
    let parsed: any = metadata
    if (typeof metadata === 'string') {
      parsed = JSON.parse(metadata)
    }
    if (parsed && typeof parsed === 'object') {
      const status: ManualNoteStatus = parsed.status === 'publish' ? 'publish' : 'draft'
      return {
        content: parsed.content || '',
        status,
        updatedAt: parsed.updated_at || parsed.updatedAt,
      }
    }
  } catch {
    /* ignore */
  }
  return null
}

export function extractExcerpt(content: string, maxLen = 80): string {
  const s = (content || '').replace(/\s+/g, ' ').trim()
  if (s.length <= maxLen) return s
  return `${s.slice(0, maxLen)}…`
}

export function pickDefaultModels(models: ModelConfig[]) {
  const embed =
    models.find((m) => m.type === 'Embedding' && m.is_default) ||
    models.find((m) => m.type === 'Embedding')
  const summary =
    models.find((m) => m.type === 'KnowledgeQA' && m.is_default) ||
    models.find((m) => m.type === 'KnowledgeQA')
  return { embed, summary }
}

/** 查找或创建「我的笔记」文档型知识库 */
export async function ensureDefaultNotebook(): Promise<string> {
  const t = i18n.global.t
  const defaultName = t('notes.defaultNotebookName')

  const res: any = await listKnowledgeBases()
  const allKbs = Array.isArray(res?.data) ? res.data : []
  const docKbs = allKbs.filter((kb: any) => kb.type === 'notebook')

  const byMarker = docKbs.find(
    (kb: any) =>
      String(kb.name || '') === defaultName || kb.type === 'notebook',
  )
  if (byMarker?.id) {
    return String(byMarker.id)
  }

  const models = await listModels()
  const { embed, summary } = pickDefaultModels(models)
  if (!embed?.id || !summary?.id) {
    throw new NoModelConfiguredError()
  }

  const createRes: any = await createKnowledgeBase({
    name: defaultName,
    description: '',
    type: 'notebook',
    embedding_model_id: embed.id,
    summary_model_id: summary.id,
    chunking_config: {
      chunk_size: 512,
      chunk_overlap: 100,
      separators: ['\n\n', '\n', '。', '！', '？', ';', '；'],
      enable_multimodal: false,
      enable_parent_child: true,
      parent_chunk_size: 4096,
      child_chunk_size: 384,
    },
    vlm_config: { enabled: false },
    asr_config: { enabled: false, model_id: '', language: '' },
    storage_provider_config: { provider: 'local' },
    storage_config: { provider: 'local' },
    indexing_strategy: {
      vector_enabled: true,
      keyword_enabled: true,
      wiki_enabled: true,
      graph_enabled: false,
    },
    wiki_config: {
      synthesis_model_id: summary.id,
      extraction_granularity: 'standard',
    },
  })
  if (!createRes?.success || !createRes?.data?.id) {
    throw new Error(createRes?.message || t('notes.errors.createNotebookFailed'))
  }
  const id = String(createRes.data.id)
  return id
}

export interface NoteListItem {
  id: string
  knowledge_base_id: string
  kbName?: string
  title: string
  file_name?: string
  excerpt: string
  status: ManualNoteStatus
  updated_at: string
  raw: Record<string, unknown>
}

function isManualKnowledgeRow(item: any): boolean {
  const ft = String(item.file_type || '').toUpperCase()
  return item.type === 'manual' || ft === 'MANUAL'
}

/** 拉取某知识库下的手工笔记（manual）列表，分页合并 */
export async function listNotesInKb(
  kbId: string,
  opts?: { keyword?: string; maxPages?: number; pageSize?: number },
): Promise<NoteListItem[]> {
  const pageSize = opts?.pageSize ?? 50
  const maxPages = opts?.maxPages ?? 20
  const out: NoteListItem[] = []
  let page = 1

  const kbListRes: any = await listKnowledgeBases()
  const kbs = Array.isArray(kbListRes?.data) ? kbListRes.data : []
  const kb = kbs.find((k: any) => k.id === kbId)
  const kbName = kb?.name as string | undefined

  for (; page <= maxPages; page++) {
    const r: any = await listKnowledgeFiles(kbId, {
      page,
      page_size: pageSize,
      keyword: opts?.keyword,
    })
    if (!r?.success) break
    const rows = Array.isArray(r.data) ? r.data : []
    for (const item of rows) {
      if (!isManualKnowledgeRow(item)) continue
      const meta = parseNoteMetadata(item.metadata)
      const content = meta?.content ?? ''
      const titleRaw = item.title || item.file_name || ''
      const dot = titleRaw.lastIndexOf('.')
      const title = dot > 0 ? titleRaw.slice(0, dot) : titleRaw
      out.push({
        id: String(item.id),
        knowledge_base_id: kbId,
        kbName,
        title: title || i18n.global.t('notes.list.untitled'),
        file_name: item.file_name,
        excerpt: extractExcerpt(content),
        status: meta?.status ?? 'draft',
        updated_at: item.updated_at || item.created_at || '',
        raw: item,
      })
    }
    const total = typeof r.total === 'number' ? r.total : rows.length
    if (page * pageSize >= total || rows.length === 0) break
  }
  return out
}

/** 并发上限拉取多个知识库的笔记后合并（按 updated_at 降序） */
export async function listNotesAcrossKbs(
  kbIds: string[],
  opts?: { keyword?: string; maxPerKbPages?: number },
): Promise<NoteListItem[]> {
  const concurrency = 5
  const chunks: string[][] = []
  for (let i = 0; i < kbIds.length; i += concurrency) {
    chunks.push(kbIds.slice(i, i + concurrency))
  }
  const all: NoteListItem[] = []
  for (const group of chunks) {
    const part = await Promise.all(
      group.map((id) => listNotesInKb(id, { keyword: opts?.keyword, maxPages: opts?.maxPerKbPages ?? 5 })),
    )
    part.forEach((arr) => all.push(...arr))
  }
  all.sort((a, b) => {
    const ta = new Date(a.updated_at).getTime()
    const tb = new Date(b.updated_at).getTime()
    return tb - ta
  })
  return all
}

/** 某知识库下手工（manual）笔记条数，走列表接口 total，轻量 */
export async function countManualNotesInKb(kbId: string): Promise<number> {
  const r: any = await listKnowledgeFiles(kbId, { page: 1, page_size: 1, file_type: 'manual' })
  if (!r?.success) return 0
  return typeof r.total === 'number' ? r.total : 0
}

/** 各文档库 manual 条数，含 __all__=合计 */
export async function loadManualNoteCountsByKb(): Promise<Record<string, number>> {
  const kbs = await listDocumentKbIds()
  const out: Record<string, number> = { __all__: 0 }
  let sum = 0
  const pairs = await Promise.all(
    kbs.map(async (k) => {
      const n = await countManualNotesInKb(k.id)
      return { id: k.id, n }
    }),
  )
  for (const { id, n } of pairs) {
    out[id] = n
    sum += n
  }
  out.__all__ = sum
  return out
}

/** 所有文档型知识库 id（用于「全部」筛选） */
export async function listDocumentKbIds(): Promise<{ id: string; name: string; description?: string }[]> {
  const res: any = await listKnowledgeBases()
  const all = Array.isArray(res?.data) ? res.data : []
  return all
    .filter((kb: any) => kb.type === 'notebook')
    .map((kb: any) => ({ id: String(kb.id), name: String(kb.name || ''), description: String(kb.description || '') }))
}

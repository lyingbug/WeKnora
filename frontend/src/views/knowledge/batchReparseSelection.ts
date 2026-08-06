// Helpers backing the "select all matching documents" affordance used by batch
// rebuild. Batch rebuild can address either the rows the user ticked or every
// document matching the active filter; these predicates decide when the
// cross-page option is offered and meaningful.

export type DocumentFilterParams = {
  tag_ids?: string
  keyword?: string
  file_type?: string
  parse_status?: string
  source?: string
  start_time?: string
  end_time?: string
  folder_path?: string
  folder_recursive?: boolean
}

// A recursive listing rooted at the knowledge base is the unfiltered default, so
// folder_path only counts as a filter when it points at an actual sub-folder.
export function hasActiveDocumentFilter(params: DocumentFilterParams | undefined): boolean {
  if (!params) return false
  return Boolean(
    params.tag_ids ||
    params.keyword ||
    params.file_type ||
    params.parse_status ||
    params.source ||
    params.start_time ||
    params.end_time ||
    params.folder_path,
  )
}

// Offering "select all matches" is only useful when the filter reaches beyond
// the rows already ticked; otherwise the plain selection already covers them.
export function canSelectAllFiltered(input: {
  filterActive: boolean
  allFilteredSelected: boolean
  filteredTotal: number
  selectedCount: number
}): boolean {
  return (
    input.filterActive &&
    !input.allFilteredSelected &&
    input.filteredTotal > input.selectedCount
  )
}

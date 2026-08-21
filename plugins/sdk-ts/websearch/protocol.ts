export type SearchRequest = {
  query: string
  max_results: number
  include_date: boolean
  parameters: {
    api_key?: string
    engine_id?: string
    base_url?: string
    extra_config?: Record<string, string>
  }
}

export type SearchHit = {
  title: string
  url: string
  snippet?: string
  content?: string
  source?: string
}

export type SearchResponse = {
  results: SearchHit[]
  error?: string
}

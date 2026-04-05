---
name: multi-search
description: "Fallback web search using multiple providers (Serper, Tavily, Exa, You.com). Use ONLY when the Firecrawl MCP server is unavailable or insufficient. Prefer Firecrawl MCP for all web search tasks first."
---

# Multi-Search (Fallback)

This is a **fallback** search skill. Always prefer the **Firecrawl MCP server** for web search tasks first. Use this skill only when Firecrawl MCP is unavailable, returns errors, or doesn't support the specific search type you need.

Searches the web through 4 providers. You (the agent) pick the provider based on the decision table below. If a provider fails, try another one.

## Quick Start

```bash
# Search with a specific provider
python3 scripts/search.py -p serper -q "your query"

# List available (configured) providers
python3 scripts/search.py --list-providers
```

API keys are read from environment variables. The script errors with a clear message if the required key is missing.

## Provider Decision Table

Pick the provider based on what the user needs:

| User intent | Provider | Why | Env var |
|---|---|---|---|
| General lookup, prices, local businesses, weather | **serper** | Google results, fast, broadest coverage | `SERPER_API_KEY` |
| Research, explanations, "how does X work" | **tavily** | Deep content extraction, AI-synthesized answers. Also strong for news (`--topic news`) | `TAVILY_API_KEY` |
| "Companies like X", URL similarity, academic/primary sources | **exa** | Neural/semantic search, `--similar-url` + `--category` finds actual companies/papers, not listicles. Best for research papers and original sources | `EXA_API_KEY` |
| Real-time info, RAG context, news + web combined | **you** | Web + separate news array in one call, multiple snippets per result. Best when you need both fresh news and web results together | `YOU_API_KEY` |

**When unsure:** Default to **serper** (fastest, largest free tier). For time-sensitive/news queries, prefer **you** (combined web+news) or **tavily** (`--topic news`).

## Provider-Specific Options

### Serper (Google)
```bash
python3 scripts/search.py -p serper -q "query" \
  --country us --language en \
  --type search \          # search|news|images|videos|places|shopping
  --time-range week \      # hour|day|week|month|year
  --images                 # include image results
```
- Free tier: 2,500 queries/month
- API key: [serper.dev](https://serper.dev)
- Env var: `SERPER_API_KEY`

### Tavily (Research)
```bash
python3 scripts/search.py -p tavily -q "query" \
  --depth advanced \       # basic|advanced
  --topic general \        # general|news
  --include-domains docs.python.org \
  --exclude-domains pinterest.com \
  --raw-content            # include full page content
```
- Free tier: 1,000 queries/month
- API key: [tavily.com](https://tavily.com)
- Env var: `TAVILY_API_KEY`

### Exa (Neural/Semantic)
```bash
python3 scripts/search.py -p exa -q "query" \
  --exa-type neural \      # neural|keyword
  --category company \     # company|research paper|news|pdf|github|tweet|...
  --similar-url "https://stripe.com" \
  --start-date 2025-01-01 --end-date 2025-12-31
```
- Free tier: 1,000 queries/month
- API key: [exa.ai](https://exa.ai)
- Env var: `EXA_API_KEY`

### You.com (RAG/Real-time)
```bash
python3 scripts/search.py -p you -q "query" \
  --freshness day \        # day|week|month|year
  --livecrawl web \        # web|news|all (fetch full page content)
  --include-news
```
- API key: [api.you.com](https://api.you.com)
- Env var: `YOU_API_KEY`

## Common Options

```bash
-q "query"          # search query (required)
-p provider         # provider name (required)
-n 5                # max results (default: 5)
--images            # include images (serper/tavily)
--compact           # minified JSON output
```

## Output Format

All providers return JSON with a common core plus provider-specific fields:

```json
{
  "provider": "serper",
  "query": "search terms",
  "results": [
    {
      "title": "Page Title",
      "url": "https://...",
      "snippet": "Relevant excerpt..."
    }
  ],
  "answer": "Direct answer if available",
  "images": []
}
```

Provider-specific fields on results:
- **serper**: `position` (int from Google), `date`, `sitelinks`
- **tavily**: `score` (real relevance 0-1), `raw_content` (if `--raw-content`)
- **exa**: `score` (real relevance), `highlights` (additional excerpts beyond snippet), `text` (full extracted text), `published_date`, `author`
- **you**: `additional_snippets` (all extra snippets), `description`, `date`, `thumbnail`, `raw_content` (if `--livecrawl`). Separate `news` array at top level.

## Error Handling

The script calls exactly one provider per invocation. If it fails, it exits with a non-zero code and prints a JSON error to stderr. As the agent, you decide what to do: retry the same provider, try a different one, or give up and tell the user.

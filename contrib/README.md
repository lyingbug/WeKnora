# WeKnora Contrib — Community Extensions

This directory contains community-contributed tools and extensions for WeKnora.

## Contents

### 1. MCP Tools (`mcp-tools/`)

Production-ready MCP (Model Context Protocol) server implementations that extend WeKnora's agent capabilities.

#### Template Server (`mcp-tools/template-server/`)

An MCP server providing a `save_template()` tool for chunked file uploads and template management.

**Features:**
- Chunked upload protocol (5MB chunks) for large file handling
- Session-based upload management with automatic cleanup
- Template metadata persistence via REST API
- Structured result output with template_id and oss_id

**Usage:**
```bash
cd contrib/mcp-tools/template-server
npm install
npm run build
npm start
```

**Environment Variables:**
| Variable | Description |
|----------|-------------|
| `WEKNORA_STORAGE_URL` | Storage service URL for file uploads |
| `WEKNORA_AGENT_TOKEN` | Authentication token |
| `WEKNORA_AGENT_ID` | Agent identifier |
| `WEKNORA_SESSION_ID` | Session identifier |
| `WEKNORA_SESSION_MGMT_URL` | Session management API URL |
| `WEKNORA_GATEWAY_URL` | Gateway URL (fallback for session mgmt) |

#### UI Server (`mcp-tools/ui-server/`)

An MCP server providing UI rendering tools (`show_file`, `show_web_app`) for agent-driven content presentation.

**Features:**
- `show_file` — Present files (images, videos, documents) to users
- `show_web_app` — Render running web applications in embedded previews
- Input validation for file paths, URLs, and server health checks
- Static and dynamic app detection

**Usage:**
```bash
cd contrib/mcp-tools/ui-server
npm install
npm run build
npm start
```

### 2. Service Connector (`connector/`)

A Python-based service integration manager for agent environments. Manages credentials, environment variables, and skill/plugin toggling.

**Features:**
- JSON-driven action dispatch (connect, disconnect, refresh, sync)
- Credential variable templating with `${variable}` substitution
- Multi-skill dependency management via `sync_skills`
- Extensible service definitions via `env_map.json`
- Idempotent enable/disable operations

**Supported Actions:**
| Action | .env | Skill Toggle |
|--------|------|-------------|
| connect | write | enable |
| disconnect | clear | disable |
| refresh | write | unchanged |
| sync | write+clear | enable+disable |

**Usage:**
```bash
cd contrib/connector
pip install python-dotenv

# Connect a service
python3 connector.py '{"action": "connect", "services": {"github": {"credentials": {"access_token": "ghp_xxx"}}}}'

# Disconnect
python3 connector.py '{"action": "disconnect", "services": {"github": {}}}'

# Sync (connect listed, disconnect all others)
python3 connector.py '{"action": "sync", "services": {"github": {"credentials": {"access_token": "ghp_xxx"}}}}'
```

**Environment Variables:**
| Variable | Description | Default |
|----------|-------------|---------|
| `CONNECTOR_STAGING_DIR` | Staging directory for connectors | `./connectors` |
| `SKILLS_DIR` | Skills installation directory | `./skills` |
| `ENV_MAP_FILE` | Path to service definitions JSON | `./connectors/env_map.json` |

### 3. Agent Skills Library (`skills/`)

A comprehensive collection of 20 production-ready agent skills covering document processing, financial analysis, web automation, and more.

| Skill | Description | Language |
|-------|-------------|----------|
| `btc-macro-analysis` | BTC macro-economic analysis with yield curves, real rates, and correlation stats | Python |
| `docx` | Word document creation, editing, tracked changes, and OOXML manipulation | Python + JS |
| `frontend-design` | Production-grade frontend UI design and code generation | - |
| `github` | Authenticated GitHub CLI wrapper | Shell |
| `html-report` | Single-file HTML data reports with ECharts, TailwindCSS, GSAP | - |
| `macro-analyst` | Macro-economic and cross-asset analysis for crypto market context | - |
| `market-intel` | On-chain and institutional market intelligence | - |
| `mule-pages` | Static and dynamic page publishing | Python |
| `mulerouter-skills` | Multimodal AI API integration (T2I, T2V, I2V, TTS, T2M) | Python |
| `multi-search` | Fallback web search with multiple providers (Serper, Tavily, Exa) | Python |
| `news-briefing` | Crypto and financial news aggregation and narrative synthesis | - |
| `pdf` | PDF processing: read, merge, split, watermark, OCR, form filling | Python |
| `playwright-cli` | Browser automation for testing, scraping, and data extraction | - |
| `pptx` | PowerPoint creation, editing, and OOXML manipulation | Python + JS |
| `sentiment-analyst` | Crypto market sentiment and derivatives positioning analysis | - |
| `skill-creator` | Framework for creating new agent skills | Python |
| `skill-finder` | Skill discovery and search system | Python |
| `technical-analysis` | Crypto chart analysis with 20+ indicators (RSI, MACD, BOLL, etc.) | Python |
| `template` | Session context persistence and template management | Python |
| `xlsx` | Excel spreadsheet creation, editing, charting, and data cleaning | Python |

Each skill follows a consistent structure:
- `SKILL.md` — Skill definition, triggers, and usage instructions
- `scripts/` — Executable scripts and utilities
- `references/` — Reference documentation and templates

## Integration with WeKnora

These tools complement WeKnora's existing MCP server (`mcp-server/`) and skills system (`skills/`):

- **Template Server** can be used alongside WeKnora's document processing pipeline for template-based knowledge management
- **UI Server** extends WeKnora's agent mode with file and web app rendering capabilities
- **Agent Skills Library** provides reusable skill patterns that can enhance WeKnora's agent system — particularly the document processing skills (docx, pdf, pptx, xlsx) which align with WeKnora's document understanding pipeline, and the financial analysis skills which demonstrate multi-source data integration patterns

## Tech Stack

- **MCP Tools**: TypeScript, Node.js, `@modelcontextprotocol/sdk`, `zod`
- **Connector**: Python 3, `python-dotenv`

## License

MIT — same as WeKnora

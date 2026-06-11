---
title: 网络搜索
description: 为 Agent 工作流添加网络搜索 Provider。
---

# 网络搜索

网络搜索让 Agent 在本地知识库不足时检索外部信息。WeKnora 的网络搜索实现由两部分组成：

- 租户级搜索 Provider：保存搜索引擎类型、参数和凭据。
- Agent 工具 `web_search`：在运行时按 Agent 配置或租户默认 Provider 执行搜索。

网络搜索适合补充近期信息、公开网页资料和知识库之外的背景信息。私有知识问答应优先使用知识库；如果业务不允许访问外部网络，应不要配置 Provider，或在 Agent 中关闭网络搜索。

## 支持的 Provider

当前支持以下网络搜索 Provider：

| Provider | 是否需要 API Key | 额外字段 | 说明 |
| --- | --- | --- | --- |
| DuckDuckGo | 否 | 可选 `proxy_url` | 免费搜索入口，不需要密钥。 |
| Bing | 是 | 可选 `proxy_url` | 使用 Bing Search API。 |
| Google | 是 | `engine_id`，可选 `proxy_url` | 使用 Google Custom Search API。 |
| Tavily | 是 | 可选 `proxy_url` | 使用 Tavily Search API。 |
| Ollama Web Search | 是 | 无 | 使用 Ollama Cloud web search。 |
| Baidu | 是 | 无 | 使用百度 AI Search。 |
| SearXNG | 否 | `base_url`，可选 `proxy_url` | 使用自托管 SearXNG 实例。 |

Provider 类型元数据由 `/api/v1/web-search-providers/types` 返回，前端据此动态渲染表单。

## 配置入口

### 管理界面

进入设置中的网络搜索页，可以查看和管理当前租户的搜索 Provider。

管理员可以：

- 新增 Provider。
- 编辑名称、描述、参数和默认状态。
- 单独更新或清除 API Key。
- 删除 Provider。
- 测试 Provider 是否可用。

Viewer 只能查看 Provider 列表和类型元数据，不能修改或测试。

新增第一个 Provider 时，前端会自动把它设为默认 Provider。之后也可以手动把某个 Provider 设为默认；后端会清除同租户其它默认 Provider，保证默认项唯一。

编辑已有 Provider 时，Provider 类型不可修改。如果要从 Bing 切到 SearXNG，应新建一个 Provider。

### API

新 Provider API 路由如下：

| 方法 | 路径 | 权限 | 用途 |
| --- | --- | --- | --- |
| `GET` | `/api/v1/web-search-providers/types` | Viewer | 获取 Provider 类型元数据。 |
| `GET` | `/api/v1/web-search-providers` | Viewer | 列出当前租户 Provider。 |
| `GET` | `/api/v1/web-search-providers/:id` | Viewer | 查看单个 Provider。 |
| `POST` | `/api/v1/web-search-providers/test` | Admin | 使用表单中的原始参数测试，不保存。 |
| `POST` | `/api/v1/web-search-providers` | Admin | 创建 Provider。 |
| `PUT` | `/api/v1/web-search-providers/:id` | Admin | 更新 Provider。 |
| `DELETE` | `/api/v1/web-search-providers/:id` | Admin | 删除 Provider。 |
| `PUT` | `/api/v1/web-search-providers/:id/credentials` | Admin | 更新 API Key。 |
| `DELETE` | `/api/v1/web-search-providers/:id/credentials/:field` | Admin | 清除 API Key。 |
| `POST` | `/api/v1/web-search-providers/:id/test` | Admin | 测试已保存的 Provider。 |

旧目录接口 `/api/v1/web-search/providers` 仍可返回 Provider 类型列表，但新配置应使用 `/api/v1/web-search-providers/*`。

## 参数和凭据

Provider 参数保存到 `web_search_providers.parameters`。

主要字段包括：

- `api_key`：搜索服务密钥，写入数据库时加密。
- `engine_id`：Google Custom Search Engine ID。
- `base_url`：SearXNG 自托管实例地址。
- `proxy_url`：可选代理地址。
- `extra_config`：预留的扩展配置。

API 响应不会返回 `api_key` 明文，只会返回：

```json
{
  "credentials": {
    "api_key": {
      "configured": true
    }
  }
}
```

创建 Provider 时可以直接带入初始 `api_key`。编辑 Provider 时，`PUT /web-search-providers/:id` 会保留已有密钥，即使请求体误传 `api_key` 也会忽略；更新密钥必须使用 credentials 子资源。

## 连接测试

网络搜索设置页的“测试连接”会执行一次真实搜索：

- 测试 query 固定为 `test`。
- 最多取 1 条结果。
- 如果搜索失败，返回上游错误。
- 如果返回 0 条结果，也视为测试失败。

新增模式下，测试使用当前表单参数。编辑模式下，如果没有输入新的 API Key，会使用已保存的 Provider 和已保存凭据测试。

## 安全校验

网络搜索涉及外部 HTTP 请求，WeKnora 做了以下保护：

- `api_key` 加密存储，并从响应 DTO 中移除。
- SearXNG `base_url` 必须是绝对 `http` 或 `https` URL，不能包含 query 或 fragment。
- SearXNG `base_url` 必须通过 SSRF 校验；私有或回环地址需要加入 `SSRF_WHITELIST`。
- `proxy_url` 也会做 SSRF 校验。
- 搜索 HTTP client 使用 SSRF 安全拨号和重定向校验。
- Provider 只按当前租户 ID 查询，不能跨租户读取。

## Agent 中的使用方式

Agent 启用网络搜索后，运行时会注册 `web_search` 工具。

Provider 选择优先级为：

1. Agent 配置中的 `web_search_provider_id`。
2. 当前租户 `is_default=true` 的 Provider。

最大结果数优先级为：

1. Agent 配置中的 `web_search_max_results`。
2. 租户 WebSearchConfig 中的 `max_results`。
3. 默认值。

Agent 编辑器中可以：

- 开关网络搜索。
- 指定某个 Provider。
- 设置最大搜索结果数。
- 配合 `web_fetch` 获取完整网页内容。

如果没有可用默认 Provider，聊天输入区会禁用网络搜索开关，并引导管理员去设置页配置 Provider。

## 租户级 WebSearchConfig

租户还可以保存 WebSearchConfig，用于控制运行时搜索策略。

主要字段包括：

| 字段 | 默认值 | 说明 |
| --- | --- | --- |
| `max_results` | `10` | 最大搜索结果数，保存时要求 1 到 50。 |
| `include_date` | `false` | 是否请求或保留发布时间。 |
| `compression_method` | `none` | 结果压缩方式。 |
| `blacklist` | `[]` | URL 黑名单规则。 |
| `embedding_model_id` | 空 | RAG 压缩使用的 Embedding 模型。 |
| `document_fragments` | `3` | RAG 压缩取片段数。 |
| `proxy_url` | 空 | 调用时覆盖 Provider 的代理设置。 |

黑名单支持两类规则：

- 通配模式，例如 `*://*.example.com/*`。
- 正则模式，例如 `/example\.(net|org)/`。

匹配黑名单的 URL 会在返回给 Agent 前过滤掉。

## RAG 压缩

当 `compression_method` 不是 `none` 时，`web_search` 工具会尝试对搜索结果做 RAG 压缩。

流程如下：

1. 为当前会话创建或复用一个临时知识库。
2. 把搜索结果以 passage 的形式同步写入临时知识库。
3. 使用用户 query 作为问题，对临时知识库执行混合检索。
4. 按来源 URL 轮询选择片段。
5. 把片段合并回原始搜索结果。

临时知识库标记为 `is_temporary=true`，不会出现在普通知识库列表中。会话级临时知识库状态保存在 WebSearchState 中，避免同一会话重复索引相同 URL。

RAG 压缩需要配置 `embedding_model_id`；缺失时压缩会失败并回退到原始搜索结果。

## 使用建议

- 企业私有知识优先使用知识库检索，网络搜索作为补充。
- 为生产环境设置一个默认 Provider，避免 Agent 未指定 Provider 时无法搜索。
- 对 Google Provider，确保同时配置 API Key 和 Engine ID。
- 对 SearXNG，确保实例启用 JSON 输出，并将私有实例地址加入 SSRF 白名单。
- 对需要代理访问外网的部署，在 Provider 上配置 `proxy_url`；只有确实需要逐次覆盖时才使用租户 WebSearchConfig 的 `proxy_url`。
- 对不希望 Agent 访问公网的应用，禁用 Agent 网络搜索，并不要配置默认 Provider。

## 常见问题

### 为什么 Provider 测试返回 0 条结果也算失败？

测试的目标是确认凭据和服务可用。返回 0 条结果通常意味着配置、密钥、搜索引擎或网络链路存在问题，因此后端会提示检查 API Key 和配置。

### 为什么编辑 Provider 时看不到 API Key？

API Key 不会从主响应返回。界面只显示是否已配置，修改和清除都通过 credentials 子资源完成。

### 为什么 SearXNG 私有地址保存失败？

SearXNG `base_url` 会经过 SSRF 校验。私有网段、回环地址或内部域名需要在服务端配置 `SSRF_WHITELIST`，否则保存和使用都会失败。

### Agent 没指定 Provider 时会用哪个？

运行时会使用当前租户的默认 Provider。若没有默认 Provider，网络搜索无法执行；前端也会在聊天输入区禁用网络搜索开关。

### 网络搜索是否一定会先查知识库？

Agent 工具提示要求先完成知识库检索，再在本地知识不足时使用 `web_search`。这是工具提示层的行为约束；系统是否注册该工具取决于 Agent 和请求中的网络搜索开关。

## 相关文档

- [Agent 模式](../user-guide/agent-mode.md)
- [MCP 工具](../user-guide/mcp-tools.md)
- [知识库](../user-guide/knowledge-bases.md)
- [检索流水线](../architecture/retrieval-pipeline.md)

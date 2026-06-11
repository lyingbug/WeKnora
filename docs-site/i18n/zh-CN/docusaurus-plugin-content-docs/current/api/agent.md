---
title: 智能体 API
description: 管理 Agent 和 Agent 工作流。
---

# 智能体 API

智能体 API 位于 `/api/v1/agents`，用于管理内置智能体、自定义智能体、智能体类型预设、提示词占位符、推荐问题和 IM 渠道入口。真正发起对话的接口在 [对话 API](./chat.md) 中的 `/api/v1/agent-chat/{session_id}`。

## 权限模型

| 操作 | 最低权限 |
| --- | --- |
| 列表、详情、占位符、类型预设、推荐问题 | `viewer` 及以上。 |
| 创建智能体 | `contributor` 及以上。 |
| 更新智能体 | 创建者本人或 `admin` 及以上。内置智能体按租户级资源处理，通常需要 `admin`。 |
| 删除智能体 | 创建者本人或 `admin` 及以上；内置智能体不可删除。 |
| 复制智能体 | `contributor` 及以上，副本归调用者所有。 |
| 创建、更新、删除、启停 IM 渠道 | `admin` 及以上。 |
| 查看某个智能体的 IM 渠道 | `viewer` 及以上。 |

通过 API Key 创建的智能体不会把 `system-<tenantID>` 记录为创建者，因此这类智能体按租户级资源处理，后续通常需要 Admin 权限管理。

## 智能体对象

核心结构：

```json
{
  "id": "agent-id",
  "name": "客服助手",
  "description": "用于产品答疑",
  "avatar": "robot",
  "is_builtin": false,
  "tenant_id": 1,
  "created_by": "user-id",
  "creator_name": "alice",
  "runnable_by_viewer": true,
  "config": {
    "agent_mode": "quick-answer",
    "model_id": "model-id",
    "kb_selection_mode": "selected",
    "knowledge_bases": ["kb-id"]
  },
  "created_at": "2026-06-11T10:00:00Z",
  "updated_at": "2026-06-11T10:00:00Z"
}
```

字段说明：

| 字段 | 说明 |
| --- | --- |
| `id` | 智能体 ID。内置 ID 包括 `builtin-quick-answer` 和 `builtin-smart-reasoning`。 |
| `name` / `description` / `avatar` | 基础展示信息。 |
| `is_builtin` | 是否为内置智能体。 |
| `tenant_id` | 所属租户。 |
| `created_by` / `creator_name` | 创建者 ID 和列表页展示名。 |
| `runnable_by_viewer` | 仅控制智能体模式运行权限。为 `false` 且 RBAC 强制启用时，Viewer 不能运行该智能体的工具链。 |
| `config` | 智能体运行配置。 |

## 配置字段

`config.agent_mode` 决定运行路径：

| 值 | 说明 |
| --- | --- |
| `quick-answer` | 普通 RAG / 快速问答模式。新建智能体未传时默认使用该模式。 |
| `smart-reasoning` | ReAct 智能推理模式，支持工具调用、MCP、Skills、反思和多轮迭代。 |

常用配置分组：

| 字段 | 说明 |
| --- | --- |
| `agent_type` | 智能推理下的类型预设，例如 `rag-qa`、`wiki-qa`、`hybrid-rag-wiki`、`data-analysis`、`custom`。 |
| `system_prompt` / `system_prompt_id` | 系统提示词或模板 ID。 |
| `context_template` / `context_template_id` | 普通模式下的检索上下文模板。 |
| `model_id` | 对话模型 ID。 |
| `rerank_model_id` | 重排模型 ID。 |
| `temperature` | 模型温度。小于 0 时后端默认改为 `0.7`。 |
| `max_completion_tokens` | 普通模式最大输出 token，默认 `2048`。 |
| `thinking` | 是否启用模型思考模式。 |
| `max_iterations` | 智能体最大迭代次数，默认 `10`。 |
| `llm_call_timeout` | 单次 LLM 调用超时秒数，`0` 表示使用全局默认。 |
| `allowed_tools` | 智能体允许调用的内置工具。 |
| `mcp_selection_mode` / `mcp_services` | MCP 服务选择模式：`all`、`selected`、`none`。 |
| `skills_selection_mode` / `selected_skills` | Skills 选择模式：`all`、`selected`、`none`。 |
| `kb_selection_mode` / `knowledge_bases` | 知识库选择模式：`all`、`selected`、`none`。 |
| `retrieve_kb_only_when_mentioned` | 为 `true` 时，只在用户显式 `@` 知识库或文档时检索。 |
| `retain_retrieval_history` | 是否跨轮保留检索历史。 |
| `image_upload_enabled` / `vlm_model_id` | 图片上传和 VLM 分析配置。 |
| `audio_upload_enabled` / `asr_model_id` | 音频上传和 ASR 配置。 |
| `image_storage_provider` | 图片附件存储 provider。为空时使用全局或租户默认。 |
| `supported_file_types` | 限制智能体可用附件扩展名；空数组表示不限制。 |
| `data_analysis_enabled` | 是否启用旧版 DuckDB SQL 数据分析阶段。 |
| `faq_priority_enabled` | 是否启用 FAQ 优先策略。 |
| `web_search_enabled` / `web_search_provider_id` / `web_search_max_results` | 联网搜索配置。 |
| `web_fetch_enabled` / `web_fetch_top_n` | 是否抓取重排后的网页正文以及抓取数量。 |
| `multi_turn_enabled` / `history_turns` | 多轮对话和保留轮数。智能推理模式会强制打开多轮。 |
| `embedding_top_k` / `keyword_threshold` / `vector_threshold` | 检索召回参数。 |
| `rerank_top_k` / `rerank_threshold` | 重排参数。 |
| `enable_query_expansion` / `enable_rewrite` | 查询扩展和问题改写。 |
| `query_understand_model_id` | 查询理解步骤专用模型。为空时使用主模型。 |
| `fallback_strategy` / `fallback_response` / `fallback_prompt` | 无答案时的兜底策略。默认 `fallback_strategy=model`。 |
| `intent_prompts` | 非检索意图的提示词覆盖。 |
| `suggested_prompts` | 智能体配置中的推荐问题。 |

服务层会补齐默认值：`web_search_max_results=5`、`history_turns=5`、`embedding_top_k=10`、`keyword_threshold=0.3`、`vector_threshold=0.5`、`rerank_top_k=5`。

## 创建智能体

```http
POST /api/v1/agents
Authorization: Bearer <access_token>
Content-Type: application/json
```

请求体：

```json
{
  "name": "客服助手",
  "description": "回答产品使用问题",
  "avatar": "robot",
  "config": {
    "agent_mode": "quick-answer",
    "model_id": "chat-model-id",
    "kb_selection_mode": "selected",
    "knowledge_bases": ["kb-id"],
    "web_search_enabled": false,
    "multi_turn_enabled": true
  }
}
```

成功返回 `201`：

```json
{
  "success": true,
  "data": {
    "id": "agent-id",
    "name": "客服助手",
    "is_builtin": false,
    "tenant_id": 1,
    "created_by": "user-id",
    "config": {
      "agent_mode": "quick-answer"
    }
  }
}
```

`name` 必填。后端会自动生成 UUID、写入 `tenant_id`、`created_by`、创建时间和更新时间，并强制 `is_builtin=false`。

## 列出智能体

```http
GET /api/v1/agents
Authorization: Bearer <access_token>
```

响应：

```json
{
  "success": true,
  "data": [
    {
      "id": "builtin-quick-answer",
      "name": "快速问答",
      "is_builtin": true,
      "config": {
        "agent_mode": "quick-answer"
      }
    }
  ],
  "disabled_own_agent_ids": ["agent-id"]
}
```

支持查询参数：

| 参数 | 说明 |
| --- | --- |
| `creator=mine` | 返回当前用户创建的自定义智能体。 |
| `creator=others` | 返回同租户其他成员创建的自定义智能体。 |
| `creator=all` 或省略 | 不按创建者过滤。 |

内置智能体始终保留在列表中，即使使用 `creator=mine` 或 `creator=others`。`disabled_own_agent_ids` 是当前租户对“我的智能体”下拉列表的停用记录，只影响对话下拉展示，不删除智能体。

## 获取详情

```http
GET /api/v1/agents/{id}
Authorization: Bearer <access_token>
```

成功响应：

```json
{
  "success": true,
  "data": {
    "id": "agent-id",
    "name": "客服助手",
    "config": {}
  }
}
```

如果 ID 为空返回 `400`；不存在返回 `404`。

## 更新智能体

```http
PUT /api/v1/agents/{id}
Authorization: Bearer <access_token>
Content-Type: application/json
```

请求体：

```json
{
  "name": "客服助手 v2",
  "description": "新的描述",
  "avatar": "robot",
  "config": {
    "agent_mode": "smart-reasoning",
    "agent_type": "hybrid-rag-wiki",
    "model_id": "chat-model-id",
    "allowed_tools": ["knowledge_search", "web_search"],
    "max_iterations": 10,
    "kb_selection_mode": "all",
    "web_search_enabled": true
  }
}
```

成功响应：

```json
{
  "success": true,
  "data": {
    "id": "agent-id",
    "name": "客服助手 v2",
    "config": {
      "agent_mode": "smart-reasoning"
    }
  }
}
```

自定义智能体更新会替换名称、描述、头像和完整 `config`。因此客户端应提交完整配置，而不是只提交局部字段。

内置智能体不能删除；更新路径会为该租户创建或更新一条内置智能体配置记录。内置智能体的基础信息仍来自内置注册表，实际可自定义的是 `config`。

## 删除智能体

```http
DELETE /api/v1/agents/{id}
Authorization: Bearer <access_token>
```

响应：

```json
{
  "success": true,
  "message": "Agent deleted successfully"
}
```

内置智能体不能删除，会返回 `403`。不存在返回 `404`。

## 复制智能体

```http
POST /api/v1/agents/{id}/copy
Authorization: Bearer <access_token>
```

成功返回 `201`：

```json
{
  "success": true,
  "data": {
    "id": "new-agent-id",
    "name": "客服助手 (副本)",
    "is_builtin": false,
    "config": {}
  }
}
```

复制会读取源智能体配置，创建一个新的自定义智能体。副本名称会追加 ` (副本)`，`is_builtin=false`，归当前调用者所有。复制内置智能体也是创建普通自定义副本。

## 提示词占位符

```http
GET /api/v1/agents/placeholders
Authorization: Bearer <access_token>
```

响应按字段分组：

```json
{
  "success": true,
  "data": {
    "all": [],
    "system_prompt": [],
    "agent_system_prompt": [],
    "context_template": [],
    "rewrite_system_prompt": [],
    "rewrite_prompt": [],
    "fallback_prompt": []
  }
}
```

这些占位符用于编辑器提示哪些变量能放进不同提示词字段。后端直接从 `types` 中的 placeholder registry 返回。

## 类型预设

```http
GET /api/v1/agents/type-presets
Authorization: Bearer <access_token>
```

响应：

```json
{
  "success": true,
  "data": [
    {
      "id": "rag-qa",
      "i18n": {
        "zh-CN": {
          "label": "RAG 问答",
          "description": "..."
        }
      },
      "config": {
        "system_prompt_id": "...",
        "allowed_tools": [],
        "kb_selection_mode": "all"
      },
      "kb_filter": {
        "any_of": ["vector", "keyword"]
      }
    }
  ]
}
```

预设用于智能体编辑器一键填充系统提示词、工具、知识库兼容性和部分策略字段。常见 ID：

| ID | 说明 |
| --- | --- |
| `rag-qa` | 文档/FAQ 分块 RAG。 |
| `wiki-qa` | Wiki 图谱导航问答。 |
| `hybrid-rag-wiki` | Wiki 与分块混合检索。 |
| `data-analysis` | 数据分析场景。 |
| `custom` | 完全自定义，不自动覆盖配置。 |

`kb_filter` 对应知识库 `capabilities`，用于限制当前类型适合选择哪些知识库。

## 推荐问题

```http
GET /api/v1/agents/{id}/suggested-questions?knowledge_base_ids=kb1,kb2&limit=6
Authorization: Bearer <access_token>
```

查询参数：

| 参数 | 说明 |
| --- | --- |
| `knowledge_base_ids` | 逗号分隔的知识库 ID，覆盖智能体默认知识库范围。 |
| `knowledge_ids` | 逗号分隔的知识条目 ID，把推荐问题限定到具体文档。 |
| `limit` | 返回数量，默认 `6`。非法或非正数会回退为 `6`。 |

响应：

```json
{
  "success": true,
  "data": {
    "questions": [
      {
        "question": "如何配置模型供应商？",
        "source": "document",
        "knowledge_base_id": "kb-id"
      }
    ]
  }
}
```

`source` 可能是 `agent_config`、`faq`、`document`、`wiki`。不存在的 Agent 返回 `404`。

## 智能体对话

Agent 管理接口不直接执行对话。运行智能体使用：

```http
POST /api/v1/agent-chat/{session_id}
```

请求体中传：

```json
{
  "query": "帮我分析这些资料",
  "agent_enabled": true,
  "agent_id": "agent-id",
  "knowledge_base_ids": ["kb-id"],
  "mcp_service_ids": ["mcp-service-id"]
}
```

运行规则见 [对话 API](./chat.md)。简要来说：

- 如果 `agent_id` 能解析出智能体，则以该智能体的 `config.agent_mode` 为准。
- `smart-reasoning` 会走工具调用和智能体事件流。
- `quick-answer` 会退回普通问答路径。
- 共享智能体运行时，模型、知识库和 MCP 解析会使用源租户作为有效租户。

## 工具审批

MCP 工具如果配置为需要人工审批，智能体流中会出现：

- `tool_approval_required`
- `tool_approval_resolved`

审批接口：

```http
POST /api/v1/agent/tool-approvals/{pending_id}
Authorization: Bearer <access_token>
Content-Type: application/json
```

请求体：

```json
{
  "decision": "approve",
  "modified_args": {
    "query": "新的参数"
  },
  "reason": "确认执行"
}
```

或驳回：

```json
{
  "decision": "reject",
  "reason": "参数不安全"
}
```

`decision` 只能是 `approve` 或 `reject`。`modified_args` 如果提供，必须是非空 JSON object，不能是 `null` 或数组。该接口由 MCP 服务 handler 提供，但路径属于 Agent 执行面。

## IM 渠道入口

Agent 可以绑定 IM 渠道。接口入口：

| 方法 | 路径 | 权限 | 说明 |
| --- | --- | --- | --- |
| `GET` | `/api/v1/agents/{id}/im-channels` | Viewer+ | 列出某个 Agent 的渠道，包含 credentials，适合编辑页。 |
| `POST` | `/api/v1/agents/{id}/im-channels` | Admin+ | 为 Agent 创建渠道。 |
| `GET` | `/api/v1/im-channels` | Viewer+ | 租户级概览，不返回 credentials。 |
| `PUT` | `/api/v1/im-channels/{id}` | Admin+ | 更新渠道。 |
| `DELETE` | `/api/v1/im-channels/{id}` | Admin+ | 删除渠道。 |
| `POST` | `/api/v1/im-channels/{id}/toggle` | Admin+ | 启停渠道。 |

渠道字段、平台差异和回调机制见 IM 连接器文档。

## 组织共享

Agent 共享接口挂在组织路由中：

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `POST` | `/api/v1/agents/{id}/shares` | 共享 Agent 到组织。 |
| `GET` | `/api/v1/agents/{id}/shares` | 查看 Agent 分享去向。 |
| `DELETE` | `/api/v1/agents/{id}/shares/{share_id}` | 移除分享。 |
| `GET` | `/api/v1/shared-agents` | 查看共享给当前租户的 Agent。 |
| `POST` | `/api/v1/shared-agents/disabled` | Admin+ 设置当前租户是否停用某个共享 Agent。 |
| `GET` | `/api/v1/organizations/{id}/shared-agents` | 查看某个组织空间下的 Agent。 |

共享 Agent 被用于对话时，后端会优先从分享关系中解析 Agent，并使用源租户解析模型、知识库、MCP 和工具能力。

## 常见状态码

| 状态码 | 场景 |
| --- | --- |
| `200` | 列表、详情、更新、删除、占位符、类型预设、推荐问题、审批、IM 渠道操作成功。 |
| `201` | 创建或复制智能体成功。 |
| `400` | ID 为空、请求体格式错误、`name` 缺失、审批参数非法。 |
| `401` | 缺少认证或租户上下文。 |
| `403` | 权限不足、尝试删除内置智能体、Viewer 运行受限智能体。 |
| `404` | 智能体或审批记录不存在。 |
| `500` | 数据库、服务依赖或未分类内部错误。 |

## 相关文档

- [对话 API](./chat.md)
- [MCP 工具](../user-guide/mcp-tools.md)
- [IM 连接器](../integrations/im-connectors.md)
- [知识库 API](./knowledge-base.md)

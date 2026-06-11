---
title: 知识库 API
description: 通过 API 管理知识库。
---

# 知识库 API

知识库 API 位于 `/api/v1/knowledge-bases`，用于管理知识库主资源。这里的“知识库”是文档、FAQ、Wiki、图谱抽取、检索索引和存储绑定的配置中心；知识条目上传、FAQ 条目、标签和组织共享是它的子资源，会在相关页面单独展开。

所有端点都需要认证，可以使用 Bearer Token，也可以使用租户 API Key。不同操作还会经过租户角色和知识库级权限校验。

## 权限模型

知识库 API 同时检查租户角色、创建者和跨租户共享权限。

| 操作 | 最低权限 |
| --- | --- |
| 创建知识库 | 当前租户 `contributor` 及以上。 |
| 列出知识库 | 当前租户 `viewer` 及以上。 |
| 获取详情 | 当前租户 `viewer` 及以上，并且对该知识库有读权限。 |
| 更新知识库 | 知识库创建者或租户 `admin` 及以上，并且对该知识库有写权限。 |
| 删除知识库 | 只能删除当前租户拥有的知识库；共享来的知识库不能通过此接口删除。 |
| 个人置顶 | 当前用户对知识库有读权限即可。 |
| 混合搜索 | 当前用户对知识库有读权限。 |
| 复制知识库 | 当前租户 `contributor` 及以上，且源知识库必须属于当前租户。 |

共享知识库有两种访问路径：

- 组织共享：根据共享记录返回 `viewer` 或 `editor` 权限。共享知识库的向量库 ID 和 owner 租户的存储名称会被隐藏。
- 共享智能体可见：请求带 `agent_id`，或当前租户通过某个共享智能体可访问该知识库时，按只读权限处理。

## 知识库对象

知识库响应由 `KnowledgeBase` 模型扩展而来，常见字段如下：

| 字段 | 说明 |
| --- | --- |
| `id` | 知识库 ID。 |
| `name` | 名称。 |
| `description` | 描述。 |
| `type` | 知识库类型，常见值为 `document`、`faq`。空值创建时会回退为 `document`。 |
| `tenant_id` | 所属租户。 |
| `creator_id` / `creator_name` | 创建者 ID 和列表页展示名。API Key 创建的知识库不会记录合成用户为创建者。 |
| `chunking_config` | 文档分块配置。 |
| `embedding_model_id` | Embedding 模型 ID。 |
| `summary_model_id` | 摘要模型 ID。 |
| `vlm_config` / `asr_config` | 多模态和语音识别配置。 |
| `storage_provider_config` | 知识库级存储 provider 选择。凭据仍来自租户存储配置。 |
| `vector_store_id` | 创建时绑定的租户向量库 ID；为空表示使用环境默认检索引擎。创建后不可修改。 |
| `faq_config` | FAQ 知识库配置。非 FAQ 类型会被清空。 |
| `wiki_config` | Wiki 相关配置。 |
| `indexing_strategy` | 启用哪些索引管线。 |
| `capabilities` | 后端根据配置计算出的能力标记。 |
| `is_pinned` / `pinned_at` | 当前调用用户的个人置顶状态。 |
| `knowledge_count` / `chunk_count` | 知识条目和 chunk 统计。 |
| `share_count` | 共享到组织的数量，列表页会回填。 |
| `vector_store_name` / `vector_store_source` / `vector_store_engine_type` / `vector_store_status` | 响应时追加的向量库展示信息。 |

`capabilities` 的结构：

```json
{
  "vector": true,
  "keyword": true,
  "wiki": false,
  "graph": false,
  "faq": false
}
```

它由 `indexing_strategy`、`type`、`wiki_config` 和 `extract_config` 计算，用于智能体编辑器过滤可用知识库。

## 创建知识库

```http
POST /api/v1/knowledge-bases
Authorization: Bearer <access_token>
Content-Type: application/json
```

示例请求：

```json
{
  "name": "产品文档",
  "description": "面向客服和研发的产品资料",
  "type": "document",
  "embedding_model_id": "embed-model-id",
  "summary_model_id": "summary-model-id",
  "vector_store_id": "vector-store-id",
  "storage_provider_config": {
    "provider": "local"
  },
  "chunking_config": {
    "chunk_size": 1000,
    "chunk_overlap": 200,
    "separators": ["\n\n", "\n", "。"]
  },
  "indexing_strategy": {
    "vector_enabled": true,
    "keyword_enabled": true,
    "wiki_enabled": false,
    "graph_enabled": false
  }
}
```

成功时返回 `201`：

```json
{
  "success": true,
  "data": {
    "id": "kb-id",
    "name": "产品文档",
    "type": "document",
    "tenant_id": 1,
    "vector_store_id": "vector-store-id",
    "vector_store_name": "默认向量库",
    "vector_store_source": "user",
    "vector_store_engine_type": "postgres",
    "vector_store_status": "available",
    "capabilities": {
      "vector": true,
      "keyword": true,
      "wiki": false,
      "graph": false,
      "faq": false
    }
  }
}
```

创建时后端会做这些处理：

- 自动生成 `id`、`tenant_id`、创建时间和更新时间。
- Bearer Token 调用会把真实用户 ID 写入 `creator_id`；API Key 调用不会把 `system-<tenantID>` 写入创建者。
- `type` 为空时默认为 `document`。
- `type` 不是 `faq` 时会清空 `faq_config`。
- FAQ 类型缺少配置时会使用默认 `index_mode=question_answer`、`question_index_mode=combined`。
- `indexing_strategy` 为空时默认启用向量检索和关键词检索。
- `extract_config.enabled=true` 会同步打开 `indexing_strategy.graph_enabled`。
- `storage_provider_config.provider` 为空时使用租户存储默认 provider，仍为空则回退为 `local`。
- `vector_store_id` 为空字符串会被规范化为 `null`，表示使用环境默认检索引擎。
- 非空 `vector_store_id` 必须属于当前租户，并且对应引擎在后端注册表中可用。

如果启用了图谱抽取，`extract_config` 必须同时包含非空 `text`、`tags`、`nodes`、`relations`，并且关系引用的节点必须存在。

## 列出知识库

```http
GET /api/v1/knowledge-bases
Authorization: Bearer <access_token>
```

响应：

```json
{
  "success": true,
  "data": [
    {
      "id": "kb-id",
      "name": "产品文档",
      "creator_id": "user-id",
      "creator_name": "alice",
      "knowledge_count": 12,
      "chunk_count": 240,
      "share_count": 2,
      "is_pinned": true,
      "pinned_at": "2026-06-11T10:00:00Z",
      "vector_store_source": "env",
      "vector_store_status": "available"
    }
  ]
}
```

列表接口不是分页接口，会返回当前租户内的知识库集合。后端会额外回填：

- 每个知识库的组织共享数量 `share_count`。
- 创建者展示名 `creator_name`。
- 当前用户的个人置顶状态，并把已置顶知识库排在前面；同为置顶时按置顶时间倒序；其余按创建时间倒序。
- 向量库展示信息。环境默认源为 `env`，用户绑定源为 `user`，共享来的知识库源为 `shared`，解析失败为 `unavailable`。

支持查询参数：

| 参数 | 说明 |
| --- | --- |
| `creator=mine` | 只返回 `creator_id` 等于当前用户的知识库。 |
| `creator=others` | 只返回同租户内其他成员创建的知识库。 |
| `creator=all` 或省略 | 不按创建者过滤。 |
| `agent_id=<id>` | 按共享智能体可用范围列出知识库。 |

`creator=mine|others` 不会匹配 `creator_id` 为空的历史数据。带 `agent_id` 时，后端会校验当前租户是否可访问该共享智能体，并根据智能体的知识库选择模式返回：

| 智能体 KB 模式 | 返回结果 |
| --- | --- |
| `none` | 空数组。 |
| `selected` | 仅返回智能体配置里选中的知识库。 |
| `all` | 返回智能体源租户内满足智能体能力要求的知识库。 |

## 获取详情

```http
GET /api/v1/knowledge-bases/{id}
Authorization: Bearer <access_token>
```

可选参数：

| 参数 | 说明 |
| --- | --- |
| `agent_id` | 当通过共享智能体访问 owner 租户知识库时，用于校验该智能体是否有权访问该知识库。 |

响应：

```json
{
  "success": true,
  "data": {
    "id": "kb-id",
    "name": "产品文档",
    "knowledge_count": 12,
    "chunk_count": 240,
    "is_processing": false,
    "my_permission": "viewer"
  }
}
```

详情接口会回填 `knowledge_count`、`chunk_count`、`is_processing`。当调用方访问的是非本租户知识库且具备共享权限时，响应会追加 `my_permission`，例如 `viewer` 或 `editor`。

## 更新知识库

```http
PUT /api/v1/knowledge-bases/{id}
Authorization: Bearer <access_token>
Content-Type: application/json
```

请求体：

```json
{
  "name": "产品知识库",
  "description": "更新后的描述",
  "config": {
    "chunking_config": {
      "chunk_size": 800,
      "chunk_overlap": 120,
      "separators": ["\n\n", "\n"]
    },
    "image_processing_config": {},
    "faq_config": {
      "index_mode": "question_answer",
      "question_index_mode": "combined"
    },
    "wiki_config": {
      "synthesis_model_id": "model-id",
      "max_pages_per_ingest": 20,
      "extraction_granularity": "standard"
    },
    "indexing_strategy": {
      "vector_enabled": true,
      "keyword_enabled": true,
      "wiki_enabled": true,
      "graph_enabled": false
    }
  }
}
```

成功响应：

```json
{
  "success": true,
  "data": {
    "id": "kb-id",
    "name": "产品知识库",
    "description": "更新后的描述"
  }
}
```

更新接口只能修改名称、描述和 `config` 中的配置。以下字段不会通过更新接口修改：

- `type`
- `tenant_id`
- `creator_id`
- `embedding_model_id`
- `summary_model_id`
- `vector_store_id`
- `storage_provider_config`

`vector_store_id` 是创建时绑定字段，更新接口没有对应入参。若需要换向量库，应新建知识库并重新导入或复制符合条件的内容。

当提交 `indexing_strategy` 时，至少要启用一种索引管线。启用 `wiki_enabled` 时，如果原知识库没有 `wiki_config`，后端会创建空配置容器。启用 `graph_enabled` 会同步更新旧版 `extract_config.enabled`。

## 删除知识库

```http
DELETE /api/v1/knowledge-bases/{id}
Authorization: Bearer <access_token>
```

响应：

```json
{
  "success": true,
  "message": "Knowledge base deleted successfully"
}
```

删除接口先软删除知识库记录，并删除它的组织共享记录，然后投递低优先级异步任务清理重资源，包括知识条目、文件、chunk、embedding 和图谱数据。即使异步任务投递失败，请求也可能已经成功返回，因为主记录已经被删除。

删除只允许当前租户拥有的知识库。通过组织共享或共享智能体看到的知识库不能被接收方删除。

## 个人置顶

```http
PUT /api/v1/knowledge-bases/{id}/pin
Authorization: Bearer <access_token>
```

响应：

```json
{
  "success": true,
  "data": {
    "id": "kb-id",
    "is_pinned": true,
    "pinned_at": "2026-06-11T10:00:00Z"
  }
}
```

这是一个切换接口：未置顶时调用会置顶，已置顶时调用会取消置顶。置顶状态存储在 `(user, tenant, knowledge_base)` 维度，只影响当前调用用户的列表排序。API Key 调用没有真实用户身份，不能使用个人置顶。

## 混合搜索

```http
GET /api/v1/knowledge-bases/{id}/hybrid-search
Authorization: Bearer <access_token>
Content-Type: application/json
```

请求体是检索参数：

```json
{
  "query_text": "如何配置模型供应商？",
  "top_k": 5
}
```

响应：

```json
{
  "success": true,
  "data": []
}
```

虽然路径使用 `GET`，当前 handler 会从请求体读取搜索参数。共享知识库检索时，后端会使用知识库 owner 租户作为有效检索租户，避免在接收方租户里查不到 embedding 或 chunk。

## 复制知识库

```http
POST /api/v1/knowledge-bases/copy
Authorization: Bearer <access_token>
Content-Type: application/json
```

请求体：

```json
{
  "source_id": "source-kb-id",
  "target_id": "target-kb-id"
}
```

`target_id` 可选。为空时由异步任务创建目标知识库；不为空时会复制到指定目标知识库。

成功响应：

```json
{
  "success": true,
  "data": {
    "task_id": "kb_clone:1:source-kb-id",
    "source_id": "source-kb-id",
    "target_id": "target-kb-id",
    "message": "Knowledge base copy task started"
  }
}
```

复制前会做同步预检：

- 源知识库必须属于当前租户，不能复制跨租户共享来的知识库。
- 如果指定目标知识库，目标也必须属于当前租户。
- 源和目标的 `embedding_model_id` 必须一致。
- 源和目标必须使用同一个向量库绑定；跨向量库存量复制暂不支持。
- 当租户启用了存储引擎配置时，源和目标的有效存储 provider 必须一致。

查询复制进度：

```http
GET /api/v1/knowledge-bases/copy/progress/{task_id}
Authorization: Bearer <access_token>
```

响应：

```json
{
  "success": true,
  "data": {
    "task_id": "kb_clone:1:source-kb-id",
    "source_id": "source-kb-id",
    "target_id": "target-kb-id",
    "status": "pending",
    "progress": 0,
    "message": "Task queued, waiting to start..."
  }
}
```

## 可移动目标

```http
GET /api/v1/knowledge-bases/{id}/move-targets
Authorization: Bearer <access_token>
```

响应：

```json
{
  "success": true,
  "data": []
}
```

该接口用于知识条目移动前选择目标知识库。返回目标必须满足：

- 与源知识库属于同一租户。
- 不是源知识库本身。
- 不是临时知识库。
- `type` 与源知识库一致。
- `embedding_model_id` 与源知识库一致。

实际移动接口位于 `/api/v1/knowledge/move`，属于知识条目 API 范畴。

## 向量库展示信息

知识库响应会追加只读的向量库展示字段：

| 字段 | 说明 |
| --- | --- |
| `vector_store_source=env` | 未绑定 DB 向量库，使用环境默认检索引擎。 |
| `vector_store_source=user` | 绑定到当前租户创建的向量库。 |
| `vector_store_source=shared` | 调用方通过跨租户共享访问该知识库；响应会隐藏 owner 租户的 `vector_store_id`、名称和引擎类型。 |
| `vector_store_source=unavailable` | 绑定目标不可解析，例如向量库被删、引擎未注册或临时不可用。 |

创建时可传 `vector_store_id` 绑定租户向量库；创建后不可通过知识库更新接口修改。更多向量库配置见 [向量存储](../integrations/vector-stores.md)。

## 常见状态码

| 状态码 | 场景 |
| --- | --- |
| `200` | 列表、详情、更新、删除、置顶、搜索、复制任务创建、进度查询成功。 |
| `201` | 创建知识库成功。 |
| `400` | 请求体解析失败、知识库 ID 为空、图谱抽取配置不完整、索引策略全关闭、复制预检不通过。 |
| `401` | 未认证或认证上下文缺少租户/用户。 |
| `403` | 租户角色不足、无知识库读写权限、尝试操作共享来的知识库、复制跨租户源/目标。 |
| `404` | 知识库或复制任务不存在。 |
| `500` | 存储、向量库、异步队列或数据库出现未分类错误。 |

## 相关子资源

- 文档、URL、手工录入和知识条目移动接口在知识条目 API 中说明。
- FAQ 条目接口位于 `/api/v1/knowledge-bases/{id}/faq/...`。
- 标签接口位于 `/api/v1/knowledge-bases/{id}/tags/...`。
- 组织共享接口位于 `/api/v1/knowledge-bases/{id}/shares/...` 和 `/api/v1/shared-knowledge-bases`。
- 通用认证和错误格式见 [API 认证](./authentication.md) 与 [错误处理](./errors.md)。

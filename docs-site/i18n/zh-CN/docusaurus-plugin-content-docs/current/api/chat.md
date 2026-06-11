---
title: 对话 API
description: 创建会话并发送消息。
---

# 对话 API

对话 API 分为三组：

- `/api/v1/sessions`：创建、列表、更新、删除会话，以及停止和续接流式输出。
- `/api/v1/knowledge-chat`：普通知识问答，支持纯聊天、RAG、联网搜索、图片和附件。
- `/api/v1/agent-chat`：智能体问答，支持工具调用、MCP、人审、反思和多阶段事件流。
- `/api/v1/messages`：加载历史消息、删除消息、搜索历史对话和查看聊天历史知识库统计。

所有端点都需要认证，最低租户角色为 `viewer`。会话本身是用户维度资源：Bearer Token 调用只能访问自己的 session；API Key 或历史 session 缺少 `user_id` 时按租户级可见处理。

## 典型流程

1. 调用 `POST /api/v1/sessions` 创建会话。
2. 调用 `POST /api/v1/knowledge-chat/{session_id}` 或 `POST /api/v1/agent-chat/{session_id}` 发起问答。
3. 以 SSE 方式读取 `event: message` 事件。
4. 使用 `GET /api/v1/messages/{session_id}/load` 分页加载历史消息。
5. 如用户中断生成，调用 `POST /api/v1/sessions/{session_id}/stop`。
6. 如果浏览器刷新或连接断开，调用 `GET /api/v1/sessions/continue-stream/{session_id}?message_id=...` 续接未完成输出。

## 会话管理

### 创建会话

```http
POST /api/v1/sessions
Authorization: Bearer <access_token>
Content-Type: application/json
```

请求体：

```json
{
  "title": "产品咨询",
  "description": "客服场景测试"
}
```

`title` 和 `description` 都是可选字段。创建时后端会从认证上下文写入 `tenant_id`；Bearer Token 调用还会写入当前用户为 session owner。

成功时返回 `201`：

```json
{
  "success": true,
  "data": {
    "id": "session-id",
    "tenant_id": 1,
    "title": "产品咨询",
    "description": "客服场景测试",
    "created_at": "2026-06-11T10:00:00Z",
    "updated_at": "2026-06-11T10:00:00Z"
  }
}
```

### 列出会话

```http
GET /api/v1/sessions?page=1&page_size=20
Authorization: Bearer <access_token>
```

查询参数：

| 参数 | 说明 |
| --- | --- |
| `page` | 页码。 |
| `page_size` | 每页数量。 |
| `keyword` | 按标题模糊搜索。 |
| `source` | 按来源过滤，例如 `web`、`feishu`、`wechat`、`slack`。 |
| `agent_id` | 按 Agent 过滤，主要用于 IM 会话。 |

响应：

```json
{
  "success": true,
  "data": [],
  "total": 0,
  "page": 1,
  "page_size": 20
}
```

列表项会包含置顶状态和可用的 IM 来源字段，便于前端直接渲染会话列表。

### 获取、更新和删除会话

```http
GET /api/v1/sessions/{id}
PUT /api/v1/sessions/{id}
DELETE /api/v1/sessions/{id}
```

更新请求体使用 session 对象，常用字段是：

```json
{
  "title": "新的标题",
  "description": "新的描述"
}
```

删除成功：

```json
{
  "success": true,
  "message": "Session deleted successfully"
}
```

### 批量删除

```http
DELETE /api/v1/sessions/batch
Content-Type: application/json
```

删除指定会话：

```json
{
  "ids": ["session-1", "session-2"]
}
```

删除当前可见范围内所有会话：

```json
{
  "delete_all": true
}
```

当 `delete_all` 为 `false` 或缺省时，`ids` 必须非空。

### 清空会话消息

```http
DELETE /api/v1/sessions/{id}/messages
```

该接口会删除会话中的所有消息，同时清除 LLM 上下文和聊天历史知识库条目；会话本身保留。

响应：

```json
{
  "success": true,
  "message": "Session messages cleared successfully"
}
```

### 置顶会话

```http
POST /api/v1/sessions/{session_id}/pin
DELETE /api/v1/sessions/{id}/pin
```

置顶成功：

```json
{
  "success": true,
  "is_pinned": true
}
```

取消置顶成功：

```json
{
  "success": true,
  "is_pinned": false
}
```

置顶是用户维度状态。若目标 session 不存在或对当前用户不可见，会返回 `404`。

## 发起问答

### 普通知识问答

```http
POST /api/v1/knowledge-chat/{session_id}
Authorization: Bearer <access_token>
Content-Type: application/json
Accept: text/event-stream
```

请求体：

```json
{
  "query": "如何配置模型供应商？",
  "knowledge_base_ids": ["kb-id"],
  "knowledge_ids": ["knowledge-id"],
  "agent_id": "agent-id",
  "web_search_enabled": false,
  "summary_model_id": "model-id",
  "mentioned_items": [
    {
      "id": "kb-id",
      "name": "产品文档",
      "type": "kb",
      "kb_type": "document"
    }
  ],
  "enable_memory": true,
  "disable_title": false,
  "channel": "web"
}
```

必填字段只有 `query`。其余字段含义：

| 字段 | 说明 |
| --- | --- |
| `knowledge_base_ids` | 本次请求选中的知识库 ID 列表。 |
| `knowledge_ids` | 本次请求限定的知识条目 ID 列表。 |
| `agent_id` | 可选 Agent ID；后端会解析自有或共享 Agent，并据此补充默认知识库、模型和能力。 |
| `web_search_enabled` | 是否启用联网搜索。 |
| `summary_model_id` | 普通模式下本次请求使用的总结模型。 |
| `mentioned_items` | 前端 `@` 提及的知识库或文件；后端会合并到 `knowledge_base_ids` 和 `knowledge_ids`。 |
| `enable_memory` | 记忆开关。省略时使用当前用户偏好 `preferences.enable_memory`，没有偏好则为 `false`。 |
| `disable_title` | 是否禁用自动生成会话标题。 |
| `images` | 图片附件，数组项为 `{ "data": "data:image/png;base64,..." }`。 |
| `attachment_uploads` | 文件附件，数组项为 `{ "data": "...", "file_name": "...", "file_size": 123 }`。 |
| `channel` | 来源渠道，例如 `web`、`api`、`im`。 |

普通模式既可以走 RAG，也可以在没有知识库和联网搜索时走纯聊天。图片上传要求所选 Agent 开启图片上传；如果客户端传入 `images[].url` 或 `images[].caption`，后端会清空这些字段，防止借由 LLM provider 触发 SSRF。

附件会在进入问答前解码、保存并抽取内容。文件大小上限由 `MAX_FILE_SIZE_MB` 控制，默认 50 MB。音频附件只有在 Agent 开启音频上传且配置 ASR 模型时才会走 ASR。

### 智能体问答

```http
POST /api/v1/agent-chat/{session_id}
Authorization: Bearer <access_token>
Content-Type: application/json
Accept: text/event-stream
```

请求体和普通问答共用同一个结构，额外常用字段：

```json
{
  "query": "分析这些文档并给出修订建议",
  "agent_enabled": true,
  "agent_id": "agent-id",
  "knowledge_base_ids": ["kb-id"],
  "mcp_service_ids": ["mcp-service-id"],
  "web_search_enabled": true,
  "channel": "web"
}
```

智能体模式的启用规则：

- 如果 `agent_id` 能解析出 Custom Agent，则以该 Agent 的 `agent_mode` 配置为准。
- 如果没有解析出 Agent，则使用请求体里的 `agent_enabled`。
- 当最终启用智能体模式时，`agent_id` 必须可解析，否则返回 `400`。
- 如果 Agent 设置了 `runnable_by_viewer=false`，且租户 RBAC 已强制启用，则 `viewer` 不能运行它，必须是 `contributor` 及以上。

如果最终没有启用智能体模式，`/agent-chat/{session_id}` 会退回普通问答执行路径。

## SSE 响应

问答接口返回 `text/event-stream`。每条事件使用同一个 SSE 事件名：

```text
event: message
data: {"id":"request-id","response_type":"answer","content":"你好","done":false,"data":{"event_id":"..."}}
```

统一 JSON 字段：

| 字段 | 说明 |
| --- | --- |
| `id` | 请求 ID，来自 `X-Request-ID`。 |
| `response_type` | 事件类型。 |
| `content` | 本事件文本内容，可能为空。 |
| `done` | 当前事件段是否完成。 |
| `data` | 类型相关元数据。 |
| `session_id` | 仅部分事件携带。 |
| `assistant_message_id` | 仅部分事件携带。 |
| `knowledge_references` | 引用事件会额外携带检索引用。 |

常见 `response_type`：

| 类型 | 说明 |
| --- | --- |
| `agent_query` | 智能体请求开始，通常携带 session 和 assistant message ID。 |
| `thinking` | 思考过程或普通模式的 reasoning 内容。 |
| `tool_call` | 工具调用开始，`data` 中有 `tool_name`、`arguments`、`tool_call_id`。 |
| `tool_result` | 工具调用结果，`data` 中有 `success`、`output`、`error`、`duration_ms` 等。 |
| `tool_approval_required` | MCP 工具需要人工批准。 |
| `tool_approval_resolved` | MCP 工具人工批准结果。 |
| `references` | 检索引用，`knowledge_references` 中包含 chunk、知识条目和知识库信息。 |
| `answer` | 回答文本增量。前端应按 `data.event_id` 聚合。 |
| `reflection` | 智能体反思事件。 |
| `session_title` | 自动生成的会话标题。 |
| `error` | 流内错误，`data.stage` 表示阶段。 |
| `complete` | 流完成信号。客户端应以此作为结束标记。 |
| `stop` | 用户主动停止生成。 |

`complete` 已经是终止事件；后端不会再额外发送一个空的 `answer done=true`。如果第一次模型输出没有通过 answer 事件流出，后端会在完成阶段补发 fallback answer，并在 `data.is_fallback=true` 标记。

## 停止生成

```http
POST /api/v1/sessions/{session_id}/stop
Authorization: Bearer <access_token>
Content-Type: application/json
```

请求体必须传 assistant message ID：

```json
{
  "message_id": "assistant-message-id"
}
```

成功响应：

```json
{
  "success": true,
  "message": "Generation stopped"
}
```

如果目标消息已经完成，响应为：

```json
{
  "success": true,
  "message": "Message already completed"
}
```

停止逻辑会先验证消息属于该 session，再验证 session 属于当前租户和当前用户可见范围。然后后端向 StreamManager 写入 `stop` 事件，SSE 轮询端看到后会通过事件总线取消后台生成。已经流出的内容会保留，停止中的会话不会用空答案覆盖已有内容。

## 续接流

当客户端刷新、断线或切换页面后，如果最后一条 assistant 消息尚未完成，可以续接流：

```http
GET /api/v1/sessions/continue-stream/{session_id}?message_id={assistant_message_id}
Authorization: Bearer <access_token>
Accept: text/event-stream
```

后端会：

1. 校验 session 和 message 对当前调用者可见。
2. 从 StreamManager 的 offset 0 读取已有事件并 replay。
3. 如果已有事件里包含 `complete`，直接结束。
4. 否则每 100 ms 轮询新事件并继续通过 SSE 推送。

如果没有找到未完成消息或没有流事件，会返回 `404`。

## 自动标题

问答请求默认会为无标题会话生成标题。标题事件以 SSE 形式返回：

```json
{
  "response_type": "session_title",
  "content": "模型供应商配置",
  "done": true,
  "data": {
    "session_id": "session-id",
    "title": "模型供应商配置"
  }
}
```

如果请求体设置 `disable_title=true`，普通问答不会生成标题。智能体模式始终按当前执行路径生成标题。流完成后，如果需要标题但标题事件尚未到达，后端最多等待 3 秒再关闭 SSE。

## 请求状态恢复

每次问答开始时，后端会异步写入 session 的 `last_request_state`，用于前端重新打开会话时恢复输入栏状态。该字段是 UI 记忆，不影响后端本次执行。

结构：

```json
{
  "agent_id": "agent-id",
  "agent_enabled": true,
  "model_id": "summary-model-id",
  "knowledge_base_ids": ["kb-id"],
  "knowledge_ids": ["knowledge-id"],
  "web_search_enabled": true
}
```

通过 `GET /api/v1/sessions/{id}` 可随 session 一起读取。

## 消息历史

### 加载消息

```http
GET /api/v1/messages/{session_id}/load?limit=20
Authorization: Bearer <access_token>
```

查询参数：

| 参数 | 说明 |
| --- | --- |
| `limit` | 返回数量，默认 20。非法值会回退到 20。 |
| `before_time` | 只加载该时间之前的消息，格式为 RFC3339 或 RFC3339Nano。 |

响应：

```json
{
  "success": true,
  "data": [
    {
      "id": "message-id",
      "session_id": "session-id",
      "role": "assistant",
      "content": "...",
      "is_completed": true,
      "knowledge_references": [],
      "agent_steps": []
    }
  ]
}
```

### 删除单条消息

```http
DELETE /api/v1/messages/{session_id}/{id}
```

响应：

```json
{
  "success": true,
  "message": "Message deleted successfully"
}
```

### 搜索历史消息

```http
POST /api/v1/messages/search
Content-Type: application/json
```

请求体：

```json
{
  "query": "模型供应商",
  "mode": "hybrid",
  "limit": 20,
  "session_ids": ["session-id"]
}
```

字段说明：

| 字段 | 说明 |
| --- | --- |
| `query` | 必填，搜索文本。 |
| `mode` | `keyword`、`vector` 或 `hybrid`，默认由服务层处理。 |
| `limit` | 最大返回数量。 |
| `session_ids` | 可选，只搜索指定会话。 |

响应：

```json
{
  "success": true,
  "data": {
    "total": 0,
    "results": []
  }
}
```

### 聊天历史知识库统计

```http
GET /api/v1/messages/chat-history-stats
```

响应：

```json
{
  "success": true,
  "data": {}
}
```

该接口读取当前租户聊天历史知识库的统计信息，例如已索引消息数和知识库大小。是否启用聊天历史知识库由租户 KV 配置控制。

## 客户端建议

- 发起 SSE 请求时带上 `X-Request-ID`，前后端都会用它记录 TTFB 和排查日志。
- SSE 客户端应处理 `answer` 的增量内容、`references` 的引用数据、`tool_call/tool_result` 的工具过程，以及 `complete/stop/error` 的终止状态。
- 浏览器端应设置 `X-Tenant-ID`，尤其是用户切换到非 home tenant 后，否则流式接口可能落回 home tenant。
- 如果用户点击停止，先调用 `/stop`，不要只关闭浏览器端 SSE；关闭连接只会让前端停止接收，不一定取消后端生成。
- 断线恢复优先用 `continue-stream`，不要重复提交同一条 `query`。
- 图片请求不要传客户端自造的 `url` 或 `caption`，后端会清空这些字段；只传 `data`。

## 常见状态码

| 状态码 | 场景 |
| --- | --- |
| `200` | 列表、详情、更新、删除、清空、置顶、问答 SSE、停止、续流、消息加载成功。 |
| `201` | 会话创建成功。 |
| `400` | session ID 为空、query 为空、请求体格式错误、停止缺少 `message_id`、附件超限、智能体模式缺少可解析 `agent_id`。 |
| `401` | 缺少认证或认证上下文缺少租户。 |
| `403` | 消息不属于 session、session 不属于当前租户、Viewer 运行受限 Agent。 |
| `404` | session、message、未完成流或 stream events 不存在，或当前用户不可见。 |
| `500` | 数据库、StreamManager、模型调用或其他未分类服务错误。 |

## 相关文档

- [API 认证](./authentication.md)
- [智能体 API](./agent.md)
- [知识库 API](./knowledge-base.md)
- [错误处理](./errors.md)

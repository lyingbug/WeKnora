---
title: 可观测性
description: 追踪 Agent 执行、文档入库和模型调用。
---

# 可观测性

WeKnora 的可观测性由几层能力组成：启动环境摘要、结构化请求日志、知识解析 span、异步任务失败记录、审计日志，以及可选的 Langfuse 模型调用追踪。当前代码中没有独立的 Prometheus 指标端点，生产环境通常通过容器日志、数据库查询、Langfuse 和平台侧日志系统组合排查。

## 观测对象

| 场景 | 主要观测方式 |
| --- | --- |
| 服务启动失败 | 启动环境摘要、容器日志。 |
| API 请求失败 | `X-Request-ID`、请求日志、统一错误响应。 |
| 文档解析卡住 | 知识解析 span、任务失败记录、DocReader 日志。 |
| Embedding 或向量写入失败 | 知识解析 span、模型日志、向量库日志。 |
| 聊天和 Agent 延迟 | Langfuse trace、本地 LLM 调试日志、请求日志。 |
| MCP 工具审批或调用失败 | Agent 流式事件、请求日志、Langfuse trace。 |
| 权限和组织操作 | 租户审计日志、系统审计日志。 |
| IM 消息处理 | IM 服务日志、会话消息、流式错误事件。 |

## 请求 ID

每个请求都会经过 `RequestID` 中间件：

- 如果请求头里已有 `X-Request-ID`，后端会复用它。
- 如果没有，后端会生成一个 UUID。
- 响应头会返回同一个 `X-Request-ID`。
- 请求上下文中的 logger 会带上 `request_id` 字段。

排查问题时，应让调用方记录响应头中的 `X-Request-ID`。后端日志、LLM 调试文件和 Langfuse trace metadata 都可能带有这个 ID。

示例：

```bash
curl -H "X-Request-ID: debug-20260611-001" \
  -H "Authorization: Bearer <token>" \
  http://localhost:8080/api/v1/knowledge-bases
```

## 结构化请求日志

请求日志中间件会记录：

| 字段 | 说明 |
| --- | --- |
| `request_id` | 请求关联 ID。 |
| `method` | HTTP 方法。 |
| `path` | 请求路径和 query。 |
| `status_code` | 响应状态码。 |
| `size` | 响应大小。 |
| `latency` | 请求耗时。 |
| `client_ip` | 客户端 IP。 |
| `request_body` | POST、PUT、PATCH 请求体，经过日志清洗。 |
| `response_body` | JSON 或文本响应体，过长会截断。 |

SSE 流式响应不会完整写入日志，会显示为“已跳过”的占位文本，避免把长流式内容打爆日志。

日志相关环境变量：

| 变量 | 说明 |
| --- | --- |
| `LOG_LEVEL` | 日志级别，可选 `debug`、`info`、`warn`、`error`、`fatal`。未配置或无效时默认 `debug`。 |
| `LOG_PATH` | 日志文件路径。容器部署通常依赖标准输出；桌面版可写入本地日志文件。 |
| `LOG_FORMAT` | 自定义日志格式模板。支持 `%d`、`%level`、`%thread`、`%logger`、`%traceId`、`%msg`。 |

桌面版 macOS `.app` 未设置 `LOG_PATH` 时，默认日志目录类似：

```text
~/Library/Logs/WeKnora Lite/WeKnora Lite.log
```

## 启动环境摘要

服务启动时会打印一组关键环境变量，格式类似：

```text
[startup-env] resolved environment:
[startup-env]   DB_DRIVER=postgres
[startup-env]   DB_HOST=postgres
[startup-env]   JWT_SECRET=set (32 chars)
[startup-env]   SYSTEM_AES_KEY=set (32 chars)
```

敏感变量只打印是否设置和长度，不打印原文。当前启动摘要覆盖：

- `SYSTEM_AES_KEY`、`JWT_SECRET`
- `GIN_MODE`、`AUTO_MIGRATE`
- `DB_DRIVER`、`DB_HOST`、`DB_PORT`、`DB_USER`、`DB_NAME`、`DB_PATH`、`DB_PASSWORD`
- `REDIS_ADDR`、`REDIS_PASSWORD`
- `STORAGE_TYPE`、`MINIO_ENDPOINT`、`MINIO_BUCKET_NAME`、`MINIO_SECRET_ACCESS_KEY`
- `TOS_ENDPOINT`、`TOS_BUCKET_NAME`、`TOS_SECRET_KEY`
- `DOCREADER_ADDR`、`RETRIEVE_DRIVER`

如果 `SYSTEM_AES_KEY` 设置了但不是 32 字节，启动日志会明确警告加密被禁用。这类日志适合放进部署健康检查和首屏排障流程。

## 本地 LLM 调试日志

`LLM_DEBUG_LOG` 可以把模型调用的完整请求和响应写到文件，适合排查 Prompt、上下文、工具调用和模型返回格式。

| 配置 | 行为 |
| --- | --- |
| 空、`false`、`0` | 关闭。 |
| `true` 或 `1` | 写到默认调试目录。若设置了 `LOG_PATH`，目录为同级 `llm_debug/`。 |
| 文件夹路径 | 写到指定目录。 |

每个请求会按 `request_id` 写入一个日志文件；没有 request ID 时使用时间戳文件名。记录内容包括：

- 调用类型，例如 Chat、Chat Stream、Embedding、Rerank、VLM。
- 模型名称。
- 调用耗时。
- 分段内容，例如消息、工具调用、响应、错误。

旧文件会自动清理，默认保留 7 天。

注意：该日志可能包含用户问题、检索上下文、模型输入输出、工具参数和图片信息。生产环境只建议短期开启，并确保目录权限和日志采集规则符合数据安全要求。

## Langfuse

Langfuse 集成是可选能力。未启用时，相关代码路径是低成本 no-op；启用后会记录模型调用、检索阶段、异步任务和 Agent 工作流中的 trace/span/generation。

启用条件：

```bash
LANGFUSE_PUBLIC_KEY=pk-lf-xxxxxxxx
LANGFUSE_SECRET_KEY=sk-lf-xxxxxxxx
LANGFUSE_HOST=https://cloud.langfuse.com
```

只要同时设置了 Public Key 和 Secret Key，就会自动启用。也可以用 `LANGFUSE_ENABLED=false` 显式关闭。

### 配置项

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `LANGFUSE_ENABLED` | 根据 key 自动判断 | 主开关。 |
| `LANGFUSE_HOST` | `https://cloud.langfuse.com` | Langfuse 地址。 |
| `LANGFUSE_PUBLIC_KEY` | 空 | Public Key。 |
| `LANGFUSE_SECRET_KEY` | 空 | Secret Key。 |
| `LANGFUSE_RELEASE` | 空 | 版本标签。 |
| `LANGFUSE_ENVIRONMENT` | 空 | 环境标签。 |
| `LANGFUSE_FLUSH_AT` | `15` | 缓冲区达到多少条后批量上报。 |
| `LANGFUSE_FLUSH_INTERVAL` | `3s` | 自动 flush 间隔。 |
| `LANGFUSE_QUEUE_SIZE` | `2048` | 本地队列大小，避免远端不可用时无限占内存。 |
| `LANGFUSE_REQUEST_TIMEOUT` | `10s` | 单次上报超时。 |
| `LANGFUSE_SAMPLE_RATE` | `1.0` | 采样率，范围 0 到 1。 |
| `LANGFUSE_DEBUG` | `false` | 输出更详细的上报错误日志。 |

### 被追踪的请求

Gin 中间件不会追踪所有 API，只追踪可能触发模型调用或异步模型任务的路径，包括：

- `/api/v1/knowledge-chat`
- `/api/v1/agent-chat`
- `/api/v1/knowledge-search`
- 会话标题生成接口
- 初始化阶段的模型检查、Embedding 测试、Rerank 检查、ASR 检查、多模态测试和抽取接口
- Evaluation 接口
- 知识上传、URL 导入、手动知识更新、重解析、迁移、复制、FAQ 导入
- Wiki 自动修复和链接重建
- Chunk 更新
- 数据源手动同步

认证、列表、配置读取、静态资源和健康检查不会进入 Langfuse，减少噪声。

### 异步任务追踪

异步任务入队前会把当前 Langfuse trace 信息写入任务 payload。Asynq worker 侧中间件会：

1. 从 payload 读取 trace/span 信息。
2. 恢复 HTTP 请求发起的 trace，或为定时任务创建独立 trace。
3. 为任务处理创建 span。
4. 把 task type、task id、queue、retry、max retry、payload bytes 写入 metadata。

这样上传文档的 HTTP 请求、后续解析、Embedding、VLM 和后处理可以在 Langfuse 中连成一棵树。Lite 模式没有 Asynq，但同步任务仍会使用请求上下文中已有的观测信息。

### 自建 Langfuse

Docker Compose 提供了 `langfuse` profile：

```bash
docker compose --profile langfuse up -d
```

自建栈会复用 WeKnora 的 PostgreSQL 和 Redis：

- 在已有 PostgreSQL 中创建独立的 `langfuse` 数据库。
- 使用 Redis 的独立 DB 号。
- 额外启动 Langfuse Web、Langfuse Worker、ClickHouse 和 Langfuse 专用 MinIO。

首次启动后，等待 ClickHouse 迁移和 Langfuse Web 健康，再进入 Langfuse UI 创建 API Key。容器化 WeKnora App 访问自建 Langfuse 时，`LANGFUSE_HOST` 通常设置为：

```bash
LANGFUSE_HOST=http://langfuse-web:3000
```

生产自建时必须修改：

- `LANGFUSE_SALT`
- `LANGFUSE_ENCRYPTION_KEY`
- `LANGFUSE_NEXTAUTH_SECRET`
- `LANGFUSE_MINIO_USER`
- `LANGFUSE_MINIO_PASSWORD`
- `LANGFUSE_CLICKHOUSE_USER`
- `LANGFUSE_CLICKHOUSE_PASSWORD`

## 知识解析 Span

知识解析流水线会把阶段状态写入 `knowledge_processing_spans`，并通过 API 返回树形结构：

```http
GET /api/v1/knowledge/{id}/spans
GET /api/v1/knowledge/{id}/spans?attempt=2
```

响应中的主要字段：

| 字段 | 说明 |
| --- | --- |
| `knowledge_id` | 知识 ID。 |
| `parse_status` | 当前解析状态。 |
| `current_attempt` | 当前或指定的解析尝试次数。 |
| `current_stage` | 正在运行的阶段。 |
| `trace` | root → stage → subspan 的树。 |
| `last_error` | 最近失败 span 的摘要。 |

固定的五个阶段：

| 阶段 | 说明 |
| --- | --- |
| `docreader` | 文档读取和解析。 |
| `chunking` | 文本切分。 |
| `embedding` | 向量化和向量库写入。 |
| `multimodal` | 图片、OCR、多模态处理。 |
| `postprocess` | 后处理，例如问题生成、Wiki 处理等。 |

Span 状态：

| 状态 | 含义 |
| --- | --- |
| `pending` | 尚未运行。 |
| `running` | 正在运行。 |
| `done` | 成功完成。 |
| `failed` | 当前 span 自身失败。 |
| `skipped` | 有意跳过，例如纯文本文件没有多模态阶段。 |
| `cancelled` | 上游失败导致没有运行。 |

即使数据库中还没有 span 行，接口也会合成五个阶段占位，保证前端时间线始终可展示。

失败时的 `last_error` 示例：

```json
{
  "last_error": {
    "stage": "embedding",
    "code": "EMBEDDING_RATE_LIMIT",
    "message": "embedding provider rate limited",
    "finished_at": "2026-06-11T10:00:00Z"
  }
}
```

`error_detail` 默认不会通过 API 返回，避免把底层堆栈或敏感信息暴露给普通调用方。

## 异步任务失败

标准版使用 Asynq。任务中间件包含 dead-letter 处理：当文档相关任务耗尽重试预算时，会尽快把对应知识的 `parse_status` 标记为 `failed`，并关闭或取消相关 span，避免文档长期停留在 `processing`。

Lite 模式没有 Redis 和 Asynq Inspector，任务在本进程内执行。日志中会出现类似：

```text
[SyncTask] Executing task type=...
[SyncTask] Retrying task type=...
[SyncTask] Task failed (exhausted retries) ...
```

排查 Lite 卡住问题时，应同时看应用进程日志和知识 span API。

## 审计日志

租户级审计日志接口：

```http
GET /api/v1/tenants/{id}/audit-log
```

系统级审计日志接口：

```http
GET /api/v1/system/admin/audit-log
```

支持 query：

| 参数 | 说明 |
| --- | --- |
| `after_id` | 游标，返回 ID 小于该值的记录。 |
| `limit` | 页大小，默认由仓储层控制，通常 50。 |
| `action` | 按 action 精确过滤。 |
| `outcome` | 按 outcome 精确过滤，例如 `success`、`denied`。 |
| `actor` | 按操作者用户 ID 过滤。 |

响应结构：

```json
{
  "success": true,
  "data": [
    {
      "id": 100,
      "tenant_id": 1,
      "actor_user_id": "user-id",
      "actor_role": "admin",
      "action": "rbac.member_added",
      "target_type": "member",
      "target_id": "target-id",
      "request_path": "/api/v1/tenants/1/members",
      "request_method": "POST",
      "outcome": "success",
      "details": {},
      "created_at": "2026-06-11T10:00:00Z"
    }
  ],
  "next_cursor": 99
}
```

租户审计日志需要租户 Admin 权限。系统审计日志挂在系统管理员路由下，用于查看 `tenant_id=0` 的平台级事件，例如系统设置变更、系统管理员提升或撤销。

审计日志保留时间由 `audit.retention_days` 或 `WEKNORA_AUDIT_RETENTION_DAYS` 控制。省略配置时默认保留 90 天，设置为 `0` 表示不自动清理。

## 流式错误

聊天、Agent 和 IM 流式链路可能通过事件返回错误，而不是只依赖最终 HTTP 状态。错误事件通常包含：

```json
{
  "error": "model call failed",
  "error_code": "MODEL_PROVIDER_FAIL",
  "stage": "generation",
  "session_id": "session-id"
}
```

排查流式问题时需要同时检查：

- HTTP 请求日志中的 `request_id`。
- SSE 事件中的 `error`、`error_code`、`stage`。
- 会话消息是否已经落库。
- Langfuse 中是否有对应 trace。
- 模型供应商或 MCP 服务日志。

## 生产接入建议

### 容器日志

Docker Compose：

```bash
docker compose logs -f app
docker compose logs -f docreader
docker compose logs -f redis
docker compose logs -f postgres
```

Kubernetes：

```bash
kubectl logs -n weknora deploy/weknora-app -f
kubectl logs -n weknora deploy/weknora-docreader -f
```

如果 release 名称不同，Deployment 名称会随 Helm fullname 变化，可先运行：

```bash
kubectl get deploy -n weknora
```

### Langfuse 采样

高流量生产环境建议：

```bash
LANGFUSE_SAMPLE_RATE=0.1
LANGFUSE_FLUSH_AT=50
LANGFUSE_QUEUE_SIZE=10000
LANGFUSE_FLUSH_INTERVAL=5s
```

如果远端 Langfuse 不稳定，队列满后可能丢弃观测数据，但不应阻断业务请求。

### 日志安全

不要长期打开：

```bash
LLM_DEBUG_LOG=true
LANGFUSE_DEBUG=true
LOG_LEVEL=debug
```

这些配置可能增加日志量，并包含用户输入、模型上下文、工具参数和第三方响应。生产默认建议使用 `LOG_LEVEL=info` 或 `warn`，只在问题复现窗口内临时提高日志级别。

## 常见排查路径

### API 返回错误

1. 从响应头拿到 `X-Request-ID`。
2. 在 App 日志中搜索该 request ID。
3. 查看统一错误响应中的 `error.code` 和 `error.message`。
4. 如果请求触发模型调用，到 Langfuse 中按 `request_id` 搜索 trace。

### 文档解析失败

1. 查看知识详情中的 `parse_status`。
2. 调用 `/api/v1/knowledge/{id}/spans`。
3. 查看 `last_error.code`、`last_error.stage`。
4. 如果失败在 `docreader`，看 DocReader 日志。
5. 如果失败在 `embedding` 或 `postprocess`，看模型配置、Langfuse 和 LLM 调试日志。

### Agent 响应慢

1. 找到 `agent-chat` 请求的 request ID。
2. 在 Langfuse 中看 trace 的各个 span 和 generation 耗时。
3. 检查是否有 MCP 工具审批等待。
4. 检查 `WEKNORA_AGENT_LLM_TIMEOUT` 和工具服务超时配置。

### 权限异常

1. 查看 API 错误是否为 403。
2. 查询租户审计日志，过滤 `outcome=denied`。
3. 检查当前用户在租户或组织中的角色。
4. 确认 `WEKNORA_TENANT_ENABLE_RBAC` 启动摘要是否符合预期。

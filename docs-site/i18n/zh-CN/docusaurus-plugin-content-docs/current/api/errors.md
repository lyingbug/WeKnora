---
title: 错误码
description: 理解 API 错误响应。
---

# 错误码

WeKnora 的大多数 HTTP API 使用统一错误封装。调用方应优先读取 HTTP 状态码和 `error.code`，再用 `error.message` 展示或记录具体原因。

## 统一错误响应

通过统一错误中间件返回的错误结构如下：

```json
{
  "success": false,
  "error": {
    "code": 1000,
    "message": "Knowledge base ID cannot be empty",
    "details": "kb_id is required"
  }
}
```

字段说明：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `success` | boolean | 错误响应固定为 `false`。 |
| `error.code` | number | 稳定的应用错误码。 |
| `error.message` | string | 可展示或记录的人类可读错误信息。 |
| `error.details` | any | 可选字段，通常包含校验细节或底层错误补充信息。 |

`details` 不是所有错误都会返回。客户端不要依赖它一定存在，也不要用它替代 `code` 做主要分支判断。

## 通用错误码

| 错误码 | HTTP 状态码 | 含义 | 常见处理方式 |
| --- | --- | --- | --- |
| `1000` | 400 | 请求参数或请求体不合法 | 检查必填字段、路径参数、JSON 格式和枚举值。 |
| `1001` | 401 | 未认证或认证信息失效 | 重新登录或刷新访问令牌。 |
| `1002` | 403 | 已认证但无权限 | 切换有权限的租户、组织或角色。 |
| `1003` | 404 | 资源不存在 | 检查 ID、租户上下文或资源是否已删除。 |
| `1004` | 405 | 请求方法不支持 | 检查 HTTP 方法和接口路径。 |
| `1005` | 409 | 资源冲突 | 处理重复创建、状态冲突或并发更新。 |
| `1006` | 429 | 请求过多或配额限制 | 降低频率，稍后重试。 |
| `1007` | 500 | 服务内部错误 | 记录请求 ID 和错误信息，联系管理员排查。 |
| `1008` | 503 | 服务暂时不可用 | 按退避策略重试。 |
| `1009` | 504 或内部超时场景 | 请求超时 | 缩小请求规模或稍后重试。 |
| `1010` | 400 | 参数校验失败 | 根据 `message` 或 `details` 修正输入。 |

## 业务错误码

部分模块会使用更细的应用错误码，便于客户端不解析错误文本也能判断原因。

### 租户

| 错误码 | HTTP 状态码 | 含义 |
| --- | --- | --- |
| `2000` | 404 | 租户不存在。 |
| `2001` | 409 | 租户已存在。 |
| `2002` | 403 | 租户已停用。 |
| `2003` | 400 | 租户名称缺失。 |
| `2004` | 400 | 租户状态不合法。 |

### 智能体配置

| 错误码 | HTTP 状态码 | 含义 |
| --- | --- | --- |
| `2100` | 400 | 启用 Agent 模式前没有选择思考模型。 |
| `2101` | 400 | Agent 模式没有选择任何允许工具。 |
| `2102` | 400 | 最大迭代次数不在允许范围内。 |
| `2103` | 400 | 温度参数不在允许范围内。 |

### 向量存储绑定

| 错误码 | HTTP 状态码 | 含义 |
| --- | --- | --- |
| `2200` | 400 | 知识库绑定的向量存储 ID 不合法、不存在或不属于当前租户。 |
| `2201` | 400 | 向量存储记录存在，但当前服务实例不可用。 |

这两个错误主要出现在创建或更新知识库时。客户端可以把 `2200` 解释为用户输入或选择错误，把 `2201` 解释为管理员配置或运行时状态问题。

## 前端请求封装

前端 `request` 封装会把统一错误响应转换成 rejected 对象，并把 `error.message` 提升到顶层：

```json
{
  "status": 400,
  "message": "Knowledge base ID cannot be empty",
  "success": false,
  "error": {
    "code": 1000,
    "message": "Knowledge base ID cannot be empty"
  }
}
```

因此前端业务代码通常可以用：

```ts
try {
  await createKnowledgeBase(payload)
} catch (err: any) {
  const status = err.status
  const code = err.error?.code
  const message = err.message
}
```

网络不可达时没有后端响应，前端会抛出本地化后的网络错误：

```json
{
  "message": "网络错误，请检查连接"
}
```

上传超过网关限制时，前端会把 413 转成文件大小提示，并返回：

```json
{
  "status": 413,
  "message": "文件大小超过限制",
  "success": false
}
```

## 直接返回的历史错误

少数接口没有走统一错误封装，而是直接返回简单 JSON。调用方需要兼容这些格式：

```json
{
  "error": "unauthorized"
}
```

```json
{
  "success": false,
  "error": "vector store not found"
}
```

常见来源包括数据源、IM 渠道、Wiki 页面、部分系统管理接口、向量存储和 Web Search Provider 管理接口。前端请求封装会尽量把字符串 `error` 提升成顶层 `message`，但这些响应通常没有数字 `error.code`。

## 业务成功但操作失败

有些接口为了让页面展示诊断信息，会在 HTTP 200 中返回业务失败状态。调用方必须检查业务字段，而不是只看 HTTP 状态码。

例如 MCP 连接测试失败时：

```json
{
  "success": true,
  "data": {
    "success": false,
    "message": "Test failed: connection timeout"
  }
}
```

向量存储和 Web Search Provider 的连通性测试也可能返回 HTTP 200，并在响应体中用 `success: false` 表示测试未通过。

## 重复知识错误

上传文件或导入 URL 时，如果检测到重复知识，接口返回 409，但结构不同于统一错误封装：

```json
{
  "success": false,
  "message": "duplicate knowledge",
  "code": "duplicate_file",
  "data": {
    "id": "existing-knowledge-id"
  }
}
```

可能的 `code` 包括：

| `code` | 含义 |
| --- | --- |
| `duplicate_file` | 文件已存在。 |
| `duplicate_url` | URL 已存在。 |

这里的 `code` 是字符串业务码，不是统一错误响应里的数字 `error.code`。

## 解析任务错误码

知识解析流水线会记录阶段级错误码，字段名通常是 `error_code` 或 `last_error.code`。这套错误码是字符串，用于定位解析、切分、向量写入等异步任务失败原因。

查询知识解析追踪时，失败响应片段可能类似：

```json
{
  "success": true,
  "data": {
    "knowledge_id": "kid",
    "parse_status": "failed",
    "last_error": {
      "stage": "doc_reader",
      "code": "DOCREADER_TIMEOUT",
      "message": "document read failed",
      "finished_at": "2026-06-11T10:00:00Z"
    }
  }
}
```

解析阶段错误码如下：

| 错误码 | 含义 | 建议处理 |
| --- | --- | --- |
| `DOCREADER_TIMEOUT` | DocReader 调用超过配置超时 | 拆分大文件，检查 DocReader 负载或超时配置。 |
| `DOCREADER_UNAVAILABLE` | 没有可用 DocReader 或服务拒绝连接 | 检查 DocReader 服务和文件类型引擎配置。 |
| `DOCREADER_PARSE_FAILED` | DocReader 返回解析失败 | 检查文件是否损坏、编码是否异常或 OCR 引擎状态。 |
| `CHUNKING_FAILED` | 文本切分失败 | 检查文本大小和切分配置。 |
| `EMBEDDING_RATE_LIMIT` | Embedding Provider 限流 | 稍后重试或扩容模型服务。 |
| `EMBEDDING_PROVIDER_FAIL` | Embedding Provider 非限流失败 | 检查模型凭证、模型名称和输入内容。 |
| `VECTORSTORE_WRITE_FAILED` | 向量写入失败 | 检查向量数据库连接、配额、索引或 schema。 |
| `MULTIMODAL_VLM_FAILED` | 单个图片的多模态处理失败 | 可检查图片格式、VLM 配置或重试。 |
| `MULTIMODAL_ALL_FAILED` | 所有图片多模态任务失败 | 检查 VLM 服务整体可用性。 |
| `TASK_TIMEOUT` | 异步任务重试耗尽或任务级超时 | 检查队列积压、任务耗时和 worker 状态。 |
| `UNKNOWN` | 未分类错误 | 查看日志中的 `error_detail` 或联系管理员。 |

`UPSTREAM_FAILED` 也可能出现在阶段追踪里，表示当前阶段因为上游阶段失败而被取消。

## 流式接口错误

聊天和智能体流式接口可能通过事件发送错误数据，而不是只依赖最终 HTTP 状态。错误事件中的数据结构包含：

```json
{
  "error": "model call failed",
  "error_code": "MODEL_PROVIDER_FAIL",
  "stage": "generation",
  "session_id": "session-id",
  "query": "用户问题"
}
```

流式调用方应同时处理：

| 来源 | 处理方式 |
| --- | --- |
| HTTP 非 2xx | 按普通 API 错误处理。 |
| SSE 错误事件 | 展示事件里的 `error`，并记录 `error_code`、`stage`、`session_id`。 |
| 网络中断 | 提示用户重试，并根据会话 ID 尝试加载已落库消息。 |

## 重试建议

| 场景 | 是否建议重试 |
| --- | --- |
| `400`、`1010`、`2200` | 不建议原样重试，需要修改请求。 |
| `401` | 先刷新令牌或重新登录。 |
| `403` | 不建议重试，需要更换角色、租户或资源授权。 |
| `404` | 不建议原样重试，除非资源可能还在异步创建中。 |
| `409` | 先读取现有资源或重新计算幂等键。 |
| `429`、`EMBEDDING_RATE_LIMIT` | 建议退避后重试。 |
| `500`、`1007` | 可短暂重试一次；连续失败应排查服务日志。 |
| `503`、`1008` | 建议退避重试。 |
| `TASK_TIMEOUT` | 原因通常在任务耗时或队列状态，直接重试前应先确认负载。 |

## 排查信息

排查错误时建议记录以下信息：

- HTTP 方法和路径。
- HTTP 状态码。
- 响应体中的 `error.code`、`error.message` 和 `error.details`。
- 请求头中的 `X-Request-ID`。
- 当前租户 ID 和资源 ID。
- 对异步解析任务，记录 `knowledge_id`、`current_attempt`、`last_error.code` 和 `last_error.stage`。

不要把访问令牌、模型密钥、MCP 凭证、对象存储密钥或第三方 Webhook Secret 写入日志。

---
title: MCP 服务 API
description: 管理 MCP 服务、凭证、工具清单和工具调用审批策略。
---

# MCP 服务 API

MCP 服务 API 用于在租户内维护可被智能体调用的 MCP 服务。它覆盖服务配置、凭证写入、连接测试、工具与资源发现，以及工具调用审批策略。

智能体是否使用 MCP 服务由智能体配置决定：`config.mcp_selection_mode` 控制是否启用 MCP，`config.mcp_services` 控制选中的服务列表。MCP 服务本身通过本页接口独立管理。

## 权限

MCP 服务接口都需要通过租户鉴权。不同操作需要的最低角色如下：

| 操作 | 最低角色 | 说明 |
| --- | --- | --- |
| 新建、更新、删除 MCP 服务 | `Admin` | 会改变租户内可用工具来源。 |
| 写入或删除凭证 | `Admin` | 凭证不会通过普通服务更新接口写入。 |
| 测试连接 | `Admin` | 测试会按当前配置发起实际连接。 |
| 设置工具审批策略 | `Admin` | 决定某个工具是否每次调用前都要审批。 |
| 查看服务、工具、资源和审批策略 | `Viewer` | 只读取当前租户配置。 |
| 处理运行时工具审批 | `Viewer` | 只能处理自己发起的会话中产生的待审批请求。 |

## 服务对象

服务响应中的主要字段如下：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `id` | number | MCP 服务 ID。 |
| `tenant_id` | number | 所属租户 ID。 |
| `name` | string | 服务名称，同一租户内唯一。 |
| `description` | string | 服务说明。 |
| `enabled` | boolean | 是否启用。关闭后不会作为可用工具来源。 |
| `transport_type` | string | 传输方式，可选 `sse`、`http-streamable`、`stdio`。 |
| `url` | string | `sse` 和 `http-streamable` 服务地址。 |
| `headers` | object | 请求头配置。 |
| `auth_config.custom_headers` | object | 自定义认证头配置。 |
| `advanced_config.timeout` | number | 超时时间，默认 30。 |
| `advanced_config.retry_count` | number | 重试次数，默认 3。 |
| `advanced_config.retry_delay` | number | 重试间隔，默认 1。 |
| `stdio_config.command` | string | `stdio` 服务启动命令，通常是 `uvx` 或 `npx`。 |
| `stdio_config.args` | string[] | `stdio` 服务启动参数。 |
| `env_vars` | object | `stdio` 服务环境变量。 |
| `is_builtin` | boolean | 是否为系统内置服务。 |
| `credentials` | object | 凭证元数据，只表示字段是否已配置。 |
| `created_at` / `updated_at` | string | 创建和更新时间。 |

响应不会返回 `api_key`、`token` 等密钥原文。非内置服务会返回类似下面的凭证元数据：

```json
{
  "credentials": {
    "api_key": { "configured": true },
    "token": { "configured": false }
  }
}
```

内置服务会隐藏租户不可直接修改的传输细节，响应中不会返回 `url`、`headers`、`env_vars`、`stdio_config`、`auth_config` 和 `credentials`。

## 新建服务

```http
POST /api/v1/mcp-services
Authorization: Bearer <token>
Content-Type: application/json
```

示例请求：

```json
{
  "name": "company-tools",
  "description": "公司内部工具服务",
  "enabled": true,
  "transport_type": "http-streamable",
  "url": "https://mcp.example.com/mcp",
  "headers": {
    "X-Source": "WeKnora"
  },
  "auth_config": {
    "custom_headers": {
      "X-Tenant": "demo"
    }
  },
  "advanced_config": {
    "timeout": 30,
    "retry_count": 3,
    "retry_delay": 1
  }
}
```

成功响应使用 HTTP 200：

```json
{
  "success": true,
  "data": {
    "id": 12,
    "tenant_id": 1,
    "name": "company-tools",
    "description": "公司内部工具服务",
    "enabled": true,
    "transport_type": "http-streamable",
    "url": "https://mcp.example.com/mcp",
    "headers": {
      "X-Source": "WeKnora"
    },
    "auth_config": {
      "custom_headers": {
        "X-Tenant": "demo"
      }
    },
    "advanced_config": {
      "timeout": 30,
      "retry_count": 3,
      "retry_delay": 1
    },
    "credentials": {
      "api_key": { "configured": false },
      "token": { "configured": false }
    },
    "created_at": "2026-06-11T10:00:00Z",
    "updated_at": "2026-06-11T10:00:00Z"
  }
}
```

如果配置了 `url`，服务端会在保存前做 SSRF 安全校验。推荐先创建结构化配置，再通过凭证接口单独写入密钥。

## 查询服务

列出租户内所有 MCP 服务：

```http
GET /api/v1/mcp-services
Authorization: Bearer <token>
```

获取单个 MCP 服务：

```http
GET /api/v1/mcp-services/{id}
Authorization: Bearer <token>
```

两类接口都返回统一结构：

```json
{
  "success": true,
  "data": [
    {
      "id": 12,
      "name": "company-tools",
      "enabled": true,
      "transport_type": "http-streamable"
    }
  ]
}
```

单个服务不存在时返回 404。

## 更新服务

```http
PUT /api/v1/mcp-services/{id}
Authorization: Bearer <token>
Content-Type: application/json
```

更新接口是部分更新，只需要传入要修改的字段：

```json
{
  "enabled": false,
  "advanced_config": {
    "timeout": 60
  }
}
```

支持更新的字段包括：

| 字段 | 行为 |
| --- | --- |
| `name` / `description` / `enabled` | 直接覆盖。 |
| `transport_type` | 切换传输方式。 |
| `url` | 非空时重新做 SSRF 校验；传空字符串或 `null` 会清空。 |
| `headers` | 覆盖请求头配置。 |
| `auth_config.custom_headers` | 覆盖自定义认证头配置。 |
| `advanced_config.timeout` | 更新超时时间。 |
| `advanced_config.retry_count` | 更新重试次数。 |
| `advanced_config.retry_delay` | 更新重试间隔。 |
| `stdio_config.command` | 更新 `stdio` 启动命令。 |
| `stdio_config.args` | 更新 `stdio` 启动参数。 |
| `env_vars` | 覆盖环境变量。 |

普通更新接口会忽略 `auth_config.api_key` 和 `auth_config.token`。密钥必须通过凭证接口写入，避免编辑普通配置时误删或泄露凭证。

成功后返回重新读取的服务对象：

```json
{
  "success": true,
  "data": {
    "id": 12,
    "name": "company-tools",
    "enabled": false
  }
}
```

## 删除服务

```http
DELETE /api/v1/mcp-services/{id}
Authorization: Bearer <token>
```

成功响应：

```json
{
  "success": true,
  "message": "MCP service deleted successfully"
}
```

## 管理凭证

凭证使用独立子资源管理。系统没有提供读取凭证明文的接口，只能通过服务响应中的 `credentials` 元数据判断某个字段是否已经配置。

写入或替换凭证：

```http
PUT /api/v1/mcp-services/{id}/credentials
Authorization: Bearer <token>
Content-Type: application/json
```

```json
{
  "api_key": "sk-example",
  "token": "token-example"
}
```

响应只返回字段状态：

```json
{
  "fields": {
    "api_key": { "configured": true },
    "token": { "configured": true }
  }
}
```

凭证写入规则：

| 请求内容 | 行为 |
| --- | --- |
| 字段缺省 | 保留原值。 |
| 字段为空字符串 | 不做修改。 |
| 字段为非空字符串 | 替换该字段凭证。 |
| 请求体不包含任何凭证字段 | 返回当前凭证元数据。 |

删除某个凭证字段：

```http
DELETE /api/v1/mcp-services/{id}/credentials/{field}
Authorization: Bearer <token>
```

`field` 只支持 `api_key` 和 `token`。删除成功返回 204；字段原本未配置时也视为成功。无效字段返回 400。

## 测试连接

```http
POST /api/v1/mcp-services/{id}/test
Authorization: Bearer <token>
```

连接成功时：

```json
{
  "success": true,
  "data": {
    "success": true,
    "message": "Connection successful",
    "tools": [
      {
        "name": "search_docs",
        "description": "检索文档",
        "inputSchema": {
          "type": "object"
        }
      }
    ],
    "resources": []
  }
}
```

连接失败时仍然返回 HTTP 200，但 `data.success` 为 `false`：

```json
{
  "success": true,
  "data": {
    "success": false,
    "message": "Test failed: connection timeout"
  }
}
```

调用方需要根据 `data.success` 判断测试是否通过，而不是只看 HTTP 状态码。

## 工具和资源

查询某个服务暴露的工具：

```http
GET /api/v1/mcp-services/{id}/tools
Authorization: Bearer <token>
```

```json
{
  "success": true,
  "data": [
    {
      "name": "search_docs",
      "description": "检索文档",
      "inputSchema": {
        "type": "object",
        "properties": {
          "query": { "type": "string" }
        },
        "required": ["query"]
      },
      "require_approval": true
    }
  ]
}
```

查询某个服务暴露的资源：

```http
GET /api/v1/mcp-services/{id}/resources
Authorization: Bearer <token>
```

```json
{
  "success": true,
  "data": [
    {
      "uri": "kb://policies",
      "name": "制度库",
      "description": "公司制度相关资源",
      "mimeType": "application/json"
    }
  ]
}
```

工具和资源接口会使用当前服务配置连接 MCP 服务。如果连接或协议调用失败，接口返回 500。

## 工具审批策略

审批策略按租户、服务和工具保存。开启后，智能体运行到该工具调用时会先暂停，并通过会话事件发出待审批请求。

列出某个服务已保存的审批策略：

```http
GET /api/v1/mcp-services/{id}/tool-approvals
Authorization: Bearer <token>
```

```json
{
  "success": true,
  "data": [
    {
      "id": 1,
      "tenant_id": 1,
      "service_id": 12,
      "tool_name": "search_docs",
      "require_approval": true
    }
  ]
}
```

设置某个工具是否需要审批：

```http
PUT /api/v1/mcp-services/{id}/tool-approvals/{tool_name}
Authorization: Bearer <token>
Content-Type: application/json
```

```json
{
  "require_approval": true
}
```

成功响应：

```json
{
  "success": true
}
```

`tool_name` 来自 MCP 工具清单中的 `name`。如果工具名包含特殊字符，调用方需要按 URL 路径规则编码。

## 处理运行时审批

当智能体触发一个需要审批的 MCP 工具调用时，聊天流中会产生待审批事件。前端拿到事件中的 `pending_id` 后，通过下面的接口处理：

```http
POST /api/v1/agent/tool-approvals/{pending_id}
Authorization: Bearer <token>
Content-Type: application/json
```

通过调用：

```json
{
  "decision": "approve",
  "reason": "参数正确，可以执行"
}
```

通过并改写参数：

```json
{
  "decision": "approve",
  "modified_args": {
    "query": "只检索 2026 年政策"
  },
  "reason": "收窄检索范围"
}
```

拒绝调用：

```json
{
  "decision": "reject",
  "reason": "查询范围过大"
}
```

成功响应：

```json
{
  "success": true
}
```

运行时审批的校验规则：

| 条件 | 结果 |
| --- | --- |
| `decision` 不是 `approve` 或 `reject` | 返回 400。 |
| `approve` 时 `modified_args` 不是 JSON 对象 | 返回 400。 |
| 待审批项不存在或已完成 | 返回 404 或 400。 |
| 处理人不是该会话发起用户 | 返回 400。 |
| 请求没有认证用户 | 返回 401。 |

`modified_args` 只能是非空 JSON 对象，不能是数组、字符串或 `null`。如果不需要改写参数，可以省略该字段。

## 与智能体调用的关系

MCP 服务配置完成后，还需要在智能体配置中启用：

```json
{
  "config": {
    "mcp_selection_mode": "selected",
    "mcp_services": [12]
  }
}
```

常见模式：

| `mcp_selection_mode` | 行为 |
| --- | --- |
| `none` | 不使用 MCP 服务。 |
| `selected` | 只使用 `mcp_services` 中指定的服务。 |
| `all` | 使用租户内可用的 MCP 服务。 |

工具审批策略只决定调用前是否需要人工确认，不会自动把服务挂到智能体上。

## 常见状态码

| 状态码 | 场景 |
| --- | --- |
| 200 | 查询、创建、更新、删除、测试连接、设置审批策略成功。 |
| 204 | 删除凭证字段成功。 |
| 400 | 请求体不合法、租户缺失、审批参数不合法、字段名不支持。 |
| 401 | 运行时审批缺少认证用户。 |
| 404 | 服务不存在，或待审批项不存在。 |
| 500 | MCP 服务调用失败、审批服务未配置或内部错误。 |

测试连接接口需要特别处理：连接失败时通常仍是 HTTP 200，失败原因放在 `data.message` 中。

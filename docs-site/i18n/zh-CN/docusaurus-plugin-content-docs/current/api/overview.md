---
title: API 概览
description: 使用 WeKnora REST API。
---

# API 概览

WeKnora API 可用于管理知识库、上传知识、搜索内容、创建会话、发送消息、管理 Agent 和配置集成。

## 基础地址

业务接口统一挂载在：

```text
/api/v1
```

例如：

```text
GET /api/v1/knowledge-bases
POST /api/v1/sessions
POST /api/v1/sessions/:id/qa
```

`/health` 是健康检查接口，不带 `/api/v1` 前缀。

开发环境中，Swagger UI 会挂载在 `/swagger/*any`。当 Gin 运行在 release 模式时，Swagger UI 会被关闭。

## 认证方式

大多数接口需要认证。WeKnora 支持两种调用方式：

### Bearer Token

登录成功后，客户端使用 JWT：

```http
Authorization: Bearer <token>
```

前端请求封装会自动读取本地 token，并在 401 时尝试调用 `/api/v1/auth/refresh` 刷新 token。

JWT 默认使用登录时的租户。如果用户可访问多个租户，可以传入：

```http
X-Tenant-ID: <tenant_id>
```

后端会验证该用户是否能访问目标租户。无效、为空或不可访问的租户 ID 会被拒绝，而不是静默回退。

### 租户 API Key

自动化脚本或服务端集成可以使用租户 API Key：

```http
X-API-Key: <tenant_api_key>
```

API Key 会解析出租户，并以合成用户 `system-<tenantID>` 执行请求。该通道固定授予租户 Admin 权限，但不会授予 Owner 或 SystemAdmin 权限；删除租户、重置租户 API Key、平台级管理等高风险操作仍需要交互式登录身份。

## 公共接口

以下接口不需要普通认证：

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `GET` | `/health` | 健康检查。 |
| `POST` | `/api/v1/auth/register` | 自助注册。 |
| `POST` | `/api/v1/auth/register-by-invite` | 邀请链接注册。 |
| `POST` | `/api/v1/auth/invitations/lookup` | 查询邀请链接信息。 |
| `POST` | `/api/v1/auth/login` | 登录。 |
| `POST` | `/api/v1/auth/auto-setup` | 首次初始化。 |
| `GET` | `/api/v1/auth/config` | 获取认证配置。 |
| `GET` | `/api/v1/auth/oidc/config` | 获取 OIDC 配置。 |
| `GET` | `/api/v1/auth/oidc/url` | 获取 OIDC 授权 URL。 |
| `GET` | `/api/v1/auth/oidc/callback` | OIDC 回调。 |
| `POST` | `/api/v1/auth/refresh` | 刷新 token。 |
| `GET` / `HEAD` | `/api/v1/files/presigned` | 预签名文件访问。 |
| `GET` / `POST` | `/api/v1/im/callback/:channel_id` | IM 平台回调，使用平台签名校验。 |

邀请链接相关公开接口有基于 IP 的限流。IM 回调路由在认证中间件之前注册，不使用 Bearer Token 或 API Key，而是由对应 IM Adapter 校验平台签名、token 或 challenge。

## 通用请求头

常用请求头如下：

| Header | 说明 |
| --- | --- |
| `Authorization` | Bearer JWT。 |
| `X-API-Key` | 租户 API Key。 |
| `X-Tenant-ID` | 使用 JWT 时切换当前请求的目标租户。 |
| `X-Request-ID` | 请求追踪 ID。缺失时后端会生成，并在响应头返回。 |
| `Accept-Language` | 语言偏好。若设置了 `WEKNORA_LANGUAGE` 环境变量，服务端会优先使用环境变量。 |
| `Content-Type` | JSON 接口通常使用 `application/json`；文件上传接口使用 `multipart/form-data`。 |

跨域配置允许 `Authorization`、`X-API-Key` 和 `X-Request-ID`。

## 权限模型

WeKnora 的业务接口按租户角色和资源归属共同控制。

常见角色层级为：

| 角色 | 说明 |
| --- | --- |
| Viewer | 读取资源、查看配置和日志。 |
| Contributor | 创建或编辑部分用户内容。 |
| Admin | 管理租户级基础设施，例如模型、向量库、数据源、IM 渠道、MCP、网络搜索。 |
| Owner | 租户最高权限，管理成员、邀请、API Key 和租户高风险设置。 |
| SystemAdmin | 平台级管理权限，只能通过 JWT 登录身份获得。 |

部分资源还会检查“创建者或 Admin”：

- 知识库内容写操作通常要求知识库创建者或 Admin+。
- Agent 写操作通常要求 Agent 创建者或 Admin+。
- Wiki 页面、知识片段、知识条目等会从 URL 参数追溯到所属知识库再判断权限。

共享知识库、共享 Agent 和组织权限会影响读写可见性。具体规则见各资源文档。

如果租户 RBAC 开关关闭，路由守卫会记录日志但放行，以兼容历史部署；SystemAdmin 相关检查不属于这个放行窗口。

## 响应格式

WeKnora 代码中既有较新的统一响应，也保留了一些历史接口的直接 JSON 响应。客户端应以具体接口文档为准，并兼容以下几类形状。

常见成功响应：

```json
{
  "success": true,
  "data": {}
}
```

列表接口可能返回：

```json
{
  "success": true,
  "data": [],
  "total": 0
}
```

部分新接口直接返回资源对象或数组，例如数据源、IM、WebSearchProvider 的某些查询接口。部分删除接口返回 `204 No Content`。

应用错误通过 `ErrorHandler` 统一包装时，形状为：

```json
{
  "success": false,
  "error": {
    "code": 1000,
    "message": "invalid request",
    "details": {}
  }
}
```

一些历史 handler 会直接返回：

```json
{
  "error": "message"
}
```

或：

```json
{
  "success": false,
  "error": "message"
}
```

前端请求封装会优先提取 `error.message`，其次提取字符串 `error` 或顶层 `message`。

## 常见状态码

| 状态码 | 含义 |
| --- | --- |
| `200` | 请求成功。 |
| `201` | 创建成功。 |
| `204` | 删除或清除成功，无响应体。 |
| `400` | 请求参数、租户切换、配置校验或业务规则错误。 |
| `401` | 未认证、token 失效、API Key 无效。 |
| `403` | 已认证但权限不足，或签名校验失败。 |
| `404` | 资源不存在，或为了避免泄露跨租户资源而按不存在处理。 |
| `409` | 冲突，例如重复 bot、重复配置或资源已存在。 |
| `429` | 公开认证接口或其它限流场景。 |
| `500` | 未处理的服务端错误。 |
| `503` | 服务暂不可用，例如 IM 渠道不可用。 |

## 主要资源分组

| 分组 | 典型路径 | 说明 |
| --- | --- | --- |
| 认证 | `/auth/*` | 注册、登录、OIDC、刷新 token、当前用户。 |
| 租户与成员 | `/tenants/*`、`/me/invitations` | 租户、成员、邀请、API Key、租户配置。 |
| 知识库 | `/knowledge-bases/*` | 知识库、上传、复制、共享、配置。 |
| 知识内容 | `/knowledge/*` | 文档、URL、批量操作、解析任务。 |
| 标签与 FAQ | `/knowledge-tags/*`、`/faq/*` | 知识标签、FAQ 导入和管理。 |
| 分块 | `/chunks/*` | 分块读取、编辑、删除和调试预览。 |
| 会话与聊天 | `/sessions/*`、`/chat/*`、`/messages/*` | 会话、问答、SSE、消息历史。 |
| Agent | `/agents/*` | 自定义 Agent、类型预设、复制和配置。 |
| 模型 | `/models/*`、`/initialization/*` | 模型管理、初始化检测、Ollama 下载。 |
| MCP | `/mcp-services/*` | MCP 服务、工具、审批和凭据。 |
| 网络搜索 | `/web-search/*`、`/web-search-providers/*` | 搜索运行配置和 Provider 管理。 |
| 向量库 | `/vector-stores/*` | 租户级向量库配置。 |
| 数据源 | `/datasource/*` | 飞书、Notion、语雀等数据源同步。 |
| IM | `/im-channels/*`、`/agents/:id/im-channels` | Agent IM 渠道配置。 |
| Wiki | `/knowledgebase/:kb_id/wiki/*` | Wiki 页面和链接维护。 |
| 组织 | `/organizations/*` | 组织、成员、共享和权限。 |
| 系统 | `/system/*`、`/admin/*` | 系统设置和平台管理。 |

## 文件访问

普通文件服务代理需要认证。IM 图片等外部平台访问场景使用预签名路由：

```text
GET /api/v1/files/presigned?file_path=<provider://...>&tenant_id=<id>&expires=<unix>&sig=<hmac>
HEAD /api/v1/files/presigned?...
```

该接口不走普通认证，但会校验 HMAC 签名和过期时间。`HEAD` 用于 IM 平台预检查图片的 `Content-Type` 和可访问性。

管理员还可以使用诊断接口预览当前租户会生成的预签名 URL：

```text
GET /api/v1/files/presigned-preview?file_path=<provider://...>
```

该接口需要 Admin 权限。

## 客户端建议

- 对交互式用户使用 Bearer Token；对服务端自动化使用租户 API Key。
- 使用 JWT 跨租户访问时，始终显式传 `X-Tenant-ID`。
- 为每次请求设置 `X-Request-ID`，方便排查日志；不设置时服务端会生成。
- 不要假设所有成功响应都包在 `success/data` 中，尤其是较新的集成类接口。
- 不要解析错误字符串来判断业务类型；优先使用统一错误中的 `error.code`，其次再使用 HTTP 状态码。
- 文件上传、SSE、流式 Agent 接口的响应形状和超时行为与普通 JSON 接口不同，应单独处理。

## 相关文档

- [认证 API](./authentication.md)
- [知识库 API](./knowledge-base.md)
- [聊天 API](./chat.md)
- [Agent API](./agent.md)
- [错误处理](./errors.md)

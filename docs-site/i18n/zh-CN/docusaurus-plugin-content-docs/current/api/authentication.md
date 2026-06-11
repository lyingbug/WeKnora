---
title: API 认证
description: 认证 WeKnora API 请求。
---

# API 认证

WeKnora 的业务 API 默认由认证中间件保护，路径前缀为 `/api/v1`。客户端通常使用登录接口签发的 Bearer Token；服务端到服务端的调用也可以使用租户 API Key。

需要注意的是，认证不仅校验“是谁”，也会确定本次请求的租户上下文。后端会把 `user_id`、`tenant_id`、`tenant_role`、`is_system_admin` 写入请求上下文，后续 handler 和 RBAC 中间件都基于这些值授权。

## 认证方式

### Bearer Token

密码登录、OIDC 登录、邀请注册、Lite 自动初始化和租户切换都会返回访问令牌：

```http
Authorization: Bearer <access_token>
```

访问令牌有效期为 24 小时，刷新令牌有效期为 7 天。访问令牌的 JWT claim 中包含当前激活租户的 `tenant_id`；旧 token 如果没有该 claim，后端会回退到用户的 home tenant。

前端请求封装会自动附加：

```http
Authorization: Bearer <access_token>
X-Tenant-ID: <selected_tenant_id>
X-Request-ID: <uuid>
Accept-Language: zh-CN
```

`X-Tenant-ID` 可用于在 Bearer Token 认证下切换请求租户，但目标租户必须是用户可访问的租户。若租户 ID 格式错误会返回 `400`；若用户没有权限访问目标租户会返回 `403`。

### 租户 API Key

服务端集成可以使用租户 API Key：

```http
X-API-Key: <tenant_api_key>
```

API Key 会绑定到对应租户。通过 API Key 进入的请求会使用一个合成用户 `system-<tenantID>`，租户角色固定为 `admin`。它可以执行大多数租户级管理操作，但不会被视为 Owner，也不会拥有 SystemAdmin 权限。

## 无需登录的端点

以下端点会被认证中间件放行：

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| `GET` | `/health` | 健康检查，不在 `/api/v1` 下。 |
| `POST` | `/api/v1/auth/register` | 自助注册；当注册模式为 `invite_only` 时返回 `403`。 |
| `POST` | `/api/v1/auth/register-by-invite` | 使用共享邀请链接注册。 |
| `POST` | `/api/v1/auth/invitations/lookup` | 查询邀请链接上下文。 |
| `POST` | `/api/v1/auth/login` | 邮箱密码登录。 |
| `POST` | `/api/v1/auth/auto-setup` | Lite 版首次启动自动初始化。 |
| `GET` | `/api/v1/auth/config` | 读取公开认证配置。 |
| `GET` | `/api/v1/auth/oidc/config` | 读取 OIDC 登录入口配置。 |
| `GET` | `/api/v1/auth/oidc/url` | 生成 OIDC provider 授权地址。 |
| `GET` | `/api/v1/auth/oidc/callback` | OIDC provider 回调。 |
| `POST` | `/api/v1/auth/refresh` | 使用 refresh token 换新 token。 |
| `GET` / `HEAD` | `/api/v1/files/presigned` | 预签名文件访问。 |
| `GET` / `POST` | `/api/v1/im/callback/:channel_id` | IM 平台回调。 |

`register-by-invite` 和 `invitations/lookup` 还会经过公开认证限流：单进程内按客户端 IP 共享 30 次/分钟预算，超限返回 `429`。

## 注册模式

公开配置接口只暴露注册入口需要的字段：

```http
GET /api/v1/auth/config
```

响应：

```json
{
  "success": true,
  "registration_mode": "self_serve"
}
```

`registration_mode` 可能是：

| 值 | 行为 |
| --- | --- |
| `self_serve` | 允许调用 `/auth/register` 自助注册。 |
| `invite_only` | `/auth/register` 返回 `403`，用户必须使用邀请链接注册。 |

后端读取顺序是运行时系统设置优先，其次是启动配置，最后回退到 `self_serve`。历史环境变量禁用注册时，会在配置加载阶段等价转换为 `invite_only`。

## 邮箱密码注册与登录

### 自助注册

```http
POST /api/v1/auth/register
Content-Type: application/json
```

请求体：

```json
{
  "username": "alice",
  "email": "alice@example.com",
  "password": "secret123"
}
```

字段约束：

| 字段 | 约束 |
| --- | --- |
| `username` | 必填，2 到 50 个字符。 |
| `email` | 必填，合法邮箱。 |
| `password` | 必填，至少 6 个字符。 |

成功时返回 `201`：

```json
{
  "success": true,
  "message": "Registration successful",
  "user": {
    "id": "...",
    "username": "alice",
    "email": "alice@example.com"
  },
  "tenant": {
    "id": 1,
    "name": "..."
  }
}
```

注册成功会创建用户及其 home tenant。注意普通注册不会自动返回 token，客户端仍需调用登录接口。

### 登录

```http
POST /api/v1/auth/login
Content-Type: application/json
```

请求体：

```json
{
  "email": "alice@example.com",
  "password": "secret123"
}
```

成功响应：

```json
{
  "success": true,
  "message": "Login successful",
  "user": {
    "id": "...",
    "email": "alice@example.com",
    "tenant_id": 1,
    "preferences": {
      "enable_memory": true,
      "last_active_tenant_id": 2
    }
  },
  "active_tenant": {
    "id": 2,
    "name": "研发空间"
  },
  "memberships": [
    {
      "tenant_id": 1,
      "tenant_name": "默认空间",
      "role": "owner"
    },
    {
      "tenant_id": 2,
      "tenant_name": "研发空间",
      "role": "admin"
    }
  ],
  "token": "<access_token>",
  "refresh_token": "<refresh_token>"
}
```

登录时后端会优先尝试使用用户偏好中的 `last_active_tenant_id` 作为激活租户。该租户必须存在，并且用户仍有 active 成员关系；否则后端会清理过期偏好并回退到 home tenant。

`memberships` 只包含 active 成员关系，角色可能是 `viewer`、`contributor`、`admin`、`owner`。如果成员服务不可用，后端会合成一个最低权限的 fallback 行用于前端显示；实际 API 授权仍由后端重新计算。

## 邀请链接注册

邀请链接注册由两个公开端点组成。它绕过 `invite_only` 对普通注册的限制，因为 token 本身就是授权凭证。

### 查询邀请上下文

```http
POST /api/v1/auth/invitations/lookup
Content-Type: application/json
```

请求体：

```json
{
  "token": "<invite_token>"
}
```

响应：

```json
{
  "success": true,
  "data": {
    "tenant_id": 1,
    "tenant_name": "研发空间",
    "role": "contributor",
    "expires_at": "2026-06-11T10:00:00Z"
  }
}
```

该接口使用 `POST` 和请求体传 token，避免明文 token 出现在 URL、访问日志、浏览器历史或 tracing 中。token 无效、过期或被撤销时返回 `410`。

### 使用邀请注册

```http
POST /api/v1/auth/register-by-invite
Content-Type: application/json
```

请求体：

```json
{
  "token": "<invite_token>",
  "email": "alice@example.com",
  "username": "alice",
  "password": "secret123"
}
```

成功时返回 `201`，响应体与登录响应类似，包含 `active_tenant`、`memberships`、`token` 和 `refresh_token`。如果邮箱已经存在，会返回 `409`，提示用户先登录后加入空间。

## OIDC 登录

OIDC 登录入口由三个端点配合使用。

### 查询 OIDC 配置

```http
GET /api/v1/auth/oidc/config
```

响应：

```json
{
  "success": true,
  "enabled": true,
  "provider_display_name": "企业 SSO"
}
```

前端根据 `enabled` 决定是否展示第三方登录入口。

### 获取授权地址

```http
GET /api/v1/auth/oidc/url?redirect_uri=https%3A%2F%2Fexample.com%2Flogin
```

`redirect_uri` 必填。成功响应：

```json
{
  "success": true,
  "provider_display_name": "企业 SSO",
  "authorization_url": "https://idp.example.com/authorize?...",
  "state": "..."
}
```

后端会把 `redirect_uri` 编进 `state`，回调时再取出用于完成 code 交换。

### OIDC 回调

```http
GET /api/v1/auth/oidc/callback?code=...&state=...
```

这是 provider 调回后端的地址。成功时后端返回 `302` 到 `/`，并在 URL fragment 中携带 base64url 编码后的 `oidc_result`；失败时携带 `oidc_error` 和可选 `oidc_error_description`。

`oidc_result` 解码后形态与登录响应接近：

```json
{
  "success": true,
  "message": "Login successful",
  "user": {},
  "tenant": {},
  "memberships": [],
  "token": "<access_token>",
  "refresh_token": "<refresh_token>",
  "is_new_user": false
}
```

OIDC 响应里保留字段名 `tenant`，而普通登录使用 `active_tenant`。这是为了兼容现有前端回调处理。

## 令牌刷新、校验与登出

### 刷新令牌

```http
POST /api/v1/auth/refresh
Content-Type: application/json
```

请求体使用驼峰字段：

```json
{
  "refreshToken": "<refresh_token>"
}
```

响应使用下划线字段：

```json
{
  "success": true,
  "message": "Token refreshed successfully",
  "access_token": "<new_access_token>",
  "refresh_token": "<new_refresh_token>"
}
```

刷新成功后旧 refresh token 会被撤销。新 access token 的租户仍由用户偏好解析：如果 `last_active_tenant_id` 有效，会使用该租户；否则回到 home tenant。

### 校验访问令牌

```http
GET /api/v1/auth/validate
Authorization: Bearer <access_token>
```

响应：

```json
{
  "success": true,
  "message": "Token is valid",
  "user": {
    "id": "...",
    "email": "alice@example.com"
  }
}
```

该端点会重新解析 `Authorization` 头，并确认 token 没有被撤销。

### 登出

```http
POST /api/v1/auth/logout
Authorization: Bearer <access_token>
```

响应：

```json
{
  "success": true,
  "message": "Logout successful"
}
```

登出会撤销当前 access token。客户端也应同时清理本地保存的 access token、refresh token 和已选租户。

## 当前用户、偏好和租户切换

### 获取当前用户

```http
GET /api/v1/auth/me
Authorization: Bearer <access_token>
X-Tenant-ID: 2
```

响应：

```json
{
  "success": true,
  "data": {
    "user": {
      "id": "...",
      "email": "alice@example.com",
      "tenant_id": 1,
      "can_access_all_tenants": false,
      "is_system_admin": false,
      "preferences": {
        "enable_memory": true,
        "last_active_tenant_id": 2
      }
    },
    "tenant": {
      "id": 2,
      "name": "研发空间"
    },
    "memberships": []
  }
}
```

这里的 `tenant` 是认证中间件解析出的激活租户，不一定等于用户表上的 home tenant。

### 更新用户偏好

```http
PUT /api/v1/auth/me/preferences
Authorization: Bearer <access_token>
Content-Type: application/json
```

请求体是 PATCH 语义：只覆盖出现的字段。

```json
{
  "enable_memory": true,
  "last_active_tenant_id": 2
}
```

响应：

```json
{
  "success": true,
  "data": {
    "enable_memory": true,
    "last_active_tenant_id": 2
  }
}
```

`last_active_tenant_id` 传正整数表示设置登录后默认回到该租户；传 `0` 表示清除该偏好；字段缺省则保持原值。

### 切换租户

```http
POST /api/v1/auth/switch-tenant
Authorization: Bearer <access_token>
Content-Type: application/json
```

请求体：

```json
{
  "tenant_id": 2,
  "refresh_token": "<current_refresh_token>"
}
```

成功响应为登录响应形态：

```json
{
  "success": true,
  "message": "Tenant switched",
  "active_tenant": {
    "id": 2,
    "name": "研发空间"
  },
  "memberships": [],
  "token": "<new_access_token>",
  "refresh_token": "<new_refresh_token>"
}
```

后端会验证用户在目标租户是否有 active 成员关系；具有跨租户访问权限的用户可以切换到其他租户。若请求体里提供了旧 refresh token，后端会尽力撤销它。

### 修改密码

```http
POST /api/v1/auth/change-password
Authorization: Bearer <access_token>
Content-Type: application/json
```

请求体：

```json
{
  "old_password": "old-secret",
  "new_password": "new-secret"
}
```

`new_password` 至少 6 个字符。成功响应：

```json
{
  "success": true,
  "message": "Password changed successfully"
}
```

## Lite 自动初始化

Lite 版可以调用：

```http
POST /api/v1/auth/auto-setup
```

该端点只在 Lite edition 可用。首次调用会创建默认用户 `admin@weknora.local` 和默认租户，并返回登录响应；后续调用直接为默认用户签发新 token。非 Lite edition 调用会返回 `403`。

## 常见状态码

| 状态码 | 场景 |
| --- | --- |
| `200` | 登录、刷新、校验、OIDC 配置、当前用户、偏好更新、租户切换成功。 |
| `201` | 自助注册或邀请注册成功。 |
| `302` | OIDC callback 跳转回前端。 |
| `400` | 请求体格式错误、必填字段缺失、密码长度不足、租户 ID 格式错误。 |
| `401` | 访问令牌无效、刷新令牌无效、登录失败。 |
| `403` | invite-only 下自助注册、无目标租户成员关系、非 Lite 调用 auto-setup、OIDC 未启用。 |
| `409` | 邀请注册时邮箱已经存在。 |
| `410` | 邀请 token 无效、过期或被撤销。 |
| `429` | 邀请链接公开端点超过 IP 限流。 |

错误通常经统一错误中间件返回：

```json
{
  "success": false,
  "error": {
    "code": 1001,
    "message": "Invalid login parameters",
    "details": "..."
  }
}
```

## 最佳实践

- 浏览器客户端只保存用户登录产生的 Bearer Token，不要把租户 API Key 暴露给浏览器。
- 服务端集成优先使用 `X-API-Key`，并为每个租户单独管理和轮换 API Key。
- 登录后保存 `memberships`，用它渲染租户切换器；但前端展示不能替代后端授权。
- 刷新 token 后立即替换本地 access token 和 refresh token，旧 refresh token 已被撤销。
- 邀请链接 token 不要放在 URL path 或服务端日志中；项目内公开查询接口已经使用 `POST` body 承载 token。
- 需要复现问题时记录 `X-Request-ID`，它会进入响应头和服务端日志。

## 相关文档

- [API 总览](./overview.md)
- [错误处理](./errors.md)
- [知识库 API](./knowledge-base.md)

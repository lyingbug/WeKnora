---
title: 认证集成
description: 将 WeKnora 接入外部身份系统。
---

# 认证集成

WeKnora 支持三类认证方式：

- 用户名和密码登录。
- OIDC 第三方登录。
- 租户 API Key。

浏览器用户通常使用用户名密码或 OIDC。程序化访问、CI、自动化脚本和外部系统集成通常使用租户 API Key。

## 认证中间件

除公开接口外，所有 `/api/v1/*` 请求都会经过统一认证中间件。

中间件按顺序尝试：

1. `Authorization: Bearer <token>` JWT 认证。
2. `X-API-Key: <tenant-api-key>` 租户 API Key 认证。

认证成功后，中间件会把以下信息写入请求上下文：

- 当前租户 ID。
- 当前租户对象。
- 当前用户对象。
- 当前用户 ID。
- 当前租户角色。
- 是否系统管理员。

后续 Handler、Service 和 RBAC 中间件都依赖这些上下文值做租户隔离和权限判断。

## 公开接口

以下接口不需要登录：

| 方法 | 路径 | 用途 |
| --- | --- | --- |
| `GET` | `/health` | 健康检查。 |
| `POST` | `/api/v1/auth/register` | 自助注册。 |
| `POST` | `/api/v1/auth/login` | 用户名密码登录。 |
| `POST` | `/api/v1/auth/auto-setup` | Lite 版自动初始化。 |
| `POST` | `/api/v1/auth/invitations/lookup` | 查询邀请链接。 |
| `POST` | `/api/v1/auth/register-by-invite` | 通过邀请注册。 |
| `GET` | `/api/v1/auth/config` | 获取公开认证配置。 |
| `GET` | `/api/v1/auth/oidc/config` | 获取 OIDC 登录配置。 |
| `GET` | `/api/v1/auth/oidc/url` | 获取 OIDC 授权地址。 |
| `GET` | `/api/v1/auth/oidc/callback` | OIDC 回调。 |
| `POST` | `/api/v1/auth/refresh` | 刷新 Token。 |
| `GET` / `HEAD` | `/api/v1/files/presigned` | 预签名文件访问。 |

邀请相关公开接口带有 IP 维度限流，用于降低枚举和暴力探测风险。

## 用户名密码登录

用户名密码登录使用：

```http
POST /api/v1/auth/login
```

请求体包含：

```json
{
  "email": "user@example.com",
  "password": "secret"
}
```

登录成功后返回：

- 用户信息。
- 当前租户信息。
- 访问令牌 `token`。
- 刷新令牌 `refresh_token`。
- 用户在租户中的 memberships 快照。

前端会把 `token` 和 `refresh_token` 保存到本地，并在页面刷新时调用 `/api/v1/auth/me` 恢复会话。

## 注册模式

公开接口 `/api/v1/auth/config` 会返回当前注册模式：

- `self_serve`：允许自助注册。
- `invite_only`：关闭自助注册，只允许邀请注册。

后端注册接口和前端注册入口使用同一套配置来源，避免出现“前端显示可注册但后端拒绝”或相反的状态。

配置优先级为：

1. 数据库系统设置 `auth.registration_mode`。
2. 启动配置 `auth.registration_mode`。
3. 默认值 `self_serve`。

历史环境变量 `DISABLE_REGISTRATION=true` 会在启动配置阶段等价转换为 `invite_only`。

## OIDC 登录

OIDC 登录适合接入企业身份提供商。

### 前端发现配置

前端先调用：

```http
GET /api/v1/auth/oidc/config
```

如果返回 `enabled=true`，登录页会展示 OIDC 登录入口。`provider_display_name` 用于显示按钮文案。

### 获取授权地址

点击 OIDC 登录后，前端调用：

```http
GET /api/v1/auth/oidc/url?redirect_uri=<callback-url>
```

后端会生成 OIDC 授权 URL 和 state。state 中包含 nonce 和 redirect URI，并使用 base64 URL 编码。

### 回调处理

身份提供商回调：

```http
GET /api/v1/auth/oidc/callback?code=...&state=...
```

后端会：

1. 校验并解码 state。
2. 使用 code 向 OIDC Provider 换取 Token。
3. 获取用户信息。
4. 自动创建或匹配本地用户。
5. 签发 WeKnora 本地访问令牌和刷新令牌。
6. 重定向回前端，并把登录结果放到 URL hash 的 `oidc_result` 中。

如果 Provider 返回错误或回调缺少必要参数，后端会重定向并带上 `oidc_error`。

前端路由守卫会识别 OIDC 回跳 hash，先放行页面挂载，让应用消费登录结果，而不是立即因为“未登录”跳回登录页。

### 配置项

OIDC 配置支持：

- `enable`
- `issuer_url`
- `discovery_url`
- `provider_display_name`
- `client_id`
- `client_secret`
- `authorization_endpoint`
- `token_endpoint`
- `userinfo_endpoint`
- `scopes`
- 用户名字段映射
- 邮箱字段映射

如果只配置 `issuer_url`，系统会默认拼出 `/.well-known/openid-configuration` 作为 discovery URL。默认 scopes 为 `openid profile email`。

## Token 刷新和登出

访问令牌过期后，客户端可以调用：

```http
POST /api/v1/auth/refresh
```

请求体：

```json
{
  "refreshToken": "..."
}
```

成功后返回新的访问令牌和刷新令牌。

登出接口会撤销当前访问令牌：

```http
POST /api/v1/auth/logout
Authorization: Bearer <token>
```

## 当前用户接口

前端刷新页面时会用本地保存的 token 调用：

```http
GET /api/v1/auth/me
Authorization: Bearer <token>
```

该接口返回：

- 当前用户。
- 当前激活租户。
- 用户在各租户中的 memberships。

这里的租户是认证中间件解析出的“当前激活租户”，不是用户注册时的 home tenant。这样在切换租户后刷新页面，前端仍能恢复正确的租户和角色。

## 租户切换

浏览器用户可以通过：

```http
POST /api/v1/auth/switch-tenant
Authorization: Bearer <token>
```

请求体：

```json
{
  "tenant_id": 10000,
  "refresh_token": "..."
}
```

后端会检查用户在目标租户是否有 active membership。通过后，会签发包含目标租户 ID 的新 token。

也可以在普通 JWT 请求中使用 `X-Tenant-ID` 头临时指定目标租户。中间件会校验：

- 目标租户 ID 必须合法。
- 用户必须能访问该租户。
- 目标租户必须存在。
- 当前租户角色必须能解析出来。

跨租户超级用户也必须满足配置开关和访问条件；它不是单纯依靠 header 就能访问任意租户。

## API Key 认证

程序化访问可以使用租户 API Key：

```http
X-API-Key: <tenant-api-key>
```

API Key 认证流程：

1. 从 Key 中解析租户 ID。
2. 查询租户。
3. 使用常量时间比较校验请求 Key 是否等于数据库中的租户 Key。
4. 设置当前租户上下文。
5. 查找该租户关联用户；如果找不到，构造 `system-<tenantID>` 形式的虚拟用户。
6. 固定授予租户 `Admin` 角色。
7. 明确不授予 `SystemAdmin`。

API Key 适合自动化任务，但权限边界与 JWT 不完全相同：

- 可以执行大多数租户级 Admin 操作。
- 不会获得平台级 SystemAdmin 权限。
- Owner-only 操作仍然受限。
- 创建者字段会避免记录虚拟 system 用户为普通资源创建人。

租户 Owner 可以轮换 API Key。相关路由位于租户管理接口下：

```http
POST /api/v1/tenants/:id/api-key
```

## RBAC 角色

WeKnora 的租户角色包括：

| 角色 | 典型能力 |
| --- | --- |
| Viewer | 查看租户内资源。 |
| Contributor | 创建资源，并管理自己创建的 KB、Agent 等资源。 |
| Admin | 管理租户级基础设施和大多数租户资源。 |
| Owner | 管理租户本身、成员、API Key 等最高租户权限。 |

角色检查分两类：

- 角色门槛：例如 Viewer、Contributor、Admin、Owner。
- 所有者或角色门槛：例如“资源创建者或 Admin+”。

租户级基础设施没有个人创建者概念，通常要求 Admin+，包括：

- 模型配置。
- 向量库。
- MCP 服务。
- 网络搜索 Provider。
- IM 通道。
- 数据源。

有创建者的资源通常允许创建者管理，其他人需要 Admin+，包括：

- 知识库。
- Agent。
- 文档、Chunk、Wiki 页面、FAQ 条目等知识库子资源。

`tenant.enable_rbac` 控制租户级 RBAC 是否强制执行。关闭时，中间件会记录本应拒绝的请求但放行；开启后才真正返回 403。SystemAdmin 检查不受该开关影响，始终强制执行。

## 系统管理员

系统管理员用于平台级管理，不等同于租户 Owner。

典型 SystemAdmin 能力包括：

- 访问平台级设置。
- 管理系统管理员。
- 执行跨租户管理接口。
- 查看平台级审计数据。

API Key 认证不会授予 SystemAdmin。平台级管理必须通过交互式 JWT 登录，保留具体用户身份。

## 前端会话恢复

前端路由守卫会：

1. 检查当前路由是否需要认证。
2. 如果 Pinia 中没有登录态，尝试从 `localStorage` 读取 `weknora_token`。
3. 使用 token 调用 `/auth/me` 恢复用户、租户和 memberships。
4. 如果是 Lite 版且未初始化，会尝试 `/auth/auto-setup`。
5. 对需要 SystemAdmin 的页面，前端做 UI 级跳转保护。

服务端仍然是最终权限边界。前端路由守卫只用于用户体验，不能替代后端 RBAC。

## 接入建议

- 企业 SSO 场景优先使用 OIDC。
- 自动化脚本使用 API Key，但不要用它执行平台级管理动作。
- 生产环境建议开启 `tenant.enable_rbac`。
- 对需要跨租户管理的账号，显式配置跨租户访问能力，并保留 SystemAdmin 与租户角色的边界。
- 如果使用 invite-only 注册，确保管理员已配置邀请流程。
- 定期轮换租户 API Key，并限制其保存位置。

## 常见问题

### 为什么使用 API Key 调平台管理接口会失败？

API Key 只代表租户级程序化访问，固定为租户 Admin，但不会设置 SystemAdmin。平台级接口必须使用具有人类身份的 JWT 登录。

### 为什么切换租户后刷新页面还能保持当前租户？

切换租户会重新签发包含目标租户 ID 的 token。刷新时，前端用 `/auth/me` 恢复当前激活租户和 memberships，而不是只读取用户 home tenant。

### 为什么前端隐藏了注册入口但直接调用注册接口也被拒绝？

前端和后端都读取同一个 `auth.registration_mode` 解析结果。`invite_only` 下，后端会拒绝 `/auth/register`，不依赖前端隐藏按钮。

### 为什么 Contributor 不能修改同租户里别人的知识库？

知识库是有创建者的资源。Contributor 可以管理自己创建的资源；如果资源属于别人，需要 Admin+。

### 为什么 RBAC 关闭时请求仍然有权限日志？

关闭强制执行时，中间件会记录“本应拒绝”的请求但放行。这用于灰度启用 RBAC 前观察潜在影响。

## 相关文档

- [API 认证](../api/authentication.md)
- [环境变量](../deployment/environment-variables.md)
- [本地开发](../developer-guide/local-development.md)
- [知识库](../user-guide/knowledge-bases.md)

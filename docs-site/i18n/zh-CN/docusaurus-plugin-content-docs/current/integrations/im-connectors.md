---
title: IM 连接器
description: 将 WeKnora 连接到企业即时通讯平台。
---

# IM 连接器

IM 连接器让用户可以在企业微信、飞书、Slack、Telegram、钉钉、Mattermost 和微信中直接使用 WeKnora Agent。每个 IM 渠道绑定到一个 Agent，收到消息后会复用 WeKnora 的会话、知识库问答、Agent 工具调用和网络搜索能力。

IM 集成不是知识库同步功能。它主要处理聊天消息；只有当渠道配置了“文件知识库”时，用户发送的文件或图片才会被保存到指定知识库并进入导入流水线。

## 支持的平台

当前运行时注册了以下平台适配器：

| 平台 | 支持模式 | 主要凭据 | 说明 |
| --- | --- | --- | --- |
| 企业微信 `wecom` | `websocket`、`webhook` | WebSocket: `bot_id`、`bot_secret`、`ws_endpoint`；Webhook: `corp_id`、`agent_secret`、`token`、`encoding_aes_key`、`corp_agent_id` | WebSocket 是默认模式；Webhook 使用企业微信回调签名。 |
| 飞书 `feishu` | `websocket`、`webhook` | `app_id`、`app_secret`；Webhook 额外使用 `verification_token`、`encrypt_key` | WebSocket 使用长连接事件流；Webhook 需要配置回调 URL。 |
| Slack `slack` | `websocket`、`webhook` | WebSocket: `app_token`、`bot_token`；Webhook: `bot_token`、`signing_secret` | WebSocket 对应 Slack Socket Mode。 |
| Telegram `telegram` | `websocket`、`webhook` | `bot_token`；Webhook 可选 `secret_token` | `websocket` 在实现中是 Bot API long polling。 |
| 钉钉 `dingtalk` | `websocket`、`webhook` | `client_id`、`client_secret`、可选 `card_template_id` | WebSocket 对应钉钉 Stream Mode。 |
| Mattermost `mattermost` | `webhook` | `site_url`、`bot_token`、`outgoing_token`、可选 `bot_user_id`、`post_to_main` | 只支持 Outgoing Webhook + REST API。 |
| 微信 `wechat` | `longpoll` | 扫码后得到 `bot_token`、`ilink_bot_id`、`ilink_user_id` | 使用 iLink Bot 长轮询，固定完整输出。 |

前端创建渠道时会按平台动态显示凭据字段。渠道启用后，后端会创建对应 Adapter；禁用、删除或更新渠道时会停止旧 Adapter 并按新配置重启。

## 配置入口

### 管理界面

IM 渠道在 Agent 编辑器中配置。每个渠道属于当前租户，并绑定到正在编辑的 Agent。

管理员可以：

- 创建渠道。
- 编辑渠道名称、模式、输出模式、会话模式、文件知识库和凭据。
- 启用或停用渠道。
- 删除渠道。
- 对微信渠道发起扫码绑定。

Viewer 可以查看渠道列表，但不能新增、修改、删除或切换启用状态。

用户菜单中还有一个 IM 渠道概览面板，会按租户列出所有 Agent 的 IM 渠道。该概览不会返回凭据，只显示平台、渠道名、Agent、启用状态、模式和 bot identity。

### API

平台回调路由在认证中间件之前注册，因为它们使用各平台自己的签名或 token 校验：

| 方法 | 路径 | 权限 | 用途 |
| --- | --- | --- | --- |
| `GET` / `POST` | `/api/v1/im/callback/:channel_id` | 平台签名校验 | 接收平台回调或 URL 验证。 |

渠道管理路由需要登录态或租户 API Key：

| 方法 | 路径 | 权限 | 用途 |
| --- | --- | --- | --- |
| `POST` | `/api/v1/agents/:id/im-channels` | Admin | 为 Agent 创建 IM 渠道。 |
| `GET` | `/api/v1/agents/:id/im-channels` | Viewer | 列出某个 Agent 的 IM 渠道，包含凭据。 |
| `GET` | `/api/v1/im-channels` | Viewer | 列出当前租户所有 IM 渠道概览，不包含凭据。 |
| `PUT` | `/api/v1/im-channels/:id` | Admin | 更新渠道。 |
| `DELETE` | `/api/v1/im-channels/:id` | Admin | 删除渠道。 |
| `POST` | `/api/v1/im-channels/:id/toggle` | Admin | 启用或停用渠道。 |
| `POST` | `/api/v1/wechat/qrcode` | Admin | 获取微信扫码绑定二维码。 |
| `POST` | `/api/v1/wechat/qrcode/status` | Admin | 轮询微信扫码状态，确认后返回凭据。 |

IM 渠道是租户级基础设施。列表是 Viewer+，所有变更和微信扫码绑定都是 Admin+。

## 渠道字段

`IMChannel` 的核心字段如下：

| 字段 | 说明 |
| --- | --- |
| `tenant_id` | 所属租户。 |
| `agent_id` | 绑定的 Agent。 |
| `platform` | 平台类型。 |
| `name` | 渠道显示名。 |
| `enabled` | 是否启用。 |
| `mode` | `websocket`、`webhook` 或 `longpoll`。 |
| `output_mode` | `stream` 或 `full`。 |
| `knowledge_base_id` | 可选，文件/图片消息保存到的知识库。 |
| `credentials` | 平台凭据 JSON。 |
| `session_mode` | `user` 或 `thread`。 |
| `bot_identity` | 从凭据派生的 bot 唯一身份。 |

`bot_identity` 用来防止同一个外部 bot 同时绑定到多个渠道。后端会从平台和凭据中派生唯一键，例如：

- 企业微信 WebSocket：`wecom:ws:<bot_id>`。
- 企业微信 Webhook：`wecom:wh:<corp_id>:<corp_agent_id>`。
- 飞书：`feishu:<app_id>`。
- Telegram：`telegram:<bot_id>`。
- 钉钉：`dingtalk:<client_id>`。
- Mattermost：`mattermost:wh:<outgoing_token>`。
- 微信：`wechat:<ilink_bot_id>`。

如果另一个未删除渠道已经使用同一 bot，创建或更新会返回冲突。

## 凭据管理

IM 渠道的凭据保存在 `im_channels.credentials` 中，当前实现没有独立的 credentials 子资源。按 Agent 查询渠道时会返回完整凭据，便于编辑器回填；租户级概览接口不会返回凭据。

这与模型、网络搜索和数据源的凭据管理不同。IM 渠道修改凭据时，直接通过 `PUT /im-channels/:id` 更新 `credentials` 字段。

由于 IM 凭据能控制外部 bot，建议只让受信任管理员拥有 IM 渠道写权限，并避免把包含凭据的 per-Agent 渠道响应暴露给无关客户端。

## 消息处理流程

IM 消息进入 WeKnora 后，会被统一转换为 `IncomingMessage`：

- `platform`：平台。
- `message_type`：文本、文件或图片。
- `user_id`、`user_name`：平台用户。
- `chat_id`、`chat_type`：私聊或群聊。
- `message_id`：消息去重键。
- `thread_id`：平台线程 ID。
- `content`：文本内容。
- `file_key`、`file_name`、`file_size`：文件消息信息。
- `quote`：引用或回复的消息上下文。

回调处理会先立即返回 200，避免平台超时，然后在后台异步执行消息处理。

普通文本消息的处理流程如下：

1. 使用 `message_id` 去重，避免平台重试导致重复回答。
2. 限制消息长度，超过 4096 个字符会截断。
3. 按渠道配置检查用户限流和 QA 队列容量。
4. 获取租户、Agent 和 IM 会话映射。
5. 解析并执行斜杠命令；普通消息进入 QA。
6. 创建 WeKnora 用户消息和助手消息。
7. 根据 Agent 模式调用 `KnowledgeQA` 或 `AgentQA`。
8. 将答案清理成适合 IM 平台展示的 Markdown。
9. 通过平台 Adapter 发送流式或完整回复。

IM 回调没有普通 Web 登录用户，后端会注入合成身份 `system-<tenantID>`，并使用 Viewer 角色执行问答。这让共享知识库解析、租户上下文和检索权限仍能按租户工作。

## 会话模式

IM 渠道支持两种会话映射方式：

| 模式 | 映射键 | 适用场景 |
| --- | --- | --- |
| `user` | 平台、用户 ID、聊天 ID、租户、Agent | 默认模式。每个用户在每个群或私聊中维护自己的上下文。 |
| `thread` | 平台、聊天 ID、线程 ID、租户、Agent | 多人在线程中共享一个上下文。 |

线程模式只在前端对 Slack、Mattermost、飞书和 Telegram 开放。若平台没有返回 `thread_id`，后端会降级为 user 模式，避免所有空线程消息共享同一个会话。

如果用户在 Web 界面删除了底层 WeKnora 会话，而 IM 映射还存在，下一次消息会自动清理陈旧映射并创建新会话，避免 bot 永久不可用。

## 输出模式

`output_mode=stream` 时，如果平台 Adapter 实现了 `StreamSender`，WeKnora 会实时推送回答片段。当前多平台都实现了流式接口；如果启动流式失败，会自动回退到完整输出。

`output_mode=full` 时，会等待 QA 完成后一次性发送完整答案。微信渠道固定使用 `longpoll` 和 `full`。

流式输出会每 300ms 批量刷新一次，降低触发平台限流的概率。为了避免在半个 URL 或半个 XML 标记处截断，发送前会保留可能未完成的 `provider://` 文件 URL、Markdown 图片和内部 XML 标签，等下个片段补齐后再发。

Agent 模式下，IM 会把可见工具调用进度写入思考块。内部推理工具如 `thinking`、`todo_write` 不直接展示；知识库检索、关键词搜索、数据库查询、网络搜索、网页阅读等用户可理解的工具会显示为简短状态行。

## 内容清理和图片链接

Web 前端会把引用、知识库来源和图片上下文渲染为富 UI，但 IM 平台只接收普通文本或 Markdown。因此发送前会做清理：

- 移除 `<kb .../>` 和 `<web .../>` 引用标签。
- 把 RAG 上下文中的 `<image>...</image>` XML 块还原为原始 Markdown 图片，或直接移除。
- 把 `local://`、`minio://`、`s3://`、`cos://`、`tos://`、`oss://` 等存储 URL 改写为可外部访问的 HTTP URL。

本地存储要在 IM 平台中显示图片，通常需要配置 `APP_EXTERNAL_URL`。本地文件服务会生成 `/api/v1/files/presigned` 预签名 URL；该路由支持 `GET` 和 `HEAD`，因为部分 IM 平台会先用 `HEAD` 检查图片是否可展示。

如果未配置外部可访问地址，IM 消息中可能保留 `local://` URL，外部平台无法加载图片。

## 斜杠命令

IM 消息以已注册命令开头时，不进入 QA 管线。当前命令包括：

| 命令 | 作用 |
| --- | --- |
| `/help` | 查看可用命令。 |
| `/info` | 查看当前 Agent、知识库、Skills、MCP 和网络搜索配置。 |
| `/search <query>` | 直接检索知识库原文，不经过 AI 总结，最多展示前 5 条。 |
| `/stop` | 中止当前用户正在排队或执行中的回答。 |
| `/clear` | 清空当前 IM 会话映射，下条消息开始新会话。 |

未注册但看起来像命令的 `/xxx` 会返回帮助提示；类似 `/api/v2/users` 这种路径形式不会被当作命令，会进入普通 QA。

`/stop` 会先尝试取消本实例的队列或正在执行的请求；在多实例部署中，还会通过 Redis 和 StreamManager 写入 stop 事件，让其它实例上的同一请求也能被取消。

## 文件和图片消息

如果渠道配置了 `knowledge_base_id`，文件或图片消息会走文件入库路径，而不是进入 QA：

1. 检查 Adapter 是否实现 `FileDownloader`。
2. 校验文件扩展名。
3. 从 IM 平台下载文件。
4. 再次按下载后的真实文件名校验扩展名。
5. 调用 `CreateKnowledgeFromFile` 写入知识库。
6. 异步等待文档解析和摘要完成，并向用户推送处理结果。

支持的文件类型包括：

- PDF
- TXT、Markdown
- Word
- Excel、CSV
- PPT
- PNG、JPG、JPEG、GIF

如果渠道没有配置文件知识库，用户发送无文本的图片或文件时，bot 会提示当前渠道无法处理文件消息，建议配置文件知识库或改用文字描述。

## 多实例和背压

IM 服务支持单实例和 Redis 多实例两种运行方式。

没有 Redis 时：

- 消息去重、限流和队列都使用本地内存。
- 适合 Lite 或单实例部署。

配置 Redis 时：

- 消息去重使用 Redis `SetNX`，跨实例有效。
- WebSocket 和 long-poll 渠道使用 Redis leader lock，避免多个实例同时连接同一个外部 bot。
- `/stop` 使用 Redis marker 和 StreamManager 支持跨实例取消。
- 用户队列计数和全局并发控制可跨实例生效。

默认背压参数如下：

| 参数 | 默认值 | 说明 |
| --- | --- | --- |
| `im.workers` | `5` | 每实例 QA worker 数。 |
| `im.max_queue_size` | `50` | 每实例最大等待队列长度。 |
| `im.max_per_user` | `3` | 单个用户最多排队请求数。 |
| `im.global_max_workers` | `0` | 跨实例最大执行 worker 数，0 表示不限制，需要 Redis。 |
| `im.rate_limit_window` | `60s` | 用户限流窗口。 |
| `im.rate_limit_max` | `10` | 每个限流窗口内单用户最大消息数。 |

普通 QA 消息会进入有界队列；命令不受限流影响，确保用户始终可以使用 `/stop` 或 `/clear` 控制 bot。

## 平台模式选择

优先选择长连接或 WebSocket 模式：

- 不需要公网回调地址。
- 可减少平台回调配置。
- 多实例时通过 leader lock 保证只有一个实例连接。

Webhook 模式适合平台只支持回调、网络拓扑要求平台主动推送，或企业已有统一入口网关的场景。使用 Webhook 时，需要把前端显示的回调地址配置到平台控制台：

```text
https://<your-domain>/api/v1/im/callback/<channel_id>
```

平台回调进入 WeKnora 后，Adapter 会执行对应的签名、token 或 challenge 校验。URL 验证类请求会在 `HandleURLVerification` 中直接响应，不会进入 QA。

## 使用建议

- 一个外部 bot 只绑定一个 IM 渠道，避免重复响应。
- 生产环境优先使用独立 bot，而不是复用个人或测试 bot。
- 若要在 IM 中展示答案图片，确保对象存储 URL 或 `APP_EXTERNAL_URL` 可被 IM 平台访问。
- 群聊里优先使用 user 会话模式；需要多人围绕同一主题协作时，再为支持线程的平台开启 thread 模式。
- 如果 Agent 开启网络搜索或 MCP 工具，IM 用户也会继承这些能力；上线前应确认 Agent 权限和工具审批策略。
- 文件入库会消耗解析、Embedding 和多模态资源，只给确实需要上传文件的渠道配置文件知识库。
- 多实例部署建议配置 Redis，否则每个实例只能做本地去重、限流和队列控制。

## 常见问题

### 为什么回调接口不走 Bearer Token 或 API Key？

IM 平台回调发生在外部平台到 WeKnora 服务端之间，没有普通用户登录态。回调路由在认证中间件之前注册，并由平台 Adapter 负责校验签名、token 或 challenge。

### 为什么同一个 bot 不能绑定多个渠道？

如果同一个外部 bot 同时绑定多个渠道，平台消息可能被多个 Agent 同时处理并重复回复。后端用 `bot_identity` 和唯一索引阻止这种配置。

### 为什么群聊上下文不是全员共享？

默认 `user` 模式按用户和聊天 ID 建会话，避免群里不同用户互相污染上下文。需要共享上下文时，可以在支持线程的平台开启 `thread` 模式。

### 为什么文件发到 IM 后没有回答问题？

文件和图片消息在配置了文件知识库时会进入导入流程，不进入 QA。解析完成后可以再用文字提问。若未配置文件知识库，bot 会提示当前渠道无法处理文件。

### 为什么图片在 IM 中显示失败？

常见原因是答案里包含 `local://` 或对象存储私有地址，外部 IM 平台无法访问。对本地存储，应配置 `APP_EXTERNAL_URL`；对对象存储，应确保生成的预签名 URL 可被平台访问。

### 为什么长连接多实例只启动了一个？

这是预期行为。配置 Redis 时，每个 WebSocket 或 long-poll 渠道会用 Redis leader lock 选出一个实例连接外部平台，其它实例定期重试，leader 失效后自动接管。

## 相关文档

- [Agent 模式](../user-guide/agent-mode.md)
- [MCP 工具](../user-guide/mcp-tools.md)
- [知识库](../user-guide/knowledge-bases.md)
- [导入流水线](../architecture/ingestion-pipeline.md)
- [认证与权限](./authentication.md)

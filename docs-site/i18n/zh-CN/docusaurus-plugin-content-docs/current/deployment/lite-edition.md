---
title: Lite 版本
description: 理解何时使用 WeKnora Lite。
---

# Lite 版本

WeKnora Lite 面向本地单机、演示评估和小规模个人知识库场景。它把标准版中的多服务依赖尽量收敛到一个本地进程：主数据库使用 SQLite，检索使用 SQLite FTS5 与 sqlite-vec，流式事件使用内存模式，文件默认保存在本地目录。

如果你需要多人协作、横向扩容、托管数据库、集中队列、完整可观测性和更复杂的服务组合，应使用标准部署。

## 适用场景

Lite 更适合：

- 简化本地体验。
- 演示或评估。
- 减少外部服务依赖。
- 个人或小团队在一台机器上维护知识库。
- 离线或弱联网环境中使用本地模型。

不适合：

- 多团队协作和复杂组织权限。
- 多实例高可用部署。
- 大规模文档解析和高并发问答。
- 依赖 Redis、PostgreSQL、对象存储、集中日志平台的生产架构。

## 两种形态

Lite 有两种常见交付形态：

| 形态 | 说明 |
| --- | --- |
| 桌面应用 | Wails 桌面壳启动内嵌 Go 后端，并通过反向代理加载前端。macOS `.app` 会把数据目录重定向到用户的 Application Support 目录。 |
| 单二进制 Web 版 | `WeKnora-lite` 命令行程序内嵌前端静态文件，启动后通过浏览器访问本机端口。 |

发布流程中，单二进制版本使用 `sqlite_fts5` 构建标签编译；桌面版本额外使用桌面相关构建标签并打包前端资源、SQLite 迁移和 `.env.lite.example`。

## 默认配置

Lite 使用 `.env.lite`，可以从模板复制：

```bash
cp .env.lite.example .env.lite
```

核心配置如下：

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `GIN_MODE` | `debug` | Lite 默认保留调试体验。 |
| `LOG_LEVEL` | `debug` | 日志级别。 |
| `DB_DRIVER` | `sqlite` | 主数据库使用 SQLite。 |
| `DB_PATH` | `./data/weknora.db` | SQLite 文件路径。 |
| `RETRIEVE_DRIVER` | `sqlite` | 检索引擎使用 SQLite FTS5 与 sqlite-vec。 |
| `STORAGE_TYPE` | `local` | 文件存储使用本地目录。 |
| `LOCAL_STORAGE_BASE_DIR` | `./data/files` | 上传文件保存目录。 |
| `STREAM_MANAGER_TYPE` | `memory` | 不依赖 Redis。 |
| `OLLAMA_BASE_URL` | `http://127.0.0.1:11434` | 默认连接本机 Ollama。 |
| `TENANT_AES_KEY` | 示例值 | 租户密钥，长期使用时应修改并固定。 |
| `JWT_SECRET` | 示例值 | JWT 签名密钥，长期使用时应修改。 |
| `NEO4J_ENABLE` | `false` | 默认关闭图数据库。 |
| `WEKNORA_SANDBOX_MODE` | `disabled` | 默认关闭 Agent Skills 沙箱。 |
| `ENABLE_GRAPH_RAG` | `false` | 默认关闭 GraphRAG。 |
| `CONCURRENCY_POOL_SIZE` | `3` | 并发处理池大小。 |
| `DOCREADER_ADDR` | `127.0.0.1:50051` | DocReader 地址。 |
| `DOCREADER_TRANSPORT` | `grpc` | DocReader 传输方式。 |

桌面应用在 macOS `.app` 中运行时，会把相对路径重定向到：

```text
~/Library/Application Support/WeKnora Lite/data/
```

因此 `.env.lite` 中的 `./data/weknora.db` 和 `./data/files` 不会写入应用包内部，而是写入用户数据目录。

## 启动单二进制版本

解压发布包后：

```bash
cp .env.lite.example .env.lite
```

编辑 `.env.lite`，至少确认：

```bash
OLLAMA_BASE_URL=http://127.0.0.1:11434
JWT_SECRET=CHANGE-ME-jwt-secret
TENANT_AES_KEY=CHANGE-ME-32-char-secret-key!!!!
```

如果使用 Ollama，可以先启动模型服务并拉取模型：

```bash
ollama serve
ollama pull qwen2.5:7b
ollama pull nomic-embed-text
```

加载环境变量并启动：

```bash
set -a
source .env.lite
set +a
./WeKnora-lite
```

默认访问地址由 `config/config.yaml` 中的服务配置决定，常见为：

```text
http://localhost:8080
```

如果 `GIN_MODE` 不是 `release`，Swagger 路由也会启用。

## 桌面应用行为

桌面应用启动时会：

- 切换工作目录到应用包的 `Resources`，确保能找到 `config/config.yaml`、迁移文件和前端资源。
- 显式加载 `.env`，使 `DB_DRIVER`、`DB_PATH`、`LOCAL_STORAGE_BASE_DIR` 等变量生效。
- 在后台启动一个本地 Gin 服务。
- 使用 Wails 的 AssetServer 反向代理到本地后端。
- 在前端注入真实 API 根地址 `window.__WEKNORA_API_BASE__`。
- 默认只监听 `127.0.0.1`，可通过桌面偏好设置允许局域网访问。

当开启局域网监听时，桌面应用会额外注入局域网 API 地址，方便同网段设备访问。但这会扩大攻击面，只有在可信网络中才建议开启。

## 自动初始化和登录

Lite 版专用接口：

```http
POST /api/v1/auth/auto-setup
```

只有编译为 Lite 版本时该接口才可用。非 Lite 版本调用会返回 403。

首次调用时，后端会创建默认用户和租户：

| 字段 | 行为 |
| --- | --- |
| 默认邮箱 | `admin@weknora.local` |
| 用户名 | 随机生成。 |
| 密码 | 随机生成，不需要用户输入。 |
| 租户角色 | 默认用户拥有自己的租户，角色为 Owner。 |
| 返回内容 | 访问令牌、刷新令牌、用户、当前租户和成员关系。 |

后续调用不会重复创建用户，只会重新签发 token。前端路由会在 Lite 模式下自动尝试该接口，并把 `weknora_lite_mode` 写入本地存储。

## 任务执行差异

标准版使用 Redis 和 Asynq 执行异步任务。Lite 中如果没有配置 `REDIS_ADDR`，容器会改用同步任务执行器：

- 上传解析、FAQ 导入、问题生成、摘要生成、知识库复制、知识迁移、批量删除、数据源同步、Wiki 生成等任务会注册到本地执行器。
- 任务不是进入 Redis 队列，而是在本进程中以 goroutine 执行。
- 支持延迟执行和重试，重试退避最长 30 秒。
- 没有 Asynq Inspector，取消排队任务只能依赖任务自身的检查点。

这意味着 Lite 更简单，但任务可靠性依赖当前进程。进程退出时，内存中的待执行任务不会像 Redis 队列那样持久保留。

## 数据和备份

Lite 的关键数据主要在两个位置：

| 数据 | 默认位置 |
| --- | --- |
| SQLite 数据库 | `DB_PATH`，默认 `./data/weknora.db`。 |
| 上传文件 | `LOCAL_STORAGE_BASE_DIR`，默认 `./data/files`。 |
| 日志 | `LOG_PATH`；macOS 桌面未配置时写入系统日志目录。 |

备份时至少复制：

```text
data/weknora.db
data/files/
```

如果 SQLite 正在运行，建议先停止应用再备份，或使用 SQLite 在线备份工具，避免拷贝到不一致的 WAL 状态。

长期使用时不要随意更换 `TENANT_AES_KEY` 和 `JWT_SECRET`。如果后续补充了 `SYSTEM_AES_KEY` 等加密变量，也应固定并备份。

## 文档解析

Lite 默认仍需要 DocReader：

```bash
DOCREADER_ADDR=127.0.0.1:50051
DOCREADER_TRANSPORT=grpc
```

发布包通常会携带 Lite 所需的 SQLite 迁移和前端资源，但 DocReader 是否内置取决于具体发行形态。若解析文件时报连接失败，应检查：

- DocReader 进程是否运行。
- `DOCREADER_ADDR` 是否指向正确地址。
- 防火墙是否阻止本机端口。
- 如启用 `GRPC_AUTH_TOKEN`，客户端和服务端是否一致。
- 如启用 TLS，证书路径和 `GRPC_TLS_SERVER_NAME` 是否正确。

## 模型配置

Lite 默认指向本机 Ollama：

```bash
OLLAMA_BASE_URL=http://127.0.0.1:11434
```

也可以使用 OpenAI 兼容服务：

```bash
OPENAI_API_KEY=sk-xxx
OPENAI_BASE_URL=https://api.openai.com/v1
```

实际可用模型仍需要在界面中配置，或通过内置模型配置文件声明。使用本机模型时，应确保聊天模型和 Embedding 模型都已拉取并可访问。

## 功能差异

| 维度 | Lite | 标准版 |
| --- | --- | --- |
| 部署依赖 | 单进程为主，无 PostgreSQL、Redis 等外部服务依赖。 | 多服务部署，通常包含 PostgreSQL、Redis、DocReader、可选向量库和对象存储。 |
| 数据库 | SQLite。 | PostgreSQL 或其他受支持主库。 |
| 默认检索 | SQLite FTS5 + sqlite-vec。 | PostgreSQL、Qdrant、Milvus、Elasticsearch、Weaviate、Doris 等。 |
| 流式事件 | 内存模式。 | Redis 模式，适合多实例。 |
| 异步任务 | 本进程 goroutine 执行。 | Asynq + Redis 队列。 |
| 组织协作 | 简化，隐藏部分组织入口。 | 完整租户、组织、成员、共享能力。 |
| 沙箱 | 默认关闭。 | 可使用 Docker 沙箱。 |
| 网络暴露 | 默认本机访问。 | 通常通过 Nginx、Ingress 或网关对外服务。 |

## 迁移到标准版

Lite 和标准版的存储后端不同，不能简单把 SQLite 文件直接挂到标准版容器中使用。迁移前应先明确迁移范围：

- 知识库元数据和用户数据。
- 上传文件目录。
- 模型配置和密钥。
- 已生成的向量索引。
- 会话历史。

实践上更稳妥的方式是：

1. 在标准版中部署新的 PostgreSQL、Redis、对象存储和检索引擎。
2. 导出或重新上传 Lite 中的重要文档。
3. 在标准版中重新解析并构建索引。
4. 重新配置模型、Web Search、MCP 和 IM 渠道。

如果必须迁移数据库级数据，需要编写专门迁移脚本，并处理 SQLite 与 PostgreSQL 在 JSON、时间、向量字段和外键上的差异。

## 常见问题

### 启动后无法登录

确认当前二进制确实是 Lite 版本。`/api/v1/auth/auto-setup` 只有 Lite 编译版本可用，标准版会返回 403。

### 上传后一直解析中

检查 DocReader 是否可达，以及 `DOCREADER_ADDR` 是否配置为本机实际地址。Lite 没有 Redis 队列，解析任务在本进程中执行，进程退出会中断正在执行的任务。

### 本机模型不可用

检查 `OLLAMA_BASE_URL` 是否正确，模型是否已经拉取，Ollama 服务是否监听在该地址。容器部署常用 `host.docker.internal`，Lite 本机进程通常使用 `127.0.0.1`。

### 想让局域网设备访问

桌面版默认使用回环地址。只有明确需要时才开启公开绑定，并确保防火墙和系统网络权限允许访问。对外开放前应修改默认密钥，并评估认证和数据暴露风险。

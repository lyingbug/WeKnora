---
title: 项目结构
description: 理解 WeKnora 仓库中的主要目录。
---

# 项目结构

WeKnora 是一个 Go 后端、Vue 前端、Python DocReader、Go CLI 和 Helm/Docker 部署配置共存的单仓库。理解目录边界有助于定位功能、加接口、改前端或排查部署问题。

## 顶层目录

| 目录 | 说明 |
| --- | --- |
| `cmd/server/` | 标准后端服务入口。负责设置 Gin 模式、打印启动环境、构建依赖注入容器并启动 HTTP Server。 |
| `cmd/desktop/` | WeKnora Lite 桌面入口。启动本地后端、加载桌面配置、通过 Wails 反向代理前端。 |
| `cmd/download/` | 下载或辅助命令入口。 |
| `internal/` | 后端主体代码，包含路由、handler、service、repository、模型、检索、MCP、IM、数据源等。 |
| `frontend/` | Web UI，基于 Vue 和 TypeScript。 |
| `docreader/` | 文档解析服务，提供 gRPC/HTTP 解析能力。 |
| `mcp-server/` | 独立 MCP Server，用于把 WeKnora 能力暴露给 MCP 客户端。 |
| `cli/` | WeKnora CLI 命令实现。 |
| `client/` | Go SDK，封装 REST API 调用。 |
| `config/` | 后端配置、Prompt 模板、内置智能体和类型预设。 |
| `migrations/` | 数据库迁移，按 PostgreSQL、MySQL、SQLite 和通用版本化迁移组织。 |
| `docker/` | App、DocReader、Sandbox、OpenDataLoader hybrid 等镜像定义和辅助配置。 |
| `docker-compose.yml` | 标准本地/单机多服务部署编排。 |
| `docker-compose.dev.yml` | 开发环境依赖服务编排。 |
| `helm/` | Kubernetes Helm Chart。 |
| `docs-site/` | Docusaurus 文档站源码。 |
| `docs/` | 设计文档、历史开发文档、API 生成文件和说明。 |
| `skills/` | 预加载 Agent Skills。 |
| `examples/` | 示例技能和示例资源。 |
| `dataset/` | 示例数据集。 |
| `deploy/` | 部署辅助文件，例如 Lite systemd service。 |
| `chrome-extension/` | 浏览器扩展相关代码。 |
| `miniprogram/` | 小程序相关代码。 |

## 后端分层

后端核心在 `internal/` 下。主要分层如下：

| 目录 | 职责 |
| --- | --- |
| `internal/router/` | Gin 路由注册、文件代理、异步任务路由、RBAC 路由守卫。 |
| `internal/middleware/` | 请求 ID、语言、日志、认证、RBAC、恢复和错误处理中间件。 |
| `internal/handler/` | HTTP handler，负责绑定参数、鉴权上下文、调用 service、转换响应。 |
| `internal/application/service/` | 业务服务层，处理知识库、知识解析、聊天、模型、组织、MCP、数据源、审计等业务逻辑。 |
| `internal/application/repository/` | 数据访问层，包含主库 repository 和各类检索引擎 repository。 |
| `internal/types/` | 跨层共享的数据结构、接口常量和枚举。 |
| `internal/types/interfaces/` | 服务和仓储接口定义，用于依赖注入和解耦。 |
| `internal/container/` | `dig` 依赖注入容器，集中注册配置、数据库、外部客户端、repository、service、handler 和 router。 |
| `internal/config/` | 配置结构、配置加载、Prompt 模板加载、环境变量覆盖和配置校验。 |
| `internal/database/` | 数据库迁移执行。 |
| `internal/models/` | Chat、Embedding、Rerank、VLM、ASR 等模型适配。 |
| `internal/infrastructure/` | DocParser、Web Search、Chunker 等基础设施实现。 |
| `internal/agent/` | Agent 引擎、工具、MCP 工具封装、人工审批和记忆能力。 |
| `internal/mcp/` | MCP 客户端管理和连接复用。 |
| `internal/im/` | IM 平台适配器和通道服务。 |
| `internal/datasource/` | 数据源同步框架和 Feishu、Notion、语雀等连接器。 |
| `internal/stream/` | 流式事件管理，支持 memory 和 Redis。 |
| `internal/tracing/` | Langfuse 追踪集成。 |
| `internal/logger/` | 日志、LLM 调试日志和日志格式。 |
| `internal/errors/` | 统一应用错误码和解析阶段错误码。 |
| `internal/utils/` | 文件大小、安全校验、SSRF、加密、注入检测等工具函数。 |

## 请求链路

一个典型 API 请求的后端路径是：

```text
cmd/server/main.go
  -> container.BuildContainer
  -> router.NewRouter
  -> middleware.RequestID / Logger / Auth / RBAC
  -> handler
  -> application service
  -> repository 或外部客户端
  -> response / error middleware
```

关键点：

- `cmd/server/main.go` 只负责进程生命周期，不直接组装业务对象。
- `internal/container/container.go` 是后端依赖图的中心，新增 service、repository、handler 时通常需要在这里注册。
- `internal/router/router.go` 负责路由和角色守卫，新增 API 时应先看目标模块已有 route group。
- handler 应保持薄层：参数解析、上下文提取、错误映射和 DTO 组装。
- 业务规则优先放在 service，数据库细节放在 repository。

## 依赖注入

项目使用 `go.uber.org/dig`。`BuildContainer` 大致按以下顺序注册：

1. 配置、Langfuse、数据库、文件服务、Redis、并发池等基础设施。
2. 检索引擎注册表。
3. DocReader、Ollama、Neo4j、StreamManager、DuckDB 等外部客户端。
4. Repository。
5. Service。
6. Extract、后处理、Wiki、MCP、Agent、IM 和数据源服务。
7. Handler。
8. Router 和异步任务执行器。

当一个构造函数依赖多个对象时，通常使用 `dig.In` 参数结构，例如 router 的 `RouterParams`。如果需要给同一接口注入不同实现，会使用 `dig.Name` 或 `dig.As`。

## 路由组织

`internal/router/router.go` 的 `NewRouter` 做了基础注册：

- CORS。
- `RequestID`、`Language`、`Logger`、`Recovery`、`ErrorHandler`。
- `/health`。
- 非 release 模式下的 Swagger。
- Lite 版内嵌前端静态文件。
- IM 回调路由。
- 认证中间件。
- 文件代理和 presigned 文件访问。
- Langfuse Gin 中间件。
- 业务路由组。

业务路由大多挂在 `/api/v1` 下，并通过 route group 叠加角色守卫。常见角色包括 Viewer、Editor、Admin、Owner 和 SystemAdmin。

## 异步任务

标准版使用 Redis 和 Asynq：

- `internal/router/task.go` 注册 Asynq Server 和任务 handler。
- 文档解析、FAQ 导入、知识库复制、知识迁移、批量删除、问题生成、摘要生成、数据源同步和 Wiki 生成都会走任务队列。
- Asynq 中间件会接入 Langfuse 和 dead-letter 处理。

Lite 模式如果没有 `REDIS_ADDR`，容器会使用 `internal/router/sync_task.go` 中的同步任务执行器：

- 任务直接在本进程 goroutine 中执行。
- 支持延迟和重试。
- 没有 Redis 队列和 Asynq Inspector。

## 数据库和迁移

数据库初始化在 `internal/container/initDatabase` 中完成：

| 模式 | 说明 |
| --- | --- |
| `DB_DRIVER=postgres` | 使用 PostgreSQL/ParadeDB，迁移 DSN 会根据 `RETRIEVE_DRIVER` 决定是否跳过 embedding 迁移。 |
| `DB_DRIVER=sqlite` | Lite 使用 SQLite，启用 WAL、busy timeout、foreign keys，并加载 sqlite-vec。 |

迁移文件分布：

| 目录 | 说明 |
| --- | --- |
| `migrations/versioned/` | 版本化迁移主目录。 |
| `migrations/paradedb/` | PostgreSQL/ParadeDB 相关迁移。 |
| `migrations/sqlite/` | SQLite/Lite 迁移。 |
| `migrations/mysql/` | MySQL 相关迁移。 |

默认启动时会自动迁移。设置 `AUTO_MIGRATE=false` 可关闭自动迁移。

## 检索引擎

检索引擎注册在容器初始化阶段完成，读取 `RETRIEVE_DRIVER`。支持的环境驱动包括：

- `postgres`
- `sqlite`
- `elasticsearch_v7`
- `elasticsearch_v8`
- `opensearch`
- `qdrant`
- `milvus`
- `weaviate`
- `doris`
- `tencent_vectordb`

相关代码主要位于：

- `internal/application/service/retriever/`
- `internal/application/repository/retriever/`
- `internal/types/vectorstore.go`
- `internal/handler/vectorstore.go`

PostgreSQL 和 SQLite 是环境驱动的内置检索引擎，不作为用户通过 API 新增的外部向量库类型。

## 前端结构

`frontend/` 是 Web UI：

| 目录 | 说明 |
| --- | --- |
| `frontend/src/api/` | API 调用封装，通常每个业务模块一个目录或文件。 |
| `frontend/src/stores/` | Pinia 状态管理。 |
| `frontend/src/router/` | 路由和鉴权跳转逻辑，包括 Lite 自动初始化。 |
| `frontend/src/components/` | 通用组件。 |
| `frontend/src/pages/` 或业务目录 | 页面级实现。 |
| `frontend/src/i18n/` | 多语言文案。 |
| `frontend/public/` | 静态资源。 |

前端请求统一经过 `frontend/src/utils/request.ts`，会注入 Bearer Token、`Accept-Language`、`X-Tenant-ID` 和 `X-Request-ID`，并把统一错误响应整理成前端更易用的 rejected 对象。

## DocReader

`docreader/` 是独立 Python 服务，承担文档解析。关键目录包括：

| 目录 | 说明 |
| --- | --- |
| `docreader/proto/` | gRPC 协议定义。 |
| `docreader/parser/` | 解析器实现。 |
| `docreader/splitter/` | 文档拆分辅助。 |
| `docreader/client/` | 客户端或调用辅助。 |
| `docreader/tests/` | DocReader 测试。 |

Go App 通过 `internal/infrastructure/docparser` 连接 DocReader，并根据 `DOCREADER_ADDR` 和 `DOCREADER_TRANSPORT` 选择 gRPC 或 HTTP。

## CLI 和 Go Client

`client/` 是 Go SDK，封装 REST API 请求、认证、知识库、聊天、模型、MCP 等对象。

`cli/` 基于 Go Client 构建命令行能力：

| 目录 | 说明 |
| --- | --- |
| `cli/cmd/` | 用户可见命令。 |
| `cli/internal/` | CLI 内部工具和共享逻辑。 |
| `cli/acceptance/` | CLI 接受测试。 |
| `cli/skills/` | CLI 相关技能或扩展资源。 |

如果修改 API 响应结构，应同时检查 `client/` 和 `cli/` 是否需要同步更新。

## MCP Server

`mcp-server/` 是独立 MCP Server，和后端内置 MCP 服务管理不同：

- 后端内置 MCP 管理在 `internal/mcp`、`internal/handler/mcp_service.go`、`internal/application/service/mcp_service.go`。
- 独立 `mcp-server/` 通过 WeKnora API Key 调用 WeKnora REST API，再向 MCP 客户端暴露工具。

## 配置文件

`config/` 中的主要文件：

| 文件或目录 | 说明 |
| --- | --- |
| `config/config.yaml` | 后端主配置。 |
| `config/prompt_templates/` | Prompt 模板目录。 |
| `config/builtin_agents.yaml` | 内置智能体定义。 |
| `config/agent_type_presets.yaml` | 智能体类型预设。 |
| `config/builtin_models.yaml.example` | 内置模型声明示例，支持 `${ENV}` 占位符。 |

配置加载由 `internal/config/config.go` 完成。它会读取 `config.yaml`，替换 `${ENV}`，加载 Prompt 模板和内置 Agent，并应用 OIDC、Agent 超时、知识解析超时、RBAC、审计保留等环境变量覆盖。

## 文档站

`docs-site/` 是 Docusaurus 文档站。当前中文文档位于：

```text
docs-site/i18n/zh-CN/docusaurus-plugin-content-docs/current/
```

英文源文档位于：

```text
docs-site/docs/
```

构建命令：

```bash
cd docs-site
npm run build
```

## 修改建议

- 新增后端 API：先找相邻 handler 和 route group，再补 service、repository、DTO、前端 API 封装和文档。
- 新增业务能力：优先放在 `internal/application/service/`，不要把复杂规则堆在 handler。
- 新增持久化字段：同步更新 `internal/types/`、repository、迁移、API 响应和前端类型。
- 新增检索引擎：检查 `internal/application/repository/retriever/`、retriever registry、vector store type 和 SSRF 校验。
- 新增模型供应商：检查 `internal/models/`、模型配置 UI、凭证存储和测试接口。
- 修改异步解析流程：同时检查任务 payload、Asynq 注册、Lite 同步任务注册、span tracking 和 dead-letter 行为。

---
title: 环境变量
description: 通过环境变量配置 WeKnora 服务。
---

# 环境变量

WeKnora 的运行配置由 `config/config.yaml` 和环境变量共同决定。容器化部署通常从 `.env.example` 复制 `.env`，Docker Compose 会把 `.env` 注入 App、Frontend、DocReader 等容器；Helm 部署则通过 `values.yaml` 的 `app.env`、`app.extraEnv` 和 `secrets` 写入 Pod 环境变量。

从示例文件开始：

```bash
cp .env.example .env
```

轻量版使用独立模板：

```bash
cp .env.lite.example .env.lite
```

## 加载规则

后端启动时会读取 `config.yaml`，并支持两类环境变量：

| 机制 | 行为 |
| --- | --- |
| `${NAME}` 占位符 | `config.yaml` 或内置模型 YAML 中出现 `${NAME}` 时，启动期会用同名环境变量替换。 |
| 专用覆盖变量 | 少数字段会在读取 YAML 后再由环境变量覆盖，例如 OIDC、RBAC、Agent 超时和解析超时。 |

同时，很多运行时组件直接读取环境变量，例如数据库连接、对象存储、向量库、Redis、日志和 Langfuse。生产环境应把密钥放在平台 Secret 中，不要提交到仓库。

## 基础运行变量

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `WEKNORA_VERSION` | `latest` | Docker 镜像标签。常用 `latest` 或 `main`。 |
| `GIN_MODE` | `release` | Gin 运行模式。生产使用 `release`，开发可用 `debug`。 |
| `TZ` | `Asia/Shanghai` | 容器时区，影响日志和时间展示。 |
| `WEKNORA_LANGUAGE` | `zh-CN` | 系统默认语言，也是部分 Prompt 中语言占位符的回退值。 |
| `APP_PORT` | `8080` | App 映射到宿主机的端口。 |
| `FRONTEND_PORT` | `80` | 前端 Nginx 映射端口。 |
| `APP_HOST` | `app` | 前端 Nginx 代理到的后端主机名。 |
| `APP_BACKEND_PORT` | `8080` | 前端 Nginx 代理到的后端服务端口。 |
| `APP_SCHEME` | `http` | 前端 Nginx 代理后端时使用的协议。 |
| `APP_EXTERNAL_URL` | 空 | 对外可访问地址，local 存储在 IM 渠道中生成图片或文件链接时会用到。 |
| `MAX_FILE_SIZE_MB` | `50` | 上传文件大小限制，前端和后端都会读取。 |
| `WEKNORA_WEB_DIR` | 空 | 指定后端内置静态前端目录。常规容器部署不需要设置。 |

如果前端和后端不在同一 Compose 网络中，通常需要同时设置 `APP_HOST`、`APP_BACKEND_PORT` 和 `APP_SCHEME`。

## 日志

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `LOG_LEVEL` | `debug` | 日志级别，可选 `debug`、`info`、`warn`、`error`、`fatal`。 |
| `LOG_PATH` | 空 | 日志文件路径。未设置时通常输出到标准输出或平台默认日志位置。 |
| `LOG_FORMAT` | 空 | 日志格式控制。 |
| `LLM_DEBUG_LOG` | 空 | 写入大模型调用的完整请求和响应。`true` 表示写到日志目录下的 `llm_debug.log`，也可以直接填文件路径。 |
| `WEKNORA_LLM_STREAM_RAW_DUMP` | 空 | 开启流式模型原始响应 dump。 |
| `WEKNORA_LLM_STREAM_RAW_DUMP_DIR` | 空 | 指定流式模型原始响应 dump 目录。 |

`LLM_DEBUG_LOG` 和流式 dump 可能包含用户问题、检索上下文和模型输出，生产环境只建议短期开启并控制文件权限。

## 数据库

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `DB_DRIVER` | `postgres` | 主数据库类型。完整容器部署使用 `postgres`，轻量版使用 `sqlite`。 |
| `DB_HOST` | `localhost` | 数据库主机。Compose 中 App 容器会使用 `postgres`。 |
| `DB_PORT` | `5432` | 数据库端口。 |
| `DB_USER` | `postgres` | 数据库用户名。 |
| `DB_PASSWORD` | `postgres123!@#` | 数据库密码，生产必须修改。 |
| `DB_NAME` | `WeKnora` | 数据库名称。 |
| `DB_PATH` | `./data/weknora.db` | SQLite 数据库文件路径，仅 `DB_DRIVER=sqlite` 时使用。 |
| `AUTO_MIGRATE` | 非 `false` | 是否启动时执行迁移。设为 `false` 可关闭自动迁移。 |
| `AUTO_RECOVER_DIRTY` | `true` | 自动恢复脏迁移状态。 |

Helm 部署中，`DB_USER`、`DB_PASSWORD`、`DB_NAME` 来自 `secrets` 或 `existingSecret`。

## Redis 和异步任务

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `STREAM_MANAGER_TYPE` | `redis` | 流式事件存储后端，可选 `memory` 或 `redis`。 |
| `REDIS_ADDR` | `redis:6379` | Redis 地址。 |
| `REDIS_USERNAME` | 空 | Redis ACL 用户名。 |
| `REDIS_PASSWORD` | 空 | Redis 密码。Compose 示例默认要求密码。 |
| `REDIS_DB` | `0` | Redis 数据库编号。 |
| `REDIS_PREFIX` | `stream:` | Redis key 前缀。 |
| `WEKNORA_REDIS_OP_TIMEOUT_MS` | 内置默认 | Redis 操作超时，主要用于任务检查接口。 |
| `WEKNORA_REDIS_NAMESPACE` | 空 | MCP 工具审批 Redis Pub/Sub 命名空间，多实例或多环境共用 Redis 时可设置。 |
| `WEKNORA_ASYNQ_CONCURRENCY` | `32` | Asynq worker 并发数。 |

轻量版默认 `STREAM_MANAGER_TYPE=memory`，不依赖 Redis。多实例部署应使用 Redis，否则流式会话和审批等待只能保存在单实例内存中。

## 安全和认证

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `JWT_SECRET` | 空 | JWT 签名密钥，生产必须设置。 |
| `TENANT_AES_KEY` | 示例值 | 租户 API Key 等字段的加密密钥。 |
| `SYSTEM_AES_KEY` | 示例值 | 模型密钥、向量库凭证、Web Search Key、MCP 凭证等字段的 AES-256 加密密钥，必须为 32 字节。 |
| `CRYPTO_MASTER_KEY` | 空 | AppSecret 等敏感字段加密主密钥；未设置时会自动生成并持久化。 |
| `CRYPTO_SALT` | 空 | 加密盐值；未设置时会自动生成并持久化。 |
| `DISABLE_REGISTRATION` | `false` | 设为 `true` 时注册模式会被强制为邀请制。 |
| `WEKNORA_TENANT_ENABLE_RBAC` | `true` | 是否启用租户角色强制鉴权。 |
| `WEKNORA_TENANT_MAX_OWNED_PER_USER` | 内置默认 | 单个非超管用户可自助创建的租户数量上限。小于 0 表示关闭限制。 |
| `WEKNORA_AUDIT_RETENTION_DAYS` | `90` | 审计日志保留天数。`0` 表示不自动清理。 |
| `WEKNORA_BOOTSTRAP_SYSTEM_ADMIN_EMAIL` | 空 | 当系统还没有系统管理员时，把已注册的指定邮箱用户提升为系统管理员。 |
| `WEKNORA_INVITATION_TTL` | 内置默认 | 组织邀请有效期，使用 Go duration 格式。 |
| `FRONTEND_BASE_URL` | 空 | 生成邀请注册链接时使用的前端外部地址。 |

生产环境最重要的是固定并备份 `TENANT_AES_KEY` 和 `SYSTEM_AES_KEY`。如果密钥丢失或被替换，数据库中已加密字段将无法解密。Helm 的 `secrets.tenantAesKey` 和 `secrets.systemAesKey` 如果留空会尝试生成随机值，但生产仍建议显式设置并纳入密钥备份策略。

## OIDC 登录

OIDC 可通过 YAML 配置，也可用下列环境变量覆盖：

| 变量 | 说明 |
| --- | --- |
| `OIDC_AUTH_ENABLE` | 是否启用 OIDC，`true` 或 `false`。 |
| `OIDC_AUTH_ISSUER_URL` | Issuer URL。未显式设置 Discovery URL 时，会用它拼出 `/.well-known/openid-configuration`。 |
| `OIDC_AUTH_DISCOVERY_URL` | OIDC Discovery URL。 |
| `OIDC_AUTH_PROVIDER_DISPLAY_NAME` | 登录页展示的供应商名称，默认 `OIDC`。 |
| `OIDC_AUTH_CLIENT_ID` | 客户端 ID。 |
| `OIDC_AUTH_CLIENT_SECRET` | 客户端密钥。 |
| `OIDC_AUTH_AUTHORIZATION_ENDPOINT` | 授权端点。没有 Discovery URL 时需要配置。 |
| `OIDC_AUTH_TOKEN_ENDPOINT` | Token 端点。没有 Discovery URL 时需要配置。 |
| `OIDC_AUTH_USER_INFO_ENDPOINT` | UserInfo 端点。 |
| `OIDC_AUTH_SCOPES` | Scope 列表，可用逗号或空格分隔。默认 `openid profile email`。 |
| `OIDC_USER_INFO_MAPPING_USER_NAME` | 从 UserInfo 中读取用户名的字段，默认 `name`。 |
| `OIDC_USER_INFO_MAPPING_EMAIL` | 从 UserInfo 中读取邮箱的字段，默认 `email`。 |

启用 OIDC 时，后端会校验 `client_id`、`client_secret`，并要求配置 Discovery URL，或同时配置授权端点和 Token 端点。

## 文件存储

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `STORAGE_TYPE` | `local` | 文件存储类型，可选 `local`、`minio`、`cos`、`tos`、`s3`、`obs`、`oss`。 |
| `STORAGE_ALLOW_LIST` | 空 | 允许用户在租户设置中选择的存储类型，逗号分隔；空表示不限制。 |
| `LOCAL_STORAGE_BASE_DIR` | `/data/files` | 本地文件存储目录。 |
| `IMAGE_HOST_KEEP_URL` | 空 | 逗号分隔的图片域名白名单，命中后保留原始 URL。 |

MinIO：

| 变量 | 说明 |
| --- | --- |
| `MINIO_ENDPOINT` | MinIO 地址，例如 `minio:9000`。 |
| `MINIO_ACCESS_KEY_ID` | Access Key。 |
| `MINIO_SECRET_ACCESS_KEY` | Secret Key。 |
| `MINIO_BUCKET_NAME` | 存储桶名称。 |
| `MINIO_PATH_PREFIX` | 路径前缀。 |
| `MINIO_USE_SSL` | 是否使用 SSL。 |
| `MINIO_PORT` | Compose 中 MinIO API 暴露端口。 |
| `MINIO_CONSOLE_PORT` | Compose 中 MinIO Console 暴露端口。 |

腾讯云 COS：

| 变量 | 说明 |
| --- | --- |
| `COS_SECRET_ID` | Secret ID。 |
| `COS_SECRET_KEY` | Secret Key。 |
| `COS_REGION` | 区域。 |
| `COS_BUCKET_NAME` | 存储桶名称。 |
| `COS_APP_ID` | 应用 ID。 |
| `COS_PATH_PREFIX` | 路径前缀。 |
| `COS_ENABLE_OLD_DOMAIN` | 是否使用旧域名格式。 |
| `COS_TEMP_BUCKET_NAME` / `COS_TEMP_REGION` | 临时桶配置，可选。 |

其他对象存储：

| 类型 | 必要变量 |
| --- | --- |
| `s3` | `S3_ENDPOINT`、`S3_REGION`、`S3_ACCESS_KEY`、`S3_SECRET_KEY`、`S3_BUCKET_NAME`，可选 `S3_PATH_PREFIX`。 |
| `tos` | `TOS_ENDPOINT`、`TOS_REGION`、`TOS_ACCESS_KEY`、`TOS_SECRET_KEY`、`TOS_BUCKET_NAME`，可选 `TOS_PATH_PREFIX`、临时桶变量。 |
| `obs` | `OBS_ENDPOINT`、`OBS_ACCESS_KEY`、`OBS_SECRET_KEY`、`OBS_BUCKET_NAME`，可选 `OBS_REGION`、`OBS_PATH_PREFIX`、`OBS_PROXY_DOMAIN`。 |
| `oss` | `OSS_ENDPOINT`、`OSS_REGION`、`OSS_ACCESS_KEY`、`OSS_SECRET_KEY`、`OSS_BUCKET_NAME`，可选 `OSS_PATH_PREFIX`、临时桶变量。 |

如果使用 local 存储并接入飞书、企业微信、Slack 等 IM 平台，必须配置公网可达的 `APP_EXTERNAL_URL`，否则外部平台无法访问本地文件链接。

## 向量库

`RETRIEVE_DRIVER` 决定默认向量检索引擎，支持用逗号配置多个环境驱动的向量库。

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `RETRIEVE_DRIVER` | `postgres` | 向量存储类型，可选 `postgres`、`sqlite`、`elasticsearch_v7`、`elasticsearch_v8`、`opensearch`、`qdrant`、`milvus`、`weaviate`、`doris`、`tencent_vectordb`。 |
| `MULTI_STORE_RETRIEVE_TIMEOUT_SEC` | 内置默认 | 多向量库并行检索超时。 |

各引擎常用变量：

| 引擎 | 变量 |
| --- | --- |
| Elasticsearch | `ELASTICSEARCH_ADDR`、`ELASTICSEARCH_USERNAME`、`ELASTICSEARCH_PASSWORD`、`ELASTICSEARCH_INDEX`。 |
| OpenSearch | `OPENSEARCH_ADDR`、`OPENSEARCH_USERNAME`、`OPENSEARCH_PASSWORD`、`OPENSEARCH_INSECURE_SKIP_VERIFY`。 |
| Qdrant | `QDRANT_HOST`、`QDRANT_PORT`、`QDRANT_COLLECTION`、`QDRANT_API_KEY`、`QDRANT_USE_TLS`。 |
| Milvus | `MILVUS_ADDRESS`、`MILVUS_COLLECTION`、`MILVUS_METRIC_TYPE`、`MILVUS_USERNAME`、`MILVUS_PASSWORD`、`MILVUS_DB_NAME`。 |
| Weaviate | `WEAVIATE_HOST`、`WEAVIATE_GRPC_ADDRESS`、`WEAVIATE_SCHEME`、`WEAVIATE_AUTH_ENABLED`、`WEAVIATE_API_KEY`。 |
| Doris | `DORIS_ADDR`、`DORIS_HTTP_PORT`、`DORIS_DATABASE`、`DORIS_USERNAME`、`DORIS_PASSWORD`、`DORIS_TABLE_PREFIX`、`DORIS_COMPAT_MODE`。 |
| 腾讯云 VectorDB | `TENCENT_VECTORDB_ADDR`、`TENCENT_VECTORDB_USERNAME`、`TENCENT_VECTORDB_API_KEY`、`TENCENT_VECTORDB_DATABASE`。 |

通过 API 创建或测试向量库连接时会执行 SSRF 校验。Compose 默认通过 `SSRF_WHITELIST_EXTRA` 放行内置服务名，例如 `qdrant`、`milvus`、`weaviate` 和 `doris-fe`。

## 模型和内置模型

| 变量 | 说明 |
| --- | --- |
| `OLLAMA_BASE_URL` | Ollama 服务地址，默认示例为 `http://host.docker.internal:11434`。 |
| `OLLAMA_OPTIONAL` | 设为 `true` 时，Ollama 不可用不会阻断部分启动或检查流程。 |
| `BATCH_EMBED_SIZE` | Embedding 批量大小。 |
| `VLM_HTTP_TIMEOUT_SECONDS` | 远程 VLM 调用超时。 |
| `WEKNORA_LLM_CHAT_TIMEOUT_SECONDS` | 非流式模型调用超时兜底。 |
| `WEKNORA_LLM_STREAM_TIMEOUT_SECONDS` | 流式模型调用超时兜底。 |

内置模型可以通过 `config/builtin_models.yaml` 声明。该文件支持 `${ENV}` 占位符，因此 `.env.example` 中给出了常见变量名：

```bash
LLM_MODEL_NAME=
LLM_BASE_URL=
LLM_API_KEY=
LLM_PROVIDER=openai

EMBEDDING_MODEL_NAME=
EMBEDDING_BASE_URL=
EMBEDDING_API_KEY=
EMBEDDING_PROVIDER=openai

RERANK_MODEL_NAME=
RERANK_BASE_URL=
RERANK_API_KEY=
RERANK_PROVIDER=generic
```

这些变量名不是硬编码接口，实际以 `builtin_models.yaml` 中引用的占位符为准。也可以用 `BUILTIN_MODELS_CONFIG` 指定内置模型配置文件路径。

## 文档解析

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `DOCREADER_ADDR` | `docreader:50051` | App 访问 DocReader 的地址。轻量版常用 `127.0.0.1:50051`。 |
| `DOCREADER_TRANSPORT` | `grpc` | DocReader 传输方式，支持 `grpc` 或 `http`。 |
| `DOCREADER_PORT` | `50051` | DocReader 容器端口。 |
| `WEKNORA_DOCUMENT_PROCESS_TIMEOUT` | `2h` | 单个文档解析任务整体超时，使用 Go duration 格式。 |
| `WEKNORA_DOCREADER_CALL_TIMEOUT` | `30m` | 单次 DocReader 调用超时，使用 Go duration 格式。 |
| `MAX_FILE_SIZE_MB` | `50` | DocReader 和后端都会读取的文件大小上限。 |
| `DOCREADER_MARKITDOWN_MAX_WORKERS` | `1` | MarkItDown 解析并发。 |
| `DOCREADER_PDF_RENDER_MAX_WORKERS` | `1` | 扫描 PDF 渲染并发。 |
| `DOCREADER_PDF_RENDER_DPI` | `200` | PDF 渲染 DPI。 |
| `DOCREADER_PDF_JPEG_QUALITY` | `90` | 扫描 PDF 渲染 JPEG 质量。 |
| `DOCREADER_ODL_HYBRID` | `off` | 是否启用 OpenDataLoader hybrid 后端。 |
| `DOCREADER_ODL_HYBRID_URL` | `http://odl-hybrid:5002` | hybrid 后端地址。 |
| `DOCREADER_ODL_HYBRID_MODE` | `auto` | hybrid 模式。 |
| `DOCREADER_ODL_HYBRID_FALLBACK` | `false` | hybrid 失败后是否回退。 |
| `DOCREADER_ODL_MARKDOWN_WITH_HTML` | `false` | 是否保留 HTML。 |

gRPC 安全相关变量：

| 变量 | 说明 |
| --- | --- |
| `GRPC_TLS_ENABLED` | 是否启用 TLS。 |
| `GRPC_TLS_CERT` / `GRPC_TLS_KEY` | 证书和私钥路径。 |
| `GRPC_TLS_CA` | CA 证书路径。 |
| `GRPC_TLS_SERVER_NAME` | 客户端校验证书时使用的服务名。 |
| `GRPC_MTLS_REQUIRE_CLIENT_CERT` | 服务端是否强制要求客户端证书。 |
| `GRPC_AUTH_TOKEN` | gRPC 认证 Token。启用时建议同时启用 TLS。 |

## Agent、MCP 和沙箱

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `WEKNORA_AGENT_LLM_TIMEOUT` | 配置默认值 | 单次 Agent LLM 调用超时。支持 `300` 或 `5m` 这类写法。 |
| `WEKNORA_AGENT_TOOL_APPROVAL_TIMEOUT` | 默认 10 分钟 | MCP 工具人工审批等待超时。支持 Go duration 或秒数。 |
| `WEKNORA_AGENT_TOOL_APPROVAL_FAIL_OPEN` | `false` | 设为 `true` 时，审批系统异常时回退为放行。生产建议保持默认。 |
| `WEKNORA_SANDBOX_MODE` | Compose 为 `docker`，轻量版为 `disabled` | Agent Skills 沙箱模式，可选 `docker`、`local`、`disabled`。 |
| `WEKNORA_SANDBOX_TIMEOUT` | `60` | 沙箱执行超时，单位秒。 |
| `WEKNORA_SANDBOX_DOCKER_IMAGE` | `wechatopenai/weknora-sandbox:<version>` | Docker 沙箱镜像。 |
| `WEKNORA_SKILLS_DIR` | 空 | 自定义预加载技能目录。 |
| `WEKNORA_API_KEY` | 空 | 独立 MCP Server 访问 WeKnora REST API 时使用的 API Key。 |
| `MCP_PORT` | `8082` | 独立 MCP Server 对外端口。 |

如果关闭沙箱，依赖沙箱执行的工具能力会不可用或受限。生产中启用 `local` 模式需要额外审计主机权限。

## GraphRAG 和图数据库

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `ENABLE_GRAPH_RAG` | `false` | 是否启用知识图谱构建和检索。 |
| `NEO4J_ENABLE` | `false` | 是否启用 Neo4j 依赖。 |
| `NEO4J_URI` | `bolt://neo4j:7687` | Neo4j 地址。 |
| `NEO4J_USERNAME` | `neo4j` | Neo4j 用户名。 |
| `NEO4J_PASSWORD` | `password` | Neo4j 密码，生产必须修改。 |

开启 GraphRAG 会在解析阶段调用模型抽取实体和关系，解析耗时和模型成本都会增加。

## Web Search 和 SSRF

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `SSRF_WHITELIST` | 空 | SSRF 校验白名单，逗号分隔。支持精确域名、通配域名、IP、IPv6 和 CIDR。 |
| `SSRF_WHITELIST_EXTRA` | Compose 默认内置服务名 | 额外白名单，通常由 Compose 注入内置服务名。 |
| `SEARXNG_PORT` | `8888` | SearXNG 对外端口。 |
| `SEARXNG_BIND` | `127.0.0.1` | SearXNG 监听地址。 |
| `SEARXNG_SECRET` | 示例默认 | SearXNG image-proxy 签名密钥；对内网或公网开放时必须设置。 |

不要为了方便把大段内网 CIDR 放进 `SSRF_WHITELIST`。如果只需要访问一个内部服务，应优先白名单精确域名或单个 IP。

## Langfuse 可观测性

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `LANGFUSE_ENABLED` | 根据 key 自动判断 | 显式启用或关闭 Langfuse。 |
| `LANGFUSE_HOST` | `https://cloud.langfuse.com` | Langfuse 地址。Compose 自建示例使用 `http://langfuse-web:3000`。 |
| `LANGFUSE_PUBLIC_KEY` | 空 | Public Key。 |
| `LANGFUSE_SECRET_KEY` | 空 | Secret Key。 |
| `LANGFUSE_RELEASE` | 空 | 版本标签。 |
| `LANGFUSE_ENVIRONMENT` | 空 | 环境标签。 |
| `LANGFUSE_FLUSH_AT` | 内置默认 | 批量上报条数阈值。 |
| `LANGFUSE_FLUSH_INTERVAL` | 内置默认 | 批量上报时间间隔。 |
| `LANGFUSE_QUEUE_SIZE` | 内置默认 | 本地队列大小。 |
| `LANGFUSE_REQUEST_TIMEOUT` | 内置默认 | 上报请求超时。 |
| `LANGFUSE_SAMPLE_RATE` | `1.0` | 采样率。 |
| `LANGFUSE_DEBUG` | `false` | 调试日志。 |

同时设置 `LANGFUSE_PUBLIC_KEY` 和 `LANGFUSE_SECRET_KEY` 后会自动启用追踪。高流量生产环境建议降低采样率或调大批量上报参数。

## Helm Secret

Helm Chart 会把关键敏感字段写入 Kubernetes Secret。`helm/values.yaml` 中的核心字段包括：

| Values 字段 | 注入的环境变量 | 说明 |
| --- | --- | --- |
| `secrets.dbUser` | `DB_USER` | 数据库用户名。 |
| `secrets.dbPassword` | `DB_PASSWORD` | 数据库密码，必填。 |
| `secrets.dbName` | `DB_NAME` | 数据库名称。 |
| `secrets.redisUsername` | `REDIS_USERNAME` | Redis 用户名，可选。 |
| `secrets.redisPassword` | `REDIS_PASSWORD` | Redis 密码，必填。 |
| `secrets.jwtSecret` | `JWT_SECRET` | JWT 签名密钥，必填。 |
| `secrets.tenantAesKey` | `TENANT_AES_KEY` | 租户密钥。 |
| `secrets.systemAesKey` | `SYSTEM_AES_KEY` | 系统字段加密密钥。 |
| `secrets.existingSecret` | 多个 | 使用已有 Secret 时，必须包含上述 key。 |

普通非敏感变量放在 `app.env`，额外变量放在 `app.extraEnv`。例如给 App 注入 Ollama 地址：

```yaml
app:
  extraEnv:
    - name: OLLAMA_BASE_URL
      value: "http://ollama:11434"
```

## 生产检查清单

- 修改 `DB_PASSWORD`、`REDIS_PASSWORD`、`JWT_SECRET`、`TENANT_AES_KEY`、`SYSTEM_AES_KEY`。
- 固定并备份所有加密密钥，不要依赖临时随机值。
- 使用外部对象存储时，确认 bucket、region、endpoint 和访问权限正确。
- 使用 IM 渠道时，确认 `APP_EXTERNAL_URL` 或对象存储域名可被对应平台服务器访问。
- 使用内网模型、向量库或搜索服务时，只把必要地址加入 `SSRF_WHITELIST`。
- 开启 Langfuse 或 LLM 调试日志前，确认日志中允许包含用户问题和模型上下文。
- Kubernetes 中优先使用 Secret 或外部密钥系统注入敏感变量。

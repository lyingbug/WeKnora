---
title: 日志
description: 使用日志诊断 WeKnora 问题。
---

# 日志

日志是排查服务启动、Provider 错误、入库失败和权限问题的第一入口。
排查时优先保留同一次请求的 `X-Request-ID`、后端服务日志、DocReader 日志、队列任务日志，以及必要时的 Langfuse Trace。

## 先定位请求

WeKnora 的请求中间件会读取 `X-Request-ID` 请求头；如果调用方没有传入，服务会自动生成一个 UUID，并把它写回响应头。后端日志会把这个值记录为 `request_id` 字段，所以排查单次接口调用时建议显式传入一个容易搜索的值：

```bash
curl -i \
  -H "X-Request-ID: debug-20260611-001" \
  http://localhost:8080/health
```

然后在后端日志、Langfuse Trace、模型调试日志中搜索同一个请求标识。如果问题来自前端页面，先在浏览器开发者工具里找到失败请求的响应头，再用响应里的 `X-Request-ID` 回查服务端日志。

## 在哪里看日志

Docker Compose 部署时，常用命令如下：

```bash
docker compose logs -f --tail=500 app
docker compose logs -f --tail=500 docreader
docker compose logs -f --tail=500 redis postgres
```

如果启用了内置 Langfuse：

```bash
docker compose logs -f --tail=500 langfuse-web langfuse-worker langfuse-clickhouse
```

Kubernetes 部署时，先确认命名空间和工作负载名称，再查看日志：

```bash
kubectl logs -n weknora deploy/weknora-app -f
kubectl logs -n weknora deploy/weknora-docreader -f
```

本地开发时，后端日志直接输出在运行 `make dev-app` 或 `go run ./cmd/server` 的终端中；Compose 依赖服务日志可以通过 `make dev-logs` 查看。

Lite 或桌面形态运行时，如果没有显式设置 `LOG_PATH`，macOS `.app` 会把日志写到用户日志目录，例如：

```bash
~/Library/Logs/WeKnora Lite/WeKnora Lite.log
```

具体目录会随应用包名变化。

## 日志开关

后端日志由这些环境变量控制：

| 变量 | 作用 |
| --- | --- |
| `LOG_LEVEL` | 日志级别，支持 `debug`、`info`、`warn`、`warning`、`error`、`fatal`。未设置或非法值会回落到 `debug`。 |
| `LOG_PATH` | 写入文件路径。设置后日志会同时输出到标准输出和文件，并按 50 MB、3 个备份、28 天保留策略轮转压缩。 |
| `LOG_FORMAT` | 自定义日志格式，支持 `%d`、`%level`、`%thread`、`%logger`、`%traceId`、`%msg`。 |
| `GIN_MODE` | `release` 时 Gin 以发布模式运行；否则使用调试模式。 |

生产环境通常使用：

```bash
GIN_MODE=release
LOG_LEVEL=info
```

复现问题时可以临时切到：

```bash
LOG_LEVEL=debug
```

如果使用 `LOG_FORMAT`，结构化字段会追加到 `%msg` 后面；如果后续需要按 `request_id`、`status_code`、`path` 等字段检索，默认格式通常更方便。

## 启动日志

后端启动时会输出一段 `[startup-env] resolved environment:`，列出实际读取到的关键配置，包括数据库、Redis、对象存储、DocReader、检索驱动等。敏感值只显示是否设置和长度，不会打印明文。

排查启动失败时，先搜索这些关键字：

- `[startup-env]`：确认服务实际读到的环境变量，而不是只看 `.env` 文件。
- `SYSTEM_AES_KEY`：如果不是 32 字节，启动日志会提示加密能力被禁用。
- `[gin] registered`：确认路由已经注册完成。
- `Server is running`：确认 HTTP 服务已经开始监听。
- `Failed to initialize`、`connect`、`migration`：定位初始化、数据库连接或迁移错误。

如果容器里 `.env` 与宿主机预期不一致，以容器启动日志为准。

## 请求日志

后端请求日志会记录方法、路径、状态码、耗时、客户端 IP、响应大小和 `request_id`。对 `POST`、`PUT`、`PATCH` 请求，服务会在请求体是 JSON、表单或文本时记录请求体；二进制内容会被跳过。

实现里有几项保护机制：

- 请求体和响应体最多记录约 10 KB，超出后会截断。
- `password`、`token`、`api_key`、`secret`、`authorization` 等常见敏感字段会被替换成 `***`。
- `text/event-stream` 流式响应不会完整记录，只会标记为已跳过。

这些保护不等于完整的数据脱敏系统。对外分享日志前，仍然需要人工检查租户 ID、用户输入、文档内容、模型上下文和第三方响应。

## 模型调用日志

模型调用默认只在普通服务日志里留下请求、耗时、错误和用量等摘要。需要复现模型参数、工具调用或流式输出问题时，可以临时开启 `LLM_DEBUG_LOG`。

| 配置 | 行为 |
| --- | --- |
| `LLM_DEBUG_LOG=false` 或留空 | 关闭模型调试日志。 |
| `LLM_DEBUG_LOG=true` 或 `1` | 写入默认调试目录；如果设置了 `LOG_PATH`，会写到同级 `llm_debug/` 目录。 |
| `LLM_DEBUG_LOG=/path/to/dir` | 写入指定目录。 |

调试日志会按请求生成文件，记录 messages、options、tools、response、usage 和 error。它通常包含用户问题、召回上下文、工具参数和模型输出，只建议在短时间复现问题时开启，收集完证据后及时关闭。

## Langfuse Trace

启用 Langfuse 后，知识库问答、Agent 对话、知识库检索、标题生成、模型测试、评测、文档上传重解析、Wiki 修复、分片更新和数据源同步等链路会写入 Trace。认证、静态资源、健康检查等接口不会写入 Trace。

如果看不到 Trace，按顺序检查：

1. 后端启动日志是否出现 `[Langfuse] enabled`。
2. `LANGFUSE_PUBLIC_KEY`、`LANGFUSE_SECRET_KEY`、`LANGFUSE_HOST` 是否为真实可用配置。
3. 是否设置了 `LANGFUSE_ENABLED=false`。
4. 临时设置 `LANGFUSE_DEBUG=true` 后，日志里是否有 `[Langfuse] flush ... failed`。
5. 自托管 Langfuse 的 Web、Worker、ClickHouse、MinIO 是否都在运行。

需要特别注意：示例 `.env` 里的占位 key 如果被原样带到运行环境，后端会尝试启用 Langfuse，但实际写入会因为认证或地址错误失败。不使用 Langfuse 时，建议清空相关 key，或显式设置：

```bash
LANGFUSE_ENABLED=false
```

## 常见关键字

用日志排查时，可以先按组件搜索：

| 场景 | 关键字 |
| --- | --- |
| 启动和配置 | `[startup-env]`、`Server is running`、`AUTO_MIGRATE`、`connect` |
| 权限和租户 | `[rbac]`、`Cross-tenant`、`No permission`、`Unauthorized` |
| 文档入库 | `KnowledgePostProcess`、`DocReader`、`dead-letter callback`、`Housekeeping` |
| 模型调用 | `[LLM Request]`、`[LLM Usage]`、`Embedding`、模型供应商名称 |
| 检索链路 | `retrieve`、`rerank`、`vector`、`chunk` |
| Langfuse | `[Langfuse] enabled`、`[Langfuse] flush`、`trace` |
| 文件和图片 | `/files`、`presigned`、`GetFileURL`、对象存储供应商名称 |

如果同一个请求跨越后端、DocReader、队列 Worker 和模型供应商，先用 `request_id` 缩小范围；没有统一请求标识时，再按知识库 ID、文件 ID、会话 ID、任务 ID 或时间窗口交叉定位。

## 分享日志前

提交问题或给团队排查前，建议整理这些信息：

- 失败接口的路径、HTTP 状态码和 `X-Request-ID`。
- 对应时间窗口内的后端日志。
- 如果是入库问题，附上 DocReader 日志和队列任务日志。
- 如果是模型问题，附上供应商错误码、模型名称、用量摘要和必要的 LLM 调试日志。
- 如果启用了 Langfuse，附上 Trace 链接或 Trace ID。

不要直接分享完整 `.env`、密钥、访问令牌、原始文档、完整用户对话和未脱敏的 LLM 调试日志。

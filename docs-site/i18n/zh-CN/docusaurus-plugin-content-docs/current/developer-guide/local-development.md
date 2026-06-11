---
title: 本地开发
description: 搭建 WeKnora 本地开发环境。
---

# 本地开发

本地开发模式适合频繁修改后端、前端或文档解析能力的场景。它不会把 App 和前端一起放进 Docker 容器，而是用 `docker-compose.dev.yml` 启动基础设施，再让 Go 后端和 Vite 前端直接在宿主机运行。

这种方式的核心收益是：前端可以使用 Vite 热更新，后端可以用 Air 自动重启，修改代码后不需要反复构建 Docker 镜像。

## 前置条件

本地开发依赖以下工具：

| 工具 | 用途 |
| --- | --- |
| Go `1.26.0` | 运行 `cmd/server`，版本来自 `go.mod`。 |
| Node.js / npm | 安装并运行 `frontend` 的 Vite 开发服务器。 |
| Docker | 启动 PostgreSQL、Redis、DocReader 和可选依赖。 |
| Docker Compose | `scripts/dev.sh` 会优先使用 `docker compose`，找不到时再尝试 `docker-compose`。 |
| Air | 可选，用于 Go 后端热重载。未安装时脚本会退回 `go run`。 |

可以先运行环境检查脚本：

```bash
./scripts/check-env.sh
```

如果缺少 Air，可以按脚本提示安装：

```bash
go install github.com/air-verse/air@latest
```

## 准备配置

首次开发先从示例文件生成 `.env`：

```bash
cp .env.example .env
```

开发脚本会读取 `.env`，但 `make dev-app` 启动后端时会覆盖一组容器地址，让宿主机进程直接连本机端口：

| 变量 | 本地开发覆盖值 |
| --- | --- |
| `DB_HOST` | `localhost` |
| `REDIS_ADDR` | `localhost:6379` |
| `DOCREADER_ADDR` | `localhost:50051` |
| `DOCREADER_TRANSPORT` | `grpc` |
| `MINIO_ENDPOINT` | `localhost:9000` |
| `QDRANT_HOST` | `localhost` |
| `MILVUS_ADDRESS` | `localhost:19530` |
| `NEO4J_URI` | `bolt://localhost:7687` |

`.env.example` 中 `LOCAL_STORAGE_BASE_DIR` 默认为 `/data/files`，这是容器内挂载路径。`make dev-app` 在宿主机运行时，如果你没有显式改这个变量，脚本会自动改成仓库下的 `.local-data/files`，避免本机没有 `/data` 或无写权限。

如果模型服务也在本机运行，例如 Ollama，建议把 `.env` 中的模型地址改成宿主机可访问地址：

```bash
OLLAMA_BASE_URL=http://127.0.0.1:11434
```

容器部署常用 `host.docker.internal`，但本地后端进程运行在宿主机上，使用 `127.0.0.1` 更直接。

## 启动顺序

建议开三个终端。

第一个终端启动基础设施：

```bash
make dev-start
```

默认会启动 PostgreSQL、Redis、DocReader 和 Langfuse。启动成功后，脚本会打印常用端口：

| 服务 | 默认地址 |
| --- | --- |
| PostgreSQL | `localhost:5432` |
| Redis | `localhost:6379` |
| DocReader | `localhost:50051` |
| Langfuse | `http://localhost:3000` |

第二个终端启动后端：

```bash
make dev-app
```

脚本会加载 `.env`，设置本地开发覆盖变量，然后运行后端。如果检测到 `air`，会使用热重载；否则执行：

```bash
go run -ldflags="$LDFLAGS" ./cmd/server
```

后端默认监听 `http://localhost:8080`。

第三个终端启动前端：

```bash
make dev-frontend
```

如果 `frontend/node_modules` 不存在，脚本会先执行 `npm install`。随后启动 Vite，默认访问地址是：

```text
http://localhost:5173
```

`frontend/vite.config.ts` 会把 `/api` 和 `/files` 代理到后端。代理目标按以下优先级读取：

1. `VITE_DEV_PROXY_TARGET`
2. `FRONTEND_BACKEND_URL`
3. `http://localhost:8080`

如果你的后端不是监听 `8080`，可以在启动前端前设置代理目标：

```bash
VITE_DEV_PROXY_TARGET=http://localhost:18080 make dev-frontend
```

## 可选依赖

`make dev-start` 可以通过 `DEV_ARGS` 传递 profile 参数：

| 命令 | 启动内容 |
| --- | --- |
| `make dev-start DEV_ARGS=--minio` | 额外启动 MinIO，对象存储端口 `9000`，控制台端口 `9001`。 |
| `make dev-start DEV_ARGS=--qdrant` | 额外启动 Qdrant，HTTP 端口 `6333`，gRPC 端口 `6334`。 |
| `make dev-start DEV_ARGS=--neo4j` | 额外启动 Neo4j，浏览器端口 `7474`，Bolt 端口 `7687`。 |
| `make dev-start DEV_ARGS=--dex` | 额外启动 Dex，默认端口 `5556`。 |
| `make dev-start DEV_ARGS=--langfuse` | 显式启动 Langfuse。默认已经开启。 |
| `make dev-start DEV_ARGS=--no-langfuse` | 不启动 Langfuse。 |
| `make dev-start DEV_ARGS=--odl-hybrid` | 启动 OpenDataLoader hybrid 服务，并在需要时重建 DocReader。 |
| `make dev-start DEV_ARGS=--full` | 启动 MinIO、Qdrant、Neo4j 和 Dex；不包含 `odl-hybrid`。 |

`--odl-hybrid` 首次启动会构建镜像，脚本会等待 `http://localhost:5002/health` 就绪。如果要让 DocReader 使用它，还需要在 `.env` 中启用对应模式，例如：

```bash
DOCREADER_ODL_HYBRID=docling-fast
```

`--full` 不包含 `odl-hybrid`，需要组合使用时应显式追加：

```bash
make dev-start DEV_ARGS="--full --odl-hybrid"
```

## 常用命令

| 命令 | 说明 |
| --- | --- |
| `make dev-status` | 查看开发 Compose 服务状态。 |
| `make dev-logs` | 跟随查看开发 Compose 服务日志。 |
| `make dev-restart` | 停止后重新启动开发基础设施。 |
| `make dev-stop` | 停止开发基础设施。 |
| `make test` | 运行 Go 测试：`go test -v ./...`。 |
| `make fmt` | 执行 `go fmt ./...`。 |
| `make lint` | 执行 `golangci-lint run`。 |
| `make docs` | 通过 `swag init` 生成后端 Swagger 文档。 |

数据库迁移也通过 Makefile 暴露：

| 命令 | 说明 |
| --- | --- |
| `make migrate-up` | 执行全部向上迁移。 |
| `make migrate-down` | 回滚一步迁移。 |
| `make migrate-version` | 查看当前迁移版本。 |
| `make migrate-create name=xxx` | 创建新的迁移文件。 |
| `make migrate-force version=123` | 强制设置迁移版本。 |
| `make migrate-goto version=123` | 迁移到指定版本。 |

## 调试建议

如果后端连不上数据库，先确认基础设施已启动：

```bash
make dev-status
```

再检查 `.env` 中的 `DB_PORT`、`DB_USER`、`DB_PASSWORD`、`DB_NAME` 是否与 PostgreSQL 容器一致。`make dev-app` 会覆盖 `DB_HOST=localhost`，所以本地开发时通常不需要把它改成 `postgres`。

如果 Redis 认证失败，确认 `.env` 中 `REDIS_PASSWORD` 与容器启动时一致。修改密码后，旧容器可能仍使用旧环境变量，最直接的处理方式是停止并重新启动开发基础设施。

如果前端请求打到了错误后端，检查 `VITE_DEV_PROXY_TARGET` 或 `FRONTEND_BACKEND_URL`。Vite 只代理 `/api` 和 `/files`，其余前端路由由开发服务器自己处理。

如果本地 OpenSearch 或 SearXNG 被 SSRF 校验拦截，需要按 `.env.example` 的提示加入白名单：

```bash
# 本机 OpenSearch 9200 端口
SSRF_WHITELIST=localhost

# 本机 SearXNG
SSRF_WHITELIST=127.0.0.1
```

只把开发确实需要访问的主机加入白名单。不要为了省事放开大段内网网段。

---
title: 测试
description: 贡献变更前运行检查。
---

# 测试

WeKnora 的测试分布在后端主模块、独立 CLI 模块、前端和文档站中。提交变更前不要只跑一个固定命令，应根据修改范围选择对应检查；跨层改动需要把相关检查组合起来跑。

当前仓库中，根目录的 `Makefile` 暴露了后端常用检查，`frontend/package.json` 暴露了前端检查，CLI 有独立 GitHub Actions 矩阵和端到端工作流。

## 后端

根目录后端测试使用 Go 标准测试框架：

```bash
make test
```

等价于：

```bash
go test -v ./...
```

这会覆盖根模块下的后端、DocReader client、通用 client 等测试。源码中大量测试使用内存 SQLite、`httptest.Server`、mock repository 或 `sqlmock`，因此默认不要求真实 PostgreSQL、Redis、OpenSearch、Doris 或模型服务。

常见测试位置包括：

| 路径 | 覆盖重点 |
| --- | --- |
| `internal/application/service/*_test.go` | 知识库、会话、检索、Agent、MCP、向量库等应用服务行为。 |
| `internal/application/repository/*_test.go` | GORM 行为、SQLite 兼容性、任务队列和数据访问边界。 |
| `internal/handler/*_test.go` | HTTP handler 的参数解析、错误响应和权限上下文。 |
| `internal/middleware/*_test.go` | RBAC、知识库访问控制等中间件。 |
| `internal/infrastructure/*_test.go` | 文档解析、网页搜索、外部连接适配。 |
| `internal/agent/*_test.go` | Agent 引擎、工具注册、参数校验、记忆和观测。 |

修改 Go 代码后，至少运行：

```bash
make test
make fmt
```

如果变更会进入提交或 PR，还应运行：

```bash
make lint
```

`make lint` 依赖本机已安装 `golangci-lint`。如果本机没有该工具，先安装再跑，不要把 lint 未执行当成已通过。

### 定向测试

开发时可以先跑更窄的包：

```bash
go test -v ./internal/application/service/...
go test -v ./internal/handler/...
go test -v ./internal/agent/...
```

也可以指定单个测试：

```bash
go test -v ./internal/application/service -run TestResolveProcessConfig
```

涉及竞态、后台任务或流式输出的改动，建议额外跑 `-race`：

```bash
go test -race ./internal/application/service/...
```

## CLI

`cli/` 是独立 Go 模块，根目录的 `make test` 不会替你进入 CLI 模块。改 CLI 时需要单独检查：

```bash
cd cli
go build ./...
go test ./...
go vet ./...
```

仓库的 `.github/workflows/cli.yml` 在 Linux、macOS、Windows 三个平台上运行：

```bash
go build ./...
go test -race -coverprofile=coverage.out ./...
go vet ./...
```

本地复现更接近 CI 的命令是：

```bash
cd cli
go test -race -coverprofile=coverage.out ./...
go tool cover -func=coverage.out
```

CLI 的普通测试大量使用 fake SDK、`httptest.Server`、临时 XDG 目录和 golden wire contract，不需要真实 WeKnora 服务。

## CLI 端到端测试

`cli/acceptance/e2e/e2e_test.go` 带有 build tag：

```go
//go:build acceptance_e2e
```

因此默认 `go test ./...` 不会运行它。它会针对真实 WeKnora 服务执行完整 RAG 流程：创建知识库、上传文档、等待索引、搜索分片、发起聊天，并在测试清理阶段删除临时知识库。

手动运行需要提供真实服务地址和 token：

```bash
cd cli
WEKNORA_E2E_HOST=https://your-weknora.example.com \
WEKNORA_E2E_TOKEN=your-token \
go test -tags=acceptance_e2e -v -timeout=8m ./acceptance/e2e/...
```

GitHub Actions 中的 `.github/workflows/cli-e2e.yml` 是按需触发的：维护者可以手动触发，或给 PR 打 `acceptance-e2e` 标签。该工作流依赖仓库 secret `WEKNORA_E2E_HOST` 和 `WEKNORA_E2E_TOKEN`，缺少 secret 时会跳过。

## 前端

前端在 `frontend` 目录中运行检查：

```bash
cd frontend
npm test
npm run type-check
```

`npm test` 实际执行 `node --test`。当前前端测试是纯 TypeScript 逻辑测试，示例包括：

| 文件 | 覆盖重点 |
| --- | --- |
| `src/views/knowledge/kbListMerge.test.ts` | 知识库列表合并、去重、排序和权限归并。 |
| `src/views/knowledge/wikiStatusRefresh.test.ts` | 文档轮询状态变化触发 Wiki 状态刷新。 |
| `src/utils/thinkingControl.test.ts` | 前端默认思考控制与后端 provider 默认值保持一致。 |
| `src/components/modelEditorSourceState.test.ts` | 模型配置来源状态的展示逻辑。 |

`npm run type-check` 执行：

```bash
vue-tsc --build
```

改动 Vue 组件、路由、store、API 类型或 i18n 类型时，应同时运行：

```bash
npm run build
```

前端生产构建会检查 Vite 打包路径、依赖解析和静态资源引用，比 `node --test` 覆盖面更大。

## 文档站

文档站在 `docs-site` 目录中：

```bash
cd docs-site
npm run build
```

该命令会同时构建英文和中文站点。修改中文文档时也要跑它，因为 Docusaurus 会同时校验 MDX、front matter、侧边栏链接和本地化目录结构。

如果只想本地预览中文站点，可以运行：

```bash
cd docs-site
npm run start:zh
```

但预览不能替代 `npm run build`，因为开发服务器不会暴露所有生产构建错误。

## 推荐检查组合

| 修改范围 | 建议检查 |
| --- | --- |
| 后端业务逻辑、handler、repository | `make test`、`make fmt`，提交前再跑 `make lint`。 |
| 数据库字段、迁移、GORM tag | 定向 repository/service 测试，再跑 `make test`。 |
| Agent、MCP、工具调用、流式输出 | 相关 `internal/agent/...` 测试，必要时加 `go test -race`。 |
| 前端页面或组件 | `cd frontend && npm test && npm run type-check && npm run build`。 |
| 前后端 API 契约 | 后端相关包测试、前端 type-check，必要时启动本地开发环境做手工验证。 |
| CLI | `cd cli && go build ./... && go test ./... && go vet ./...`。 |
| CLI 真实服务流程 | `go test -tags=acceptance_e2e -v -timeout=8m ./acceptance/e2e/...`。 |
| 文档 | `cd docs-site && npm run build`。 |

## 写新测试的原则

优先把核心行为下沉到可直接调用的函数或 service，再写纯单元测试。前端的 `kbListMerge.ts`、后端的 process config、向量库配置校验、Agent tool 参数校验等都是这种模式。

需要验证 HTTP 层时，优先使用 `httptest.NewRecorder` 或 `httptest.NewServer`。这能覆盖请求解析和响应格式，同时避免依赖真实网络服务。

需要验证数据库行为时，优先使用内存 SQLite 或 `sqlmock`。如果测试目标是 GORM tag、软删除、事务保护或 schema 兼容性，应像现有 repository 测试一样显式准备最小 DDL，避免 AutoMigrate 掩盖真实迁移差异。

需要真实外部服务时，必须用 build tag、环境变量或显式 skip 隔离，不能让默认 `go test ./...` 在普通开发机上因为缺少服务而失败。

## 常见问题

如果 `go test ./...` 在向量库、网页搜索或外部连接相关测试中失败，先看测试是否使用了 `httptest` 或 mock。默认测试通常不应该访问真实外部服务；如果新增测试需要外部服务，应改成 fake server 或显式隔离。

如果前端 `npm test` 没有发现新测试，确认文件名是否匹配 Node test runner 的默认发现规则。当前仓库使用 `*.test.ts`。

如果文档站构建时出现 Docusaurus update check 写入 `~/.config` 的警告，但构建已显示 `Generated static files`，这只是更新检查权限问题，不代表文档构建失败。

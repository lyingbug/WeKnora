---
title: 贡献指南
description: 向 WeKnora 贡献代码和文档。
---

# 贡献指南

欢迎通过 Issue、Discussion 和 Pull Request 参与 WeKnora。一次高质量贡献应包含清晰动机、聚焦的变更、可复现的验证步骤，以及必要的文档和截图。

仓库当前没有单独的 `CONTRIBUTING.md`，贡献流程主要由 README、Issue 模板、PR 模板、`SECURITY.md`、各模块 README 和 GitHub Actions 共同约束。本文把这些约束整理成一个可执行流程。

## 选择入口

如果要报告缺陷，请使用 Bug Report 模板。模板会要求提供：

- 受影响组件，例如前端、后端、DocReader、向量数据库或模型服务。
- 最小复现步骤。
- 期望行为与实际行为。
- 具体版本号，不要只写 `latest`、`main` 或 `master`。
- 部署方式，例如 Docker Compose、源码构建、Lite、桌面版或 Helm。
- 操作系统、相关日志和错误信息。

日志收集可以参考模板中的命令：

```bash
docker compose logs -f --tail=1000 app docreader postgres
```

如果是功能建议，请使用 Feature Request 模板。它会要求说明当前问题、建议方案、替代方案、影响范围和具体使用场景。

如果只是使用问题，请使用 Question 模板或 Discussions。提问时应写明你已经查过的文档、Issue 和尝试过的操作。

安全漏洞不要提交公开 Issue。请按 `SECURITY.md` 使用 GitHub 私密漏洞报告功能；无法使用时再联系仓库维护者。报告中应包含漏洞描述、复现步骤、受影响版本、影响范围和建议缓解方式。

## 开始开发

推荐流程是：

1. Fork 仓库。
2. 从最新主分支创建一个聚焦的功能分支。
3. 在本地复现问题或确认需求边界。
4. 修改代码并补充测试。
5. 运行与变更范围匹配的检查。
6. 提交 PR，并在描述中写清楚变更、验证和兼容性影响。

如果改动需要本地联调，可以使用开发模式：

```bash
cp .env.example .env
make dev-start
make dev-app
make dev-frontend
```

`make dev-start` 只启动基础设施，Go 后端和 Vite 前端在宿主机运行。需要 Qdrant、MinIO、Dex、Neo4j 或 OpenDataLoader hybrid 时，通过 `DEV_ARGS` 打开对应 profile。

## 分支与提交

PR 标题应遵循仓库 PR 模板中的提交语义，例如：

```text
feat: add datasource sync retry settings
fix: prevent duplicate knowledge-base cards
docs: expand local development guide
chore(deps): update cli dependency group
```

提交和 PR 应尽量聚焦。不要把格式化、依赖升级、文档重写和业务修复混在同一个 PR 中，除非它们是完成同一变更不可分割的一部分。

依赖升级要谨慎。仓库的 Dependabot 策略是：`/cli` 可以按月收到常规版本更新；根模块、`frontend`、`docreader`、`client`、`miniprogram` 和 GitHub Actions 默认只接受安全更新。手动升级这些依赖时，需要在 PR 中说明原因、影响范围和验证结果。

## 变更范围与验证

后端 Go 代码变更通常需要：

```bash
make fmt
make test
make lint
```

`make test` 执行 `go test -v ./...`。`make lint` 依赖本机安装 `golangci-lint`。

前端变更通常需要：

```bash
cd frontend
npm test
npm run type-check
npm run build
```

前端测试使用 `node --test`，类型检查使用 `vue-tsc --build`。

CLI 变更需要进入独立模块：

```bash
cd cli
go build ./...
go test ./...
go vet ./...
```

更接近 CI 的检查是：

```bash
cd cli
go test -race -coverprofile=coverage.out ./...
go tool cover -func=coverage.out
```

CLI 真实服务端到端测试默认不会运行，因为它使用 `acceptance_e2e` build tag。需要真实服务地址和 token：

```bash
cd cli
WEKNORA_E2E_HOST=https://your-weknora.example.com \
WEKNORA_E2E_TOKEN=your-token \
go test -tags=acceptance_e2e -v -timeout=8m ./acceptance/e2e/...
```

文档站变更需要：

```bash
cd docs-site
npm run build
```

如果改了 API 注释或 Swagger 相关 handler，需要重新生成接口文档：

```bash
make docs
```

`docs/docs.go` 是 `swaggo/swag` 生成文件，文件头标注了不要手动编辑。应修改 Go 注释或类型定义后再用 `make docs` 生成。

如果改了 Lite、桌面版或打包链路，要注意前端静态资源同步路径。`make build-lite`、`scripts/package-lite.sh` 和 `scripts/package-mac-app.sh` 会构建 `frontend/dist` 并同步到 `web/`。如果使用 `SKIP_FRONTEND=1`，PR 描述里应说明你使用的是已有 `web/` 产物。

## CI 与发布相关影响

仓库当前包含以下主要工作流：

| 工作流 | 触发范围 | 作用 |
| --- | --- | --- |
| `.github/workflows/cli.yml` | `cli/**` 和该工作流自身 | 在 Linux、macOS、Windows 上构建、测试和 `go vet` CLI。 |
| `.github/workflows/cli-e2e.yml` | 手动触发，或带 `acceptance-e2e` 标签的 CLI PR | 针对真实 WeKnora 服务运行 CLI RAG 全链路测试。 |
| `.github/workflows/docker-image.yml` | `main` 分支和 `v*` tag | 构建并推送前端、DocReader、App 等 Docker 镜像。 |
| `.github/workflows/release-lite.yml` | Lite 发布流程 | 构建 Lite 二进制、前端资源和桌面相关产物。 |

如果变更影响镜像构建、前端静态资源、DocReader、Lite 或 Helm 部署，应在 PR 中明确写出部署影响和回滚方式。

## 文档同步

行为变化应同步更新文档。常见位置包括：

| 变更类型 | 建议同步位置 |
| --- | --- |
| 用户可见功能 | `README.md`、`README_CN.md`、`docs-site/` 对应页面。 |
| API 变更 | Swagger 注释、`docs/api/`、文档站 API 页面。 |
| 环境变量或部署参数 | `.env.example`、Compose、Helm、部署文档。 |
| CLI 命令或输出契约 | `cli/README.md`、`cli/AGENTS.md`、CLI 测试。 |
| 故障排查经验 | `docs/QA.md` 或文档站 troubleshooting 页面。 |
| 发布说明 | `CHANGELOG.md`，尤其是破坏性变更和迁移步骤。 |

文档站规则写在 `docs-site/README.md`：英文是默认语言，中文页面位于 `docs-site/i18n/zh-CN/docusaurus-plugin-content-docs/current/`，目录结构和 slug 应与默认语言保持对齐。

## PR 描述

PR 模板要求填写：

- 变更描述。
- 变更类型。
- 关联 Issue，例如 `Fixes #123` 或 `Closes #123`。
- 测试方式和验证步骤。
- 是否更新测试与文档。
- 是否存在破坏性变更。
- 用户可见 UI 变更的截图或录屏。

描述应让 reviewer 能快速回答三个问题：为什么要改、具体改了什么、怎么确认它有效。

## 代码评审前自查

提交前请确认：

- PR 只解决一个清晰问题或一组强相关问题。
- 新行为有测试覆盖，或 PR 中解释了无法自动化测试的原因。
- 错误信息、日志和用户提示足够具体。
- 涉及认证、租户、文件访问、SSRF、凭据、MCP 或外部连接时，已经考虑安全边界。
- 涉及数据库或迁移时，已说明兼容性、回滚和数据影响。
- 涉及 UI 时，已验证桌面和移动端关键路径，并附截图或录屏。
- 涉及配置、部署或默认值时，已同步 `.env.example`、Compose、Helm 或文档。

如果 PR 需要维护者额外手工验证，请把命令、测试账号、样例文件、预期结果和清理步骤写在描述中。

---
title: FAQ
description: 常见问题。
---

# FAQ

本文收集跨模块的高频问题。更细的解析、模型调用、日志和检索质量问题，请继续查看 troubleshooting 目录下的专项页面。

## 如何确认当前版本？

在 Web UI 中打开用户菜单，进入“设置 / 系统信息”。该页面会显示：

- App 版本，也就是后端 `weknora-app` 版本。
- UI 版本，也就是前端 `weknora-ui` 构建版本。
- Commit ID、构建时间和 Go 版本。
- 数据库迁移版本。

提交 Issue 时请提供具体版本号，不要只写 `latest`、`main` 或 `master`。如果 App 版本和 UI 版本不一致，也要同时写出来，因为这通常意味着后端和前端镜像不是同一次发布。

## 如何查看日志？

Docker Compose 部署可以先看核心服务日志：

```bash
docker compose logs -f --tail=1000 app docreader postgres
```

如果问题与 Redis 队列、MinIO、向量库或可观测性有关，再追加对应服务名：

```bash
docker compose logs -f --tail=1000 redis minio qdrant langfuse
```

本地开发模式可以查看开发 Compose 日志：

```bash
make dev-logs
```

Lite、桌面版或源码运行时，请提供终端输出或 `logs/*.log` 中的相关片段。

## Web UI 无法打开怎么办？

先确认部署方式。

Docker Compose 标准部署通常由前端容器提供 UI，后端 App 提供 API。检查服务状态：

```bash
docker compose ps
```

如果前端或 App 没有启动，继续看日志：

```bash
docker compose logs -f --tail=300 app frontend
```

本地开发模式下，前端默认运行在：

```text
http://localhost:5173
```

后端默认运行在：

```text
http://localhost:8080
```

确认三个终端都已启动：

```bash
make dev-start
make dev-app
make dev-frontend
```

如果页面能打开但 API 全部失败，检查前端代理目标。`frontend/vite.config.ts` 会把 `/api` 和 `/files` 代理到 `VITE_DEV_PROXY_TARGET`、`FRONTEND_BACKEND_URL` 或默认的 `http://localhost:8080`。

## 如何判断后端是否存活？

后端注册了无需认证的健康检查接口：

```bash
curl http://localhost:8080/health
```

正常响应类似：

```json
{"status":"ok"}
```

如果健康检查失败，优先看 App 日志、端口占用、数据库连接和 Redis 连接。

## Swagger 为什么打不开？

Swagger 只在非生产模式下注册。后端路由里会检查 `GIN_MODE`：当 Gin 处于 `release` 模式时，`/swagger/*any` 不会启用。

本地开发或调试模式下，启动后端后可以访问：

```text
http://localhost:8080/swagger/index.html
```

如果你修改了 API 注释或响应结构，需要重新生成：

```bash
make docs
```

`docs/docs.go` 是生成文件，不要手动编辑。

## 服务启动后无法上传文档怎么办？

上传失败最常见原因是模型、DocReader 或存储配置不完整。

先确认基础模型配置是否存在。至少需要对话模型和 Embedding 模型；如果使用 Ollama，还要确认 Ollama 服务可访问：

```bash
OLLAMA_BASE_URL=http://127.0.0.1:11434
INIT_LLM_MODEL_NAME=your_llm_model
INIT_EMBEDDING_MODEL_NAME=your_embedding_model
INIT_EMBEDDING_MODEL_DIMENSION=1024
INIT_EMBEDDING_MODEL_ID=your_embedding_model_id
```

如果使用远程模型，还需要配置对应地址和密钥：

```bash
INIT_LLM_MODEL_BASE_URL=https://api.example.com/v1
INIT_LLM_MODEL_API_KEY=your_key
INIT_EMBEDDING_MODEL_BASE_URL=https://api.example.com/v1
INIT_EMBEDDING_MODEL_API_KEY=your_key
```

再确认 DocReader 可用。Compose 部署查看：

```bash
docker compose logs -f --tail=300 docreader
```

本地开发模式下，后端会连接：

```text
localhost:50051
```

如果上传的是图片、音频或扫描件，还要确认知识库或本次上传的 `process_config` 已配置 VLM / ASR / 解析引擎。批次中包含图片但没有有效 VLM，或包含音频但没有有效 ASR，后端会拒绝这次上传。

## 上传时如何临时调整解析配置？

从 `0.6.2` 开始，文件、URL、文件夹上传都可以携带 `process_config`，只覆盖本次批次，不修改知识库默认配置。Web UI 会在上传前弹出确认对话框；API 和 `weknora doc upload` 使用同名 JSON 字段。

`process_config` 可覆盖解析引擎、分块、多模态、问题生成和图谱抽取等设置。未传字段会沿用知识库默认值。

重新解析已有文档时，也可以在请求体中传入 `process_config`：

```text
POST /knowledge/:id/reparse
```

覆盖项会保存到文档元数据中，便于追踪这次重解析实际使用了哪些配置。

## 图片显示无效链接怎么办？

先判断图片来自哪里。

如果使用本地存储，后端通过 `/files` 或预签名 `/files/presigned` 代理读取文件。反向代理必须允许这些路径，并保留认证或签名参数。

如果使用 MinIO，确认 MinIO 已启动：

```bash
docker compose --profile minio up -d
```

再检查 `.env` 中的 bucket、访问密钥和公开访问地址。`MINIO_PUBLIC_ENDPOINT` 默认常写成 `http://localhost:9000`，这只适合浏览器也在同一台机器上访问的场景。若从其他设备访问，应改成可被浏览器访问的主机名或 IP。

Bucket 名称建议只使用小写字母、数字和连字符。不要使用中文或特殊字符。

## 为什么刚保存的配置几秒后又消失？

这通常不是后端清空了配置，而是浏览器代理、缓存、抓包工具或插件让前端读到了旧响应。

排查顺序：

1. 关闭浏览器代理、抓包工具和会改写请求的插件。
2. 确认 `localhost`、`127.0.0.1` 和实际部署域名没有走代理。
3. 使用无痕窗口重新登录后保存一次。
4. 打开浏览器开发者工具的网络面板，确认保存请求返回的是最新内容。
5. 重启 App 服务后再验证：

```bash
docker compose restart app
```

如果重启后短时间恢复，但再次访问又出现相同现象，仍应优先检查浏览器代理、缓存和多环境串连。

## 登录后为什么没有回到上次工作区？

系统会记录最后活跃工作区，并在登录后尝试恢复。没有恢复通常有三类原因：

- 浏览器清理了本地存储，或你换了浏览器。
- 上次访问的工作区已经把你移除，系统会回退到默认工作区。
- JWT 中携带的租户已失效，需要退出后重新登录。

如果是 Lite 模式，前端路由守卫还会在硬刷新后尝试恢复本次会话中最后访问的 `/platform` 子路径。

## 升级后为什么提示权限不足？

从 `0.6.0` 开始，WeKnora 引入租户内 RBAC。所有写操作都会结合角色、资源归属和租户上下文鉴权。

常见现象：

- 能看但不能改：你可能是 `Viewer`，或是非创建者的 `Contributor`。
- 共享空间里的知识库或 Agent 默认按只读处理；要写入需要源租户授予更高权限。
- API Key 调用会合成所属租户的管理员身份，但删除租户仍需要所有者权限。
- 跨租户超级管理员需要同时满足用户具备跨租户权限，以及请求中启用对应租户上下文。

先检查用户菜单中的当前工作区角色。如果需要调整协作权限，按 `Viewer`、`Contributor`、`Admin`、`Owner` 的职责分配。

## 回答没有引用来源怎么办？

先确认不是“没有检索到内容”：

1. 选中的知识库文档是否已经解析完成。
2. 文档是否有可检索文本分块。
3. 问题是否与知识库内容相关。
4. 检索参数是否过窄，例如 top k 太小、相似度阈值太高、只选了错误知识库。
5. Rerank 失败时是否仍有原始检索结果。

如果文档刚上传，等待解析状态进入完成后再试。若检索结果为空，优先看专项的检索质量排查页。

## Agent 为什么没有调用工具？

Agent 调工具需要同时满足几个条件：

- Agent 配置中启用了对应工具。
- MCP 服务或内置工具处于可用状态。
- 工具 schema 能被模型理解。
- 当前模型支持或适配工具调用。
- 用户问题确实需要工具，而不是普通问答即可回答。
- 高风险工具如果需要审批，审批流程没有被拒绝或超时。

如果是 MCP 工具，先在设置中测试 MCP 服务连接，再查看后端日志中与 MCP 初始化、工具列表和调用失败相关的记录。

## 数据分析功能怎么用？

数据分析工具主要面向 CSV 和 Excel 文件。复杂 Excel 如果读取失败，建议先另存为标准 CSV 后重新上传。

Agent 中的数据分析工具只允许只读查询，例如：

```sql
SELECT
SHOW
DESCRIBE
EXPLAIN
PRAGMA
```

不允许执行 `INSERT`、`UPDATE`、`DELETE`、`CREATE`、`DROP` 等修改语句。

## 本机 OpenSearch 或 SearXNG 为什么连接被拒？

WeKnora 对用户可配置 URL 做 SSRF 校验。回环地址、内网地址和部分敏感端口默认会被拦截。

本地调试 OpenSearch `9200` 时，可按需加入：

```bash
SSRF_WHITELIST=localhost
```

本地调试 SearXNG 时，常用：

```bash
SSRF_WHITELIST=127.0.0.1
```

只加入确实需要访问的目标。生产环境不要为了方便放开大段内网网段。

## 升级后数据库迁移失败怎么办？

先查看系统信息页中的数据库版本。如果显示 dirty 或 failed，需要查看 App 启动日志，找到失败迁移版本和 SQL 错误。

常用检查：

```bash
make migrate-version
```

如果 `golang-migrate` 标记了 dirty state，需要先确认失败语句是否已经部分执行，再决定是否 force 到上一个成功版本：

```bash
make migrate-force version=<上一个成功版本>
make migrate-up
```

不要在不了解失败 SQL 状态时直接 force。涉及生产数据时应先备份数据库。

## 升级到 0.6.2 后 CLI 登录或 MCP 工具报错怎么办？

`0.6.2` 随附的 CLI v0.9 有破坏性变更。常见迁移点：

- `auth login` 不再创建 profile。先执行 `weknora profile add <name> --host <url> --use`，再执行 `weknora auth login`。
- `auth logout` 和 `auth refresh` 作用于当前 active profile，不再使用 `--name`。
- MCP 工具 `agent_invoke` 改名为 `session_ask`，外部 MCP 客户端需要刷新工具 schema。
- `agent create --kb` 改为 `--attach-kb`。
- `doc delete --all`、`search chunks`、`search docs` 的 `--kb` 必填，支持知识库名称或 ID。

如果你的自动化脚本依赖旧命令，需要同时更新命令参数和错误处理逻辑。

## 仍然无法定位问题时，应提供哪些信息？

提交 Issue 时请尽量提供：

- App 版本、UI 版本和 commit。
- 部署方式和操作系统。
- 复现步骤、期望行为、实际行为。
- 相关截图或录屏。
- 相关日志，至少包含 App、DocReader、PostgreSQL；需要时补充 Redis、MinIO、向量库和模型服务。
- 相关配置片段，但不要公开 API Key、Token、Cookie、数据库密码或私钥。

如果问题与某个文档、模型、MCP 服务或向量库有关，请提供最小可复现样例，而不是完整生产数据。

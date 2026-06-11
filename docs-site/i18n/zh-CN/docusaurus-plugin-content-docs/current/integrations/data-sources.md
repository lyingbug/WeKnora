---
title: 数据源
description: 将外部内容同步到 WeKnora。
---

# 数据源

数据源把飞书、Notion、语雀等外部知识系统同步到 WeKnora 知识库。它不是一次性上传入口，而是一套带凭据管理、资源选择、定时调度、增量游标和同步日志的长期同步机制。

一个数据源固定绑定到一个知识库。同步任务会把外部文档转换为 WeKnora 知识条目，后续检索、问答和 Agent 使用方式与普通上传文档一致。

## 支持的连接器

当前运行时注册了以下连接器：

| 类型 | 凭据字段 | 可选配置 | 可同步资源 | 内容处理 |
| --- | --- | --- | --- | --- |
| `feishu` | `app_id`、`app_secret` | 无 | Wiki 空间、Wiki 节点子树 | `docx`、`doc`、`sheet`、`bitable` 通过飞书导出接口下载为文件；`file` 下载原文件。 |
| `notion` | `api_key` | 可在设置中覆盖 `base_url` | 页面、数据库 | 页面和数据库记录转换为 Markdown；数据库可转换为 Markdown 表格；非图片附件会单独下载。 |
| `yuque` | `api_token` | `base_url`，默认 `https://www.yuque.com` | 知识库/书籍 | 正文以 Markdown 入库，跳过草稿和非文档条目。 |

代码中预留了 Confluence、GitHub、Google Drive、OneDrive、钉钉、网页爬虫、Slack、IMAP、RSS 等连接器类型元数据，但只有注册到连接器 Registry 的类型才能创建和同步。当前容器启动时只注册飞书、Notion 和语雀。

## 配置入口

### 管理界面

打开知识库设置中的数据源页，可以查看当前知识库绑定的数据源。Viewer 可以查看列表和同步日志；Admin 可以新增、编辑、删除、测试连接、触发同步、暂停和恢复。

新增数据源分为四步：

1. 选择连接器类型。
2. 填写凭据并测试连接。
3. 读取外部资源树并选择要同步的资源。
4. 配置同步计划、同步模式、冲突策略和删除同步开关。

创建过程中，前端会先创建一个 `paused` 状态的临时数据源来拉取资源列表。用户取消时会删除这个临时记录；提交成功后会把状态改为 `active`，并立即触发一次同步。

编辑已有数据源时，凭据不会出现在主表单。界面只显示“已配置”，需要点击替换或移除才会调用凭据子资源。

### API

数据源 API 路由如下：

| 方法 | 路径 | 权限 | 用途 |
| --- | --- | --- | --- |
| `GET` | `/api/v1/datasource/types` | Viewer | 获取连接器类型元数据。 |
| `POST` | `/api/v1/datasource/validate-credentials` | Admin | 使用原始凭据测试连接，不保存。 |
| `POST` | `/api/v1/datasource` | Admin | 创建数据源。 |
| `GET` | `/api/v1/datasource?kb_id=...` | Viewer | 列出某个知识库的数据源。 |
| `GET` | `/api/v1/datasource/:id` | Viewer | 查看单个数据源。 |
| `PUT` | `/api/v1/datasource/:id` | Admin | 更新数据源非凭据配置。 |
| `DELETE` | `/api/v1/datasource/:id` | Admin | 软删除数据源。 |
| `PUT` | `/api/v1/datasource/:id/credentials` | Admin | 整体替换连接器凭据。 |
| `DELETE` | `/api/v1/datasource/:id/credentials/credentials` | Admin | 清除连接器凭据。 |
| `POST` | `/api/v1/datasource/:id/validate` | Admin | 测试已保存的数据源连接。 |
| `GET` | `/api/v1/datasource/:id/resources` | Admin | 列出可同步资源。 |
| `POST` | `/api/v1/datasource/:id/sync` | Admin | 立即触发同步。 |
| `POST` | `/api/v1/datasource/:id/pause` | Admin | 暂停定时同步。 |
| `POST` | `/api/v1/datasource/:id/resume` | Admin | 恢复定时同步。 |
| `GET` | `/api/v1/datasource/:id/logs` | Viewer | 查看数据源同步日志。 |
| `GET` | `/api/v1/datasource/logs/:log_id` | Viewer | 查看单条同步日志。 |

所有数据源都会通过绑定知识库校验租户归属。即使知道数据源 ID，也不能跨租户读取或修改。

## 配置结构

数据源主记录包含以下核心字段：

| 字段 | 说明 |
| --- | --- |
| `knowledge_base_id` | 目标知识库 ID，创建后不能修改。 |
| `name` | 数据源显示名，也会作为同步内容的自动标签名。 |
| `type` | 连接器类型，例如 `feishu`、`notion`、`yuque`。 |
| `config.type` | 连接器类型，通常与主字段一致。 |
| `config.resource_ids` | 选中的外部资源 ID。 |
| `config.settings` | 连接器非密钥配置。 |
| `sync_schedule` | 秒级 cron 表达式，例如 `0 0 */6 * * *`。 |
| `sync_mode` | `incremental` 或 `full`。 |
| `status` | `active`、`paused`、`error`。 |
| `conflict_strategy` | `overwrite` 或 `skip`，当前同步更新路径按 `external_id` 替换已有知识。 |
| `sync_deletions` | 是否在结果中统计源端删除。 |
| `last_sync_cursor` | 增量同步游标。 |
| `last_sync_result` | 最近一次同步汇总。 |
| `sync_log_retention_days` | 同步日志保留天数，默认 30。 |

`resource_ids` 的含义由连接器决定：

- 飞书可以是 Wiki 空间 ID，也可以是 `space_id:node_token` 形式的节点子树。
- Notion 可以是页面 ID、数据库 ID 或 data source ID。
- 语雀是知识库/书籍 ID。

前端资源选择器会把选中的资源压缩成最小覆盖集合：如果选中了父节点，就不再逐个提交所有子节点。这样后续源端在父节点下新增页面时，也会被下一次同步包含。

## 凭据管理

数据源凭据是一个连接器级别的原子 map，不按字段逐个更新。原因是许多连接器的认证需要一组字段同时有效，例如飞书的 `app_id` 和 `app_secret`。

API 响应不会返回凭据明文，只返回是否已配置：

```json
{
  "credentials": {
    "credentials": {
      "configured": true
    }
  }
}
```

创建数据源时可以在 `config.credentials` 中带入初始凭据。编辑数据源时，`PUT /datasource/:id` 会强制保留已有凭据，即使请求体误带 `config.credentials` 也会忽略；替换凭据必须使用 `PUT /datasource/:id/credentials`。

如果配置了 `SYSTEM_AES_KEY`，`config.credentials` 中的字符串字段会以 AES-256-GCM 加密后写入数据库。读取时会自动解密；历史明文凭据仍可兼容读取。

## 同步流程

数据源同步由两条路径触发：

- 手动同步：`POST /datasource/:id/sync` 立即创建同步日志并投递任务。
- 定时同步：调度器启动时加载所有 `active` 且配置了 `sync_schedule` 的数据源，通过 cron 定期投递任务。

定时同步有两层防重复机制：

1. 如果数据库中已有 `running` 状态的同步日志，会跳过本次触发，避免同一数据源重叠同步。
2. 分布式任务使用确定性的 asynq TaskID，格式为 `dssync:<dataSourceID>:<minute>`，多实例同一分钟触发时只有一个实例能入队。

任务执行时会：

1. 读取数据源、同步日志和连接器。
2. 解析并解密配置。
3. 按 `sync_mode` 调用 `FetchAll` 或 `FetchIncremental`。
4. 为当前数据源查找或创建同名知识标签。
5. 把抓取到的内容写入目标知识库。
6. 更新同步日志、数据源状态、最近同步时间、增量游标和同步结果。

同步任务会把租户 ID 和租户信息写入上下文，确保后续知识库写入仍按当前租户执行。

## 入库语义

连接器返回统一的 `FetchedItem`：

- `external_id`：源端唯一 ID。
- `title`：标题。
- `content`：已下载内容，优先走文件入库管线。
- `url`：远程 URL；没有内容但有 URL 时由 WeKnora 下载解析。
- `file_name`：建议文件名。
- `updated_at`：源端更新时间。
- `metadata`：保留 channel、源端 ID、创建者等信息。
- `is_deleted`：源端删除标记。

WeKnora 会把 `datasource_id`、`external_id`、`source_resource_id` 和连接器元数据写入知识条目 metadata。

如果 `external_id` 已经存在，当前实现会先删除旧知识，再重新创建。这样可以复用完整的文件解析、分块和索引流程。重复文件或重复 URL 错误不会算作同步失败，而会计入 skipped。

当连接器返回 `is_deleted=true` 时，若 `sync_deletions=true`，同步结果会统计 deleted 数量；当前实现不会自动删除知识库中的内容。这样可以避免源端误判、权限变化或资源重选导致知识库内容被意外移除。需要删除知识时，应在知识库界面显式操作。

## 增量同步

增量同步依赖连接器维护的游标：

- 飞书记录每个资源下节点的编辑时间，优先使用对象内容编辑时间，并检测节点删除。
- Notion 记录页面和数据库记录的最后编辑时间；首次增量同步会先执行全量同步并建立游标。
- 语雀记录每个知识库下文档的 `content_updated_at`，并检测之前存在但当前列表中不存在的文档。

如果 `sync_mode=full`，或任务显式设置强制全量，同步会调用连接器的全量接口，不使用上一次游标。

## 同步日志

每次同步都会生成 `SyncLog`，主要字段包括：

| 字段 | 说明 |
| --- | --- |
| `status` | `running`、`success`、`partial`、`failed`、`canceled`。 |
| `items_total` | 本次处理的源端条目数。 |
| `items_created` | 新建知识数。 |
| `items_updated` | 替换更新知识数。 |
| `items_deleted` | 检测到的源端删除数。 |
| `items_skipped` | 跳过数量。 |
| `items_failed` | 失败数量。 |
| `error_message` | 失败原因。 |
| `result` | 详细同步结果 JSON。 |

前端会在数据源卡片上显示最近一次同步状态，并在有运行中同步时每 3 秒轮询列表。同步历史抽屉按时间展示日志、耗时和各类计数。

如果所有抓取到的条目都处理失败，并且没有 created、updated、deleted 或 skipped，后端会把整次同步标记为 failed。部分条目抓取失败时，连接器通常会返回带 `metadata.error` 的占位条目，日志中会计入 failed，但其余条目仍会继续处理。

## 暂停、恢复和删除

暂停数据源会把状态设为 `paused`，并移除 cron 定时任务。手动同步仍允许对 paused 数据源触发，任务完成后状态会保持 paused。

恢复数据源会把状态设为 `active`，并重新注册 cron 计划。

删除数据源是软删除，会移除 cron 计划，并把该数据源还在运行或待处理的同步日志标记为 canceled。删除数据源不会自动删除已经入库的知识内容。

## 连接器细节

### 飞书

飞书连接器使用自建应用的 `app_id` 和 `app_secret` 获取 tenant access token。token 会在内存中缓存，并在过期前留出安全余量。

资源列表会返回 Wiki 空间和空间内的 Wiki 节点。节点资源 ID 使用 `space_id:node_token`，表示同步该节点及其子树。

支持的内容类型包括：

- `docx`、`doc`：导出为文档文件。
- `sheet`、`bitable`：导出为表格文件。
- `file`：下载原始文件。

`mindnote` 和 `slides` 当前没有可用内容读取接口，会跳过。

### Notion

Notion 连接器使用 Internal Integration Token。资源列表来自搜索 API，并保留父子关系，前端可以渲染树形选择。

页面会被转换为 Markdown。数据库有两种处理方式：

- 数据库整体可转换为一个 Markdown 表格知识条目。
- 数据库记录可转换为带属性头部和正文块内容的 Markdown 条目。

Notion 页面中的图片会作为 Markdown 图片链接保留；非图片附件会被下载为独立知识条目。

### 语雀

语雀连接器使用 `api_token`，默认连接 `https://www.yuque.com`，也可以配置企业或私有部署地址。`base_url` 会自动补齐 `https://` 并去掉末尾斜杠。

资源列表会合并个人知识库和加入团队的知识库。若 token 对应的是团队账号，会直接列出团队知识库。

同步时只处理正式文档，跳过草稿和非文档类型。为了降低触发语雀接口限流的概率，拉取文档详情时会在请求之间做短暂停顿。

## 扩展连接器

新增连接器需要实现统一接口：

- `Type()`：返回连接器类型。
- `Validate()`：校验凭据和连通性。
- `ListResources()`：列出可选择资源。
- `FetchAll()`：全量抓取。
- `FetchIncremental()`：按游标增量抓取。

然后在容器初始化时注册到 `ConnectorRegistry`。前端还需要在数据源编辑器中补充连接器卡片、凭据字段、权限提示和资源类型展示。

连接器输出应尽量使用 Markdown 或 WeKnora 已支持解析的文件格式，并提供稳定的 `external_id`。如果源端支持层级资源，应在 `Resource.parent_id` 和 `has_children` 中返回树结构，便于前端做最小覆盖选择。

## 使用建议

- 优先使用增量同步，只有需要周期性重建全部内容时才选择全量同步。
- 定时 cron 不要设置得过密，尤其是语雀和 Notion 等上游存在限流的服务。
- 使用能稳定代表源端文档的 `external_id`，避免同一文档反复创建重复知识。
- 给数据源取清晰名称；同步内容会自动打同名标签，后续排查和筛选更方便。
- 对飞书应用，确保至少具备 Wiki、Drive 导出、文档读取等权限，并把应用加入目标空间。
- 对 Notion，确保 integration 已被授权访问目标页面或数据库。
- 对语雀，确认 token 具备知识库和文档读取权限。
- 删除源端文档后，不要假设 WeKnora 会自动删除知识；需要在知识库中显式清理。

## 常见问题

### 为什么编辑数据源时看不到凭据？

数据源响应会移除凭据明文，只返回是否已配置。替换和清除凭据必须通过 `/credentials` 子资源完成。

### 为什么修改知识库 ID 会失败？

数据源创建后固定绑定到一个知识库。更新接口会保留原 `knowledge_base_id`，如果请求试图切换知识库会返回错误。需要同步到另一个知识库时，应新建数据源。

### 为什么资源列表为空？

通常是上游权限不足或 integration 没有被加入目标空间。飞书需要应用有对应 Wiki/Drive 权限且能访问空间；Notion 需要页面或数据库授权给 integration；语雀需要 token 能读取目标知识库。

### 为什么状态变成 error？

连接测试、资源抓取或同步任务失败时，数据源会记录 `error_message` 并进入 error 状态。修复凭据或权限后，可以测试连接或手动同步；成功后状态会恢复 active。

### 为什么源端删除没有删除知识库文档？

当前实现只统计源端删除数量，不自动删除 WeKnora 知识内容。这是为了避免源端权限变化、同步配置变化或连接器误判造成不可预期的数据丢失。

## 相关文档

- [知识库](../user-guide/knowledge-bases.md)
- [导入流水线](../architecture/ingestion-pipeline.md)
- [检索流水线](../architecture/retrieval-pipeline.md)
- [认证与权限](./authentication.md)

---
title: 入库问题
description: 排查文档解析和索引失败。
---

# 入库问题

入库问题通常发生在上传、解析、切分、向量化、多模态处理或后处理阶段。WeKnora 会把文档状态写入 `knowledges.parse_status`，并在启用解析追踪后把阶段明细写入 `knowledge_processing_spans`。

排查时不要只看“处理中”三个字。先确认文档处在哪个状态，再根据时间线定位阶段。

## 状态含义

文档解析状态由后端常量定义：

| 状态 | 含义 | 排查重点 |
| --- | --- | --- |
| `pending` | 已创建记录，等待异步任务执行。 | Redis / Asynq 队列是否拥塞，任务是否入队失败。 |
| `processing` | 主解析流程正在运行。 | DocReader、切分、Embedding、多模态阶段。 |
| `finalizing` | 主解析已完成，摘要、问题生成、图谱、Wiki 等后处理还在运行。 | 后处理队列、模型调用、`pending_subtasks_count`。 |
| `completed` | 主解析和所有后处理子任务都已结束。 | 如果仍无法检索，转到检索质量排查。 |
| `failed` | 解析失败。 | 查看错误信息、时间线最后失败阶段和 App / DocReader 日志。 |
| `cancelled` | 用户取消了解析。 | 已写入的分块和索引会保留，可重新解析。 |
| `deleting` | 文档正在删除。 | 等待删除任务完成，避免同时重解析。 |

`finalizing` 不是卡死。它表示文档主内容通常已经可用于检索，但系统仍在执行会消耗模型或图谱资源的后处理任务。

## 先看时间线

文档详情里有解析时间线入口。后端接口会返回固定 5 段：

| 阶段 | 说明 |
| --- | --- |
| `docreader` | 读取原始文件或 URL，调用 DocReader 或其它解析引擎。 |
| `chunking` | 把解析后的文本切分为分块，处理父子分块等配置。 |
| `embedding` | 调用 Embedding 模型并写入向量索引。 |
| `multimodal` | 图片 OCR、图片描述、音频 ASR 等多模态处理。 |
| `postprocess` | 摘要、问题生成、图谱、Wiki 等后处理任务扇出。 |

如果某个阶段失败，时间线会显示错误阶段和错误信息。若没有真实 span 数据，后端也会根据 `parse_status` 合成这 5 段占位状态，避免旧数据永远显示为待处理。

也可以直接请求接口查看：

```text
GET /api/v1/knowledge/{id}/spans
GET /api/v1/knowledge/{id}/spans?attempt=2
```

`attempt` 用于查看历史解析尝试。不传时返回最新尝试。

## 上传阶段失败

### 文件过大

上传文件大小由 `MAX_FILE_SIZE_MB` 控制，默认 `50` MB。这个值会影响四层：

- Go 后端上传校验。
- 前端 Nginx 请求体大小。
- DocReader gRPC 消息大小。
- 前端浏览器侧校验。

因此它是部署期配置，不是运行时系统设置。修改后必须同步重启相关容器或进程。

```bash
MAX_FILE_SIZE_MB=200
```

如果只改后端环境变量，Nginx 或前端 bundle 仍可能按旧值拒绝请求，表现为浏览器看到 `413` 或前端仍提示旧限制。

### 文件类型不支持

前端会根据知识库解析器配置计算可上传格式。后端解析器注册表中主要有这些能力：

| 解析器 | 典型支持类型 | 依赖 |
| --- | --- | --- |
| `simple` | `md`、`markdown`、`txt`、`csv`、`json`、图片、音频 | Go 进程内处理，无需 DocReader。 |
| `builtin` | `docx`、`doc`、`pdf`、`md`、`xlsx`、`xls`、图片、音频 | DocReader 必须连接成功。 |
| `weknoracloud` | `docx`、`doc`、`pdf`、`md`、`xlsx`、`xls`、`pptx`、`ppt` | WeKnora Cloud 凭据。 |
| `mineru` / `mineru_cloud` | `pdf`、图片、Office 文档、PPT | MinerU 服务或云端 API。 |
| `paddleocr_vl` / `paddleocr_vl_cloud` | `pdf`、图片 | PaddleOCR-VL 服务或云端 Token。 |

如果知识库页面提示“部分文档类型暂无可用解析引擎”，应进入知识库设置中的解析器配置，确认对应扩展名有可用引擎。

### URL 导入被拒绝

URL 导入会经过 SSRF 校验。回环地址、内网地址或敏感端口可能被拒绝。日志里通常能看到类似“SSRF validation failed for knowledge URL”。

本地调试时可以只把需要访问的主机加入白名单：

```bash
SSRF_WHITELIST=127.0.0.1
```

生产环境不要放开大段内网网段。

### 重复文件或重复 URL

上传接口遇到重复内容会返回冲突响应。此时不要反复重试同一个请求，应在 UI 中确认是否已存在同名或同源文档，或改用重新解析。

## DocReader 阶段失败

`docreader` 阶段失败通常来自文件解析器、OCR、PDF 渲染、gRPC 连接或解析服务资源不足。

Compose 部署先看 DocReader 日志：

```bash
docker compose logs -f --tail=500 docreader
```

本地开发模式下，`make dev-start` 会暴露 DocReader 到：

```text
localhost:50051
```

`make dev-app` 会覆盖后端环境变量：

```bash
DOCREADER_ADDR=localhost:50051
DOCREADER_TRANSPORT=grpc
```

标准 Docker Compose 部署中，App 默认连接容器网络内的：

```bash
DOCREADER_ADDR=docreader:50051
DOCREADER_TRANSPORT=grpc
```

如果你启用了 docreader gRPC TLS 或 Token，确认 App 侧和 DocReader 侧的证书、CA、Token 和服务名一致。

大文件或复杂 PDF 可能需要更长时间。可调的关键项：

```bash
WEKNORA_DOCREADER_CALL_TIMEOUT=30m
WEKNORA_DOCUMENT_PROCESS_TIMEOUT=2h
```

`WEKNORA_DOCREADER_CALL_TIMEOUT` 应小于 `WEKNORA_DOCUMENT_PROCESS_TIMEOUT`，否则单次 DocReader 调用可能比整个文档任务超时还长。

## 切分阶段失败

`chunking` 阶段通常受知识库分块配置或本次上传的 `process_config.chunking_config` 影响。

重点检查：

- `chunk_size` 是否过小，导致分块数量异常膨胀。
- `chunk_overlap` 是否大于或接近 `chunk_size`。
- 自定义分隔符是否把内容切成大量空片段。
- 父子分块开启后，父分块和子分块大小是否合理。
- 解析器是否返回了空文本。

如果文档是图片或扫描 PDF，切分失败可能只是上游 OCR 没有产出文本，应回到 `docreader` 或 `multimodal` 阶段排查。

## Embedding 阶段失败

`embedding` 阶段失败会影响检索和引用。常见原因：

- Embedding 模型没有配置。
- 模型服务不可达。
- API Key 无效。
- 模型维度与向量索引维度不一致。
- 单次批量文本过大或模型超时。

基础配置至少应包含：

```bash
INIT_EMBEDDING_MODEL_NAME=your_embedding_model
INIT_EMBEDDING_MODEL_ID=your_embedding_model_id
INIT_EMBEDDING_MODEL_DIMENSION=1024
```

远程模型还需要：

```bash
INIT_EMBEDDING_MODEL_BASE_URL=https://api.example.com/v1
INIT_EMBEDDING_MODEL_API_KEY=your_key
```

如果使用 PostgreSQL pgvector，升级到 `0.6.2` 后会有 1024 维 HNSW 索引迁移。其它维度的模型需要按实际维度维护索引策略。

## 多模态阶段失败

`multimodal` 阶段处理图片 OCR、图片描述和音频 ASR。

上传图片时，如果本次配置或知识库默认配置启用了图片处理，必须有可用 VLM。后端校验失败时会返回：

```text
上传图片文件需要设置VLM模型
```

上传音频时，必须有可用 ASR 配置。后端校验失败时会返回：

```text
上传音频文件需要设置ASR语音识别模型
```

如果你在上传确认对话框中临时打开了多模态，但知识库默认没有 VLM / ASR，也会触发同样校验。

图片或音频处理慢时，先看时间线中 `multimodal` 下的子节点，再看模型服务日志和 App 日志。多模态任务使用独立队列，默认并发较低，批量图片可能排队。

## 后处理阶段卡在 finalizing

主解析完成后，`postprocess` 会根据配置扇出后处理任务。源码中会先计算将要产生的子任务数，再把文档从 `processing` 原子切换到 `finalizing`，并写入 `pending_subtasks_count`。

可能产生的子任务包括：

- 摘要生成。
- 问题生成。
- 图谱抽取。
- Wiki 生成。

每个子任务结束时会调用 `FinalizeSubtask`，原子递减 `pending_subtasks_count`。当计数归零且状态仍是 `finalizing`，文档才会晋升为 `completed`。

如果长期停在 `finalizing`：

1. 打开时间线，确认是 summary、question、graph 还是 wiki 子任务没有结束。
2. 查看 App 日志中的 `KnowledgePostProcess`、`QuestionGeneration`、`SummaryGeneration`、`Graph`、`WikiIngest` 关键词。
3. 检查模型服务是否超时或限流。
4. 检查 Redis / Asynq 队列是否堆积。
5. 如果只是队列忙，不要立即判定为卡死，可以提高异步并发或延长任务超时。

相关配置：

```bash
WEKNORA_ASYNQ_CONCURRENCY=32
WEKNORA_REDIS_OP_TIMEOUT_MS=500
WEKNORA_DOCUMENT_PROCESS_TIMEOUT=2h
```

`WEKNORA_ASYNQ_CONCURRENCY` 变更需要重启进程。

## 任务真的卡死怎么办？

WeKnora 有两道兜底：

- Asynq 任务耗尽重试后进入死信，死信回调会把文档 `parse_status` 标记为 `failed`，并写入错误信息。
- Housekeeping 会扫描长期停留在 `processing` 或 `finalizing` 的文档。它会结合文档更新时间、span 心跳和队列中是否仍有相关任务，避免误杀正常长任务。

Housekeeping 相关配置：

```bash
WEKNORA_HOUSEKEEPING_ENABLED=true
WEKNORA_DOCUMENT_PROCESS_TIMEOUT=2h
```

默认扫描阈值是文档处理超时时间再加缓冲时间。日志中出现“span heartbeat within threshold”说明候选文档仍有活跃阶段；出现“tasks still queued”说明更可能是队列堆积而不是任务丢失。

## 如何取消解析？

正在 `pending`、`processing` 或 `finalizing` 的文档可以取消解析：

```text
POST /api/v1/knowledge/{id}/cancel-parse
```

取消后：

- `parse_status` 会变为 `cancelled`。
- `pending_subtasks_count` 会清零。
- 已写入的分块、文件和索引会保留。
- 队列中尚未执行的相关任务会尽量删除。
- 正在执行的 worker 会在检查点发现取消状态后退出。
- 之后可以通过重新解析再次触发。

已 `completed`、`failed` 或 `deleting` 的文档不能取消。

## 如何重新解析？

重新解析接口是：

```text
POST /api/v1/knowledge/{id}/reparse
```

可以不传 body，表示使用知识库默认配置；也可以传本次重解析专用配置：

```json
{
  "process_config": {
    "chunking_config": {
      "chunk_size": 800,
      "chunk_overlap": 100
    },
    "question_generation_config": {
      "enabled": true
    }
  }
}
```

重新解析会把覆盖项保存到该文档元数据中，后续时间线和处理逻辑都按这次有效配置执行。

## 排查顺序

遇到入库失败时，建议按下面顺序处理：

1. 在文档列表查看 `parse_status` 和错误信息。
2. 打开解析时间线，定位失败阶段。
3. 根据阶段查看 App、DocReader、模型服务、向量库或对象存储日志。
4. 检查 `.env` 中的文件大小、DocReader、模型、Redis 和队列超时配置。
5. 如果是配置问题，修正后使用重新解析；不要重复上传同一个文件制造重复记录。
6. 如果是长任务，先确认 span 心跳和队列状态，不要只凭更新时间判断卡死。
7. 如果任务确实无法继续，取消解析或等待死信 / housekeeping 标记失败后再重试。

提交 Issue 时，请附上文档 ID、知识库 ID、`parse_status`、时间线最后失败阶段、App 日志和 DocReader 日志。不要公开上传文件中的敏感内容、模型密钥或对象存储凭据。

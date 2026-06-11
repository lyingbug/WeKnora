---
title: 模型调用问题
description: 排查模型供应商调用失败。
---

# 模型调用问题

模型调用失败可能来自凭据、端点配置、额度限制、不支持的参数或网络限制。
排查时不要只看“聊天是否能回答”，要把 Chat、Embedding、Rerank、VLM 和 ASR 分开验证，因为它们走不同的模型类型、适配器和测试接口。

## 先确认失败位置

先记录这些信息：

- 失败请求的 `X-Request-ID`。
- 失败发生在模型配置页、知识库入库、知识库问答、Agent 对话、图片理解还是音频转写。
- 使用的模型 ID、模型类型、模型名称、Provider、Base URL。
- 上游返回的 HTTP 状态码和错误正文。
- 是否启用了 Langfuse Trace 或 `LLM_DEBUG_LOG`。

同一个 Provider 下，不同模型类型也可能使用不同端点。例如 Chat 通常调用 `/chat/completions`，Embedding 调用 `/embeddings`，Rerank 调用 `/rerank`，ASR 调用 `/audio/transcriptions`。一个端点可用不代表另一个端点也可用。

## 配置是否真的生效

模型配置保存后，运行时会从数据库里的 Model 记录构造调用客户端。核心字段包括：

| 字段 | 排查点 |
| --- | --- |
| `type` | `KnowledgeQA`、`Embedding`、`Rerank`、`VLLM`、`ASR` 决定使用哪条调用链路。 |
| `source` | `remote` 走远程 API，`local` 走 Ollama。Rerank 当前不走本地 Ollama。 |
| `name` | 运行时真正传给上游的模型名。 |
| `parameters.provider` | 优先决定 Provider 适配器；为空时才从 Base URL 推断。 |
| `parameters.base_url` | 远程模型服务地址，会经过 SSRF 校验。 |
| `parameters.embedding_parameters.dimension` | Embedding 维度，必须与向量库已有索引一致。 |
| `parameters.extra_config` | 供应商特定参数，例如 Azure API version、远程模型名覆盖、思考模式编码等。 |
| `parameters.custom_headers` | 企业网关、路由和追踪类请求头；保留头会被过滤。 |

模型列表和详情不会返回 `api_key`、`app_secret` 明文，只返回 `credentials` 元数据表示是否已配置。编辑已有模型时，如果前端不展示密钥，不代表密钥丢失；需要通过凭据子资源更新或清除。

如果启动日志里出现类似 `SYSTEM_AES_KEY missing/rotated` 或模型参数解密失败，说明保存过的密钥可能无法解密。此时模型列表仍可能可见，但运行时会把凭据视为未配置，需要重新写入凭据。

## 使用内置测试接口

模型管理页的“测试连接”会调用后端初始化接口。也可以直接用 API 复现。

测试远程 Chat：

```bash
curl -X POST http://localhost:8080/api/v1/initialization/remote/check \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -H "X-Request-ID: model-chat-debug-001" \
  -d '{
    "modelName": "gpt-4o-mini",
    "baseUrl": "https://api.openai.com/v1",
    "apiKey": "<api-key>",
    "provider": "openai"
  }'
```

这个接口会用生产同一套构造路径创建 Chat 客户端，并发送一条内容为 `test` 的最小消息。它会把 `401`、`403`、`404`、超时、连接失败等错误归类成中文提示，同时保留原始错误信息。

测试 Embedding：

```bash
curl -X POST http://localhost:8080/api/v1/initialization/embedding/test \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -H "X-Request-ID: model-embedding-debug-001" \
  -d '{
    "source": "remote",
    "modelName": "text-embedding-3-small",
    "baseUrl": "https://api.openai.com/v1",
    "apiKey": "<api-key>",
    "provider": "openai"
  }'
```

Embedding 测试会实际对 `hello` 做一次向量化，并返回 `dimension`。如果测试通过但入库失败，重点检查知识库里绑定的模型 ID、向量维度是否与已建索引一致，以及批量文本是否触发上游长度限制。

测试 Rerank：

```bash
curl -X POST http://localhost:8080/api/v1/initialization/rerank/check \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -H "X-Request-ID: model-rerank-debug-001" \
  -d '{
    "modelName": "jina-reranker-v2-base-multilingual",
    "baseUrl": "https://api.jina.ai/v1",
    "apiKey": "<api-key>",
    "provider": "jina"
  }'
```

Rerank 测试会用 `ping` 和 `pong` 发起最小重排请求，返回结果数量大于 0 才认为功能正常。

测试 ASR 时，后端会发送一段极短的静默 WAV 到转写端点：

```bash
curl -X POST http://localhost:8080/api/v1/initialization/asr/check \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -H "X-Request-ID: model-asr-debug-001" \
  -d '{
    "modelName": "whisper-1",
    "baseUrl": "https://api.openai.com/v1",
    "apiKey": "<api-key>",
    "provider": "openai"
  }'
```

## 常见错误含义

| 错误现象 | 可能原因 | 处理方式 |
| --- | --- | --- |
| `401`、`unauthorized` | API Key 错误、密钥未保存、密钥解密失败。 | 重新写入凭据；确认当前租户使用的是正确模型。 |
| `403`、`forbidden` | Key 没有模型权限、项目权限不足、区域不匹配。 | 检查供应商控制台权限、项目、地域和模型授权。 |
| `404`、`not found` | Base URL 拼错、模型名不存在、端点路径不兼容。 | 确认 Base URL 不要重复带 `/chat/completions`；模型名用供应商要求的真实 ID。 |
| `timeout`、`context deadline exceeded` | 后端到上游网络不通、上游响应慢、代理或防火墙拦截。 | 从后端容器或 Pod 内测试网络；必要时调大超时。 |
| `connection refused`、`no such host`、`dial tcp` | DNS、容器网络、Ollama 地址或私有网关地址错误。 | 检查服务端能否解析和访问该域名。 |
| `baseURL SSRF check failed` | Base URL 指向回环地址、内网地址、直接 IP 或敏感端口。 | 使用公开域名，或把可信内部地址加入 `SSRF_WHITELIST_EXTRA`。 |
| `unsupported chat model source`、`unsupported embedder source` | `source` 字段不是当前模型类型支持的值。 | 远程 API 用 `remote`，本地 Ollama 用 `local`。 |
| `model is currently downloading` | 本地 Ollama 模型还在拉取。 | 等待下载完成后重试。 |
| `model download failed` | Ollama 拉取失败。 | 检查 Ollama 服务、模型名、网络和磁盘空间。 |
| `EmbedBatch API error` | Embedding 上游返回非 200，可能是模型、长度、权限或维度参数问题。 | 查看错误正文；缩小文本长度；确认模型类型是 Embedding。 |
| `Rerank API error` | Rerank 端点非 200。 | 确认 Provider、Base URL 和模型名使用的是重排接口。 |

## Chat 排查

Chat 远程模型默认走 OpenAI 兼容请求；Anthropic、Azure OpenAI、WeKnoraCloud 以及部分思考模型会走专门适配逻辑。排查时重点看：

- `provider` 是否填对。填空时系统根据 Base URL 推断，但私有网关或自定义域名通常会被识别为 `generic`。
- Base URL 是否只到 API 根路径，例如 `https://api.example.com/v1`，不要把具体接口路径重复拼进去。
- Azure OpenAI 是否配置了正确的 `api_version`，并使用 Azure 资源地址。
- WeKnoraCloud 是否有可用的 AppID 和 AppSecret；如果模型自身没有凭据，会尝试从租户 WeKnoraCloud 凭据补全。
- 是否开启了模型不支持的工具调用、图片输入、思考参数或采样参数。

远程 Chat 有兜底超时。没有上层 deadline 时，非流式调用使用 `WEKNORA_LLM_CHAT_TIMEOUT_SECONDS`，流式调用使用 `WEKNORA_LLM_STREAM_TIMEOUT_SECONDS`。如果日志显示请求长时间挂起，可以临时调小超时复现，也可以在网络恢复后调大避免长文档生成被过早中断。

## Embedding 排查

Embedding 失败通常影响文档入库、重建索引和向量检索。重点检查：

- 知识库绑定的 Embedding 模型 ID 是否存在且状态为 `active`。
- 实际返回的向量维度是否等于模型配置里的维度。
- 向量库已有集合或索引维度是否和当前模型一致。
- 批量文本是否过长；OpenAI 兼容实现会记录输入长度，默认会提示空文本或超过 8192 字符的输入。
- `BATCH_EMBED_SIZE` 是否过大，导致上游限流或请求体过大；默认批量大小为 5。
- 阿里云纯文本 Embedding 应使用兼容接口；多模态 Embedding 检测接口会直接提示暂不支持。

如果只是换了 Embedding 模型但没有重建知识库索引，常见结果是入库或检索时维度不匹配。此类问题不能只通过更新模型配置解决，需要重新导入文档或重建索引。

## Rerank 排查

Rerank 是检索后处理步骤。Rerank 失败不一定导致 Chat 模型不可用，但会影响问答质量或让检索链路报错。

排查顺序：

1. 先用 `/initialization/rerank/check` 验证 Rerank 模型本身。
2. 再检查知识库或 Agent 是否真的配置了 Rerank 模型。
3. 查看日志里的 `rerank request endpoint=... model=...`，确认请求发到了预期端点。
4. 如果使用 LKEAP，确认 SecretId、SecretKey 和 `extra_config.region` 的配置方式。
5. 如果返回空结果，确认上游返回格式里有 `results`，并且分数字段是 `relevance_score` 或 `score`。

## 本地 Ollama 排查

本地模型来源为 `local` 时，后端通过 `OLLAMA_BASE_URL` 访问 Ollama。常见部署差异：

- Docker Compose 访问宿主机 Ollama 通常使用 `http://host.docker.internal:11434`。
- Lite 或本机进程通常使用 `http://127.0.0.1:11434`。
- Kubernetes 需要使用集群内可解析的 Service 地址。

如果模型状态一直是 `downloading` 或 `download_failed`，查看后端日志里的 Ollama 下载任务日志，并确认 Ollama 服务可达、模型名存在、磁盘空间充足。

## 打开详细诊断

复现问题时可以临时打开：

```bash
LOG_LEVEL=debug
LLM_DEBUG_LOG=true
LANGFUSE_DEBUG=true
```

`LOG_LEVEL=debug` 会显示更多模型调用摘要，例如 `[LLM Request]`、`[LLM Usage]`、Embedding 请求长度和 Rerank 请求预览。

`LLM_DEBUG_LOG` 会把 Chat、Embedding、Rerank、VLM 等完整调用写入每个请求对应的调试文件，包括 messages、options、tools、response、usage 和 error。它可能包含用户输入、知识库召回内容、工具参数、图片引用和模型输出，只建议短时间开启，收集完证据后关闭。

启用 Langfuse 时，可以在 Trace 中查看模型调用、Embedding、Rerank 和异步任务的 span。若 Trace 不出现，先回到日志页检查 `[Langfuse] enabled` 和 `[Langfuse] flush` 相关日志。

## 对外报告问题

提交模型调用问题时，至少附上：

- 失败接口、`X-Request-ID`、时间窗口。
- 模型类型、Provider、Base URL 主机名、模型名称。
- 测试接口返回的 `available` 和 `message`。
- 后端日志中对应的上游状态码和错误正文。
- 如果是 Embedding，附上测试接口返回的 `dimension` 和知识库期望维度。
- 如果开启了 `LLM_DEBUG_LOG` 或 Langfuse，只分享脱敏后的片段。

不要分享 API Key、AppSecret、完整用户对话、原始文档内容或未脱敏的 LLM 调试日志。

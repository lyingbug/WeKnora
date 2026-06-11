---
title: 模型供应商
description: 连接对话、Embedding、Rerank、VLM 和 ASR 供应商。
---

# 模型供应商

WeKnora 把模型配置作为租户级基础设施管理。一个租户可以同时配置多种模型类型和多个上游供应商，用于知识库构建、检索增强问答、Agent 推理、图片理解和语音识别。

模型配置的核心字段包括：

- 模型名称 `name`：运行时调用上游 API 使用的模型名。
- 展示名称 `display_name`：界面显示用，可选。
- 模型类型 `type`：决定模型用于哪条业务路径。
- 来源 `source`：本地 Ollama 或远程 API。
- 供应商 `parameters.provider`：决定使用哪套调用适配器。
- Base URL `parameters.base_url`：远程 API 地址。
- 凭据：`api_key`、`app_secret` 等密钥字段。
- 扩展参数：Embedding 维度、自定义请求头、供应商特定参数等。

## 模型类型

后端模型类型与前端展示类型的对应关系如下：

| 后端类型 | 前端类型 | 用途 |
| --- | --- | --- |
| `KnowledgeQA` | `chat` | 对话生成、知识库摘要、Agent 推理和答案合成。 |
| `Embedding` | `embedding` | 文档分块向量化和向量检索。 |
| `Rerank` | `rerank` | 对召回结果进行重排。 |
| `VLLM` | `vllm` | 视觉语言模型，处理图片理解。 |
| `ASR` | `asr` | 语音识别，把音频转为文本。 |

知识库通常至少需要一个 `KnowledgeQA` 模型；启用向量或关键词索引时还需要 `Embedding` 模型。Rerank、VLLM 和 ASR 是可选能力，只有在对应功能打开时才需要配置。

## 支持的供应商

模型供应商由后端 provider 注册表统一维护。`GET /api/v1/models/providers` 会返回所有可用供应商，传入 `model_type=chat|embedding|rerank|vllm|asr` 时会按模型类型过滤。

当前注册表包含：

- WeKnoraCloud
- Generic，OpenAI 兼容的自定义接口
- OpenAI
- Azure OpenAI
- Anthropic
- Google Gemini
- 阿里云 DashScope
- 智谱 AI
- 火山引擎 Ark
- 腾讯混元
- DeepSeek
- MiniMax
- 小米 Mimo
- OpenRouter
- 硅基流动
- Jina AI
- NVIDIA
- Novita AI
- GPUStack
- ModelScope
- 百度千帆
- 七牛云
- Moonshot
- LongCat AI
- 腾讯云 LKEAP

不同供应商支持的模型类型不同。前端新增模型时会先按当前模型类型加载可选供应商，再根据供应商填入默认 Base URL。

## 本地模型与远程模型

### 本地 Ollama

来源为 `local` 时，WeKnora 使用 Ollama：

- 前端会检查 Ollama 服务状态。
- 模型选择器会列出已下载模型。
- 如果输入了未下载的模型名，可以触发下载任务。
- 下载进度通过初始化接口查询。

本地模型适合开发、内网验证和低成本场景。Rerank 不支持 Ollama，本地来源在 Rerank 类型下会被禁用。

### 远程 API

来源为 `remote` 时，模型通过 Base URL 和供应商适配器调用上游 API。远程模型支持：

- 供应商选择。
- 模型名称。
- Base URL。
- API Key 或供应商特定凭据。
- 自定义 HTTP 请求头。
- 连接测试。

创建或测试远程模型时，后端会对 Base URL 执行 SSRF 校验，避免用户配置指向受保护的内部地址。

## 配置入口

### 管理界面

进入设置中的模型管理页，可以按类型查看模型：

- 全部
- Chat
- Embedding
- Rerank
- VLM
- ASR

管理员可以新增、编辑和删除普通模型。内置模型会显示锁标记，不能在界面中修改或删除。

新增模型时，编辑器会先选择模型类型，再选择来源和供应商。编辑模型时，模型类型由已有数据决定；凭据不在主表单里直接展示。

### API

模型 API 路由如下：

| 方法 | 路径 | 权限 | 用途 |
| --- | --- | --- | --- |
| `GET` | `/api/v1/models/providers` | Viewer | 获取模型供应商列表，可按 `model_type` 过滤。 |
| `GET` | `/api/v1/models` | Viewer | 获取当前租户模型列表。 |
| `GET` | `/api/v1/models/:id` | Viewer | 获取模型详情。 |
| `POST` | `/api/v1/models` | Admin | 创建模型。 |
| `PUT` | `/api/v1/models/:id` | Admin | 更新模型配置。 |
| `DELETE` | `/api/v1/models/:id` | Admin | 删除模型。 |
| `PUT` | `/api/v1/models/:id/credentials` | Admin | 写入或更新模型凭据。 |
| `DELETE` | `/api/v1/models/:id/credentials/:field` | Admin | 清除单个凭据字段。 |

初始化检测接口也和模型配置相关：

| 方法 | 路径 | 权限 | 用途 |
| --- | --- | --- | --- |
| `GET` | `/api/v1/initialization/ollama/status` | Viewer | 检查 Ollama 服务状态。 |
| `GET` | `/api/v1/initialization/ollama/models` | Viewer | 列出本地 Ollama 模型。 |
| `POST` | `/api/v1/initialization/ollama/models/check` | Admin | 检查 Ollama 模型是否存在。 |
| `POST` | `/api/v1/initialization/ollama/models/download` | Admin | 拉取 Ollama 模型。 |
| `GET` | `/api/v1/initialization/ollama/download/progress/:taskId` | Viewer | 查看下载进度。 |
| `POST` | `/api/v1/initialization/remote/check` | Admin | 测试 Chat 远程模型。 |
| `POST` | `/api/v1/initialization/embedding/test` | Admin | 测试 Embedding，并返回向量维度。 |
| `POST` | `/api/v1/initialization/rerank/check` | Admin | 测试 Rerank。 |
| `POST` | `/api/v1/initialization/asr/check` | Admin | 测试 ASR。 |
| `POST` | `/api/v1/initialization/multimodal/test` | Admin | 上传图片测试多模态能力。 |

## 凭据管理

模型主响应不会返回密钥明文。`api_key` 和 `app_secret` 会在数据库中加密保存，响应 DTO 会完全移除这些字段，只返回是否已配置的元数据。

凭据管理遵循以下规则：

- 创建模型时，可以在 `POST /models` 请求中带入初始 `api_key` 或 `app_secret`。
- 编辑模型时，`PUT /models/:id` 会保留已有密钥；即使请求体里误带密钥字段，也不会覆盖。
- 更新密钥必须使用 `PUT /models/:id/credentials`。
- 清除密钥必须使用 `DELETE /models/:id/credentials/:field`。
- 内置模型不能通过凭据子资源修改密钥。

普通供应商通常使用 `api_key`。WeKnoraCloud 和 LKEAP 等特殊供应商还会使用 `app_secret`。

## Embedding 配置

Embedding 模型必须配置向量维度：

- 前端允许范围是 128 到 4096。
- 远程 Embedding 可以通过测试接口调用一次 `Embed("hello")` 检测实际维度。
- 本地 Ollama Embedding 也可以通过检测按钮自动填充维度。

维度需要与向量库中已写入的向量一致。修改 Embedding 模型或维度后，已有知识库通常需要重新导入或重建索引，否则会出现向量维度不匹配或检索异常。

阿里云多模态 Embedding 在检测接口中会被拦截，建议使用纯文本 Embedding 模型。

## Chat 与思考参数

Chat 模型用于问答、摘要、Agent 推理和 Wiki 生成。远程 Chat 模型可配置：

- 供应商。
- 模型名称。
- Base URL。
- API Key。
- 自定义 HTTP 请求头。
- 是否支持图片输入。
- 思考模式参数格式。

思考模式参数会写入 `parameters.extra_config.thinking_control`，用于适配不同上游接口对“开启/关闭思考”的参数表达。例如某些模型使用 `chat_template_kwargs`，某些模型使用 `enable_thinking` 或 `thinking_type`。

`supports_vision` 只表示 Chat 模型是否可接收图片输入；VLM 类型模型会固定标记为支持视觉。

## Rerank 配置

Rerank 模型用于对召回结果排序。检测接口会用最小请求执行一次重排：

- query 为简单测试文本。
- documents 为一个短文本列表。
- 返回结果数量大于 0 才认为功能正常。

Rerank 不支持本地 Ollama。腾讯云 LKEAP Rerank 使用 `api_key` 保存 SecretId，使用 `app_secret` 保存 SecretKey，并把地域写入 `extra_config.region`。

## VLM 和 ASR

VLM 模型用于图片理解。知识库启用多模态解析或聊天启用图片输入时，会通过 VLM 模型分析图片内容。

ASR 模型用于音频识别。聊天附件或知识库导入音频时，可以通过 ASR 把音频转成文本。ASR 检测接口会发送一段极短的静默 WAV 音频测试转写端点。

## 自定义请求头

远程模型支持 `parameters.custom_headers`，用于给上游 API 附加额外 HTTP 请求头。典型用途包括：

- 企业网关鉴权。
- 路由标识。
- 追踪 ID。
- 代理层租户标识。

运行时会过滤保留头，避免覆盖 `Authorization`、`Content-Type` 等由 SDK 或适配器维护的关键请求头。

## 内置模型

WeKnora 支持通过 YAML 声明内置模型。默认路径是 `config/builtin_models.yaml`，也可以通过 `BUILTIN_MODELS_CONFIG` 指定路径。

启动时，后端会读取 YAML 并同步 `managed_by="yaml"` 的模型：

- YAML 中存在的模型会 upsert。
- 从 YAML 中移除的模型会软删除。
- 手动创建的模型不会被 YAML 同步逻辑修改。
- `is_builtin=true` 的模型会在界面中加锁。
- 内置模型不能通过界面或 API 更新、删除，也不能通过 credentials 子资源改密钥。

YAML 支持 `${ENV_NAME}` 形式的环境变量插值。未设置的变量会保留原字面量，方便在后续调用失败时暴露配置问题。

## 选择建议

- 生产环境优先把 Chat、Embedding、Rerank 分开配置，便于按成本和性能选择模型。
- Embedding 模型一旦用于知识库，应避免随意更换维度。
- 对 OpenAI 兼容服务或私有网关，使用 `generic` 供应商并配置 Base URL。
- 对需要企业网关透传的场景，使用自定义请求头，不要把网关令牌写进 Base URL。
- 对图片输入，优先明确配置 VLM；如果只是在 Chat 模型里打开 `supports_vision`，需要确认该 Chat 上游确实支持图片消息。
- 对内置模型，优先用 YAML 管理生命周期，避免在数据库中手工修改 `managed_by="yaml"` 的行。

## 常见问题

### 为什么模型列表看不到 API Key？

模型列表和详情响应不会返回密钥明文，只会返回 `credentials` 元数据表示是否已配置。编辑已有模型时，需要通过凭据组件单独更新或清除密钥。

### 为什么远程模型测试提示 Base URL 被拒绝？

后端会对 Base URL 做 SSRF 校验。如果地址指向受保护网段、危险端口或格式异常，检测和创建都会失败。应使用可从服务端安全访问的正式 API 地址。

### 为什么内置模型不能编辑？

内置模型由 YAML 文件管理。界面和 API 会阻止更新、删除和凭据修改，避免运行时手动改动被下次启动同步覆盖。

### 为什么 Embedding 维度必须填写？

向量库需要固定维度的向量字段或集合结构。维度缺失会导致索引创建和向量写入不可预测，因此前端要求在 128 到 4096 范围内显式填写。

### 为什么 Rerank 不能选本地模型？

当前本地 Ollama 路径只用于 Chat、Embedding、VLM 等模型调用，Rerank 没有本地 Ollama 适配器，因此前端会强制使用远程来源。

## 相关文档

- [模型配置](../user-guide/model-configuration.md)
- [知识库](../user-guide/knowledge-bases.md)
- [Agent 模式](../user-guide/agent-mode.md)
- [环境变量](../deployment/environment-variables.md)

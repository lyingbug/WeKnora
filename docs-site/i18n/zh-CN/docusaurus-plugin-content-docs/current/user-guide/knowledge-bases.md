---
title: 知识库
description: 创建、配置和管理 WeKnora 知识库。
---

# 知识库

知识库是 WeKnora 中组织知识、控制索引策略、绑定模型与存储后端、授权给用户或 Agent 的核心容器。文档、FAQ、数据源同步内容、Wiki 页面和图谱抽取结果都会挂在某个知识库下。

一个知识库至少包含：

- 基本信息：名称、描述、类型、创建者、所属租户。
- 入库配置：分块策略、解析引擎规则、图片处理、多模态、ASR、问题生成。
- 索引策略：向量检索、关键词检索、Wiki 生成、知识图谱抽取。
- 模型绑定：Embedding、摘要/生成、VLM、ASR 等模型。
- 存储和检索后端：对象存储 provider、可选的 VectorStore 绑定。
- 权限和共享状态：创建者、租户权限、组织共享、共享 Agent 可见性、个人置顶。

## 知识库类型

WeKnora 当前支持三类知识库。

| 类型 | 适合内容 | 主要能力 |
| --- | --- | --- |
| `document` | PDF、Word、Markdown、网页、表格、图片、音频等非结构化或半结构化内容 | 文档解析、分块、Embedding、关键词索引、图谱、Wiki、问答 |
| `faq` | 标准问答对、相似问法、负例、分类标签 | FAQ 管理、FAQ 检索、批量导入、优先回答 |
| `wiki` | 需要自动组织成互链知识页的资料集合 | 文档入库后自动生成 Wiki 页面、索引页、图谱和 Agent Wiki 工具 |

类型会影响可用配置。FAQ 类型会保留 `FAQConfig`，非 FAQ 类型会清空这类配置。Wiki 不只由 `type=wiki` 决定，真正的 Wiki 能力以 `IndexingStrategy.WikiEnabled` 为准。

## 创建知识库

创建知识库需要 Contributor 及以上权限。后端会自动补齐：

- `id`：未传时生成 UUID。
- `tenant_id`：来自当前请求上下文。
- `creator_id`：来自当前用户；API Key 的系统用户不会写入普通创建者 ID。
- 默认配置：例如默认索引策略、FAQ 默认索引配置、租户默认存储 provider。

创建时可以选择或配置：

- 知识库名称和描述。
- 知识库类型。
- Embedding 模型。
- 摘要/生成模型。
- 分块配置。
- 存储 provider。
- 可选 VectorStore。
- 索引策略。
- FAQ、Wiki、图谱、多模态、ASR 等高级配置。

如果创建时传入 `vector_store_id`，系统会校验它是否属于当前租户、是否已注册到检索引擎 registry、格式是否为 UUID。校验失败时会返回明确的 400 类错误，而不是等到检索时才失败。

## 索引策略

知识库的 `IndexingStrategy` 决定文档上传后会跑哪些处理链路。

| 字段 | 含义 | 是否需要 chunk |
| --- | --- | --- |
| `vector_enabled` | 启用语义向量检索。 | 是 |
| `keyword_enabled` | 启用关键词/BM25 检索。 | 是 |
| `wiki_enabled` | 启用 Wiki 页面生成。 | 是 |
| `graph_enabled` | 启用知识图谱抽取。 | 是 |

默认策略是开启向量检索和关键词检索，关闭 Wiki 和图谱。这保持了旧版本的默认行为。

更新知识库配置时，系统要求至少开启一种索引策略。如果开启 `wiki_enabled` 但还没有 `wiki_config`，后端会自动创建空的 Wiki 配置对象。如果开启 `graph_enabled`，后端会同步设置 `ExtractConfig.Enabled`，用于兼容旧图谱配置。

需要注意：

- 向量和关键词索引都需要 Embedding 模型。
- Wiki 和图谱不一定需要向量索引，但需要文档 chunk。
- 如果知识库只开启 Wiki 或图谱，普通 RAG 检索能力会受限；Agent 或图谱/Wiki 入口仍可使用对应能力。

## 分块和解析配置

分块配置保存在 `ChunkingConfig` 中，常用字段包括：

- `chunk_size`：普通分块大小。
- `chunk_overlap`：相邻分块重叠。
- `separators`：递归分割使用的分隔符。
- `parser_engine_rules`：按文件类型指定解析引擎。
- `enable_parent_child`：启用父子分块。
- `parent_chunk_size`：父块大小。
- `child_chunk_size`：子块大小。
- `strategy`：分块策略，可使用 legacy、auto、heading、heuristic、recursive 等。
- `token_limit`：按近似 token 限制块大小。
- `languages`：给启发式分块的语言提示。

`parser_engine_rules` 可以让不同格式走不同解析引擎。例如 PDF 走 MinerU，Markdown 走 simple。没有命中规则时默认使用内置解析能力。

父子分块适合长文档：小子块用于向量命中，大父块用于返回给模型作为上下文。短 FAQ、结构化表格或已经很短的文档通常不需要父子分块。

## FAQ 配置

FAQ 类型知识库使用 `FAQConfig`。默认值是：

- `index_mode = question_answer`
- `question_index_mode = combined`

含义如下：

| 配置 | 选项 | 说明 |
| --- | --- | --- |
| `index_mode` | `question_only` | 只索引问题和相似问题。 |
| `index_mode` | `question_answer` | 问题和答案一起参与索引，默认值。 |
| `question_index_mode` | `combined` | 主问题和相似问题合并索引，默认值。 |
| `question_index_mode` | `separate` | 主问题和相似问题分别索引。 |

FAQ 适合答案相对稳定、希望命中后直接返回标准口径的场景。FAQ 条目也可以带标签、相似问题、负例和多答案。批量导入、编辑和搜索测试在 FAQ 详情页完成。

## Wiki 配置

当 `IndexingStrategy.WikiEnabled` 开启时，文档解析后的后处理会排入 Wiki 入库任务。Wiki 配置位于 `WikiConfig`，常见字段包括：

- `synthesis_model_id`：用于 Wiki 抽取和页面合成的聊天模型；为空时回退到知识库摘要模型。
- `max_pages_per_ingest`：限制单次入库生成或更新的页面数量。
- `extraction_granularity`：抽取强度，支持 `focused`、`standard`、`exhaustive`。
- `ingest_batch_size`：Wiki worker 每批处理的文档数。
- `ingest_map_parallel`：Map 阶段并行度。
- `ingest_reduce_parallel`：Reduce 阶段并行度。

Wiki 生成是异步的。文档可能先显示为解析完成或 finalizing，随后 Wiki worker 继续生成 summary、entity、concept、index 和链接图谱。排障时可查看 Wiki stats、log、pending task 和 dead letter。

更多实现细节见 [Wiki 模式架构](../architecture/wiki-mode.md)。

## 图谱抽取配置

图谱能力由 `IndexingStrategy.GraphEnabled` 和 `ExtractConfig.Enabled` 共同决定。启用时，`ExtractConfig` 必须包含完整配置：

- `text`：抽取提示或规则说明，不能为空。
- `tags`：标签列表，不能为空。
- `nodes`：节点类型列表，名称不能为空且不能重复。
- `relations`：关系定义，必须引用已存在节点，并提供关系类型。

如果 `ExtractConfig.Enabled=false`，系统会把配置归一化为只保留关闭状态。启用图谱但配置不完整，创建接口会返回 400。

图谱适合需要实体关系查询、路径探索或结构化关系分析的知识库。它会增加解析后的异步处理成本，建议先在小样本上验证抽取 schema。

## 多模态和 ASR

图片、多模态和音频能力由知识库上的模型配置控制。

| 配置 | 用途 |
| --- | --- |
| `ImageProcessingConfig.ModelID` | 图片处理相关模型。 |
| `VLMConfig` | 图片/扫描件理解。新配置要求 `enabled=true` 且有 `model_id`；旧配置兼容 `model_name + base_url`。 |
| `ASRConfig` | 音频转写。要求 `enabled=true` 且有 `model_id`，可携带 `language` 作为语言提示。 |
| `QuestionGenerationConfig` | 解析后为 chunk 生成问题，默认每 chunk 可生成若干问题以提升召回。 |

如果扫描 PDF、图片或音频内容回答效果差，优先检查这些模型是否配置、解析结果里是否真的生成了文本，而不是只调整聊天模型。

## 存储 Provider

知识库可以选择对象存储 provider。存储 provider 只记录选择，具体凭据来自租户级 StorageEngineConfig。

支持的 provider 包括：

- `local`
- `minio`
- `cos`
- `tos`
- `s3`
- `oss`
- `ks3`
- `obs`

如果创建知识库时没有显式设置 provider，后端会使用租户默认 provider；租户没有配置时回退为 `local`。管理员可以通过 `STORAGE_ALLOW_LIST` 限制允许创建的 provider。该环境变量为空时允许全部支持项；不为空时只允许列表中的 provider。

跨存储后端复制知识库不受支持。如果源知识库和目标知识库实际使用不同存储后端，复制请求会被拒绝。

## VectorStore 绑定

知识库可以使用两种检索后端来源：

- 环境变量默认检索引擎：`vector_store_id` 为空时使用租户有效的 env store。
- DB 管理的 VectorStore：创建知识库时传入 `vector_store_id`，绑定某个租户内的向量库实例。

`vector_store_id` 是创建时绑定字段，创建后不可通过普通更新接口修改。这样做是为了避免同一知识库的 chunk 和 embedding 被写入一个后端、检索时却读另一个后端。

知识库列表和详情会返回向量库展示字段：

- `vector_store_name`
- `vector_store_source`
- `vector_store_engine_type`
- `vector_store_status`

如果当前用户是跨租户查看共享知识库，响应会隐藏原始 `vector_store_id` 和拥有方的存储名称，避免泄露对方租户的基础设施信息。

## 列表、详情和计数

知识库列表会按当前租户返回可见知识库，并补充：

- `knowledge_count`：文档型知识库的知识条目数量。
- `chunk_count`：FAQ 等以 chunk 为主要条目的数量。
- `is_processing`：是否有 pending 或 processing 状态的条目。
- `processing_count`：处理中条目数量。
- `share_count`：共享数量。
- `creator_name`：列表场景批量回填的创建者展示名。
- `capabilities`：根据类型和索引策略计算的能力标记。

`capabilities` 包含：

- `vector`
- `keyword`
- `wiki`
- `graph`
- `faq`

Agent 编辑器会使用这些能力过滤可选知识库。例如 RAG 工具需要 vector 或 keyword，Wiki 工具需要 wiki。

## 置顶和排序

知识库置顶是每个用户自己的状态，不再是整个租户共享的排序字段。置顶记录按 `(tenant, user, kb)` 存储。

置顶接口只要求 Viewer 及以上并且对该知识库有读权限。也就是说，通过共享 Agent 或组织共享可见的知识库，用户也可以为自己置顶。列表排序会让置顶知识库排在前面，多个置顶按置顶时间倒序。

API Key 调用如果没有真实用户身份，不能创建个人置顶状态。

## 权限和共享

知识库访问受租户 RBAC、创建者、组织共享和共享 Agent 共同影响。

常见规则：

- 创建知识库：Contributor 及以上。
- 列表：Viewer 及以上。
- 详情：Viewer 及以上，并且对目标知识库有读权限。
- 更新：创建者本人或 Admin，并且有写权限。
- 删除：只允许拥有该知识库的租户侧执行；共享视图不能删除源知识库。
- 混合搜索：Viewer 及以上，并且有读权限。
- 置顶：Viewer 及以上，并且有读权限。
- 复制：Contributor 及以上，源知识库必须属于当前租户。

跨租户共享知识库时，检索和详情读取会使用知识库拥有方租户作为 effective tenant，确保模型、向量库和内容读取在正确租户范围内完成。

## 复制和移动

复制知识库是异步任务。复制到已有目标知识库时，系统会先做预检：

- 源和目标必须属于当前租户。
- Embedding 模型必须一致。
- VectorStore 绑定必须一致。
- 如果租户配置了存储引擎，源和目标的实际存储 provider 必须一致。

这些限制是为了避免复制后混用不兼容的向量空间、跨后端移动物理向量或跨对象存储复制文件。

移动文档到其他知识库时，目标候选列表会过滤为：

- 同一类型。
- 相同 Embedding 模型。
- 不是当前知识库本身。
- 非临时知识库。

## 删除知识库

删除知识库会先软删除知识库记录，然后排入低优先级异步任务做重清理。清理内容包括：

- 删除知识条目。
- 删除 chunk。
- 删除向量索引。
- 删除上传文件和抽取图片。
- 回收租户存储用量。
- 删除图谱数据。
- 删除组织共享关系。

删除请求返回成功不代表所有物理资源已经立即清理完毕；重清理由后台任务完成。若清理任务中发现 VectorStore 绑定已经失效，会跳过不可恢复重试，避免队列反复消耗。

## 使用建议

按以下边界拆分知识库通常更稳定：

- 按访问权限拆分：不同团队、客户、组织共享边界不要混在一个知识库。
- 按回答领域拆分：产品文档、售后 FAQ、内部制度、研发规范分别管理。
- 按内容形态拆分：FAQ 与长文档分开，便于使用不同分块和索引配置。
- 按生命周期拆分：高频更新资料和稳定归档资料分开，降低重解析和重建索引成本。
- 按检索后端拆分：需要不同 VectorStore、不同 Embedding 模型的内容不要放进同一个知识库。

如果回答质量不好，先检查知识库配置和入库产物：解析文本、分块边界、索引策略、Embedding 模型、FAQ 命中、Wiki/图谱是否已完成。不要只从聊天模型开始排查。

相关页面：

- [文档入库](./document-ingestion.md)
- [模型配置](./model-configuration.md)
- [Wiki 模式](./wiki-mode.md)
- [检索链路架构](../architecture/retrieval-pipeline.md)

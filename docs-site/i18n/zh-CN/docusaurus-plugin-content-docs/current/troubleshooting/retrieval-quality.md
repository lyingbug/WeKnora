---
title: 检索质量
description: 改善搜索和回答质量。
---

# 检索质量

检索效果差通常来自知识领域混杂、解析质量弱、分块不合适、元数据缺失或模型不匹配。
排查时要先判断问题发生在“召回不到”、“召回到了但排序差”，还是“证据进了上下文但回答仍然差”。这三类问题对应不同的源码链路和调参方式。

## 先做纯搜索验证

不要一开始就用完整问答判断检索质量。完整问答会叠加 QueryUnderstand、Web Search、Rerank、Merge、上下文模板和 LLM 生成，容易把问题混在一起。

先用不经过 LLM 总结的搜索接口验证召回：

```bash
curl -X POST http://localhost:8080/api/v1/sessions/search \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -H "X-Request-ID: retrieval-debug-001" \
  -d '{
    "query": "要排查的问题",
    "knowledge_base_ids": ["<kb-id>"]
  }'
```

如果这个接口没有结果，问题通常在知识库范围、索引、Embedding、关键词索引或阈值。如果纯搜索有结果，但问答没有引用或回答不对，再看 Rerank、Merge 和最终 Prompt。

也可以直接调用知识库混合搜索接口，显式控制阈值和候选数：

```bash
curl -X POST http://localhost:8080/api/v1/knowledge-bases/<kb-id>/hybrid-search \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -H "X-Request-ID: hybrid-debug-001" \
  -d '{
    "query_text": "要排查的问题",
    "vector_threshold": 0.15,
    "keyword_threshold": 0.3,
    "match_count": 20
  }'
```

## 看日志分段定位

开启 `LOG_LEVEL=debug` 后，检索链路会留下比较清晰的阶段日志。优先按同一个 `X-Request-ID` 搜索这些关键字：

| 阶段 | 关键字 | 看什么 |
| --- | --- | --- |
| 目标解析 | `SearchTargets`、`embedding_groups`、`group_plan` | 是否搜到了预期知识库，是否按 Embedding 模型分组。 |
| HybridSearch | `Hybrid search parameters`、`Starting multi-store retrieval` | query、KB 范围、store group 数量是否正确。 |
| 底层召回 | `Retrieval results` | 每个引擎、每种召回方式返回多少条。 |
| 融合 | `Result count before fusion`、`Result count after RRF fusion` | 向量和关键词各有多少结果，融合后是否被去重。 |
| 结果组装 | `Search results processed` | 回表后最终可返回结果数。 |
| Query 扩展 | `recall_low`、`expansion_hits` | 低召回时是否触发本地扩展搜索。 |
| Rerank | `Rerank`、`top_score`、`fallback_top1` | Rerank 分数、阈值和降级情况。 |
| Merge | `Merge`、`parent_resolve`、`need_expand`、`final_dedup` | 上下文是否被父块、相邻块、去重逻辑改写。 |
| Prompt | `IntoChatMessage`、`merge_result_cnt`、`template_len` | 最终有多少上下文进入 LLM。 |

如果启用了 Langfuse，可以在 Trace 里看 `retrieve`、模型改写、Rerank 和最终生成的耗时与输入输出。

## 没有召回

没有召回时，按下面顺序检查。

### 知识库是否可检索

确认文档状态已经是 `completed`，或至少主解析流程已完成。`pending`、`processing`、`failed`、`deleting` 状态都可能导致无法检索。

知识库必须启用可检索索引：

- 向量召回要求知识库启用 vector indexing，并配置可用 Embedding 模型。
- 关键词召回要求知识库启用 keyword indexing。
- Wiki-only 或 graph-only 知识库不会自动走普通 HybridSearch；普通搜索可能返回空。
- FAQ 知识库走 FAQ 向量索引，不走文档关键词索引。

如果知识库刚改过索引策略或 Embedding 模型，旧文档通常需要重新解析或重建索引。

### 搜索范围是否正确

多知识库检索会逐个做权限校验；跨租户共享知识库必须有可读权限。范围不对时，系统可能返回“没有结果”，而不是泄露未经授权知识库是否存在。

还要确认 Agent 的知识库选择策略。Agent 可以选择所有知识库、指定知识库、用户提及的知识库或具体知识条目。如果 Agent 设置为“仅提及时检索”，用户没有提及知识库时可能不会搜到预期内容。

### Embedding 模型是否一致

多知识库搜索要求这些知识库处在同一个 Embedding 模型空间。源码里会按模型名称和 Base URL 解析实际模型身份，而不只是比较模型 ID。不同模型或不同 endpoint 产生的向量分数不可直接比较，系统会拒绝这类混搜。

常见问题：

- 同名模型在不同供应商 endpoint 上配置了两个记录。
- 复制知识库后换了 Embedding 模型，但没有重建索引。
- 共享知识库来自其他租户，当前租户的同名模型并不等价。

### 阈值是否太高

全局默认检索配置是：

| 参数 | 默认值 | 作用 |
| --- | --- | --- |
| `embedding_top_k` | `50` | 进入召回阶段的目标候选数量。 |
| `vector_threshold` | `0.15` | 向量召回最低相似度。 |
| `keyword_threshold` | `0.3` | 关键词召回最低分。 |
| `rerank_top_k` | `10` | Rerank 后保留的上下文数量。 |
| `rerank_threshold` | `0.2` | Rerank 最低相关分。 |

Agent 可以覆盖这些值，前端 Agent 编辑器默认常见值更严格，例如 `vector_threshold=0.5`、`rerank_threshold=0.5`。如果纯搜索或 Agent 搜索结果很少，先把向量阈值和关键词阈值降到全局默认附近，再逐步收紧。

## 召回不相关

召回不相关通常不是单个参数能解决。先看命中结果的 `match_type` 和 `chunk_type`：

- `embedding`：向量语义命中。
- `keywords`：关键词命中。
- `parent_chunk`、`nearby_chunk`、`relation_chunk`：上下文扩展带来的结果。
- `history`：历史轮次引用。
- `direct_load`：指定知识条目时直接加载。

如果主要是 `embedding` 命中但语义偏离，优先检查 Embedding 模型语言能力、文档解析质量和分块粒度。  
如果主要是 `keywords` 命中但上下文噪声多，优先检查关键词阈值、文档中重复术语、知识库是否混入无关材料。

## 分块问题

WeKnora 默认分块大小是 `512` 字符，默认 overlap 是 `80` 字符。这个配置适合多数普通文档，但不是所有材料都适合。

| 现象 | 可能原因 | 调整方向 |
| --- | --- | --- |
| 需要的答案总在相邻分块里 | 分块过小或 overlap 太小。 | 增大 chunk size 或 overlap；启用父子分块。 |
| 召回结果太长、主题混杂 | 分块过大。 | 降低 chunk size；按章节或业务域拆文档。 |
| 标题下的内容失去上下文 | 解析或分块没有保留标题语义。 | 检查解析 Markdown；使用带标题上下文的分块策略。 |
| 表格内容搜不到 | 表格被解析成低质量文本或被错误切断。 | 检查原始解析结果；调整解析器或分块策略。 |
| 图片类文档搜不到 | OCR / caption 没有产出可检索文本。 | 检查多模态阶段和 VLM/OCR 配置。 |

源码里分块会把 `ContextHeader` 只用于 Embedding，不直接写入存储的 `Content`，这样可以让向量看到章节标题，又不破坏原文位置。排查时不要只看返回的 chunk 文本，也要检查解析后的标题层级是否正确。

## 向量和关键词的取舍

HybridSearch 会同时构造向量和关键词检索参数。召回后如果只有一种结果，就按分数去重；如果向量和关键词都有结果，会使用 RRF 融合：

```text
RRF score = vectorWeight / (k + vectorRank) + keywordWeight / (k + keywordRank)
```

默认 RRF 参数是：

| 参数 | 默认值 |
| --- | --- |
| `rrf_k` | `60` |
| `rrf_vector_weight` | `0.7` |
| `rrf_keyword_weight` | `0.3` |

调参建议：

- 问题更偏概念、语义、同义表达：提高向量召回占比，降低过高的 `vector_threshold`。
- 问题更偏错误码、产品型号、函数名、字段名：确保关键词索引启用，适当提高关键词权重。
- 关键词命中大量模板页或重复页：提高 `keyword_threshold`，或拆分知识库。
- 两路都能命中但排序不稳定：启用 Rerank，并观察 Rerank 前后的 top 结果。

注意：RRF 分数不是 0 到 1 的语义相似度。Agent 工具里也有注释说明，RRF 后的分数范围很小，不应该再用旧的 `min_score` 方式硬过滤。

## Rerank 后结果变差

Rerank 失败时，聊天管线会回退到原始召回结果；但 Rerank 成功后，如果阈值或 passage 质量不合适，也可能让好结果被过滤。

排查顺序：

1. 先确认 Rerank 模型本身通过 `/initialization/rerank/check`。
2. 查看日志里的 `Rerank input`，确认候选数量不是 0。
3. 查看 `top_score`，确认模型分布是否整体偏低。
4. 如果出现 `threshold_degrade` 或 `fallback_top1`，说明原阈值可能过高。
5. 如果 `empty_passage_skip` 很多，说明候选文本清洗后为空，可能是图片、表格、链接或 Markdown 噪声问题。
6. 如果内容重复很多，观察 MMR 的 `avg_redundancy`，必要时降低 `rerank_top_k` 或改善分块。

Rerank 传给模型的 passage 会清理 Markdown 图片、URL、HTML 标签、代码块、表格分隔符等噪声，并把图片 OCR / caption 等文本拼入 passage。若原文主要是代码块、表格或图片，清洗后内容可能与预期不同。

## 上下文合并问题

纯搜索结果正确，但回答引用缺失或上下文不完整时，重点看 Merge 和 IntoChatMessage。

聊天管线的 Merge 会：

- 优先使用 Rerank 结果；Rerank 为空时按召回分数回退。
- 按 chunk ID 和内容签名去重。
- 注入历史轮次中仍相关的知识引用。
- 对父子分块命中解析父 chunk。
- 合并同一文档内位置重叠的片段。
- 对 FAQ chunk 补答案。
- 对过短文本 chunk 拉取前后相邻 chunk，扩展到约 350 到 850 字符。
- 最后移除被高分片段大量包含的弱重复内容。

因此，如果纯搜索返回很多结果，但最终回答只引用少量内容，可能是 Merge 去重、MMR 或 `rerank_top_k` 在起作用。看日志里的 `Merge input`、`candidate_ready`、`group_output`、`final_dedup` 和 `IntoChatMessage input`。

## FAQ 检索

FAQ 知识库有独立路径：

- FAQ 使用向量索引，不走文档关键词索引。
- Rerank 后可以对 FAQ chunk 做 boost。
- 高置信 FAQ 可能在上下文模板里被标记为高优先级。
- FAQ 搜索页面可以单独传 `vector_threshold` 和 `match_count` 测试。

如果 FAQ 问答不准，先在 FAQ 管理页面搜索同一个问题，确认标准问、相似问、反例和答案是否配置正确。相似问不足时，Embedding 可能无法把用户问法拉近；反例缺失时，相近但不应回答的问题可能误命中。

## 多知识库与共享知识库

多知识库检索会按 `(vector_store_id, owner tenant)` 分组并发检索。默认每次最多并发 4 个 store group，每组默认超时 30 秒，可通过下面环境变量调整：

```bash
MULTI_STORE_RETRIEVE_TIMEOUT_SEC=30
```

如果某个绑定的向量库不可用，当前策略是整次搜索失败，而不是只返回其它知识库的部分结果。日志里会看到 `multi-store retrieve failed` 或向量库不可用错误。

跨多个检索引擎时，系统会对向量分数做归一化；关键词分数不做归一化，因为后续 RRF 使用排名融合。

## 推荐排查顺序

1. 用 `/sessions/search` 或 `/knowledge-bases/:id/hybrid-search` 验证纯召回。
2. 如果没有召回，检查文档状态、索引开关、Embedding 模型、向量库、权限和阈值。
3. 如果召回弱相关，检查解析文本、分块、知识库范围、向量/关键词权重。
4. 如果召回正确但问答差，检查 QueryUnderstand、Rerank、Merge、ContextTemplate 和 LLM 生成。
5. 如果只在 Agent 下差，检查 Agent 覆盖的 `embedding_top_k`、`vector_threshold`、`keyword_threshold`、`rerank_top_k`、`rerank_threshold`、改写开关和知识库选择策略。

对外反馈问题时，附上纯搜索返回的前 5 条结果、完整问答的 `X-Request-ID`、相关日志里的 Search/Rerank/Merge 阶段摘要，以及知识库的分块和检索配置。

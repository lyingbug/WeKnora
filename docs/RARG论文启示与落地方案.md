# RARG 论文对 WeKnora 的启示与落地方案

## 论文信息

- 标题：*A New Role for Relevance: Guiding Corpus Interaction in Agentic Search*
- 作者：腾讯（Jiangnan Li、Mo Yu、Jinchao Zhang、Jie Zhou）与中科院信工所（Yuqing Li）
- 链接：<https://arxiv.org/abs/2607.24223> ／ 代码：<https://github.com/LeqsNaN/RARG>

## 一句话结论

**相关性分数不应该只用来决定"把哪些内容塞给大模型"，还应该用来决定"关键词搜索先扫哪些文档、先把哪些匹配显示出来"。**

WeKnora 的 `grep_chunks` 工具在架构上恰好就是论文里被批评的 DCI（Direct Corpus Interaction）：它能做精确的正则匹配，但完全不知道哪些文档更值得先看。本文档记录论文的核心机制、与 WeKnora 现状的逐条对照、已经落地的改动，以及建议后续推进的方向。

---

## 一、论文在解决什么问题

搜索智能体面对十万级文档时有两条路，各有硬伤：

| 路线 | 做法 | 硬伤 |
| --- | --- | --- |
| 检索式（RAG / Retrieval Agent） | 向量检索取 top-k，把内容喂给模型 | 相关文档里的关键证据可能只占几行，切片时容易漏掉；多跳问题里，一份文档的价值要等到发现中间实体后才显现 |
| 直接语料交互（DCI） | 把原始文档暴露给智能体，用 `rg`／`Read` 反复搜索 | 关键词匹配不理解问题，把所有文档视作同等值得扫描；一次命中几百条时输出被截断，真正有用的那几行可能根本没机会显示 |

RARG 的主张是把相关性当作**执行先验（execution prior）**而非内容通道，分两个粒度：

1. **文档级相关性 → 决定遍历顺序。** 新增 `embed_recall(scope_query)` 工具，它**不返回文档内容**，只把按相关性排好序的文档路径写进 `/tmp/scope_N.txt`，然后引导模型用 `cat /tmp/scope_N.txt | xargs -d '\n' rg -j1 "PATTERN"` 按序扫描。`-j1` 是关键细节：`rg` 默认多线程并行读文件，输出顺序取决于线程结束时间，会直接打乱注入的排名。
2. **匹配级相关性 → 决定可见性。** 一次 `rg` 最多收集 M=500 条匹配，用嵌入模型重排后只展示 top m=30 条。重排 query 由规则拼接而成：`Query: [scope query]` + `RG focus: [从正则里提取的关键词]`——既表达全局目标，又表达本次扫描的局部意图。

中间还有一层 **RARG+（入口初始化）**：从排名靠前的文档里切出 400–1000 字符的段落，用嵌入模型选出 top-10 直接附在 scope 清单后面，给模型一个"第一条精确搜索词该怎么写"的起点。

### 实验结论（BrowseComp-Plus，10 万文档，GPT-5.4-mini）

| 方法 | 准确率 | 平均工具调用 | 平均回合 |
| --- | --- | --- | --- |
| DCI（纯 grep） | 78% | 99.1 | 48.8 |
| RISE（BM25 构建交互空间） | 78% | 28.7 | 24.3 |
| RARG | 80% | 29.8 | 18.2 |
| RARG+ | 81% | 29.6 | 20.2 |
| **RARG++** | **84%** | **23.9** | **17.6** |

换成 GPT-5.4 时 RARG++ 达到 91%，比 RISE 高 9 个点。语料扩到 100 万文档后 RARG++ 保持 79%，仍领先 RISE-BM25 十个点。

两个值得注意的对照：

- 把 RISE 的 BM25 换成同一个 Qwen3-Embedding-4B 后准确率**反而降到 69%**。说明收益不是来自"换了更强的嵌入模型"，而是来自"让排名去控制搜索顺序"。
- 智能体平均只调用 `embed_recall` 1.2–1.6 次，后续探索全部由范围内的 `rg` 完成。相比之下 RISE 每次 Search 都会返回文档片段，等于把检索同时当成信息通道，反而**激励模型反复调用检索**（13.1 次）。

---

## 二、WeKnora 现状对照

### 2.1 我们已经有 DCI 了，而且是"相关性无感"的

WeKnora 的 Agent 工具集里，`grep_chunks` 就是 DCI 的对应物：用 PostgreSQL `~*` / MySQL `REGEXP` 直接对 chunk 内容做正则匹配。改动前它的行为是：

```go
// internal/agent/tools/grep_chunks.go（改动前）
const maxFetchLimit = 500
query.Order("chunks.created_at DESC").Limit(maxFetchLimit).Find(&results)
```

这一行同时踩中了论文点名的两个问题：

1. **候选池按创建时间截断。** 一个宽泛的正则（例如按产品名做 `|` 分支）在几万 chunk 的知识库上轻易匹配上千条，但只有最新的 500 条会被取出来。一份两年前写的、正好包含答案的文档，无论正则写得多好都进不了候选池——这就是论文说的"相关文档若被较晚扫描，智能体连看到它的机会都没有"。
2. **500 → 30 的收敛纯靠词频。** `calculateMatchScore` 只统计匹配次数和最早出现位置，`scoreChunks` 再加一个标题命中的加成。一份反复提到某关键词的长 FAQ 会稳定压过那一段真正回答了问题、但只提了一次该词的正文。

`grep_chunks` 的 `maxFetchLimit = 500` 和输出上限 `limit = 30` 与论文的 M=500／m=30 **完全一致**——只是从 500 挑 30 的那一步，论文用嵌入重排，我们用词频。

### 2.2 我们的语义检索反而更接近论文批评的 RISE

`knowledge_search` 把完整 chunk 内容（`<content>` 里是 `result.Content` 全文，没有单条字符上限）直接返回给模型，只有工具层 24000 runes 的整体截断兜底。这正是论文分析 RISE 时指出的模式：*"every space-constructing Search also returns snippets of the top documents, so retrieval simultaneously serves as an information channel to the model. This has the risk of incentivizing issuing more Search calls, since each one directly supplies content."*

论文的 `embed_recall` 刻意**不返回内容**，把 Search 调用次数从 13.1 压到 1.2–1.6。

### 2.3 两个工具之间没有任何信息传递

改动前，`knowledge_search` 辛辛苦苦算出来的相关性排序，在它自己返回之后就被丢弃了。同一轮对话里紧接着运行的 `grep_chunks` 对此一无所知，只能从头按创建时间扫。这就是论文说的"relevance is used primarily to construct the space, but is not explicitly propagated to order local traversal"。

---

## 三、已落地的改动

核心是引入一个**每次 Agent 运行共享一份**的 `RelevanceScope`（`internal/agent/tools/relevance_scope.go`），由 `ToolRegistry` 持有——它是唯一生命周期恰好等于一次 Agent 运行、且工具注册和引擎主循环都能拿到的对象。

```
用户提问 ──► RelevanceScope.SetUserQuery      (engine.Execute)
                     │
knowledge_search ────┼──► RecordRankedDocuments(重排后的文档)   ← 最可信，但只有几条
                     ├──► RecordRankedDocuments(重排前的候选池) ← 上百条，同样按相关性排序
                     │
grep_chunks ─────────┼──► RankedDocuments() ──► 遍历顺序
                     └──► Query()           ──► 匹配级重排的 query
```

### 3.0 scope 的宽度

论文的 scope 是全库嵌入排序后的前 10000 条路径（占 10 万文档的 10%）。WeKnora 里如果只记录 `knowledge_search` 最终输出的那几条（受 `EmbeddingTopK` 限制，默认 5），排序面太窄，起不到引导遍历的作用。

因此记录分两层：**重排后的结果**排在最前（最可信），紧接着是**重排前的候选池**——`HybridSearch` 内部本来就会过取到 `max(EmbeddingTopK*5, 50) * KB数`（上限 500）条，这批文档同样按相关性排过序，复用它零额外成本。一份"只是不值得引用"的文档，终归比一份检索压根没浮现过的文档更值得先 grep。两层通过 `RecordRankedDocuments` 的"保留最优排名"语义自然叠加，scope 宽度从个位数扩到上百份。

### 3.1 文档级：候选池按相关性排序（对应 RARG）

`fetchRelevanceOrdered` 把单次取数拆成两趟：

- **第一趟**：限制在已排序文档内，用 `CASE chunks.knowledge_id WHEN ? THEN ? ... END` 把排名带进 SQL 的 `ORDER BY`，文档 ID 全部以参数绑定。
- **第二趟**：用剩余预算扫描排名之外的文档，仍按创建时间。

**排序只用来引导，不用来过滤。** 这一点是对论文的有意改造：论文的 scope 是对全库做嵌入排序后取前 10000 个路径（占 10 万文档的 10%，范围召回率 95–97%）；而 WeKnora 的 scope 来自 `knowledge_search`，只有几十份文档。在一个四万文档的 Wiki 知识库上把 grep 硬限制在几十份文档里，召回会崩掉——而"找到语义检索漏掉的那个字符串"恰恰是 grep 存在的理由。因此第二趟必须保留。

顺带修掉了一个隐蔽问题：原来的排序没有 tie-break，同一排序键下的 chunk 由存储引擎决定顺序，同一条命令重跑可能给模型看到不同的证据。现在补上了 `knowledge_id, chunk_index, id` 的确定性 tie-break——相当于论文里 `-j1` 解决的那个"并行读取打乱注入排名"的问题在 SQL 层的对应物。

> 实现坑：GORM 的 `Order()` 只接受 `string`、`clause.OrderBy`、`clause.OrderByColumn`，传 `clause.Expr` 会被**静默丢弃**。必须包成 `clause.OrderBy{Expression: expr}`，且 tie-break 要拼进同一个表达式里（`OrderBy` 设了 `Expression` 后 `Columns` 不再生效）。

### 3.2 匹配级：语义重排（对应 RARG++）

`applyMatchRelevance`（`internal/agent/tools/grep_chunks_relevance.go`）在候选数超过输出上限时，对前 120 条按论文的构造式 query 做重排：

```
Query: <scope query：本轮语义检索的 query，没有则回退到用户原问题>
Search focus: <从正则里按规则提取的字面关键词>
```

`regexFocusKeywords` 按 `|` 拆分支，剥掉 `\b`、`\d`、字符类、量词、锚点等元字符，只留字面词；单个拉丁字母丢弃，单个汉字保留（一个汉字的信息量远大于一个字母）。

与论文的差异：论文直接用重排结果替换排序，我们**按 0.6 语义 + 0.4 词频加权**。原因是 WeKnora 的 grep 有大量"查错误码、查工单号、查产品 ID"的用法，这类场景下词频命中就是全部信号，语义分说明不了什么。权重保留让这类查询不被翻盘。

三条保护：没有配置 rerank 模型时不做；候选数已经装得下输出预算时不做（重排改变不了模型能看到什么，纯属加延迟——与论文 "Reranking is skipped when the number of matches is below m" 一致）；重排调用失败时降级回词频排序而不是丢结果。池大小可用 `WEKNORA_GREP_RERANK_POOL` 覆盖，设为 0 即完全关闭。

论文用 M=500 是因为它跑本地嵌入模型；WeKnora 的 reranker 大多是远程 API，所以默认池收到 120，走"词法粗排 → 语义精排"的级联。

### 3.3 顺带：告诉模型它没看全

输出里新增一行：

```xml
<scan_summary candidates="1843" shown="30" note="Matches were ranked by relevance to the current question; narrow the regex if the answer is not among them." />
```

改动前模型无从知道这 30 条是从 1843 条里挑的，很容易把"没搜到"当成"知识库里没有"。

---

## 四、建议后续落地（按优先级）

### P1：给 `knowledge_search` 增加"只建范围、不返回内容"的模式

这是论文收益最大、我们**尚未**落地的一块。论文把 Search 调用从 13.1 次压到 1.2–1.6 次，靠的就是让检索不再兼任信息通道。

具体可以给 `knowledge_search` 加一个 `scope_only` 参数，或者新增一个 `build_search_scope` 工具：只回 `<scope documents="137" query="..."/>` 加一份文档标题清单（对应论文的 scope 文件），内容一律不返回，让模型改用 `grep_chunks` / `list_knowledge_chunks` 去取证据。

注意这一项和 3.0 里已经做掉的"scope 宽度"是两件事：宽度问题（scope 只有几份文档）已经通过复用重排前候选池解决了；这里要解决的是**内容通道**问题——`knowledge_search` 每次都返回完整 chunk 正文，会持续激励模型重复调用它。

### P2：入口段落初始化（RARG+）

论文里这一步对"何时第一次看到决定性线索"影响最大——案例中把关键简历的出现从第 7 回合提前到第 2 回合，而且**不需要额外的大模型回合**。

在 WeKnora 里对应：`knowledge_search`（或上面的 scope 工具）在返回排序的同时，附带 10 段来自 top 文档的 400–1000 字符片段。注意 WeKnora 已有 parent-child 分块，父块正好是天然的段落单元，`SplitParentChild` 的产物可以直接复用，不用重新切。

值得一提的是，论文在 BRIGHT 上的最佳成绩是 RARG+（53.36）而不是 RARG++（50.55）——入口初始化在"要广泛召回"的任务上比匹配级重排更稳。

### P3：区分"问答"和"盘点"两类任务的收敛策略

论文最有工程价值的一条负面结论：RARG++ 在 QA 上最好（84%），在 BRIGHT 排序任务上**最差**（50.55，低于不做匹配重排的 51.75）。原因是匹配级重排让智能体更快收敛到少数强匹配，问答只需要一条闭合的证据链，这是优点；而排序任务要尽量找全相关候选，过早收敛就损失覆盖面。

映射到 WeKnora：

| 任务 | 形态 | 应该的策略 |
| --- | --- | --- |
| RAG 问答 / Agent 问答 | 深度优先 | 开匹配级重排，证据链闭合即停 |
| Wiki 自动生成、文档盘点、"列出所有提到 X 的文档" | 广度优先 | 关掉或调低匹配级重排，保留更宽的文档覆盖，停止条件更保守 |

论文原话：*"工程上不宜把 RARG++ 当成固定升级项。"* 建议把 `WEKNORA_GREP_RERANK_POOL` 这个全局开关升级成按 Agent 类型（`config/agent_type_presets.yaml` 里已经区分了 `wiki-qa` / `hybrid-rag-wiki` 等）配置。

### P4：先埋点，再谈收益

论文自己在 Limitations 里承认：只报告了工具调用次数和估算成本，**没有给出端到端时延对比**。`rg -j1` 的顺序扫描和 500 条匹配的嵌入重排都会增加单次搜索延迟，"调用更少"不等于"总耗时更短"。

我们的对应风险：两趟 SQL 取数 + 一次远程 rerank 调用。上线前至少要在 Langfuse 里记录三个量——`grep_chunks` 端到端耗时、两趟 SQL 各自耗时、rerank 耗时。WeKnora 已有 Langfuse OTLP 链路，加 span 属性的成本很低。

### P5：模型指令遵循度是前提

论文里 GPT-5.4-nano 的 scope 召回率明显低于 mini，`embed_recall` 被反复触发（9.1 次 vs 1.2 次），"先建范围、再范围内搜索"的协议它守不住。WeKnora 支持 20+ 家模型供应商，其中不乏能力弱于 nano 的。如果后续做 P1 的显式 scope 协议，必须在**服务端**兜底（比如服务端自动维护 scope，而不是依赖模型正确串联工具调用）——这也正是当前实现的做法：`RelevanceScope` 由服务端自动填充和消费，模型完全无感，不需要它遵守任何新协议。

---

## 五、论文的边界（哪些不要照搬）

1. **依赖嵌入模型在两个粒度上都可靠。** 论文在 BRIGHT 上发现 NV 模型排长文档很好、排短片段很差，被迫在匹配重排上换成 Qwen3-Embedding-4B。WeKnora 的租户可以自由配置 embedding / rerank 模型，不能假设同一个模型两个粒度都行——这也是当前实现保留 40% 词法权重的另一个理由。
2. **长文档噪声无法根治。** 语料从 10 万扩到 100 万（新增的都是长文档）后，范围召回率几乎没掉（95.4%），但 `rg` 覆盖率从高位掉到 75.9%——长文档制造了大量偶然同词的匹配。匹配级重排只能部分缓解。WeKnora 用的是 chunk 而不是整篇文档做匹配单元，粒度更细，这个问题会比论文场景轻，但不会消失。
3. **让大模型自己生成重排 query 是负优化。** 论文测过 `RerankAwareBash(command, rerank_query)` 这个变体：收敛最快（17.8 次工具调用）但准确率掉到 75%。作者归因于"额外要求模型输出重排 query，改变了它原本熟悉的 Bash/rg 行为"。所以当前实现坚持用规则构造 query，不给 `grep_chunks` 增加参数。
4. **验证范围有限。** 论文只测了 GPT-5.x 系列、固定本地语料和两个基准，开放网页和其他模型上的表现待验证。

---

## 六、改动清单

| 文件 | 内容 |
| --- | --- |
| `internal/agent/tools/relevance_scope.go` | 新增 `RelevanceScope`：每轮共享的相关性先验，线程安全 |
| `internal/agent/tools/grep_chunks_relevance.go` | 新增匹配级重排、正则关键词提取、构造式 query |
| `internal/agent/tools/grep_chunks.go` | 候选池改为相关性引导的两趟取数；接入匹配级重排；输出 `<scan_summary>` |
| `internal/agent/tools/knowledge_search.go` | 把排序后的文档 ID 写入 scope |
| `internal/agent/tools/registry.go` | `ToolRegistry` 持有 scope |
| `internal/agent/engine.go` | `Execute` 时把用户原问题写入 scope |
| `internal/application/service/agent_service.go` | 给两个工具注入共享 scope，给 `grep_chunks` 注入 reranker |

配置项：`WEKNORA_GREP_RERANK_POOL`（默认 120，设为 0 关闭匹配级重排）。

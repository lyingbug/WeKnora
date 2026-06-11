---
title: 向量库
description: 配置和扩展向量数据库后端。
---

# 向量库

向量库负责保存文档分块的 embedding，并在问答、Agent 工具调用和混合检索中提供语义召回能力。

WeKnora 的向量库配置分成两层：

- 系统默认向量库：由 `RETRIEVE_DRIVER` 和对应环境变量在服务启动时加载。
- 租户级向量库：由管理员在设置页或 `/api/v1/vector-stores` API 中创建，保存到 `vector_stores` 表。

知识库创建时可以选择一个租户级向量库；如果不选择，就使用系统默认向量库。知识库创建后，向量库绑定不可在编辑页修改，因为已经入库的向量、索引名和集合结构都依赖创建时的选择。

## 支持的后端

### 系统默认后端

`RETRIEVE_DRIVER` 支持用逗号配置一个或多个检索后端。启动时，后端会把这些配置转换成只读的虚拟向量库，ID 形如 `__env_qdrant__`、`__env_postgres__`。

常见取值包括：

| `RETRIEVE_DRIVER` | 后端 | 说明 |
| --- | --- | --- |
| `postgres` | PostgreSQL / pgvector | 使用应用默认数据库连接。 |
| `sqlite` | SQLite | 轻量本地检索后端。 |
| `elasticsearch_v7` | Elasticsearch 7 | 使用 Elasticsearch 7 客户端。 |
| `elasticsearch_v8` | Elasticsearch 8 | 使用 Elasticsearch 8 客户端。 |
| `opensearch` | OpenSearch | 使用 OpenSearch k-NN。 |
| `qdrant` | Qdrant | 使用 gRPC 连接。 |
| `milvus` | Milvus | 使用 Milvus collection。 |
| `tencent_vectordb` | 腾讯云 VectorDB | 使用腾讯云 VectorDB SDK。 |
| `weaviate` | Weaviate | 使用 HTTP 与可选 gRPC 地址。 |
| `doris` | Apache Doris | 使用 FE MySQL 地址和 Stream Load HTTP 端口。 |

环境变量配置的向量库会出现在向量库列表中，但标记为 `env` 和 `readonly`，不能通过 API 或界面修改、删除。

### 租户级可新增后端

管理员可在界面中新增的后端来自 `/api/v1/vector-stores/types`，当前包括：

- Elasticsearch
- OpenSearch
- Qdrant
- Milvus
- Weaviate
- Apache Doris
- Tencent VectorDB

PostgreSQL 和 SQLite 不在新增列表中。它们只通过系统默认配置暴露，因为这两类后端依赖应用默认数据库或本地文件，不适合作为多个租户级独立向量库重复注册。

## 配置入口

### 管理界面

进入设置中的向量库管理页可以查看所有向量库：

- `env` 来源：由环境变量创建，只读。
- `user` 来源：管理员在当前租户内创建，可改名称，可删除。

新增向量库时，前端会先调用 `/api/v1/vector-stores/types` 获取字段 schema，再按后端类型动态渲染连接字段和高级索引字段。

编辑已有向量库时，只能修改名称。`engine_type`、`connection_config` 和 `index_config` 创建后不可修改；如果连接地址、索引名或集合配置需要变化，应新建一个向量库，再让新的知识库绑定它。

### API

向量库 API 路由如下：

| 方法 | 路径 | 权限 | 用途 |
| --- | --- | --- | --- |
| `GET` | `/api/v1/vector-stores/types` | Viewer | 获取可新增类型和字段 schema。 |
| `GET` | `/api/v1/vector-stores` | Viewer | 列出环境变量向量库和租户级向量库。 |
| `GET` | `/api/v1/vector-stores/:id` | Viewer | 查看单个向量库。 |
| `POST` | `/api/v1/vector-stores/test` | Admin | 用表单中的原始连接信息测试连接，不保存。 |
| `POST` | `/api/v1/vector-stores` | Admin | 创建租户级向量库。 |
| `PUT` | `/api/v1/vector-stores/:id` | Admin | 更新名称。 |
| `DELETE` | `/api/v1/vector-stores/:id` | Admin | 删除租户级向量库。 |
| `POST` | `/api/v1/vector-stores/:id/test` | Admin | 测试已保存或环境变量向量库。 |

连接信息中的 `password` 和 `api_key` 会在数据库中加密保存，API 响应会用脱敏占位符返回，不会回传明文。

## 创建时的校验

创建租户级向量库时，后端会按顺序执行以下校验：

1. 检查名称、租户 ID 和后端类型。
2. 检查后端所需的连接字段。
3. 对用户输入的地址执行 SSRF 校验，阻止未允许的内部地址或危险端口。
4. 校验索引名、集合名前缀和分片副本等索引配置。
5. 检查同一租户下是否已经存在相同后端、相同端点和相同索引或集合的向量库。
6. 与环境变量向量库做重复检查。
7. 执行真实连接测试，成功后保存检测到的服务端版本。
8. 保存数据库记录，并尽量动态注册到当前进程的检索引擎注册表。

如果动态注册失败，数据库记录仍会保留；服务重启后会重新加载 `vector_stores` 表中的配置。

## 连接字段

不同后端需要的连接字段不同：

| 后端 | 关键连接字段 |
| --- | --- |
| Elasticsearch | `addr`、可选 `username`、`password`。 |
| OpenSearch | `addr`、可选 `username`、`password`、`insecure_skip_verify`。 |
| Qdrant | `host`、可选 `port`、`api_key`、`use_tls`。 |
| Milvus | `addr`、可选 `database`、`username`、`password`。 |
| Weaviate | `host`、可选 `grpc_address`、`scheme`、`api_key`。 |
| Apache Doris | `addr`、`database`、可选 `http_port`、`username`、`password`。 |
| Tencent VectorDB | `addr`、`username`、`api_key`、可选 `database`。 |

`insecure_skip_verify` 只用于跳过 TLS 证书校验，适合自签名开发环境；生产环境不建议开启。

## 索引和集合配置

向量库创建时可以配置索引名、集合名或表名前缀。若不填写，后端会使用默认值。

常见字段包括：

| 字段 | 适用后端 | 含义 |
| --- | --- | --- |
| `index_name` | Elasticsearch、OpenSearch | 索引名。 |
| `number_of_shards` | Elasticsearch、OpenSearch | 索引分片数。 |
| `number_of_replicas` | Elasticsearch、OpenSearch | 副本数。 |
| `collection_prefix` | Qdrant、Weaviate、Doris | collection 或表名前缀。 |
| `collection_name` | Milvus、Tencent VectorDB | collection 名称。 |
| `shard_number` | Qdrant | collection 分片数。 |
| `replication_factor` | Qdrant、Weaviate | 副本因子。 |
| `shards_num` | Milvus、Tencent VectorDB | collection 分片数。 |
| `replica_number` | Milvus、Tencent VectorDB | 查询副本数。 |
| `desired_shard_count` | Weaviate | collection 分片数。 |
| `buckets_num` | Doris | Doris 表 bucket 数。 |
| `replication_num` | Doris | Doris 表副本数。 |

名称类字段必须以字母开头，只能包含字母、数字、下划线和连字符，最长 128 个字符。分片类字段最大为 64，副本类字段最大为 10。

OpenSearch 还支持 HNSW 参数：

- `hnsw_m`
- `hnsw_ef_construction`
- `hnsw_ef_search`
- `knn_engine`，可选 `lucene` 或 `faiss`

这些参数会影响索引结构，创建后不可修改。

## 知识库绑定

创建知识库时，向量库设置区会展示：

- 系统默认：对应环境变量向量库，前端会显示其引擎类型。
- 租户级向量库：只展示当前租户创建的 `user` 来源向量库。

选择后，知识库记录会保存 `vector_store_id`。如果为空，表示使用系统默认向量库。

编辑知识库时，向量库区只展示当前绑定状态，不提供切换入口。后端也不会在知识库更新请求中接收新的 `vector_store_id`。

知识库列表和详情会返回安全的展示字段：

- `vector_store_name`
- `vector_store_source`
- `vector_store_engine_type`
- `vector_store_status`

如果知识库来自组织共享视图，后端会隐藏源租户的向量库名称和引擎类型，只返回共享状态，避免泄露源租户基础设施信息。

## 删除约束

租户级向量库删除前会检查是否仍有知识库绑定：

- 如果还有知识库绑定，删除会失败。
- PostgreSQL 下会对向量库行加锁，减少删除与新建知识库之间的竞态。
- 删除成功后会软删除数据库记录，并从当前进程的检索引擎注册表注销。

环境变量向量库不能删除。多副本部署时，注销只影响当前进程；其他进程可能仍保留旧注册表，通常需要依赖重启或运维流程让所有副本一致。

## 选择建议

- 本地开发或轻量部署：优先使用 `postgres` 或 `sqlite` 作为系统默认后端。
- 已有 Elasticsearch / OpenSearch 运维体系：可使用它们承载向量与关键词混合检索。
- 大规模向量检索：优先评估 Qdrant、Milvus、Weaviate 或 Tencent VectorDB。
- 已有 Doris 数仓体系：可使用 Doris 后端，注意表前缀、bucket 和副本配置。

同一个租户可以配置多个租户级向量库，用于区分不同集群、冷热数据或不同运维边界。知识库一旦绑定某个向量库，后续索引和检索都会沿用该绑定。

## 常见问题

### 为什么新增向量库时连接测试失败？

常见原因包括：

- 地址、端口或 TLS 配置错误。
- 账号密码或 API Key 无效。
- 后端服务不可达。
- 地址被 SSRF 策略拦截。
- OpenSearch / Elasticsearch 版本与协议不匹配。

创建接口会在保存前执行连接测试，因此不可达的向量库不会被保存。

### 为什么列表里有向量库但不能编辑？

来源为 `env` 的向量库来自环境变量，是进程级配置，只能通过部署配置修改。界面和 API 会把它标记为只读。

### 为什么知识库不能切换向量库？

知识库创建后，分块 embedding 已写入对应后端的索引或集合。直接切换会导致检索不到历史向量，或产生跨后端数据不一致。需要切换时，建议新建目标知识库并重新导入数据。

### 为什么删除向量库提示仍有知识库绑定？

删除保护会统计当前租户下仍绑定该 `vector_store_id` 的知识库。需要先删除相关知识库，或新建知识库迁移后再删除旧向量库。

### 为什么共享知识库不显示向量库名称？

跨租户共享时，后端会返回 `shared` 来源展示，不暴露源租户的向量库名称、引擎类型和连接信息。这是共享视图的隐私边界。

## 相关文档

- [知识库](../user-guide/knowledge-bases.md)
- [检索流水线](../architecture/retrieval-pipeline.md)
- [环境变量](../deployment/environment-variables.md)

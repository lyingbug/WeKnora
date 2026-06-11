---
title: Docker Compose
description: 使用 Docker Compose 部署 WeKnora。
---

# Docker Compose

Docker Compose 是评估 WeKnora 或运行小规模自托管部署的推荐方式。

如果你希望在单机上最快得到一个可用环境，优先使用这种方式。更大规模的生产环境建议使用 Kubernetes，或把数据库、对象存储、向量库拆到托管基础设施中。

## 环境要求

- Docker Engine
- Docker Compose
- Git
- 可访问计划使用的模型供应商

## 基础启动

```bash
git clone https://github.com/Tencent/WeKnora.git
cd WeKnora
cp .env.example .env
docker compose up -d
```

打开：

```text
http://localhost
```

## 常用 Profile

```bash
docker compose --profile neo4j up -d
docker compose --profile minio up -d
docker compose --profile langfuse up -d
docker compose --profile full up -d
```

| Profile | 服务 | 用途 |
| --- | --- | --- |
| `neo4j` | Neo4j | 知识图谱和图谱增强检索 |
| `minio` | MinIO | 本地 S3 兼容对象存储 |
| `langfuse` | Langfuse | 模型、入库和 Agent Trace |
| `full` | 多个可选服务 | 更完整的本地评估 |

## 服务地址

- Web UI：`http://localhost`
- 后端 API：`http://localhost:8080`
- Langfuse：`http://localhost:3000`

## 验证服务

```bash
docker compose ps
```

查看日志：

```bash
docker compose logs app
docker compose logs docreader
docker compose logs frontend
```

测试时跟随日志：

```bash
docker compose logs -f app docreader
```

## 配置

对外提供服务前，需要编辑 `.env`。重点关注：

- 数据库和 Redis 设置。
- 模型供应商凭据。
- 对象存储。
- 向量库选择。
- 认证和安全设置。
- 可选 Trace 和图谱设置。

不要把生产 `.env` 提交到源码仓库。

## 升级流程

简单单机升级可以执行：

```bash
git pull
docker compose pull
docker compose up -d
```

跨版本升级前应阅读版本说明，特别是包含数据库迁移或破坏性配置变化的版本。

## 停止服务

```bash
docker compose down
```

只有在明确要删除本地状态时，才删除本地卷：

```bash
docker compose down -v
```

## 生产注意事项

生产环境需要额外检查环境变量、持久化卷、TLS、对象存储、向量库容量和备份策略。

至少应做到：

- 为数据库和对象文件使用持久化卷或托管存储。
- 通过反向代理或 Ingress 终止 HTTPS。
- 把密钥放在仓库之外。
- 为 PostgreSQL 和对象存储配置备份。
- 监控应用、解析器、队列和模型供应商错误。
- 限制内部服务端口访问。

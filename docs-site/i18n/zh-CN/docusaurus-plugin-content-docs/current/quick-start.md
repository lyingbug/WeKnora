---
title: 快速开始
description: 使用 Docker Compose 本地启动 WeKnora。
---

# 快速开始

本页目标是从干净仓库启动到完成第一次知识库问答。生产部署细节请在跑通本页后继续阅读部署指南。

## 环境要求

- Docker
- Docker Compose
- Git

建议本地资源：

- 4 核 CPU 或以上。
- 8 GB 内存或以上。
- 为 PostgreSQL 数据、上传文件和索引预留足够磁盘空间。

## 启动 WeKnora

```bash
git clone https://github.com/Tencent/WeKnora.git
cd WeKnora
cp .env.example .env
docker compose up -d
```

打开 Web UI：

```text
http://localhost
```

检查服务状态：

```bash
docker compose ps
```

如果服务反复重启，查看日志：

```bash
docker compose logs app
docker compose logs docreader
```

## 可选服务

```bash
docker compose --profile neo4j up -d
docker compose --profile minio up -d
docker compose --profile langfuse up -d
```

| Profile | 启用能力 | 适用场景 |
| --- | --- | --- |
| `neo4j` | 知识图谱存储 | 需要图谱增强检索或 Wiki 图谱 |
| `minio` | 本地 S3 兼容对象存储 | 不使用云存储但希望模拟对象存储 |
| `langfuse` | Trace UI | 需要查看模型调用、Agent 步骤和入库链路 |
| `full` | 常见可选服务组合 | 需要更完整的本地评估环境 |

## 第一次使用

1. 打开 Web UI。
2. 创建或选择工作区。
3. 配置至少一个对话模型和一个 Embedding 模型。
4. 创建知识库。
5. 上传文档或导入 URL。
6. 提问并查看引用来源。

## 配置模型

WeKnora 至少需要：

- 一个对话模型，用于生成回答。
- 一个 Embedding 模型，用于索引和检索。

根据工作流，还可以配置：

- Rerank 模型，用于优化检索排序。
- VLM 模型，用于图片较多的文档。
- ASR 模型，用于音频内容。

你可以使用托管模型供应商，也可以接入自托管模型端点。

## 停止服务

```bash
docker compose down
```

如果确定要删除本地数据卷，可以显式执行：

```bash
docker compose down -v
```

只有在确认本地数据可以删除时才使用 `-v`。

## 常见排查

| 现象 | 检查项 |
| --- | --- |
| Web UI 打不开 | 检查 80 端口是否被占用，以及前端或反向代理服务是否运行 |
| 后端 API 不可用 | 查看 `docker compose logs app` 和数据库连接 |
| 文档解析失败 | 查看 `docker compose logs docreader` 和文件格式 |
| 回答没有引用 | 确认文档已经解析完成并生成分块 |
| 模型调用失败 | 检查模型凭据、端点、模型名称和网络访问 |

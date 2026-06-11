---
title: Quick Start
description: Run WeKnora locally with Docker Compose.
---

# Quick Start

This guide starts the core WeKnora services with Docker Compose.

The goal is to get from a clean checkout to your first knowledge-base answer. For production deployment details, use the deployment guides after completing this page.

## Requirements

- Docker
- Docker Compose
- Git

Recommended local resources:

- 4 CPU cores or more.
- 8 GB memory or more.
- Enough disk space for PostgreSQL data, uploaded files, and model-related indexes.

## Start WeKnora

```bash
git clone https://github.com/Tencent/WeKnora.git
cd WeKnora
cp .env.example .env
docker compose up -d
```

Open the Web UI:

```text
http://localhost
```

Check service status:

```bash
docker compose ps
```

If a service keeps restarting, inspect logs:

```bash
docker compose logs app
docker compose logs docreader
```

## Optional profiles

Use Docker Compose profiles to enable optional services:

```bash
docker compose --profile neo4j up -d
docker compose --profile minio up -d
docker compose --profile langfuse up -d
```

| Profile | Enables | Use when |
| --- | --- | --- |
| `neo4j` | Knowledge graph storage | You want graph-enhanced retrieval or Wiki graph features |
| `minio` | Local S3-compatible object storage | You want object storage behavior without using a cloud bucket |
| `langfuse` | Tracing UI | You want to inspect model calls, Agent steps, and ingestion spans |
| `full` | Common optional services | You want a broader local evaluation environment |

## First workflow

1. Open the Web UI.
2. Create or select a workspace.
3. Configure at least one chat model and one embedding model.
4. Create a knowledge base.
5. Upload a document or import a URL.
6. Ask a question and inspect the cited sources.

## Configure models

WeKnora needs at least:

- A chat model for answer generation.
- An embedding model for indexing and retrieval.

Depending on the workflow, you may also configure:

- A rerank model for better retrieval ordering.
- A VLM model for image-heavy documents.
- An ASR model for audio content.

You can use hosted providers or self-hosted model endpoints, depending on your deployment policy.

## Stop services

```bash
docker compose down
```

To remove volumes as well, run the destructive command explicitly:

```bash
docker compose down -v
```

Only remove volumes when you are sure local data can be deleted.

## Troubleshooting

| Symptom | What to check |
| --- | --- |
| Web UI does not open | Check whether port 80 is occupied and whether frontend/proxy services are running |
| Backend API is unavailable | Check `docker compose logs app` and database connectivity |
| Document parsing fails | Check `docker compose logs docreader` and the file format |
| Answers have no citations | Confirm that documents finished parsing and chunks were created |
| Model calls fail | Verify model provider credentials, endpoint, model name, and network access |

For deployment details, see [Docker Compose Deployment](./deployment/docker-compose.md).

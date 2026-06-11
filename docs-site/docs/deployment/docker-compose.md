---
title: Docker Compose
description: Deploy WeKnora with Docker Compose.
---

# Docker Compose

Docker Compose is the recommended way to evaluate WeKnora or run a compact self-hosted deployment.

Use this deployment when you want the fastest path to a working environment on one machine. For larger production deployments, use Kubernetes or split the backing services into managed infrastructure.

## Requirements

- Docker Engine
- Docker Compose
- Git
- Access to the model providers you plan to use

## Basic startup

```bash
git clone https://github.com/Tencent/WeKnora.git
cd WeKnora
cp .env.example .env
docker compose up -d
```

Open:

```text
http://localhost
```

## Common profiles

```bash
docker compose --profile neo4j up -d
docker compose --profile minio up -d
docker compose --profile langfuse up -d
docker compose --profile full up -d
```

| Profile | Service | Purpose |
| --- | --- | --- |
| `neo4j` | Neo4j | Knowledge graph and graph-enhanced retrieval |
| `minio` | MinIO | Local S3-compatible object storage |
| `langfuse` | Langfuse | Model, ingestion, and Agent tracing |
| `full` | Multiple optional services | Broader local evaluation |

## Service addresses

- Web UI: `http://localhost`
- Backend API: `http://localhost:8080`
- Langfuse: `http://localhost:3000`

## Verify services

```bash
docker compose ps
```

Inspect logs:

```bash
docker compose logs app
docker compose logs docreader
docker compose logs frontend
```

Follow logs while testing:

```bash
docker compose logs -f app docreader
```

## Configuration

Edit `.env` before exposing the deployment to other users. Important areas include:

- Database and Redis settings.
- Model provider credentials.
- Object storage.
- Vector store selection.
- Authentication and security settings.
- Optional tracing and graph settings.

Never commit production `.env` files to source control.

## Upgrade workflow

For a simple single-node upgrade:

```bash
git pull
docker compose pull
docker compose up -d
```

Review release notes before upgrading between versions that contain database migrations or breaking configuration changes.

## Stop services

```bash
docker compose down
```

Remove local volumes only when you intentionally want to delete local state:

```bash
docker compose down -v
```

## Production notes

For production deployments, review environment variables, persistent volumes, TLS termination, object storage, vector database capacity, and backup strategy before exposing the service.

At minimum, production deployments should:

- Use persistent volumes or managed storage for databases and object files.
- Terminate HTTPS at a reverse proxy or ingress.
- Store secrets outside the repository.
- Configure backups for PostgreSQL and object storage.
- Monitor application, parser, queue, and model-provider errors.
- Limit access to internal service ports.

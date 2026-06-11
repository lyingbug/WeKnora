---
title: Observability
description: Trace Agent execution, document ingestion, and model usage.
---

# Observability

WeKnora integrates observability into key workflows such as document ingestion, Agent execution, and model calls.

## What to observe

- Document parsing stages
- Chunking and embedding
- Retrieval requests
- Agent tool calls
- Token usage
- Model latency and errors

## Langfuse

Langfuse can be enabled to trace model and Agent workflows. For local deployments, start the Langfuse profile with Docker Compose.

```bash
docker compose --profile langfuse up -d
```

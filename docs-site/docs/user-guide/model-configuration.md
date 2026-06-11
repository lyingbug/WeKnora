---
title: Model Configuration
description: Configure chat, embedding, rerank, VLM, and ASR models.
---

# Model Configuration

WeKnora can work with multiple model providers and model types.

## Model types

- Chat models for answer generation and Agent reasoning.
- Embedding models for vector indexing.
- Rerank models for retrieval refinement.
- VLM models for image understanding.
- ASR models for audio transcription.

## Minimum required models

Most deployments need at least:

- One chat model.
- One embedding model.

Without a chat model, WeKnora cannot generate answers. Without an embedding model, semantic retrieval and document indexing will be limited.

## Optional model roles

| Role | When to enable |
| --- | --- |
| Rerank | Relevant chunks are recalled but final ordering is weak |
| VLM | Documents include images, screenshots, scanned pages, or visual content |
| ASR | Audio files or voice content need to be indexed |
| Separate Agent model | Agent reasoning needs a different cost, latency, or capability profile |

## Configuration scope

Models can be configured globally, by tenant, or by knowledge workflow depending on deployment and permission settings.

## Provider strategy

Choose providers according to privacy, latency, cost, context length, and language requirements. Self-hosted deployments commonly combine private model endpoints with local vector storage.

## Selection checklist

Before enabling a provider for production, verify:

- API endpoint and credentials.
- Model name and supported parameters.
- Context length.
- Streaming support.
- Rate limits and quota.
- Data retention policy.
- Language quality for your documents.
- Cost per expected workload.

## Troubleshooting

If model calls fail, test chat and embedding models separately. A working chat model does not prove the embedding model is configured correctly, and embedding failures can break ingestion even when chat still works.

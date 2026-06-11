---
title: Core Concepts
description: Learn the key concepts behind WeKnora.
---

# Core Concepts

This page defines the terms used across the WeKnora documentation.

## Tenant and workspace

A tenant is an isolated workspace for members, knowledge bases, agents, models, and audit logs. Role-based access control decides what each member can view or manage.

Use tenants to separate teams, business units, customers, or environments. A tenant boundary should usually match an access-control boundary.

## Knowledge base

A knowledge base stores content that WeKnora can retrieve and use for answers. A knowledge base can contain documents, FAQs, URLs, or generated Wiki pages.

Knowledge bases should be organized by domain and permission scope. For example, product manuals, support playbooks, and internal policies should usually be separate knowledge bases if they serve different audiences or answer different types of questions.

## Knowledge item

A knowledge item is one source record inside a knowledge base, such as an uploaded PDF, a Markdown page, a URL import, or a manually created FAQ.

Each knowledge item carries metadata such as title, source, tags, status, and parsing information. This metadata helps users manage content and helps the retrieval system filter results.

## Chunk

A chunk is a smaller piece of content produced from a knowledge item. Chunks are indexed for retrieval and later used as answer context.

Chunk quality directly affects answer quality. Chunks that are too small lose context; chunks that are too large may dilute retrieval relevance. WeKnora supports configurable chunking strategies so deployments can tune for document type and model context length.

## RAG

RAG means retrieval-augmented generation. WeKnora searches relevant chunks first, then sends selected context to the model to generate grounded answers.

RAG is best for direct knowledge questions where the system should answer from known sources and show citations.

## Agent

An Agent can reason through multiple steps, call tools, use MCP services, search knowledge bases, and produce a final answer.

Agent mode is best for tasks that need planning, tool use, or multiple lookups. It is more flexible than simple RAG, but it also needs clearer tool permissions and observability.

## MCP service

An MCP service exposes tools that an Agent can call. WeKnora supports built-in MCP services and external MCP servers.

MCP lets WeKnora connect Agents to external capabilities without hard-coding every tool into the core application. Sensitive tools can be protected with approval flows.

## Agent Skill

An Agent Skill is a packaged capability that can give an Agent instructions, reference material, and optional scripts. Skills are useful when a repeated workflow needs specialized behavior.

## Wiki mode

Wiki mode turns source documents into structured Markdown pages and a browsable knowledge network.

Wiki mode is different from ordinary RAG. RAG answers questions from chunks; Wiki mode creates a persistent knowledge layer that users can browse, link, review, and reuse.

## Model roles

WeKnora uses different model roles for different jobs:

| Role | Purpose |
| --- | --- |
| Chat model | Generates answers and performs Agent reasoning |
| Embedding model | Converts text into vectors for semantic search |
| Rerank model | Reorders retrieved candidates for better relevance |
| VLM | Understands images or visual document content |
| ASR | Converts speech into text |

## Retrieval components

Retrieval can combine several signals:

- Dense vector search for semantic similarity.
- Sparse or keyword search for exact terms.
- Graph-enhanced search when a knowledge graph is enabled.
- Reranking to refine the final context set.

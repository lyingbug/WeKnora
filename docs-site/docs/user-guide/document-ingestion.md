---
title: Document Ingestion
description: Understand how files and URLs become searchable knowledge.
---

# Document Ingestion

Document ingestion turns source content into indexed chunks that can be searched and used in answers.

Use this page when you want to understand what can be imported, how processing works, and how to diagnose ingestion failures.

## Supported sources

- Local file upload
- URL import
- Manual Markdown entry
- Data source synchronization
- FAQ entry creation

## Supported formats

WeKnora supports common document formats including PDF, Word, Markdown, text, HTML, images, CSV, Excel, PowerPoint, and JSON.

Exact parser behavior can vary by deployment because parser engines and optional OCR or multimodal capabilities can be configured.

## Ingestion stages

1. Source content is uploaded or synchronized.
2. DocReader extracts text and structured content.
3. Content is split into chunks.
4. Embeddings are generated.
5. Chunks and metadata are stored.
6. Retrieval indexes are updated.

## Choosing ingestion settings

The right settings depend on the source material.

| Content type | Suggested focus |
| --- | --- |
| Product documentation | Clean headings, stable chunking, citation quality |
| PDFs with layout | Parser quality, table extraction, page references |
| Images or scanned files | OCR and VLM configuration |
| Large spreadsheets | Sheet structure, row grouping, metadata |
| FAQs | Question normalization and tags |
| Wiki generation | Graph extraction and page structure |

## Reparse workflow

Reparse a knowledge item when:

- Parser settings changed.
- Chunking settings changed.
- A better embedding model is configured.
- Multimodal or graph extraction was enabled after the first parse.
- The previous parse failed or produced poor text.

Reparsing should preserve the original source while replacing derived chunks, embeddings, and indexes for the selected run.

## What users should check

After ingestion, inspect:

- Item status.
- Extracted text preview.
- Chunk list and chunk boundaries.
- Source metadata and tags.
- Whether citations point to useful passages.

If answers are poor, the first debugging step is usually to inspect the extracted text and chunks before changing the model.

For the architecture view, see [Ingestion Pipeline](../architecture/ingestion-pipeline.md).

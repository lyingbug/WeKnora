---
title: Wiki Mode
description: Generate linked Markdown knowledge from source documents.
---

# Wiki Mode

Wiki mode helps teams convert raw documents into structured, linked, and browsable knowledge.

Use Wiki mode when the source material is too large or interconnected for simple question answering to be the only interface.

## What Wiki mode produces

- Markdown Wiki pages
- Links between related concepts
- Browsable page hierarchy
- Knowledge graph visualization

## When to use Wiki mode

Use Wiki mode when users need to explore a body of knowledge, not only ask isolated questions.

Good candidates:

- Product manuals.
- Technical design documents.
- Research reports.
- Policy collections.
- Project archives.
- Onboarding material.

Poor candidates:

- Very small FAQ sets.
- Temporary notes that do not need structure.
- Content that is not allowed to be transformed or summarized.

## Typical workflow

1. Create a Wiki knowledge base.
2. Upload or synchronize source documents.
3. Let WeKnora generate Wiki pages.
4. Review generated pages and graph relationships.
5. Use the Wiki as a structured knowledge layer for search and reasoning.

## Review workflow

Generated pages should be reviewed like any other knowledge artifact. Check:

- Whether the page title matches the concept.
- Whether the page is grounded in the source.
- Whether links point to useful related pages.
- Whether important source sections are missing.
- Whether graph relationships are meaningful.

## Relationship to RAG

Wiki mode does not replace RAG. It creates a structured knowledge layer that RAG and Agent workflows can use later. Users can browse the Wiki to understand the domain, then ask questions against the same knowledge base.

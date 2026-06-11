---
title: Chat and RAG
description: Ask questions over knowledge bases with cited answers.
---

# Chat and RAG

RAG mode answers questions by retrieving relevant knowledge before calling the language model.

Use RAG mode when the user needs a grounded answer from known sources. It is the default choice for most knowledge-base questions.

## Request flow

1. The user asks a question.
2. WeKnora searches selected knowledge bases.
3. Relevant chunks are ranked and assembled as context.
4. The model generates an answer.
5. WeKnora returns the answer with source citations.

## When to use RAG mode

Use RAG mode for direct knowledge questions where source grounding matters more than multi-step tool use.

Good examples:

- "What does our deployment guide say about enabling Neo4j?"
- "Summarize the refund policy from the uploaded document."
- "Which section explains API key authentication?"

Use Agent mode instead when the task requires multiple tool calls, planning, web search, or cross-system actions.

## Source citations

Citations are part of the trust model. A useful answer should let the user inspect where the information came from.

If citations are missing or weak, check:

- Whether the selected knowledge base contains parsed chunks.
- Whether retrieval returns relevant chunks.
- Whether the answer was generated from context or from model prior knowledge.
- Whether the prompt or answer mode suppresses citation output.

## Improving answer quality

- Keep knowledge bases focused.
- Use clear document titles and metadata.
- Configure a suitable embedding model.
- Enable rerank when the candidate set is large.
- Inspect cited chunks when answers are incomplete.

## Common tuning path

1. Confirm document parsing quality.
2. Inspect chunk boundaries.
3. Test retrieval with representative questions.
4. Add keyword or hybrid search if exact terms are missed.
5. Add rerank if relevant chunks are recalled but ordered poorly.
6. Split unrelated content into separate knowledge bases.

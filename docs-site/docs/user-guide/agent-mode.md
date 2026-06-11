---
title: Agent Mode
description: Use Agents for multi-step reasoning and tool use.
---

# Agent Mode

Agent mode is designed for tasks that need more than one retrieval or model call.

An Agent can:

- Search one or more knowledge bases.
- Call MCP tools.
- Use web search.
- Run approved skills in a sandbox.
- Produce a final answer after multiple reasoning steps.

## When to use Agent mode

Use Agent mode when the task requires planning, tool calls, cross-source reasoning, or iterative lookup.

Good examples:

- "Compare the policy in this knowledge base with the latest public information."
- "Find the relevant internal procedure, then draft a response."
- "Search the knowledge base, call an MCP tool, and summarize the result."

For direct questions over a known knowledge base, RAG mode is usually faster and easier to audit.

## Configuring an Agent

When creating or editing an Agent, define:

- The Agent purpose.
- The system instruction.
- Which knowledge bases it can search.
- Which MCP tools it can call.
- Whether web search is enabled.
- Whether skills are enabled.
- Model, timeout, and approval settings.

Avoid giving one Agent every tool and every knowledge base. Narrow Agents are easier to understand, safer to operate, and easier to debug.

## Tool approval

Some tool calls can require human approval before execution. This is useful for sensitive operations, external calls, or tools with side effects.

Approval is recommended for tools that:

- Use private credentials.
- Send data to third-party services.
- Write, delete, or trigger actions.
- Are expensive or long-running.

## Reading Agent traces

When debugging Agent output, inspect:

- Which knowledge bases were searched.
- Which tools were called.
- Whether a tool call was blocked by approval.
- What observations came back from tools.
- Which model was used.
- Whether the final answer cites the right evidence.

For the execution model, see [Agent Execution](../architecture/agent-execution.md).

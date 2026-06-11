---
title: MCP Tools
description: Connect Agents to tools through MCP services.
---

# MCP Tools

MCP services expose tools that WeKnora Agents can call during reasoning.

MCP is the main mechanism for connecting Agents to external capabilities without baking every integration into the core application.

## MCP service types

- Built-in MCP services managed by WeKnora.
- External MCP servers connected through supported transports.
- Custom services created by your team.

## Common use cases

- Query business systems.
- Fetch external data.
- Run safe automation.
- Let Agents call domain-specific tools.

## Configuration model

An MCP service should define:

- Service name and description.
- Transport configuration.
- Credentials or environment variables.
- Available tools.
- Tenant or Agent access.
- Approval requirements for sensitive tools.

## Tool approval

Approval should be enabled for tools that can expose private information, call external systems, mutate state, or create cost. The user reviewing the approval should be able to see what tool will run and what arguments will be sent.

## Safety controls

Use tool approval and scoped credentials for tools that access external systems or perform side effects.

## Debugging MCP tools

When a tool does not appear or does not run, check:

- Whether the MCP service is enabled.
- Whether the Agent is allowed to use that service.
- Whether credentials are present and valid.
- Whether the transport is reachable from the backend.
- Whether the tool call is waiting for approval.
- Whether the tool result exceeded size or formatting limits.

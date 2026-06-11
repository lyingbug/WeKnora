---
title: IM Connectors
description: Connect WeKnora to enterprise messaging platforms.
---

# IM Connectors

IM connectors let users ask WeKnora questions from messaging platforms.

Common integration targets include WeCom, Feishu, Slack, Telegram, DingTalk, Mattermost, and WeChat.

## Connector responsibilities

- Receive user messages.
- Map the message to a tenant, user, and session.
- Send the request to WeKnora.
- Return answers with citations where supported.
- Preserve thread or channel context when available.

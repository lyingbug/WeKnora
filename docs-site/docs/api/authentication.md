---
title: API Authentication
description: Authenticate requests to the WeKnora API.
---

# API Authentication

API requests should authenticate with a supported method such as bearer tokens or API keys, depending on deployment settings.

## Best practices

- Store API keys securely.
- Rotate credentials periodically.
- Scope credentials to the minimum required tenant and operation.
- Do not expose server-side credentials in browser code.

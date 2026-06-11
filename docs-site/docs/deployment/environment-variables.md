---
title: Environment Variables
description: Configure WeKnora services through environment variables.
---

# Environment Variables

WeKnora uses environment variables to configure model providers, storage, database connections, vector stores, security settings, and optional services.

Start from:

```bash
cp .env.example .env
```

## Configuration groups

- Database and Redis
- Object storage
- Vector database
- Model providers
- Document parsing
- MCP and Agent sandbox
- Authentication and security
- Observability

Keep production secrets outside source control and rotate credentials according to your organization policy.

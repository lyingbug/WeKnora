---
title: Testing
description: Run checks before contributing changes.
---

# Testing

Run the checks that match the code you changed.

## Backend

```bash
go test ./...
```

## Frontend

```bash
cd frontend
npm test
npm run type-check
```

## Documentation site

```bash
cd docs-site
npm run build
```

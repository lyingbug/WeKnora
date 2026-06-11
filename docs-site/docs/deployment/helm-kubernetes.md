---
title: Helm and Kubernetes
description: Deploy WeKnora to Kubernetes with Helm.
---

# Helm and Kubernetes

Use the Helm chart when deploying WeKnora into Kubernetes environments.

## Chart location

The Helm chart lives in the repository-level `helm/` directory.

## Typical workflow

```bash
helm install weknora ./helm
```

Customize `helm/values.yaml` for image tags, ingress, storage, database, Redis, Neo4j, and service-level settings.

## Production checklist

- Configure persistent volumes.
- Use managed PostgreSQL and Redis if available.
- Configure ingress and TLS.
- Store secrets through the cluster secret manager.
- Enable resource requests and limits.
- Connect observability and log collection.

---
title: Helm 与 Kubernetes
description: 使用 Helm 在 Kubernetes 中部署 WeKnora。
---

# Helm 与 Kubernetes

WeKnora 仓库内置 Helm Chart，可在 Kubernetes 集群中部署后端 App、前端、DocReader、PostgreSQL、Redis，以及可选的 MinIO、Neo4j、Qdrant 等组件。

Chart 位于仓库的 `helm/` 目录，要求 Kubernetes 1.25+ 和 Helm 3.10+。集群需要有可用的 PV provisioner；如果要对外访问，还需要 Ingress Controller。

## 组件结构

默认安装会创建这些核心资源：

| 组件 | Kubernetes 资源 | 服务名 | 说明 |
| --- | --- | --- | --- |
| App | `Deployment` + `Service` | `app` | Go API 服务，监听 8080。 |
| Frontend | `Deployment` + `Service` | `frontend` | Nginx 托管的 Web UI，监听 80。 |
| DocReader | `Deployment` + `Service` | `docreader` | 文档解析 gRPC 服务，监听 50051。 |
| PostgreSQL | `Deployment` + `Service` + PVC | `postgres` | 默认使用 ParadeDB 镜像，承担主库和默认检索存储。 |
| Redis | `Deployment` + `Service` + PVC | `redis` | 流式事件和异步任务依赖。 |
| data-files | PVC | 无 | App 挂载到 `/data/files`，用于 local 文件存储。 |

Ingress 开启后会把 `/api` 转发到 `app`，把 `/` 转发到 `frontend`。

## 快速安装

最小安装需要提供数据库密码、Redis 密码和 JWT 密钥：

```bash
helm install weknora ./helm \
  --namespace weknora \
  --create-namespace \
  --set secrets.dbPassword='<数据库密码>' \
  --set secrets.redisPassword='<Redis 密码>' \
  --set secrets.jwtSecret="$(openssl rand -base64 32)"
```

安装后查看 Pod：

```bash
kubectl get pods -n weknora
```

未启用 Ingress 时，可以临时端口转发前端服务：

```bash
kubectl port-forward -n weknora svc/frontend 8080:80
```

然后访问 `http://127.0.0.1:8080`。

## 启用 Ingress

Ingress 配置位于 `ingress`：

```bash
helm install weknora ./helm \
  --namespace weknora \
  --create-namespace \
  --set ingress.enabled=true \
  --set ingress.host=weknora.example.com \
  --set ingress.tls.enabled=true \
  --set ingress.tls.secretName=weknora-tls \
  --set secrets.dbPassword='<数据库密码>' \
  --set secrets.redisPassword='<Redis 密码>' \
  --set secrets.jwtSecret="$(openssl rand -base64 32)"
```

默认 Ingress class 是 `nginx`，并带有适合上传和流式响应的注解：

| 注解 | 默认值 | 作用 |
| --- | --- | --- |
| `nginx.ingress.kubernetes.io/proxy-body-size` | `100m` | 允许较大的上传请求。 |
| `nginx.ingress.kubernetes.io/proxy-connect-timeout` | `60` | 后端连接超时。 |
| `nginx.ingress.kubernetes.io/proxy-read-timeout` | `3600` | 支持长时间流式读取。 |
| `nginx.ingress.kubernetes.io/proxy-send-timeout` | `3600` | 支持长时间上传或流式发送。 |

如果集群使用其他 Ingress Controller，需要按该控制器的规则改写 `ingress.annotations`。

## 使用 Values 文件

生产环境建议使用单独的 values 文件，而不是把所有参数写在命令行：

```yaml
# values-production.yaml
global:
  storageClass: fast-ssd

app:
  replicaCount: 2
  resources:
    requests:
      cpu: 500m
      memory: 1Gi
    limits:
      cpu: "2"
      memory: 4Gi
  env:
    GIN_MODE: release
    RETRIEVE_DRIVER: postgres
    STORAGE_TYPE: local
    LOCAL_STORAGE_BASE_DIR: /data/files
    AUTO_RECOVER_DIRTY: "true"
    STREAM_MANAGER_TYPE: redis
    CONCURRENCY_POOL_SIZE: "5"
    ENABLE_GRAPH_RAG: "false"
    TZ: Asia/Shanghai
  extraEnv:
    - name: OLLAMA_BASE_URL
      value: "http://ollama.ollama:11434"

ingress:
  enabled: true
  host: weknora.example.com
  tls:
    enabled: true
    secretName: weknora-tls

dataFiles:
  persistence:
    size: 100Gi

postgresql:
  persistence:
    size: 100Gi

redis:
  persistence:
    size: 10Gi
```

安装：

```bash
helm install weknora ./helm \
  --namespace weknora \
  --create-namespace \
  -f values-production.yaml \
  --set secrets.dbPassword='<数据库密码>' \
  --set secrets.redisPassword='<Redis 密码>' \
  --set secrets.jwtSecret="$(openssl rand -base64 32)" \
  --set secrets.tenantAesKey='<32字节租户密钥>' \
  --set secrets.systemAesKey='<32字节系统密钥>'
```

## Secret 管理

Chart 会创建一个 Opaque Secret，注入下列 key：

| Secret key | 来源 | 说明 |
| --- | --- | --- |
| `DB_USER` | `secrets.dbUser` | 数据库用户名。 |
| `DB_PASSWORD` | `secrets.dbPassword` | 数据库密码，必填。 |
| `DB_NAME` | `secrets.dbName` | 数据库名。 |
| `REDIS_USERNAME` | `secrets.redisUsername` | Redis 用户名，可选。 |
| `REDIS_PASSWORD` | `secrets.redisPassword` | Redis 密码，必填。 |
| `JWT_SECRET` | `secrets.jwtSecret` | JWT 签名密钥，必填。 |
| `TENANT_AES_KEY` | `secrets.tenantAesKey` | 租户级密钥。留空时模板会生成 32 位随机值，并在 upgrade 时复用已有 Secret。 |
| `SYSTEM_AES_KEY` | `secrets.systemAesKey` | 系统字段加密密钥，必须为 32 字节。留空时模板会生成并复用已有 Secret。 |
| `NEO4J_USERNAME` | `neo4j.username` | 仅 `neo4j.enabled=true` 时注入。 |
| `NEO4J_PASSWORD` | `neo4j.password` | 仅 `neo4j.enabled=true` 时注入，必填。 |

生产环境建议显式设置 `TENANT_AES_KEY` 和 `SYSTEM_AES_KEY`，并纳入备份。如果 Secret 被删除后重新生成，数据库中已加密的租户 API Key、模型密钥、向量库凭证、Web Search Provider Key 等字段可能无法解密。

也可以使用已有 Secret：

```yaml
secrets:
  existingSecret: weknora-secrets
```

已有 Secret 必须包含：

```text
DB_USER
DB_PASSWORD
DB_NAME
REDIS_USERNAME
REDIS_PASSWORD
JWT_SECRET
TENANT_AES_KEY
SYSTEM_AES_KEY
```

如果启用 Neo4j，还需要包含 `NEO4J_USERNAME` 和 `NEO4J_PASSWORD`。

## 持久化存储

PVC 由 `helm/templates/pvc.yaml` 创建。可通过 `global.storageClass` 控制 StorageClass：

```yaml
global:
  storageClass: fast-ssd
```

如果设置为 `"-"`，模板会渲染 `storageClassName: ""`，表示使用没有动态 StorageClass 的绑定方式。

主要持久化项：

| Values | 默认大小 | 挂载或用途 |
| --- | --- | --- |
| `dataFiles.persistence` | `10Gi` | App 的 `/data/files`，local 文件存储。 |
| `postgresql.persistence` | `10Gi` | PostgreSQL 数据目录。 |
| `redis.persistence` | `1Gi` | Redis 数据目录。 |
| `neo4j.persistence` | `10Gi` | Neo4j 数据目录，仅启用 Neo4j 时创建。 |
| `minio.persistence` | `20Gi` | MinIO 数据，仅启用 MinIO 时创建。 |
| `qdrant.persistence` | `10Gi` | Qdrant 数据，仅启用 Qdrant 时创建。 |

每个持久化项都支持 `existingClaim`，可绑定已有 PVC：

```yaml
dataFiles:
  persistence:
    enabled: true
    existingClaim: weknora-data-files
```

如果 `dataFiles.persistence.enabled=false`，App 会使用 `emptyDir`，Pod 重建后 local 文件会丢失。

## App 环境变量

`app.env` 只覆盖模板中显式支持的基础变量：

```yaml
app:
  env:
    GIN_MODE: release
    RETRIEVE_DRIVER: postgres
    STORAGE_TYPE: local
    LOCAL_STORAGE_BASE_DIR: /data/files
    AUTO_RECOVER_DIRTY: "true"
    STREAM_MANAGER_TYPE: redis
    CONCURRENCY_POOL_SIZE: "5"
    ENABLE_GRAPH_RAG: "false"
    TZ: UTC
```

其他环境变量通过 `app.extraEnv` 注入：

```yaml
app:
  extraEnv:
    - name: OLLAMA_BASE_URL
      value: "http://ollama.ollama:11434"
    - name: WEKNORA_AGENT_TOOL_APPROVAL_TIMEOUT
      value: "10m"
```

当前 App 模板固定注入 `DB_DRIVER=postgres`、`DB_HOST=postgres`、`REDIS_ADDR=redis:6379` 和 `DOCREADER_ADDR=docreader:50051`。如果要改成外部托管 PostgreSQL、Redis 或 DocReader，应审查并调整 Chart 模板或维护自己的 values 扩展，不能只看 `values.yaml` 中是否有启用开关。

## 前端代理

Frontend 容器通过环境变量配置 Nginx 代理：

| 变量 | 来源 | 默认 |
| --- | --- | --- |
| `APP_HOST` | `frontend.appHost` | `app` |
| `APP_PORT` | `frontend.appPort` 或 `app.service.port` | `8080` |

Ingress 已经直接把 `/api` 路由到 App，因此常规 Chart 部署不需要改前端代理。只有把前端单独暴露、或把后端部署到外部服务时，才需要改这两个字段。

## 可选组件

### MinIO

启用 MinIO：

```yaml
minio:
  enabled: true
  rootUser: minioadmin
  rootPassword: "<强密码>"

app:
  env:
    STORAGE_TYPE: minio
  extraEnv:
    - name: MINIO_ENDPOINT
      value: "minio:9000"
    - name: MINIO_ACCESS_KEY_ID
      value: "minioadmin"
    - name: MINIO_SECRET_ACCESS_KEY
      valueFrom:
        secretKeyRef:
          name: weknora-minio-secret
          key: MINIO_SECRET_ACCESS_KEY
    - name: MINIO_BUCKET_NAME
      value: "weknora"
```

Chart 提供了 MinIO 组件开关，但 App 的 MinIO 访问变量仍需要通过 `app.extraEnv` 注入。

### Neo4j 和 GraphRAG

启用 Neo4j：

```yaml
neo4j:
  enabled: true
  password: "<强密码>"

app:
  env:
    ENABLE_GRAPH_RAG: "true"
```

启用后 App 会自动注入 `NEO4J_URI=bolt://neo4j:7687`，并从 Secret 读取 `NEO4J_USERNAME` 和 `NEO4J_PASSWORD`。

### Qdrant

启用 Qdrant：

```yaml
qdrant:
  enabled: true

app:
  env:
    RETRIEVE_DRIVER: qdrant
  extraEnv:
    - name: QDRANT_HOST
      value: "qdrant"
    - name: QDRANT_PORT
      value: "6334"
    - name: QDRANT_COLLECTION
      value: "weknora_embeddings"
```

如果使用外部 Qdrant，应把地址、TLS、API Key 等变量放入 `app.extraEnv` 或 Secret 引用。

## 升级

普通升级：

```bash
helm upgrade weknora ./helm \
  --namespace weknora \
  --reuse-values
```

带 values 文件升级：

```bash
helm upgrade weknora ./helm \
  --namespace weknora \
  -f values-production.yaml
```

升级前建议先渲染模板检查差异：

```bash
helm template weknora ./helm \
  --namespace weknora \
  -f values-production.yaml
```

如果使用 Chart 自动生成的 `TENANT_AES_KEY` 或 `SYSTEM_AES_KEY`，模板会通过 `lookup` 复用已有 Secret 中的值。仍然建议把密钥显式写入安全的 Secret 管理系统，避免命名空间重建、GitOps prune 或误删 Secret 后生成新密钥。

## 卸载

卸载 Release：

```bash
helm uninstall weknora --namespace weknora
```

PVC 默认不会随 Helm Release 自动删除。确认不再需要数据后再删除：

```bash
kubectl delete pvc -n weknora -l app.kubernetes.io/instance=weknora
```

删除 PVC 会删除 PostgreSQL、Redis、本地文件存储以及可选组件的数据。

## 生产检查

- 设置 `ingress.host`、TLS Secret 和合适的上传大小、流式超时注解。
- 为 `dataFiles`、PostgreSQL、Redis 和可选组件配置合适的 PVC 容量与 StorageClass。
- 显式设置并备份 `JWT_SECRET`、`TENANT_AES_KEY`、`SYSTEM_AES_KEY` 和数据库、Redis 密码。
- 根据真实流量调整 `app.replicaCount`、`app.resources`、`docreader.resources` 和异步任务并发。
- 如果启用 GraphRAG、Qdrant 或 MinIO，同时确认 App 环境变量已经指向对应服务。
- 如果需要外部托管 PostgreSQL 或 Redis，先确认 Chart 模板是否已经按目标环境改造。
- 接入日志采集和模型调用可观测性，并避免把密钥写入日志。

# Docker 沙箱后端重做调研

面向要决定「docker 后端往哪走」的人。结论基于一份直接打 Docker Engine API 的可复现 PoC
（[docs/poc/docker-sandbox](./poc/docker-sandbox)，29 项检查全部通过），不是纸面推演。

协议层的整体立场见 [沙箱协议接入说明](./sandbox-protocol.md)，集群与模板见
[沙箱集群与标准模板](./sandbox-cluster.md)。

## 结论

- 现在的 `docker` 后端和 E2B 不是「能力少一点」，而是**模型不同**：它每次执行都是
  `docker run --rm` + 只读 bind mount，没有会话、没有文件面、超时不真正终止负载。
  差距不可能靠打补丁抹平，要对齐就得换成「一个会话一个长驻容器」。
- **Docker Engine API 能覆盖 `RemoteSandboxClient` 的全部方法**，也能承载 Snapshot 工作流
  （管理沙箱装 skill → commit 成快照 → 会话从快照起容器 → 增量出下一版），实测通过。
  代价是控制面职责转移到 WeKnora：空闲回收、执行超时、快照 GC、跨主机分发都要自己做。
- **Docker 官方的 Docker Sandboxes（`sbx`）不能当后端**。它是开发者本机 CLI：要 Docker 账号登录、
  工作区是宿主机目录直挂、没有多租户服务端 API。它解决的是「我本机放心跑 Claude Code」，
  不是「平台给每个租户会话分配隔离环境」。
- 建议：把 `docker` 从「一次性执行器」改写为 **`RemoteSandboxClient` 适配器**——会话级长驻容器、
  镜像即快照——定位成单机 / 私有化部署的一等后端；跨主机调度与内核级隔离仍然推荐 E2B 兼容实现。

## 一、现在的 docker 后端问题在哪

| # | 问题 | 位置 | 后果 |
| --- | --- | --- | --- |
| 1 | 每次执行 `docker run --rm`，执行完即销毁 | `internal/sandbox/docker.go` `Execute` | 没有会话状态。`shell_exec`、附件暂存、产物收集这些会话级能力在 `capabilities.go` 的口径下**根本不注册**，只有远端后端有。用户感知到的「和 E2B 效果不一致」主要就是这一条 |
| 2 | 超时只 kill 了 CLI 进程，没停掉负载 | `Execute` 用 `exec.CommandContext` | 实测：kill 掉 `docker run --rm` 后容器仍是 `running`，继续占 CPU/内存直到自己跑完。WeKnora 已经给调用方返回超时了 |
| 3 | 工作目录只读 | `buildDockerArgs`：`-v <scriptDir>:/workspace:ro` | 脚本写不了 `/workspace/output`，而 skills 框架恰恰把 `WEKNORA_SKILL_OUTPUT_DIR` 指到那里（`internal/agent/skills/manager.go`） |
| 4 | 容器化部署下 bind mount 语义错位 | 同上 | 路径由**宿主机** daemon 解释。WeKnora 自己跑在容器里时，挂进去的是宿主机上同名（通常不存在）的目录；何况 `docker-compose.yml` 既没挂 `docker.sock`，app 镜像里也没有 docker CLI。`internal/handler/sandbox_check.go` 里那句「容器没有看到挂载进去的脚本」就是这个坑的症状 |
| 5 | 走 CLI 而不是 API | 全文件 | 依赖宿主机装 CLI；错误只能靠字符串匹配；拿不到容器 ID，做不了对账、列举、孤儿回收 |
| 6 | 默认 `--network none` 且无租户级出网策略 | `buildDockerArgs` | 需要 `pip install` / 抓网页的 skill 直接失败；想放开只能全放开 |
| 7 | 租户配置面只有一个 `image` | `internal/sandbox/tenant_config.go`、`config_required.go` | CPU、内存、网络、TTL 都没法按空间配置，和 E2B/Cube 的配置面完全不对等 |

第 1 条决定了性质：**docker 后端不是「弱一点的 E2B」，它连 agent 侧的工具集都不一样。**

## 二、Docker Sandboxes（`sbx`）能不能用

不能，原因是产品定位而不是功能缺失：

- **身份与账号**：`sbx login` 强制 Docker 账号登录，沙箱身份绑定到自然人；默认上报 CLI 遥测。
  多租户 SaaS/私有化部署里没有「这台机器属于哪个 Docker 用户」这回事。
- **工作区模型**：工作区是宿主机目录按原绝对路径直挂进 microVM。WeKnora 的输入是脚本内容与附件字节，
  不是宿主机上的一棵目录树。
- **没有服务端 API**：交互面是 `sbx run/exec/cp/ls/ports` 和一个 TUI 面板，面向人在终端里操作，
  不存在可编排的多租户控制面。
- **资源模型**：每个沙箱一个独立 VM 和独立 Docker daemon，镜像层不跨沙箱共享，
  按「一个开发者几个项目」设计，不是「一个空间几十个会话」。

它值得关注的地方在**隔离形态**（microVM + 宿主侧出网代理 + 凭据注入），
那正是 CubeSandbox / e2b-dev/infra 这类方案给的东西；要走这条路应该接 E2B 协议实现，而不是接 `sbx`。

## 三、Engine API 与 `RemoteSandboxClient` 的映射（实测）

沙箱 = 一个长驻容器（`sleep infinity` 作 PID 1），模板 = 镜像，metadata = labels。

| 契约方法 | Docker 实现 | 实测结论 |
| --- | --- | --- |
| `Health` | `GET /_ping` | 通，返回 API 版本 |
| `Create` | `POST /containers/create` + `/start`，labels 带 tenant/session/config/createdAt | 通。资源上限（Memory / NanoCPUs / PidsLimit）、`CapDrop: ALL`、`no-new-privileges` 一次性表达 |
| `Connect` | `GET /containers/{id}/json` | 通。**换一个客户端进程照样重连**，labels 原样保留 → `SupportsReconnect` / `SupportsMetadata` 为真 |
| `Get` / `List` | `GET /containers/json?filters=label=...` | 通，服务端按 label 过滤。状态归一：`running`→Running、`paused`→Paused、`restarting/removing`→Transitioning、`exited/dead`→Terminal |
| `Delete` | `DELETE /containers/{id}?force=1` | 通 |
| `Exec` | `POST /containers/{id}/exec` → `/exec/{id}/start`（hijack）→ `/exec/{id}/json` | 通。user / workdir / env / stdin 都支持，非 TTY 下 stdout 与 stderr 天然分流（`stdcopy`），退出码从 exec inspect 取 |
| `WriteFile` / `ReadFile` | `PUT` / `GET /containers/{id}/archive`（tar 流） | 通。`copyUIDGID` 可保留属主 |
| `Stat` | `HEAD /containers/{id}/archive` | 通，返回 size / mode / mtime |
| `MakeDir` / `Remove` / `ListDir` | **没有原生 API**，靠 exec `mkdir -p` / `rm -rf` / `find -printf` | 通。`find -printf '%y\t%s\t%T@\t%p\n'` 的输出可直接解析成 `RemoteDirEntry`（类型/大小/mtime/路径一次拿全） |
| 暂停 / 恢复 | `POST /containers/{id}/pause`｜`/unpause` | 通，走 cgroup freezer。语义比 E2B 弱：进程和内存都还在**这台宿主机上**，只是不被调度，不释放内存也不能迁移 |
| 关停 / 再启 | `POST /stop` + `/start` | 通。文件系统保留，进程全丢（实测 `sleep 900` 消失）。等价于 E2B 的「仅文件系统快照 + 冷启动」 |

建议的 `RemoteSandboxCapabilities`：`SupportsReconnect`、`SupportsMetadata`、
`SupportsListSandboxes`、`SupportsFilesystemEnumeration`、`SupportsVolumes` 为 `true`；
`SupportsPauseResume` 为 `true` 但只在同机有意义；`SupportsTimeoutRefresh` 为 `false`
（daemon 没有 TTL 概念，见第五节）。

会话语义按 `internal/sandbox/e2b_compatible_integration_test.go` 的口径验证过：
同一容器里 `pip install --user` 装的包和写下的文件，在下一次 exec 里都还在。

## 四、Snapshot 工作流能否落地

计划中的流程（空间级管理沙箱初始化快照 → 装 skill → 更新快照 → 会话从快照起实例）在 Docker 上是
**原生形态**，因为它就是「容器 → 镜像」这条老路：

| E2B | Docker | 实测 |
| --- | --- | --- |
| `sandbox.createSnapshot()` → snapshotId（一对多） | `POST /commit` → 镜像 tag（一对多） | 通，140 MB 镜像 commit 约 0.5–1.0 s |
| `Sandbox.create(snapshotId)` | 以该镜像创建容器 | 通，快照里的 `requests==2.32.3` 与 `/opt/skills/pdf/SKILL.md` 都在 |
| 在快照上继续装东西，出下一版 | 从 v1 起容器 → 装 → commit 成 v2 | 通 |
| 按元数据检索快照 | `GET /images/json?filters=label=weknora.snapshot.version` | 通，commit 时用 `Changes: ["LABEL ..."]` 写入版本/租户 |
| 快照含**内存态** | commit **只含文件系统** | 实测：快照里活着的进程在新沙箱中为 0 |

两条要在实现时兜住的约束：

- **镜像层上限 127**。每次 commit 加一层，「一个空间长期增量更新」会撞上限。需要每 N 版做一次压平
  （`export | import` 或用 Dockerfile 重建），压平后失去与 base 的层共享，磁盘会涨一截。
- **快照是本机资产**。同一台 daemon 上是零成本共享；多机部署必须 push 到 registry，
  会话调度还得知道目标主机上有没有这一版。E2B 的快照存在控制面，没有这个问题。

内存态缺失对这个用法**没有影响**：这条流水线依赖的是「装完 skill 的文件系统」，不是「跑到一半的进程」。
真正需要内存态的是「会话空闲挂起再恢复」，Docker 上只能用 pause（不释放内存、绑死本机）
或 stop/start（丢进程）近似；CRIU（`docker checkpoint`）是 experimental，默认 daemon 直接拒绝。

实测时延（含 CLI 开销，本机 daemon）：

| 操作 | 耗时 |
| --- | --- |
| 从快照冷启一个容器并执行一次 python | 0.20 s |
| 已有会话容器上一次 exec | 0.07 s |
| pause | 0.01–0.03 s |
| commit（140 MB 镜像） | 0.49–0.95 s |

## 五、Docker 给不了、必须 WeKnora 自己补的

1. **空闲回收**。daemon 没有任何 TTL/`on_timeout` 概念，只给 `StartedAt` 之类时间戳。
   现在 `SessionBoundManager` 明确把回收甩给 provider（见 `session_manager.go` 头部注释），
   docker 后端要打破这个假设：新增按 label 扫描 + 对账 binding 的 idle reaper，
   多副本部署下必须借 binding store 上锁，避免两个副本同时收。`orphan_reaper.go` 已有对账骨架可复用。
2. **执行超时**。客户端取消不会终止容器内进程（实测：取消后 `sleep` 还在跑）。
   要么把命令包在容器内 `timeout -s KILL <n>`（实测退出码 137，2 s 准时返回），
   要么从 exec inspect 拿 Pid 再发一次 kill。前者更简单，但要求模板里有 `coreutils`。
3. **内存态快照**：没有（见第四节）。
4. **跨主机**：一个 daemon 就是一台机器。多机要自己选机、把快照 push 到 registry、
   在 binding 里记住会话落在哪台机器上——这就是在写一个控制面，也正是当初选择「只接 E2B 协议」的理由。
5. **隔离强度**：共享内核。要更强只能叠 gVisor / Kata（`runtime=runsc`）。
   还有个部署面的硬约束：**能访问 `docker.sock` 等于宿主机 root**，
   所以 app 与 daemon 的边界必须当作信任边界来设计（本机 socket 只给 app、或走 mTLS 的远程 daemon）。
6. **域名级出网策略**：Docker 只有 L3/L4。要做 `RemoteNetworkPolicy` 里的域名 allow/deny，
   得配一个 egress proxy 并在容器里注入 `HTTPS_PROXY`。
7. **目录权限**：`CapDrop: ALL` 之后 root 也越不过权限位（实测 `mkdir` 到 `user` 属主目录被拒）。
   `session_manager.go` 的 `ensureExecutionOutputDir` 现在是「MakeDir 后以 root chown」，
   在 docker 后端会失败。正确做法是模板里预建并 chown 好 `/workspace/{input,output}`，
   运行期只以沙箱账号操作；保留 `CAP_CHOWN`+`CAP_DAC_OVERRIDE` 是下策。

## 六、建议方案

### 形态

新增 `internal/sandbox/docker_remote_client.go`，实现 `RemoteSandboxClient`，
接入 `tenantSandboxResolver` 时和 Cube/E2B 走同一条 `NewSessionBoundManager` 分支。
session→sandbox binding、生命周期锁、能力矩阵、产物收集全部**不改**——这正是当初把
provider 抽象成 `RemoteSandboxClient` 的收益。

- 沙箱 = 长驻容器，PID 1 是 `sleep infinity`，脚本一律走 exec。
- 模板 = 镜像；`RemoteTemplateCatalog` 用 `ImageList`（按 label）+ pull/build 实现，设置页流程不变。
- metadata = labels（`weknora.tenant/session/config/createdAt`），孤儿回收按 label 列举后与 binding 对账。
- 超时 = 容器内 `timeout` 包裹；空闲回收 = 新增的 idle reaper。
- 连接方式 = 本机 unix socket 或远程 daemon over mTLS（`DOCKER_HOST` 风格配置），
  远程地址同样要过 `url_guard.go` 的出网校验。

### 租户配置需要新增的字段

`image`（已有）之外：`host` / TLS 证书、`cpu`、`memory`、`pids_limit`、`network_mode`
（`none` / `bridge` / 指定网络）、`egress_proxy`、`idle_ttl`、`runtime`（可选 `runsc`）、
`snapshot_registry`（多机时必填）。

### 安全基线（默认值）

非 root 执行、`CapDrop: ALL`、`no-new-privileges`、内存/CPU/PID 上限、
只读根文件系统 + `/tmp` tmpfs、默认 `network=none` 由配置显式放开、
默认 seccomp/AppArmor 保持开启、禁止挂载宿主机路径（附件一律走 archive API 写进去）。

### 推荐的部署形态

| 形态 | 适用 | 关键约束 |
| --- | --- | --- |
| 本机 socket | 单机、私有化、开发 | app 与 daemon 同机；`docker.sock` 只暴露给 app 进程 |
| 远程 daemon（mTLS） | 想把沙箱负载和应用分开 | 证书轮换；daemon 端口不得暴露到公网 |
| 每租户独立 daemon/主机 | 强隔离诉求但没有 KVM | 由部署方分配；WeKnora 侧就是不同 config 指不同 host |

### 拆解（按子系统，不含时间估计）

1. `docker_remote_client.go` + 单测（可复用 `remote_fake_test.go` 的口径）——最大的一块。
2. Engine API 的错误映射到 `remote_errors.go` 的 `RemoteErrorKind`（404/409/超时/权限）。
3. idle reaper 与 binding 对账（触碰多副本并发，需要设计评审）。
4. 模板目录：`ImageList`/pull/build 对齐 `RemoteTemplateCatalog`。
5. 租户配置字段、迁移与设置页表单（`frontend/src/views/settings/SandboxSettings.vue`）。
6. `ensureExecutionOutputDir` 改为模板预建 + 非 root 操作（对 Cube/E2B 同样是简化）。
7. `sandbox_check.go` 的 docker 分支从「bind mount 探针」改成「创建容器 → exec → 销毁」。
8. 依赖：引入 `github.com/moby/moby/client`（新的独立模块，比旧的 `github.com/docker/docker` 轻很多）。
9. 文档：`sandbox-protocol.md` 的后端表与选型建议同步改写。

### 不打算做的

跨主机调度器、镜像分发编排、内存态快照。需要这些就该用 E2B 兼容实现，
这条边界是这次调研的前提，不是遗漏。

## 七、复现

```bash
cd docs/poc/docker-sandbox
go run .    # 本机 daemon 用 unix socket 时可能需要 sudo -E
```

29 项检查全部 PASS，其中 5 项 `(GAP)` 断言的是「Docker 做不到什么」（客户端取消不杀进程、
CapDrop 后 root 越不过权限位、快照不含内存态、无空闲 TTL、kill `docker run` 留下在跑的容器）——
它们 PASS 表示差距被稳定复现。

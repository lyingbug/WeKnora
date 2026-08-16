# WeKnora 插件化：从 DeepSeek Harness 学什么，怎么改

本文回答两件事：

1. DeepSeek Harness（`dsh`）的 **Everything is a Plugin** 到底在解决什么问题；
2. WeKnora 现在的「扩展点都写进主仓库、注册写进 `container.go`」该如何按同一思路重构，而又不把 Go 单体硬套成 Node 动态加载器。

配套代码：`internal/plugin`（内核）、`internal/plugin/websearch`（第一条缝）、`plugins/websearch-echo`（树外插件样板）。

---

## 1. dsh 在说什么

dsh 的运行时不是「一个核心 + 一堆插件」，而是 **整棵产品都是插件树**。模型适配器、工具注册表、session 日志、agent loop、沙箱、UI 都是可替换插件。文档原话是：*there is no privileged core to patch*——扩展方式是在旁边再挂一个插件，而不是去改核心源码。

底座是 [Cordis](https://github.com/cordiverse/cordis)。作者需要先建立五个概念：

| 概念 | 含义 |
| --- | --- |
| Plugin | 实现 `Service` 的对象：函数插件（`inject` + `apply(ctx)`）或 Service 子类 |
| Context | 服务仓库。插件把能力挂到稳定的 `ctx.tools` / `ctx.llm` / `ctx.sessions`，别人按 **key** 找，不 import 具体实现 |
| inject | 声明依赖的服务 key。没有这些服务就不挂载，启动顺序由依赖表达，而不是手写 boot 序列 |
| Typed events | `emit` / `waterfall` / `parallel` / `serial`。拦截和策略走事件，直接能力调用走 service 方法 |
| Reversible effects | 注册（prompt 段、工具 schema、adapter、listener）都带 disposer。卸载/热重载按反序撤销 |

组合不是「扫目录乱加载」，而是有分层：

```text
空 entry 列表
  → profile 列出的 bundle（如 dsh-base, dsh-web-app）
  → profile 自己的 cordis.patch.yml
  → home 级 overlay
  → --patch
```

一行 patch 按 **id** 整行替换或插入。`dsh --dump-config` 打出的每一行都可以被用户覆盖。

还有两个和「扩展点接口」不一样的设计：

**Capability seam（能力缝）** 必须三角齐全：

- Service Definition：接口 + `ctx` key
- Service Provider：实现
- Consumer：通常是模型可见的 tool

只写一个实现、没有定义、没有消费者，不叫缝。所以换一个 `ctx.fs` / `ctx.subprocess` provider，Bash、PTY、LSP 会一起走新的执行世界，而不用给每个 tool 分叉。

**Session log 是模型上下文的唯一事实来源。** 能进模型请求的东西必须能从 append-only 日志重建（*Model-visible means logged*）。这是 agent harness 的约束，不是所有产品都要抄，但对 WeKnora 的对话时间轴 / 评测回放有直接借鉴。

dsh 用 TypeScript 工作区包（`packages/<group>/<pkg>`）把 Definition / Provider / Consumer 拆开。加一个包有清单、约束脚本、README 合同（含 Model Experience）。这是「社区能在核心仓库外演进」的物理条件。

---

## 2. WeKnora 现在是什么形态

WeKnora **已经有扩展点，但没有插件系统**。

九条缝都有接口 + 注册表（见 `website-docs/06-development/03-extension-points.md`）：

| 缝 | 接口 | 今天的注册点 |
| --- | --- | --- |
| 文档解析 | `BaseParser` | `docreader/parser/registry.py` 写死 `_build_default_registry()` |
| 分块 | 包级函数变量 + `runTier` switch | `internal/infrastructure/chunker/strategy.go` |
| 检索引擎 | `RetrieveEngineRepository` | `container.go` `initRetrieveEngineRegistry()` + `RETRIEVE_DRIVER` |
| 模型 | `Provider` / `providerAdapter` | `provider.Register` + chat 适配表 |
| 联网搜索 | `WebSearchProvider` | 曾是 `container.go` `registerWebSearchProviders()`，现已迁到 plugin host |
| 数据源 | `Connector` | `container.go` `initConnectorRegistry()` + 元数据 map |
| IM | `Adapter` | `container.go` `registerIMService()` |
| Agent 工具 | `types.Tool` | `definitions.go` + 会话装配 |
| 对象存储 | `FileService` | `file/factory.go` 的 `switch` |

另外还有一条 **问答流水线插件**：`chat_pipeline.Plugin` + `EventManager` 责任链（`next()`），语义上已经是 Cordis waterfall，但插件本身仍在 `container.Invoke(chatpipeline.NewPluginXxx)` 里写死，且和「检索引擎 / IM」不是同一套生命周期。

`ResourceCleaner` 已经按反序执行析构，接近 reversible effect，但只用于进程退出，不能按插件卸载。

这套模式的症状：

1. **加一个 Brave Search / 一个 GitHub 连接器 = 改主仓库 + 改 `container.go`。** ROADMAP 里「鼓励社区维护各厂商组件」在物理上做不到——社区 PR 必须打进单体。
2. **九套注册表，九套约定。** 有的 first-wins，有的 switch，有的 `init()` 覆盖函数变量，有的环境变量门控。没有统一的 dump、disable、overlay。
3. **`container.go` 是隐藏的核心。** 它同时做 DI、条件装配、副作用启动。dsh 要消灭的正是这种「必须改核心才能扩展」的特权点。
4. **实现和定义住在一起。** `internal/im/wecom`、`internal/infrastructure/web_search/bing.go` 都是主模块的一部分，Lite 二进制也会链上全套 SDK。
5. **前端表单、类型常量、工厂注册经常不同步。** 扩展指南要改 4～6 个文件才算「加完」。

WeKnora 比 dsh 重的地方：它是带租户、迁移、RBAC、异步任务的知识库产品，不是纯 agent harness。不能把 `dig` 整棵树拆掉重写；要做的是 **在扩展缝上套一层组合核，用绞杀式（strangler）把注册中枢从 `container.go` 挪走**。

---

## 3. 对得上的映射（抄思路，不抄运行时）

| dsh / Cordis | WeKnora 落地 |
| --- | --- |
| Plugin / `apply(ctx)` | `plugin.Plugin`（`Name` / `Inject` / `Apply`） |
| `ctx.tools` 等 key | `plugin.ServiceWebSearch` 等稳定字符串；现有 `*web_search.Registry` 作为 service |
| inject | `Plugin.Inject()`；Host 按依赖轮转挂载 |
| emit / waterfall / parallel / serial | `plugin.EventBus` 四种模式。`chat_pipeline` 的 `next()` 就是 waterfall，后续可迁到 `chat/*` |
| `ctx.effect()` | `Context.Effect` + 每插件一个 Isolate；`Registry.Unregister` 是 disposer |
| profile / bundle / patch | `config/plugin_profile.yaml` + 代码里的 `base` bundle + `WEKNORA_PLUGINS` |
| `--dump-config` | `Host.Dump()`（启动 debug 日志会打印整棵树） |
| capability seam | 继续用 `internal/types/interfaces` 当 Definition；插件只做 Provider |
| TS 动态 `import()` | **不能进 Go 进程。** 语言插件走同一套 ABI（JSON-RPC），绑定 `runtime: stdio`；轻脚本用 `runtime: js`（goja） |
| `go plugin` `.so` | **不用。** 构建标签、libc、无法跨版本，社区插件会碎 |

Go 没有 Node 那种「`pnpm add` 完同一进程就能 `import`」。接近 TS 手感、且符合业界惯例的是：

1. **`runtime: stdio`（推荐给任意语言）**：Host `exec` 你的进程，在 stdin/stdout 上讲 JSON-RPC 2.0。作者实现 `websearch.search`，**不要自己开端口**。MCP、LSP、Dify 本地插件、HashiCorp 系的「进程外插件」都是这条路。
2. **`runtime: js`**：丢 `search.js`，goja 进程内执行。适合几行适配逻辑；出网走宿主 `httpRequest`（SSRF 白名单）。
3. **Go `plugin.Register` + blank import**：只有要链进主二进制的实现才走这条。
4. **`runtime: http`（fallback）**：对方**已经是**一个远程服务时才写 endpoint。不要为了写插件去起 sidecar。

热重载 / `!!js` 配置表达式仍未做：改磁盘插件后需要重启进程。

### 3.1 业界对照：为什么不把 HTTP 当主路径

「让插件作者开一个 HTTP 服务」看起来语言无关，实际多了端口、健康检查、生命周期和「谁先起来」四个问题。业界把 **协议** 和 **传输绑定** 拆开，本地插件几乎都选字节流，而不是让作者当服务器：

| 方案 | 代表 | 插件作者写什么 | 结论 |
| --- | --- | --- | --- |
| **stdin/stdout + JSON-RPC** | [MCP stdio](https://modelcontextprotocol.io/specification/2026-07-28/basic/transports)、LSP、[Dify 本地 runtime](https://github.com/langgenius/dify-plugin-daemon) | 读 stdin、写 stdout | **语言插件的标准。** 无端口、无 CORS、无鉴权；Host 拉起并回收进程 |
| **子进程 + gRPC / 握手** | [HashiCorp go-plugin](https://github.com/hashicorp/go-plugin)、Terraform Provider、Grafana backend plugin | `plugin.Serve`；握手行打在 stdout，再听 unix/tcp | 适合重 SDK、强类型、双向 RPC。对「一个 search 函数」过重，且作者仍在听套接字 |
| **进程内脚本** | Traefik Yaegi、Kong Lua/PDK、本仓库 goja | 一个函数 | 配置/轻逻辑最快；完整 SDK、原生依赖不合适 |
| **进程内 WASM** | Envoy proxy-wasm、Kong/APISIX WASM | 编成 `.wasm`，同一 ABI | 沙箱最好；要自己养 WASI/host ABI。下一步可以让 WASM 走**同一套** JSON-RPC（WASI stdio），而不是新协议 |
| **`.so` / `plugin.Open`** | Tyk 旧路径、Go plugin | 按本机编译器编共享库 | 编译器、libc、Go 版本钉死，社区插件会碎 |
| **HTTP sidecar** | 早期 MCP HTTP+SSE（已弃用）、自研「插件=微服务」 | 自己 listen | 多一个服务器。MCP 2025-03 已弃用 HTTP+SSE；远程场景改走 Streamable HTTP，**本地场景仍是 stdio** |

同领域的 Dify 也拆过这件事：API 和 daemon 之间可以是 HTTP，但 **daemon 拉起的本地插件走 STDIN/STDOUT**；HTTP 留给 serverless / 已经在跑的远程运行时。WeKnora 对齐的是「作者这一侧」，不是「再让每个人写一个小网站」。

所以主路径是 **一份 ABI，多种绑定**：

```text
JSON-RPC 2.0  （方法：websearch.search / shutdown；一行一条）
    ├─ stdio   Host exec，stdin/stdout     ← 任意语言（推荐）
    ├─ js      goja 进程内                 ← 轻脚本
    ├─ http    已有远程服务                ← fallback
    └─ wasm    （未做）WASI 上同一套 RPC
```

TS 作者的表面是 `serve({ search })`，见 `plugins/sdk-ts/websearch`。样板：`plugins.d/websearch-stdio-echo`（Python）、`plugins.d/websearch-node-echo`（Node）。

---

## 4. 目标结构

```text
internal/plugin/                 内核（无业务类型）
internal/plugin/websearch/       内置联网搜索 bundle
internal/plugin/protocol/        插件 ABI（JSON-RPC 2.0，与传输无关）
internal/plugin/runtime/         stdio / js / http 绑定
internal/plugin/boot/            进程 Host
plugins.d/<id>/plugin.yaml       运行时插件（默认扫描）
plugins/sdk-ts/websearch/        TS：serve() 读 stdin，不要开 HTTP
config/plugin_profile.yaml       用户可改的组成
```

启动：

```text
dig 装配 *web_search.Registry
  → boot.NewHost
      Provide(ServiceWebSearch, registry)
      Discover(WEKNORA_PLUGIN_DIR) → RegisterManifests
      Compose(base + external + profile patch + WEKNORA_PLUGINS)
  → Effect: registry.Register + 动态写入 GetWebSearchProviderTypes()
  → ResourceCleaner 在退出时 Host.Unload()
```

加一个**内置**引擎：`builtins()` 加一行。  
加一个**免编译**引擎：在 `plugins.d/mysearch/` 放 `plugin.yaml` + 可执行入口（`runtime: stdio`）或 `search.js`，重启。类型会出现在 `/web-search-providers/types`。  
**不再改 `container.go`，也不再改 `internal/plugin/boot`。**

---

## 5. 分阶段重构（由外到内）

不要一次把九条缝和 chat pipeline 全搬迁。每条缝的完成标准是：

- Definition 仍在 `types/interfaces`（或 Python 基类）
- Provider 只通过 `plugin.Register` 出现
- `container.go` 不再出现该缝的实现 import
- profile 能 disable / 替换这一行
- 卸载会撤销注册

建议顺序（按「改 container 的收益 / 行为风险」）：

| 阶段 | 缝 | 为什么先做 |
| --- | --- | --- |
| **0（已做）** | 内核 + 联网搜索 | 工厂表最干净，租户参数运行时实例化，正好当样板 |
| **1** | 数据源连接器、IM 适配器 | 已经是 `Register(factory)`，和搜索同构；`container.go` 里 import 最多 |
| **2** | 对象存储 `factory.go` switch、模型 Provider | switch / 全局 map 改成 seam registry |
| **3** | 检索引擎 | 注意 `RETRIEVE_DRIVER`、`EngineFactory`、租户 `vector_stores` 运行时建连 |
| **4** | Agent 工具 + chat_pipeline | 工具已有 Registry；pipeline 已是 waterfall，改成 `chat/*` 事件即可与内核合流 |
| **5** | 分块策略、docreader 解析器 | 分块是函数变量；解析器在 Python 进程，需要独立 plugin 清单或 gRPC 能力协商 |
| **6** | 前端缝 | 搜索/连接器/IM 的表单按插件元数据渲染，避免再改 Vue 才能「加完」 |
| **7** | 包边界 | 低频实现迁到独立 Go module；Lite 用 build tag 或 profile 去掉重 SDK |
| **8（搜索已做）** | 进程外 / 磁盘加载 | 主路径 `runtime: stdio`（JSON-RPC）；`js` 轻脚本；`http` 仅 fallback |

每阶段保持绞杀：旧接口不变，Host 先 Provide 现有 Registry，再让插件往上注册。

### 不要做的事

- 不要用 `plugin.Open`（Go `.so`）当社区分发手段。
- 不要把 dig 换成自研 DI。dig 继续管 Handler/Service/DB；plugin Host 只管 **可替换能力**。
- 不要把业务规则（RBAC、配额、SSRF 白名单）做成「可卸载插件」。策略可以 listen 事件，但不能让社区插件关掉安全底线。
- 不要在第一阶段追求热重载。磁盘插件改完重启即可；HMR 是 Cordis 的 TS 特权。

---

## 6. 今天怎么加一个联网搜索插件

内置（仍在本仓库，但不再碰 container）：

1. `internal/infrastructure/web_search/brave.go` 实现接口（API URL 硬编码）。
2. `internal/types/web_search_provider.go` 加类型常量与 `GetWebSearchProviderTypes()` 元数据（前端下拉仍读这里，阶段 6 再改成插件清单）。
3. `internal/plugin/websearch/plugins.go` 的 `builtins()` 加 `{"brave", web_search.NewBraveProvider}`。

免编译（推荐给社区 / TS）：

```bash
# 默认扫描 plugins.d/。
# 任意语言：plugins.d/websearch-stdio-echo/（Python）或 sdk-ts 的 serve()
# 轻脚本：plugins.d/websearch-js-echo/
```

Go 样板（仍要编进二进制时）：

```bash
WEKNORA_PLUGINS=websearch.echo
```

或在 `config/plugin_profile.yaml`：

```yaml
patch:
  - id: websearch.echo
    plugin: websearch.echo
    insert: true
    config:
      title: echo
```

关掉某个内置引擎：

```yaml
patch:
  - id: websearch.exa
    disabled: true
```

环境变量：

| 变量 | 作用 |
| --- | --- |
| `WEKNORA_PLUGIN_PROFILE` | profile 路径，默认 `config/plugin_profile.yaml` |
| `WEKNORA_PLUGIN_PATCH` | 额外 overlay YAML（只读其中的 `patch`） |
| `WEKNORA_PLUGINS` | 逗号分隔 factory id，insert-if-missing |
| `WEKNORA_PLUGIN_DIR` | 运行时扫描目录，默认 `plugins.d`；`none` 关闭 |

---

## 7. 和 chat_pipeline「插件」的关系

`internal/application/service/chat_pipeline` 的 `Plugin.OnEvent(..., next)` 已经是 waterfall。它解决的是 **一次问答的阶段编排**，不是 **进程级能力组合**。两者要合并，而不是互相替代：

- 进程级：谁提供搜索 / 检索 / IM（本文的 Host）
- 请求级：`QUERY_UNDERSTAND` → `CHUNK_SEARCH` → …（现有 EventManager）

阶段 4 可以把每个 `NewPluginXxx` 改成一个 `plugin.Plugin`，在 `Apply` 里 `ctx.Events().On("chat/chunk_search", ...)`，`EventManager.Trigger` 改成 `Waterfall`。在此之前不要动问答语义。

---

## 8. 验收

内核与第一条缝的自动化测试：

```bash
go test ./internal/plugin/... ./internal/plugin/websearch/... ./internal/plugin/boot/... \
  ./plugins/websearch-echo/... ./internal/infrastructure/web_search/
```

期望：

- Context Provide / 覆盖 / 卸载还原
- waterfall 可短路，effect 反序撤销
- profile patch 能 disable `websearch.exa`
- `WEKNORA_PLUGINS=websearch.echo` 能挂上 `echo` 而不改 container
- `Host.Unload` 之后 `Registry.Has("duckduckgo") == false`

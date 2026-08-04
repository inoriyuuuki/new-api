# new-api 代码架构梳理（只读分析）

> 项目：https://github.com/inoriyuuuki/new-api （fork 自 QuantumNous/new-api，源于 one-api）
> 代码位置：`work/new-api`（main 分支，工作区干净）
> 本文仅做只读分析，未修改任何代码文件。

## 1. 技术栈

| 层 | 技术 |
| --- | --- |
| 后端语言 | Go 1.25+（`go.mod` 声明 `go 1.25.1`，官方镜像用 `golang:1.26.1-alpine`） |
| Web 框架 | Gin（`github.com/gin-gonic/gin v1.9.1`） |
| ORM | GORM v2（`gorm.io/gorm` + sqlite/mysql/postgres/clickhouse 驱动） |
| 缓存 | Redis（`go-redis/v8`）+ 进程内存缓存（双缓存，可只开内存） |
| 认证 | Session Cookie / PAT + JWT + WebAuthn Passkey + OAuth（GitHub/Discord/OIDC/LinuxDO/微信/Telegram）+ 2FA |
| 权限 | Casbin（`github.com/casbin/casbin/v2`，`service/authz/`） |
| 计费 | 自研 quota 体系（`common.QuotaPerUnit = 500 * 1000.0`，即 1 美元 = 500000 quota）+ Stripe/ePay/Creem/Waffo 支付 |
| 前端 | React 19 + TypeScript + Rsbuild（Vite 系）+ Base UI + Tailwind CSS 4 + TanStack Router/Query/Table + Zustand + i18next |
| 前端包管理 | Bun（`web/bun.lock`，Dockerfile 用 `oven/bun` 构建） |
| 其他 | WebSocket（realtime）、SSE 流式、gopool 协程池、Pyroscope 性能剖析、多语言 i18n（go-i18n） |

分层架构：`router → middleware → controller → service → model`，转发引擎独立为 `relay/`。

## 2. 入口与启动流程

入口：`main.go`（唯一 main 包）。

启动序列（`main.go`）：
1. `InitResources()`：加载 `.env`（godotenv）→ `common.InitEnv()`（解析全部环境变量）→ 初始化 logger、ratio 设置、HTTP client、token 编码器
2. `model.InitDB()`：选择主库（SQLite/MySQL/PostgreSQL）并 `AutoMigrate` 全部表（`model/main.go` 的 `migrateDB()`）
3. `authz.Init(model.DB)`：初始化 Casbin 授权
4. `model.CheckSetup()`：判断是否首次部署；当前启动链路不会调用遗留的 `createRootAccountIfNeed()`，首次管理员由匿名 `/api/setup` 初始化向导创建
5. `model.InitOptionMap()`：把数据库 `options` 表读入 `common.OptionMap` 内存
6. `model.InitLogDB()`：日志库（可独立配置，支持 ClickHouse）
7. `common.InitRedisClient()`：可选 Redis
8. 若干后台任务（goroutine）：
   - `model.InitChannelCache()` + `go model.SyncChannelCache(SyncFrequency)`：渠道缓存初始化与定时同步
   - `go model.SyncOptions(SyncFrequency)`：热更新系统配置
   - `go authz.StartPolicySync(SyncFrequency)`：授权策略多节点同步
   - `go model.UpdateQuotaData()`：看板配额数据
   - `service.StartSystemTaskRunner()` + `controller.RegisterScheduledSystemTasks()`：渠道测试/上游模型更新/异步任务轮询的定时任务（DB 租约去重）
   - `service.StartCodexCredentialAutoRefreshTask()`、`service.StartSubscriptionQuotaResetTask()`、`service.StartSystemInstanceReporter()`
9. 启动 gin server：`router.SetRouter`（见 `router/main.go`），默认端口 `3000`（`common/init.go` 的 `Port` flag）
10. 优雅退出：`SHUTDOWN_TIMEOUT_SECONDS`（默认 120s），SSE 流可拖长关闭时间；`DataExportEnabled` 时把内存看板数据落库

## 3. 主要目录/模块

| 目录 | 职责 | 关键文件 |
| --- | --- | --- |
| `router/` | 路由注册 | `api-router.go`（/api 后台）、`relay-router.go`（/v1 转发）、`dashboard.go`（/dashboard/billing）、`web-router.go`（前端 SPA）、`video-router.go` |
| `middleware/` | 认证/限流/分发/审计 | `auth.go`（UserAuth/AdminAuth/RootAuth/TokenAuth）、`distributor.go`（渠道分发）、`rate-limit.go`、`model-rate-limit.go`、`audit.go` |
| `controller/` | HTTP 处理器 | `relay.go`（转发主流程）、`channel.go`、`token.go`、`user.go`、`topup.go`、`subscription.go`、`pricing.go`、`system_task.go` |
| `service/` | 业务逻辑 | `channel_select.go`（渠道选择+重试）、`billing_session.go`/`quota.go`/`tiered_settle.go`（计费）、`task_polling.go`、`authz/`（Casbin） |
| `model/` | GORM 数据层 + 缓存 | `main.go`（DB 初始化/迁移）、`channel_cache.go`（渠道缓存）、`ability.go`、`token.go`、`user.go`、`option.go`、`log.go` |
| `relay/` | 转发引擎 | `relay_adaptor.go`（供应商注册）、`channel/adapter.go`（Adaptor 接口）、`channel/*`（各供应商）、`common/relay_info.go`（RelayInfo）、`helper/`（价格计算） |
| `setting/` | 配置子系统 | `ratio_setting/`（倍率）、`model_setting/`、`operation_setting/`、`system_setting/`、`billing_setting/`、`performance_setting/` |
| `common/` | 通用工具 | `env.go`（环境变量）、`redis.go`、`database.go`、`constants.go`、`quota_math.go`、`ssrf_protection.go` |
| `constant/` | 常量 | `channel.go`（58 种渠道类型）、`api_type.go`、`context_key.go`、`cache_key.go` |
| `dto/` | 请求/响应 DTO | `openai_request.go`、`claude.go`、`gemini.go`、`channel_settings.go` 等 |
| `types/` | 内部类型 | `error.go`（NewAPIError）、`price_data.go`、`relay_format.go` |
| `i18n/` | 后端多语言 | `keys.go`、`locales/` |
| `oauth/` | OAuth 供应商 | `github.go`、`oidc.go`、`linuxdo.go`、`registry.go`（可 DB 自定义） |
| `pkg/` | 内部包 | `cachex/`、`billingexpr/`、`ionet/`、`perf_metrics/` |
| `web/` | React 前端 | `src/routes/`、`src/features/`（auth/channels/wallet/…）、`src/i18n/` |
| `electron/` | 桌面壳（可选） | — |

## 4. 请求链路

### 4.1 转发请求（核心，`/v1/*`）

路由注册：`router/relay-router.go` → 中间件链：

```
CORS → DecompressRequest → BodyStorageCleanup → Stats
→ RouteTag("relay") → SystemPerformanceCheck → TokenAuth（sk- 令牌校验）
→ ModelRequestRateLimit → Distribute（选择渠道）
→ controller.Relay(c, relayFormat)
```

`controller/relay.go` 的 `Relay()` 主流程：
1. `helper.GetAndValidateRequest`：按格式解析请求体（OpenAI/Claude/Gemini/…）
2. `relaycommon.GenRelayInfo`：构造 `RelayInfo`（token/用户/分组/模型信息，`relay/common/relay_info.go`）
3. 敏感词检查（`service.CheckSensitiveText`）+ `service.EstimateRequestToken` 预估 token
4. `helper.ModelPriceHelper`：计算价格/预扣额度（依赖 `setting/ratio_setting` 的倍率和 `model.GetPricing` 的价格表）
5. `service.PreConsumeBilling`：预扣费（`BillingSession`，见第 7 节）
6. **重试循环**（`RetryTimes` 上限）：
   - `service.CacheGetRandomSatisfiedChannel`（`service/channel_select.go`）→ `model.GetRandomSatisfiedChannel` 从缓存取渠道
   - `middleware.SetupContextForSelectedChannel` 写入渠道上下文
   - 按格式分发：`relayHandler`（Text/Image/Audio/Embedding/Rerank/Responses）或 `ClaudeHelper`/`GeminiHelper`/`WssHelper`
   - 失败时 `processChannelError`（自动封禁/多 key 切换）→ `shouldRetry` 决定是否换渠道重试
7. 成功 → `PostTextConsumeQuota` / `SettleBilling` 按实际用量结算；失败 → `Refund` 退还预扣 + 可选违规费（`service/violation_fee.go`）

### 4.2 供应商适配

`relay/channel/adapter.go` 定义 `Adaptor` 接口：
`Init → GetRequestURL → SetupRequestHeader → ConvertOpenAIRequest/ConvertClaudeRequest/ConvertGeminiRequest → DoRequest → DoResponse → GetModelList`。

`relay/relay_adaptor.go` 的 `GetAdaptor(apiType)` 按 `constant.APIType*` 注册 40+ 供应商（openai/claude/gemini/aws/ali/baidu/zhipu/…/codex/advancedcustom 等），新增供应商只需实现接口 + 注册。

异步任务（Midjourney/Suno/视频）：`TaskAdaptor` 接口（提交/轮询/按结果结算差额），由 `service/task_polling.go` + 系统任务调度执行。

### 4.3 后台管理请求（`/api/*`）

`router/api-router.go`：/api/setup、/api/user（注册/登录/2FA/passkey）、/api/token、/api/channel、/api/topup、/api/payment（stripe/epay/creem/waffo webhook）、/api/authz 等，均挂 `UserAuth`/`AdminAuth`/`RootAuth`。

### 4.4 OpenAI 兼容计费接口

`router/dashboard.go`：`/dashboard/billing/subscription|usage`（v1 兼容），供 OpenAI SDK 查询余额。

## 5. 数据库与缓存抽象

### 5.1 数据库（`model/main.go`）
- 主库：`SQL_DSN`（mysql/postgres）或 `SQLITE_PATH`（默认 `one-api.db`）；日志库：`LOG_SQL_DSN`（额外支持 ClickHouse）
- `DB` / `LOG_DB` 两个 GORM 全局句柄；不同方言的列名/布尔值差异用 `initCol()` 归一化（`` `group` `` 等保留字）
- 主要表（`migrateDB()` AutoMigrate）：`channels`、`tokens`、`users`、`user_sessions`、`abilities`（渠道-分组-模型映射）、`options`（系统配置）、`logs`、`redemptions`、`topups`、`tasks`、`models`/`vendors`、`subscription_plans`/`user_subscriptions`、`casbin_rules`/`authz_roles` 等
- 首次启动自动创建 root 用户（`root/123456`），并有 setup 向导状态（`model/setup.go`）

### 5.2 缓存（`common/redis.go` + `model/channel_cache.go`）
- Redis 启用条件：设置了 `REDIS_CONN_STRING`；启用 Redis 时强制 `MemoryCacheEnabled=true`
- **渠道缓存**：内存 `group2model2channels map[group][model][]channelId`（按优先级降序）+ `channelsIDM`（全量渠道，含禁用）+ AdvancedCustom 配置缓存；`InitChannelCache` 启动加载，`SyncChannelCache` 每 `SYNC_FREQUENCY`（默认 60s）全量重建
- **token/用户缓存**：`model/token_cache.go`、`model/user_cache.go`（`GetUserCache`）
- 系统配置热更新：`go model.SyncOptions` 定期把 `options` 表刷入 `common.OptionMap`

## 6. 前端构建如何嵌入后端

- 前端源码在 `web/`（React 19 + Rsbuild + TS），构建产物 `web/dist`
- Dockerfile 两阶段：
  1. `oven/bun` 镜像里 `bun install --frozen-lockfile && bun run build` 产出 `web/dist`
  2. `golang:1.26.1-alpine` 里 `COPY --from=builder /build/web/dist ./web/dist`，再 `go build`（`-X common.Version=$(cat VERSION)` 注入版本号）
- 后端嵌入：`main.go` 里 `//go:embed web/dist` 的 `buildFS` + `//go:embed web/dist/index.html` 的 `indexPage`
- 服务：`router/web-router.go` 用 `common.EmbedFolder`（`common/embed-file-system.go`）包一层 embed FS，`gin-contrib/static` 托管静态资源；`NoRoute` 兜底返回 `index.html`（SPA 路由），`/v1`、`/api`、`/assets` 前缀则返回 404
- 可选项：设 `FRONTEND_BASE_URL` 时跳过内嵌前端，直接 301 到外部前端（master 节点忽略该设置）
- 开发模式：`Dockerfile.dev` 用占位 `web/dist/index.html` + 前端 `bun run dev`（5173 端口）；`makefile` 提供 `build-web/start-api/dev` 等

## 7. 配置加载

三级配置：
1. **命令行 flag**（`common/init.go`）：`--port`、`--log-dir`、`--version`、`--help`
2. **环境变量**（`common/env.go` + `common/init.go` 的 `InitEnv`）：`.env` 文件（godotenv）→ `GetEnvOrDefault*` 系列；重要项见 `.env.example`：`SESSION_SECRET`、`SQL_DSN`、`SQLITE_PATH`、`LOG_SQL_DSN`、`REDIS_CONN_STRING`、`MEMORY_CACHE_ENABLED`、`SYNC_FREQUENCY`、`NODE_TYPE`（master/slave 多节点）、`BATCH_UPDATE_ENABLED`、`TLS_INSECURE_SKIP_VERIFY`、`TRUSTED_PROXIES` 等；`initConstantEnv()` 填充 `constant.*`
3. **数据库配置**（`options` 表 → `common.OptionMap`，`model/option.go` 的 `InitOptionMap`）：系统名/公告/注册开关/支付/SMTP/倍率等运行时配置，由 `setting/*` 各子包读取；`SyncOptions` 热更新
4. 另有 `setting/ratio_setting` 管理的模型倍率、分组倍率（`GetModelRatio`/`GetGroupRatio`），以及价格表 `model.GetPricing()`（缓存 + 定时刷新 `model/pricing_refresh.go`）

## 8. 核心流程

### 8.1 鉴权
- **后台**：`middleware/auth.go` — `classifyDashboardCredential` 区分 Session Cookie / PAT / 未匹配，`authHelper` 校验用户状态与角色（common/common/root），`UserAuth`/`AdminAuth`/`RootAuth` 对应三级权限；管理写操作自动审计（`audit.go`）
- **API 令牌**：`TokenAuth` — 从 `Authorization: Bearer sk-xxx` 提取 key，`model.GetTokenByKey` 校验（Redis/内存缓存），检查 token 状态/模型白名单/分组，写 context（`id`/`token_id`/`token_key`/`using_group` 等）
- **Casbin 授权**：`service/authz/` 提供细粒度资源/操作权限（`/api/authz/catalog`），`authz.StartPolicySync` 多节点同步
- 其他：2FA（`model/twofa.go`）、Passkey（WebAuthn）、OAuth（`oauth/`）、Session 管理（`model/user_session.go`，限额/吊销/审计）

### 8.2 渠道（Channel）
- 数据结构：`model/channel.go` — 类型（58 种，`constant/channel.go`）、分组（`Group` 逗号分隔）、模型列表（`Models`）、优先级（`Priority`）、多 key 模式（轮询/负载均衡）、自动封禁、模型映射（`ModelMapping`）、`Other` 参数（azure api version、coze bot_id 等）
- 能力表：`model/ability.go`（渠道-分组-模型三要素），渠道增删改时同步刷新；渠道缓存每 `SYNC_FREQUENCY` 全量重建
- 选择逻辑：`middleware/distributor.go`（Distribute）→ `service/channel_select.go` 的 `CacheGetRandomSatisfiedChannel`：按分组→模型→优先级随机取可用渠道；`auto` 分组跨组重试；渠道亲和性（`service/channel_affinity.go`）偏好最近成功渠道；`SetupContextForSelectedChannel` 注入渠道元信息
- 健康检查：`controller/channel-test.go` 测试渠道；`controller/channel-upstream_update.go` 上游模型自动同步；定时任务自动禁用/启用异常渠道

### 8.3 计费（Quota/Billing）
- 配额单位：`common/constants.go` `QuotaPerUnit = 500000`（$1 = 500000 quota）；token 消耗按 `倍率 = 分组倍率 × 模型倍率` 计算（`setting/ratio_setting`），支持按价计费（`UsePrice`，模型价格表 `model.GetPricing()`）
- 流程（`service/billing_session.go`）：
  - `NewBillingSession` 按用户计费偏好选择资金来源：钱包（`WalletFunding`）或订阅（`SubscriptionFunding`），支持 `subscription_first/wallet_first/subscription_only/wallet_only`
  - `PreConsumeBilling` 预扣 → 请求执行 → `Settle(actualQuota)` 结算差额（多退少补）→ 失败 `Refund` 全额退还（幂等，异步 gopool）
  - 信任额度旁路（trusted）、`QuotaClamp` 封顶、订阅按剩余额度结算等细节
- 异步任务计费：`TaskAdaptor.EstimateBilling`（预扣）→ `AdjustBillingOnSubmit`（按上游实际参数调整）→ `AdjustBillingOnComplete`（终态差额结算/退款），见 `service/task_billing.go`、`service/tiered_settle.go`
- 充值：ePay（`service/epay.go`）、Stripe、Creem、Waffo（Pancake）webhook 回调 + 兑换码（`model/redemption.go`）+ 签到（`model/checkin.go`）
- 用量记录：`model/log.go`（日志可独立库）、`model/usedata.go`（看板数据，内存聚合定时落库）

## 9. 部署要点（供后续使用）

- 单二进制 `new-api`，默认监听 `3000`，工作目录 `/data`（Docker 默认 `WORKDIR /data`，SQLite 落此目录）
- 默认 SQLite，可用 `SQL_DSN` 切 MySQL/PostgreSQL；生产建议设 `SESSION_SECRET`、`SQLITE_PATH`/`SQL_DSN`、`MEMORY_CACHE_ENABLED=true`、`REDIS_CONN_STRING`（多节点时）
- 首次启动自动建 root 用户 `root/123456`，管理后台在 `/`（SPA 已内嵌）
- 提供 `new-api.service`（systemd 单元，源码部署可直接使用）和 `docker-compose.yml` / `docker-compose.dev.yml`

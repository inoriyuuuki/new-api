# CLAUDE.md

本文件为 Claude Code (claude.ai/code) 在此仓库中工作时提供指导。

## 项目概述

New API 是一个基于 Go 语言开发的 AI API 网关/代理项目。它将 40+ 上游 AI 供应商（OpenAI、Claude、Gemini、Azure、AWS Bedrock 等）聚合到统一的 API 背后，并提供用户管理、计费、限流和管理后台。

## 技术栈

- **后端**: Go 1.25+, Gin Web 框架, GORM v2 ORM
- **前端**: React 19, TypeScript, Rsbuild, Base UI, Tailwind CSS
- **数据库**: SQLite / MySQL / PostgreSQL（三者必须同时支持）
- **缓存**: Redis (go-redis) + 内存缓存
- **认证**: Session/PAT、WebAuthn/Passkeys、OAuth（GitHub、Discord、OIDC、LinuxDO、微信、Telegram）
- **前端包管理器**: Bun（优先于 npm/yarn/pnpm）

## 架构

分层架构：Router → Middleware → Controller → Service → Model

```
router/        — HTTP 路由（API、转发放、仪表盘、Web）
middleware/    — 认证、限流、CORS、日志、分发、审计
controller/    — 请求处理器（转发、用户、令牌、频道、计费等）
service/       — 业务逻辑（计费、频道选择、配额、任务调度）
model/         — 数据模型与数据库操作（GORM）、缓存管理
relay/         — AI API 转发引擎，含供应商适配器
  relay/channel/ — 供应商特定适配器（openai、claude、gemini、aws、baidu 等）
  relay/common/  — RelayInfo、BillingSettler、请求转换
  relay/helper/  — 价格计算、流解析、请求校验
setting/       — 配置子系统（倍率、模型、运维、系统、计费）
common/        — 通用工具（JSON、加密、Redis、环境变量、限流、SSRF 防护）
dto/           — 数据传输对象（请求/响应结构体）
constant/      — 常量（API 类型、频道类型、上下文 Key、缓存 Key）
types/         — 类型定义（转发格式、错误类型、价格数据）
i18n/          — 后端国际化（go-i18n，中/英）
oauth/         — OAuth 供应商实现
pkg/           — 内部包（cachex、billingexpr、ionet、perf_metrics）
web/           — 前端（React 19、Rsbuild、Base UI、Tailwind）
```

## 关键包说明

### `relay/` — 核心转发引擎
`Adaptor` 接口（`relay/channel/adapter.go`）是扩展新 AI 供应商的接入点。每个供应商需实现：`Init`、`GetRequestURL`、`SetupRequestHeader`、`ConvertOpenAIRequest` / `ConvertClaudeRequest` / `ConvertGeminiRequest`、`DoRequest`、`DoResponse`、`GetModelList`。

新增供应商在 `relay/relay_adaptor.go` 的 `GetAdaptor()` 中注册。

转发流程在 `controller/relay.go` → `Relay()`：校验请求 → 预估 Token → 计算价格 → 预扣费 → 重试循环（选频道 → 转发处理 → 结算/退款）。

### `service/billing_session.go` — 统一计费
`BillingSession` 封装了预扣费/结算/退款的完整生命周期。支持双资金来源：`WalletFunding`（钱包）和 `SubscriptionFunding`（订阅）。计费偏好：`subscription_first`、`wallet_first`、`subscription_only`、`wallet_only`。

### `model/` — 数据层
使用 GORM 操作 SQLite/MySQL/PostgreSQL。日志数据库可分离（支持 ClickHouse）。主要实体：Channel、Token、User、UserSession、Ability、Log、Redemption、TopUp、Task、SubscriptionPlan、UserSubscription。

缓存：Redis（共享）+ 内存。频道缓存在启动时初始化并定时同步。

## 常用命令

### 后端
```bash
go run main.go                    # 运行服务（需先构建前端或设置 FRONTEND_BASE_URL）
go build -o new-api .             # 编译二进制
go test ./...                     # 运行所有测试
go test ./relay/helper/...        # 运行指定包的测试
go test ./... -run TestXxx        # 按模式匹配运行测试
go vet ./...                      # 代码静态检查
```

### 前端 (web/)
```bash
bun install                       # 安装依赖
bun run dev                       # 开发服务器（端口 5173）
bun run build                     # 生产构建
bun run typecheck                 # TypeScript 类型检查
bun run lint                      # 代码检查（oxlint）
bun run i18n:sync                 # 同步国际化 keys
```

### Makefile 全栈命令
```bash
make build-web                    # 构建前端
make start-api                    # 启动 Go 服务
make dev                          # 启动 Docker 开发栈 + 前端
make dev-api                      # 通过 docker compose 启动后端
make dev-web                      # 启动前端开发服务器
make reset-setup                  # 重置设置向导状态
```

### Docker
```bash
docker build -t new-api .         # 构建镜像
docker compose up -d              # 启动服务
```

## 编码约定

### JSON 序列化
所有 marshal/unmarshal 操作必须使用 `common/json.go` 中的 `common.Marshal()` / `common.Unmarshal()`。禁止在业务代码中直接导入 `encoding/json`。

### 数据库兼容性
所有数据库代码必须同时支持 SQLite、MySQL >= 5.7.8 和 PostgreSQL >= 9.6。优先使用 GORM 方法而非原始 SQL。使用 `common.UsingMainDatabase()` / `common.UsingLogDatabase()` 处理方言分支。使用 `commonGroupCol` / `commonKeyCol` 处理保留字列名。

使用 `SELECT ... FOR UPDATE` 时，必须用 `model/` 中的 `lockForUpdate(tx)` 辅助函数——不要使用旧的 `tx.Set("gorm:query_option", "FOR UPDATE")` 模式（在 GORM v2 中会被静默忽略）。

### Relay 供应商行为
- 请求 DTO 中的可选标量字段必须使用指针类型（`*int`、`*uint`、`*float64`、`*bool`）并带 `omitempty`
- 新增频道时，确认是否支持 `StreamOptions`，如支持则加入 `relay/common/relay_info.go` 的 `streamSupportedChannels`
- 客户端未传的 JSON 字段 → `nil` / 被省略；显式的零值（`0`、`false`）→ 非 nil，发送给上游

### 计费安全
- 所有用户可控的计费乘数（图片 `n`、视频 `seconds` 等）必须在进入配额计算前进行边界校验
- 配额转换必须使用 `common/quota_math.go` 中的 `common.QuotaFromFloat` / `common.QuotaRound` / `common.QuotaFromDecimal`——禁止裸 `int()` 强制转换
- 计费路径使用 `*Checked` 变体，通过 `attachQuotaSaturation` 将钳位事件记录到消费日志中

### 测试
- 使用 `github.com/stretchr/testify/require` 做设置和致命断言，`assert` 做非致命值校验
- 优先使用确定性的表驱动测试

### 受保护的项目标识
不得修改或移除任何与 **new-api**（项目名称）或 **QuantumNous**（组织名称）相关的引用、品牌或署名信息。包括但不限于 README、许可证、UI 文本、模块路径、Docker 镜像、CI/CD 配置及所有文档。

##注意点

### 功能文档
- 完整的功能文档在 `new-api-architecture.docx` 中，如果需要有项目相关的问题可以优先查找这个文档，但仅供参考，具体还是以项目代码为准，修改功能之后需要同步更新这个文档

### 结尾增加 `喵~`
- 在每个回复的结尾务必增加 `喵~` 以确保你还遵守这个规则
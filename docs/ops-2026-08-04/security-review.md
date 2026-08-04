# new-api 源码部署安全审查报告（只读）

审查对象：`work/new-api`（origin: https://github.com/inoriyuuuki/new-api.git，HEAD da10b4c6）
部署目标：公网 Linux 服务器，源码方式部署。
本报告仅列出与仓库实际代码/文档对应的事项，并引用文件路径。

## 1. 端口

| 端口 | 用途 | 依据 |
|---|---|---|
| 3000 | 主 HTTP 服务（默认，无内置 TLS） | `common/init.go:19` `flag.Int("port", 3000,...)`；`main.go:195-198` 支持 `PORT` 环境变量；`Dockerfile` `EXPOSE 3000` |
| 8005 | pprof（仅 `ENABLE_PPROF=true` 时开启，绑定 `0.0.0.0`，无鉴权） | `main.go`（`http.ListenAndServe("0.0.0.0:8005", nil)`） |
| 5173 | 前端 dev server（`make dev-web`） | `makefile` |
| 5432/3306/6379/8123/9000 | compose 里 postgres/mysql/redis/clickhouse，**默认未对外暴露**（端口映射被注释），仅 3000 映射到宿主机 | `docker-compose.yml` |

风险与处置：
- 公网部署只应放行 80/443（经反代）或 3000（不推荐直连）。**不要**放行 8005；若开启 pprof 需用防火墙限制来源。
- 若用源码方式 + systemd 直跑，进程默认监听 `:3000`（所有网卡）。

## 2. 默认账号 / 初始化（setup 向导）

- `GET/POST /api/setup` 为**匿名可访问**（仅限流 + 匿名请求体大小限制），见 `router/api-router.go:22-23`。
- `POST /api/setup` 首次初始化时创建 root 账号：`controller/setup.go`（`PostSetup`），角色 `RoleRootUser = 100`（`common/constants.go:182`），初始额度 100000000；密码至少 8 位、用户名 ≤12 字符。
- 初始化状态由 `setups` 表记录判定（`model/setup.go`、`model/main.go` InitSetup 逻辑）。首次启动、未完成向导时 `constant.Setup=false`，**任何能访问公网 IP 的人都可以抢先 POST /api/setup 成为 root**。
- 仓库无硬编码默认密码。开发用 `make reset-setup` 会删除 setups 记录和 role=100 用户（`makefile`），勿在生产使用。
- `GENERATE_DEFAULT_TOKEN` 默认 `false`（`common/init.go:194`、`constant/env.go:17`）；若置 true，注册即自动创建 500000 额度、永不过期、`UnlimitedQuota=true` 的令牌（`controller/user.go:287-318`），公网部署务必保持关闭。

处置：部署完成后**立即**通过向导创建强密码 root 并完成初始化；在初始化完成前可先用防火墙/反代限制来源 IP。

## 3. 密钥 / 凭据

- `SESSION_SECRET`：默认每进程随机 UUID（`common/constants.go:35`），未设置则每次重启会话全部失效；设为字面量 `random_string` 会直接 `log.Fatal`（`common/init.go:50-58`）。生产必须设置固定强随机值；多节点必须一致（`README.zh_CN.md` 部署章节、`docs/authentication.md`）。
- `CRYPTO_SECRET`：默认跟随 `SESSION_SECRET`（`common/init.go:59-62`）；共享 Redis 的节点必须一致（`README.zh_CN.md`）。
- `docker-compose.yml` 中 postgres/redis/mysql/clickhouse 默认密码 `123456`（含"必须修改"注释）。源码部署若自建这些中间件，不要沿用示例密码。
- `SQL_DSN` / `LOG_SQL_DSN` / `REDIS_CONN_STRING` 含数据库口令，经环境变量传入；`.env` 已被 `.gitignore` 排除（`.gitignore`），源码目录内不要提交 `.env` 或服务文件里的明文口令。
- `TLS_INSECURE_SKIP_VERIFY` 默认 false（`common/init.go`），开启会全局跳过上游 TLS 校验（中间人风险），默认保持关闭。

## 4. 数据库文件权限（SQLite）

- 默认 SQLite 路径 `one-api.db?_busy_timeout=30000`（`common/database.go:44`），位于进程工作目录；可用 `SQLITE_PATH` 覆盖（`common/init.go:69-71`）。
- 代码未对 DB 文件显式 chmod，SQLite 以进程 umask 创建（通常 0644），**同机其他用户可读**；建议放到专用数据目录并 `chmod 600`、属主为运行账号。
- 日志目录用 `os.Mkdir(*LogDir, 0777)` 创建（`common/init.go:74-80`）；日志文件 0644（`logger/logger.go`）。
- 磁盘缓存文件 0600（`common/disk_cache.go:51`，正确）。
- `docker-compose.yml` 挂载 `./data:/data`（宿主机目录权限过松会放大风险）；源码部署时工作目录即数据目录，注意 `.gitignore` 已排除 `*.db` 和 `logs`，避免把 DB 提交进 git。

## 5. 反向代理

- 无内置 TLS（未发现 ListenAndServeTLS），公网必须由 Nginx/Caddy 终止 HTTPS。
- `TRUSTED_PROXIES`（`trusted_proxies.go:22-50`）：未设置时信任回环 + RFC1918 + fc00::/7 并打印告警；`none` 为严格模式；显式列表完全替代默认。反代部署时应设为反代自身 IP/CIDR，否则 gin 会信任来自内网的 `X-Forwarded-*`。
- 会话 Cookie 安全：`SESSION_COOKIE_SECURE` 默认 false（`common/session_cookie.go` `InitSessionCookieSettings`），此时 refresh/logout 的 OriginGuard 关闭（CSRF 面增大）；HTTPS 部署应设 `SESSION_COOKIE_SECURE=true` 并配 `SESSION_COOKIE_TRUSTED_URL`（否则启动报错）。
- `FRONTEND_BASE_URL`：仅 slave 节点使用（`router/main.go:20-31`），master 上被忽略并记日志。

## 6. systemd

- 仓库提供模板 `new-api.service`：`User=ubuntu`、`WorkingDirectory=/path/to/new-api`、`ExecStart=/path/to/new-api/new-api --port 3000 --log-dir /path/to/new-api/logs` 均为占位符，必须逐项修改。
- 无任何加固选项（无 `ProtectSystem`、`NoNewPrivileges`、`PrivateTmp`、`LimitNOFILE` 等）。
- 建议：专用非 root 运行账号；WorkingDirectory 指向独立数据目录（SQLite+logs），源码目录只读；日志交给 logrotate。

## 7. 日志

- `logger/logger.go`：日志文件 0644、目录 0777；"轮转"仅是累计 100 万行后另开新时间戳文件（`maxLogCount=1000000`），**无按大小轮转、无保留期清理**，长期运行会持续占用磁盘，需 logrotate。
- Gin 访问日志只记 method/path/status/ip（`middleware/logger.go`），不记 Authorization 头（良好）。
- 错误日志入库默认关闭 `ERROR_LOG_ENABLED=false`（`common/init.go:195`）；DB 消费日志的 Content 只含价格/操作等摘要（`service/quota.go`、`relay/mjproxy_handler.go`），未见记录完整请求体/密钥。
- 默认 `--log-dir ./logs`（`common/init.go:20`）。

## 8. 升级 / 备份

- 启动时（master 节点）自动执行 GORM AutoMigrate（`model/main.go` `InitDB`→`migrateDB`，`model/main.go:263-...`），**升级即自动改表结构**，升级前必须备份数据库。
- `bin/migration_v0.2-v0.3.sql`、`bin/migration_v0.3-v0.4.sql` 是历史手动迁移脚本，不会自动执行。
- `VERSION` 文件为空（`cat VERSION` 无内容），Dockerfile 用 `$(cat VERSION)` 注入版本号；源码构建需自己保证版本标识，否则 `--version`/`/api/status` 版本为空，影响升级与回滚识别。
- 仓库无备份工具/文档；README 只给 Docker 升级路径（`README.zh_CN.md` 部署章节），配置/备份指引在外部文档站。备份重点是 SQLite 文件（或 SQL_DSN 对应库）+ `.env`。
- 数据导出：`common/constants.go:28-30`（DataExportEnabled/Interval/DefaultTime）提供额度数据导出能力，可作辅助备份手段。

## 9. 源码构建前提（部署前必读）

- `main.go` 通过 `//go:embed web/dist` 内嵌前端，`web/dist` 已被 gitignore 且当前不存在；源码构建必须先 `cd web && bun run build` 生成 `web/dist`，再 `go build`（`makefile`、`Dockerfile`）。
- `go.mod` 声明 `go 1.25.1`，Dockerfile 使用 `golang:1.26.1-alpine` + `oven/bun:1`，服务器需对应 Go 工具链与 bun。

## 结论（部署清单）

1. 放行 80/443 或仅 3000；不开 8005；初始化完成前限制来源 IP。
2. 立即完成 setup 向导创建强 root；保持 GENERATE_DEFAULT_TOKEN=false。
3. 固定 SESSION_SECRET（强随机）；共享 Redis 时固定 CRYPTO_SECRET；勿用 123456 示例密码。
4. SQLite 放专用数据目录，chmod 600；日志目录收紧权限 + logrotate。
5. 反代终止 TLS；设 TRUSTED_PROXIES；SESSION_COOKIE_SECURE=true + TRUSTED_URL。
6. 修改 new-api.service 占位符，用非 root 专用账号，加 systemd 加固。
7. 升级前备份 DB；VERSION 文件写入版本号。

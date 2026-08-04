# new-api 源码部署交接记录（公开脱敏版）

记录日期：2026-08-04
部署方式：源码构建 + systemd，未使用 Docker 运行 new-api
基线提交：`da10b4c6f7442b716cdfd907ae621b7ec688163e`
构建标识：`20260804-da10b4c6`

## 当前架构决策

- new-api 直接监听宿主机 TCP `9025`。
- new-api **不经过 Caddy 反向代理**；访问必须显式携带 `:9025`。
- Caddy 保持其原有配置，不应为了 new-api 再次修改，除非后续需求明确改变该决策。
- 同机已有服务使用宿主机 `9012`；对 new-api 运维时不得修改或重启该服务。
- 生产数据使用 SQLite。

```text
公网 TCP :9025
  → systemd: new-api.service
  → $HOME/new-api/new-api
  → $HOME/new-api_data/one-api.db
```

## 服务器布局

| 内容 | 路径 |
|---|---|
| Git 源码 | `$HOME/new-api` |
| Go 二进制 | `$HOME/new-api/new-api` |
| 前端产物 | `$HOME/new-api/web/dist`（构建时嵌入 Go 二进制） |
| 生产配置 | `$HOME/new-api_data/.env` |
| SQLite | `$HOME/new-api_data/one-api.db` |
| 日志目录 | `$HOME/new-api_data/logs` |
| 备份目录 | `$HOME/new-api_data/backups` |
| systemd 单元 | `/etc/systemd/system/new-api.service` |

建议权限：数据目录 `0700`；`.env` 与 SQLite 文件 `0600`；服务使用非 root 用户运行。

## 构建链路

1. 安装 Node.js 20+、Bun 和 Go。
2. 在 `web/` 执行 `bun install --frozen-lockfile`。
3. 在 `web/` 执行 `bun run build` 生成 `web/dist`。
4. 在仓库根目录执行 `CGO_ENABLED=0 go build`。
5. Go 的 `go:embed` 将前端产物嵌入单二进制。
6. 由 systemd 使用 `--port 9025` 启动。

可使用本目录中的 `scripts/build.sh`、`systemd/new-api.service.example` 作为模板。

## 已验证的运行边界

- `/api/status` 返回 HTTP 200，且 `success=true`、`setup=true`。
- 未携带令牌访问 `/v1/models` 返回 HTTP 401。
- systemd 单元处于 `active` 且已启用。
- 进程监听 `*:9025`，旧端口 `3000` 不再由 new-api 使用。
- 不带端口访问宿主机不是 new-api 的入口。

## 升级与回滚原则

new-api 启动时会执行 GORM AutoMigrate。升级前必须备份：

```bash
./docs/ops-2026-08-04/scripts/backup.sh
```

升级建议顺序：

1. 记录当前 commit 和二进制版本。
2. 备份 `.env` 与 `one-api.db`。
3. `git pull --ff-only`。
4. 重新构建前端和后端。
5. `sudo systemctl restart new-api`。
6. 执行健康检查并检查 journal。
7. 若迁移或启动失败，停止服务、恢复数据库与配置、回退代码/二进制后再启动。

## 常用命令

```bash
sudo systemctl status new-api
sudo journalctl -u new-api -f
sudo systemctl restart new-api
curl --noproxy '*' http://127.0.0.1:9025/api/status
ss -tlnp | grep ':9025'
```

## 私有信息

真实主机地址、SSH 用户、SSH/管理员密码和 API 凭据不进入 Git。请基于 `private.env.example` 在本地创建 `private.env`，或使用密码管理器保存；不要提交该文件。

# 2026-08-04 源码部署运维归档

本目录保存 new-api 源码部署中可公开提交的关键资料，用于后续维护、故障排查和任务中断后的快速恢复。

## 归档内容

| 文件 | 用途 |
|---|---|
| `deployment-handoff.md` | 当前部署决策、目录布局、验证结果和恢复上下文 |
| `architecture-review.md` | 项目架构、启动链路、数据层、鉴权与转发流程分析 |
| `security-review.md` | 源码部署安全边界与加固建议 |
| `nonstandard-port-troubleshooting.md` | 非标准端口经代理访问失败的诊断方法 |
| `systemd/new-api.service.example` | systemd 单元模板，安装前必须替换占位符 |
| `scripts/build.sh` | 前后端源码构建脚本 |
| `scripts/backup.sh` | SQLite 与 `.env` 的升级前备份脚本 |
| `scripts/health-check.sh` | 服务状态和鉴权边界检查脚本 |
| `scripts/ssh_exec.exp` | 从 `SSH_PASSWORD` 环境变量读取密码的 SSH 辅助脚本 |
| `private.env.example` | 私有连接信息模板；真实文件禁止提交 |

## 安全约定

- 本归档不保存公网 IP、SSH 用户、SSH 密码、管理员密码、API Key、Cookie、数据库、生产 `.env` 或日志。
- `private.env`、数据库、日志和备份已由本目录 `.gitignore` 排除；真实凭据应放入密码管理器。
- 若密码曾在聊天、终端历史或日志中明文出现，应视为已泄露并立即轮换。
- 生产端口开放前，应优先使用云安全组限制来源地址；裸 IP + HTTP 不提供传输加密。

## 恢复任务时的阅读顺序

1. 阅读 `deployment-handoff.md`，确认当前端口和“不经过 Caddy”的决策。
2. 阅读 `architecture-review.md`，理解构建与运行链路。
3. 在服务器上只读执行 `scripts/health-check.sh`。
4. 升级前先执行 `scripts/backup.sh`，再构建并重启。
5. 不要修改同机的其他服务，除非新的需求明确授权。

## 基线版本

- 归档日期：2026-08-04
- 源码提交：`da10b4c6f7442b716cdfd907ae621b7ec688163e`
- 构建标识：`20260804-da10b4c6`

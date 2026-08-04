# 非标准端口访问失败排查

## 典型现象

服务端进程正常监听，服务器本机和客户端直连都返回 HTTP 200，但浏览器经本地代理访问高位端口时返回 502。此时问题通常位于客户端代理节点或代理规则，而不是应用、systemd 或云安全组。

## 分层检查

### 1. 服务端监听

```bash
sudo systemctl is-active new-api
ss -tlnp | grep ':9025'
curl -i http://127.0.0.1:9025/api/status
```

### 2. 客户端绕过代理

```bash
curl --noproxy '*' -i http://<SERVER_IP>:9025/api/status
nc -vz <SERVER_IP> 9025
```

### 3. 显式经过代理

```bash
curl -x http://127.0.0.1:<PROXY_PORT> -i \
  http://<SERVER_IP>:9025/api/status
```

如果第 2 步成功而第 3 步返回 502，可将目标主机加入代理直连规则，例如 Clash/Mihomo：

```yaml
prepend:
  - IP-CIDR,<SERVER_IP>/32,DIRECT,no-resolve
```

重新加载代理配置后，再分别验证直连和代理路径。不要为了客户端代理故障修改服务器反向代理或应用端口。

## 安全提示

- 不要把真实公网 IP、家庭出口 IP、代理订阅文件、节点地址或本机配置路径提交到公开仓库。
- 非标准端口一旦公网开放会被自动扫描；自用部署应在云安全组中限制来源。

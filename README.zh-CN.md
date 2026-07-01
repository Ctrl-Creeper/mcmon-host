# MCMon Host

[English](README.md) | [简体中文](README.zh-CN.md)

MCMon Host 是 MCMon 部署中的中心管理服务。它运行在 Linux 上，使用 SQLite 保存节点、目标服务器和监控指标，提供网页面板，并为远程 `mcmon-agent` 节点生成一键安装命令。

Host 的使用方式是：先在网页面板里创建节点、配置目标服务器和监控项，再复制生成的一键安装命令到目标机器运行。agent 运行后没有自己的 UI，也不提供本地配置流程；需要变更配置时，在 host 修改后重新运行生成的安装命令覆盖。

## 项目关系

- `mcmon`：本地桌面 app，可独立使用，不需要 host。
- `mcmon-host`：Linux-only 的中心面板和 API。
- `mcmon-agent`：跨平台轻量节点进程，无 UI，向 host 上报数据。

个人本地监控优先使用 `mcmon`。需要集中管理多个节点或多个网络位置时，使用 `mcmon-host` + `mcmon-agent`。

## 功能

- Linux host 服务，内置网页 dashboard。
- SQLite 持久化 agents、targets 和 metric samples。
- 在 host 端配置节点和目标服务器。
- 每个目标支持四类监控：
  - online 状态
  - players 人数
  - latency 延迟
  - loss 丢包
- 每类监控都可以单独开关并设置检测周期。
- latency 支持独立的 burst、gap 和 protocol version。
- 生成 Linux/macOS agent 一键安装命令。
- 生成 Windows PowerShell agent 一键安装命令。
- admin API 和 agent RPC 均使用 token 保护。
- agent 支持 WebSocket 和 HTTP fallback。
- 网页面板支持浅色/深色主题和中英文切换。

## 要求

- 生产部署需要 Linux 服务器。
- 从源码构建需要 Go 1.25.4 或兼容版本。
- 如果 agent 不在同一台机器或同一内网，需要一个 agent 可访问的 URL。

`mcmon-agent` 的 release assets 必须存在于 GitHub Releases：

```text
https://github.com/Ctrl-Creeper/mcmon-agent/releases
```

## 从源码运行

```sh
go run ./cmd/mcmon-host -config config.json
```

首次运行时，如果配置里缺少 `admin_username` 或 `admin_password`，host 会自动写入这两个字段，并同步到单管理员账户。之后要改登录账号密码，直接编辑配置并重启 host。配置中的 `admin_token` 仍作为 legacy API 兼容 fallback 保留。

默认 dashboard：

```text
http://localhost:9090
```

如果 host 位于反向代理后，请让代理转发 `X-Forwarded-Proto` 和 `X-Forwarded-Host`。dashboard 会使用当前浏览器访问地址生成 agent 安装命令。

## Linux 部署

推荐默认路径：

- 二进制：`/usr/local/bin/mcmon-host`
- 配置：`/etc/mcmon-host/config.json`
- 数据：`/var/lib/mcmon-host/mcmon-host.db`
- systemd：`/etc/systemd/system/mcmon-host.service`

一键安装：

```sh
curl -fsSL https://raw.githubusercontent.com/Ctrl-Creeper/mcmon-host/main/install.sh | sudo sh -s -- install
```

指定版本：

```sh
curl -fsSL https://raw.githubusercontent.com/Ctrl-Creeper/mcmon-host/main/install.sh | sudo sh -s -- install --version v0.1.0
```

服务管理：

```sh
systemctl status mcmon-host
journalctl -u mcmon-host -f
sudo systemctl restart mcmon-host
```

升级或卸载：

```sh
curl -fsSL https://raw.githubusercontent.com/Ctrl-Creeper/mcmon-host/main/install.sh | sudo sh -s -- upgrade
curl -fsSL https://raw.githubusercontent.com/Ctrl-Creeper/mcmon-host/main/install.sh | sudo sh -s -- uninstall
```

`uninstall` 会删除 systemd 服务和二进制，但保留 `/etc/mcmon-host` 和 `/var/lib/mcmon-host`。

## Docker

```sh
docker run -d \
  --name mcmon-host \
  --restart unless-stopped \
  -p 9090:9090 \
  -v mcmon-host-data:/data \
  ghcr.io/ctrl-creeper/mcmon-host:latest
```

容器会把 `config.json` 和 SQLite 数据库保存到 `/data`。如果 `config.json` 不存在，首次启动会自动创建。

## Docker Compose

```sh
docker compose up -d
```

默认数据目录为 compose 文件旁边的 `./data`。

## 配置

示例 `config.json`：

```json
{
  "listen": ":9090",
  "db_path": "mcmon-host.db",
  "discovery_key": "replace-with-random-discovery-key",
  "admin_token": "replace-with-random-admin-token",
  "admin_username": "admin",
  "admin_password": "replace-with-a-strong-password",
  "ws_allowed_origins": "https://monitor.example.com"
}
```

字段：

- `listen`：HTTP 监听地址。
- `db_path`：SQLite 数据库路径。
- `discovery_key`：legacy/automatic discovery 使用的 bearer token。
- `admin_token`：legacy admin API 兼容 fallback。dashboard 和桌面 app 使用用户名/密码 session 登录。
- `admin_username`：单管理员用户名，自动生成时默认 `admin`。
- `admin_password`：单管理员密码。host 启动时会同步覆盖管理员账户密码。请确保配置文件权限私密。
- `public_url`：高级可选项，用于覆盖 agent endpoint。大多数部署保持为空即可，让 dashboard/API 根据当前请求推断。
- `ws_allowed_origins`：WebSocket Origin 允许列表，逗号分隔。同 host origin 默认允许。

## Dashboard 工作流

1. 启动 `mcmon-host`。
2. 打开 dashboard，使用管理员用户名和密码登录。
3. 进入 `Agents`。
4. 创建节点。
5. 在弹窗表单里配置目标服务器和监控项。
6. 从 dashboard 复制生成的一键安装命令。
7. 在目标机器上运行该命令。

如果要修改已有 agent，先在 host dashboard 更新节点配置，然后重新运行生成的一键安装命令覆盖。Linux 上也可以先手动用 `systemctl` 停止 agent。

如果要删除 agent，在 `Agents` 页面使用删除操作。host 会同时删除该 agent、它的目标配置和已保存的指标历史。

## Agent 安装

agent 安装命令由 `mcmon-host` 在创建节点并配置目标后生成。不要手写 agent 命令；生成脚本会包含正确的 host endpoint、agent token、agent ID 和不可变的 base64 目标配置。

进入 `Agents`，选择节点，然后复制 Linux/macOS 或 Windows PowerShell 安装命令。脚本会从 `mcmon-agent` 仓库下载对应平台 installer，写入 host 提供的配置，并启动后台服务或任务。

## 开发检查

```sh
go test ./...
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o /tmp/mcmon-host-linux-amd64 ./cmd/mcmon-host
```

# MCMon Host

[English](README.md) | [简体中文](README.zh-CN.md)

MCMon Host is the central management server for MC Server Monitor deployments.
It runs on Linux, stores metrics in SQLite, serves the web dashboard, and
generates one-line install commands for remote `mcmon-agent` nodes.

The host is designed for managed monitoring. Configure nodes and targets in the
host dashboard first, then install or overwrite an agent with the generated
command. Once an agent is running, it does not expose its own UI or local
configuration workflow.

## Project Relationship

MC Server Monitor can be used in two modes:

- `mc-latency-monitor`: standalone desktop app for local monitoring. It can run
  without any host.
- `mcmon-host`: Linux-only central dashboard and API for managed monitoring.
- `mcmon-agent`: lightweight cross-platform node process. It reports to
  `mcmon-host` and has no UI.

Use the desktop app when you want a local personal monitor. Use host plus agents
when you want centralized monitoring across multiple machines or network
locations.

## Features

- Linux host server with embedded web dashboard.
- SQLite persistence for agents, targets, legacy ping samples, and metric
  samples.
- Host-side node and target configuration.
- Per-target monitor settings:
  - online state
  - player count
  - latency
  - packet loss
- Independent enable switches and intervals for each monitor.
- Latency-specific burst settings and protocol version.
- One-line Linux/macOS agent install script generation.
- One-line Windows PowerShell agent install script generation.
- Token-protected admin API and agent RPC endpoints.
- WebSocket and HTTP fallback RPC support for agents.

## Requirements

- Linux server for production host deployment.
- Go 1.25.4 or newer compatible Go toolchain for building from source.
- A reachable URL if agents run outside the host machine or LAN.

`mcmon-agent` release assets must exist on GitHub Releases for one-line agent
installation. The installer downloads assets from:

```text
https://github.com/Ctrl-Creeper/mcmon-agent/releases
```

## Run From Source

```sh
go run ./cmd/mcmon-host -config config.json
```

On first run, the host generates `discovery_key` and `admin_token` if they are
missing, saves them to the config file, and prints them to the terminal.

The default dashboard is:

```text
http://localhost:9090
```

If `mcmon-host` is behind a reverse proxy, make sure the proxy forwards
`X-Forwarded-Proto` and `X-Forwarded-Host`. The dashboard uses the browser's
current host URL to generate agent install commands.

## Deploy On Linux

Production host deployment is Linux-only. The recommended default layout is:

- Binary: `/usr/local/bin/mcmon-host`
- Config: `/etc/mcmon-host/config.json`
- Data: `/var/lib/mcmon-host/mcmon-host.db`
- Service: `/etc/systemd/system/mcmon-host.service`

Install with the one-line systemd installer:

```sh
curl -fsSL https://raw.githubusercontent.com/Ctrl-Creeper/mcmon-host/main/install.sh | sudo sh -s -- install
```

Use a specific release tag:

```sh
curl -fsSL https://raw.githubusercontent.com/Ctrl-Creeper/mcmon-host/main/install.sh | sudo sh -s -- install --version v0.1.0
```

Manage the service:

```sh
systemctl status mcmon-host
journalctl -u mcmon-host -f
sudo systemctl restart mcmon-host
```

Upgrade or uninstall with the same installer:

```sh
curl -fsSL https://raw.githubusercontent.com/Ctrl-Creeper/mcmon-host/main/install.sh | sudo sh -s -- upgrade
curl -fsSL https://raw.githubusercontent.com/Ctrl-Creeper/mcmon-host/main/install.sh | sudo sh -s -- uninstall
```

`uninstall` removes the systemd service and binary, but keeps
`/etc/mcmon-host` and `/var/lib/mcmon-host`.

## Docker

Run the published image:

```sh
docker run -d \
  --name mcmon-host \
  --restart unless-stopped \
  -p 9090:9090 \
  -v mcmon-host-data:/data \
  ghcr.io/ctrl-creeper/mcmon-host:latest
```

The container stores `config.json` and the SQLite database under `/data`.
If `config.json` does not exist, `mcmon-host` creates it on first start.

## Docker Compose

Use the included `docker-compose.yml`:

```sh
docker compose up -d
```

The compose file stores data in `./data` beside the compose file.

## Configuration

Example `config.json`:

```json
{
  "listen": ":9090",
  "db_path": "mcmon-host.db",
  "discovery_key": "replace-with-random-discovery-key",
  "admin_token": "replace-with-random-admin-token",
  "ws_allowed_origins": "https://monitor.example.com"
}
```

Fields:

- `listen`: HTTP listen address.
- `db_path`: SQLite database path.
- `discovery_key`: bearer token for legacy/automatic agent discovery.
- `admin_token`: bearer token for dashboard and admin API access.
- `public_url`: optional advanced override for the agent endpoint. Most
  deployments should leave it empty and let the dashboard/API infer the
  endpoint from the current request.
- `ws_allowed_origins`: comma-separated WebSocket origin allowlist. Same-host
  origins are always allowed.

## Dashboard Workflow

1. Start `mcmon-host`.
2. Open the dashboard and enter the `admin_token`.
3. Go to `Agents`.
4. Create a node.
5. Configure its targets and monitor settings.
6. Copy the generated install command from the dashboard.
7. Run the command on the target machine.

To change an existing agent, update the node configuration in the host dashboard
and rerun the generated install command. On Linux, you can also stop the agent
manually with `systemctl` before reinstalling.

## Target Configuration

Targets are stored per agent. A target looks like this:

```json
{
  "target_id": "server-1",
  "name": "Minecraft Server",
  "host": "mc.example.com",
  "port": 25565,
  "timeout_ms": 1500,
  "monitors": {
    "online": {
      "enabled": true,
      "interval_sec": 60
    },
    "players": {
      "enabled": true,
      "interval_sec": 60
    },
    "latency": {
      "enabled": true,
      "interval_sec": 60,
      "probes_per_burst": 5,
      "probe_gap_ms": 1500,
      "protocol_version": 760
    },
    "loss": {
      "enabled": true,
      "interval_sec": 60,
      "probes_per_burst": 5,
      "probe_gap_ms": 1500
    }
  }
}
```

`protocol_version` is only used by the latency monitor. Other monitors use the
standard status request path.

## Agent Installation

Agent installation commands are generated by `mcmon-host` after you create a
node and configure its targets in the dashboard. Do not hand-write the agent
command: the generated script includes the correct host endpoint, agent token,
agent ID, and immutable base64 target configuration.

Open `Agents`, select a node, then copy either the Linux/macOS command or the
Windows PowerShell command. The command downloads the platform installer from
the `mcmon-agent` repository, writes the host-provided config, and starts the
agent as a background service or task.

## API Summary

Admin endpoints require:

```text
Authorization: Bearer <admin_token>
```

Agent endpoints require:

```text
Authorization: Bearer <agent_token>
```

Main endpoints:

- `GET /api/agents`
- `POST /api/agents`
- `GET /api/agents/{id}/targets`
- `PUT /api/agents/{id}/targets`
- `GET /api/agents/{id}/install.sh?token=...`
- `GET /api/agents/{id}/install.ps1?token=...`
- `GET /api/targets`
- `GET /api/series?agent=...&target=...&range=...`
- `GET /api/metrics?agent=...&target=...&metric=...&range=...`
- `GET /api/ws`
- `GET|POST /api/agents/v2/rpc`

Supported metric names:

- `online`
- `players`
- `latency`
- `loss`

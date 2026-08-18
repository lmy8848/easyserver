# EasyServer

**All-in-One Linux Server Management Panel** — Built with Go + React. Lightweight, secure, and feature-rich for managing your Linux servers from a web browser.

[简体中文](README_ZH.md) · [Documentation Site](https://lmy8848.github.io/easyserver/) · [API Reference](https://lmy8848.github.io/easyserver/api-reference) · [Deployment Guide](https://lmy8848.github.io/easyserver/linux-deploy)

![EasyServer Login](screenshots/login.png)

![EasyServer Dashboard](screenshots/dashboard.png)

---

## Features

- **System Monitoring** — Real-time CPU / memory / disk / network monitoring with historical charts
- **Process Management** — systemd service control and process supervision with auto-restart
- **Web Terminal** — Browser-based terminal backed by a real PTY shell
- **File Management** — Online browse / edit / upload / download / compress / extract, with a built-in editor
- **Web Server** — Nginx / Apache installation, configuration, and site management
- **Database** — Multi-version management for MySQL / PostgreSQL / Redis with a SQL console and backups
- **Container Management** — Docker / Compose / images / volumes / networks
- **Firewall** — iptables / nftables rule management
- **Runtime Environment** — Node.js / Python / Go / Java runtime management (via mise)
- **Scheduled Tasks** — Cron job management with a script library
- **Remote Deployment** — SSH remote server management and one-click deployment
- **Notifications & Alerts** — Webhook notifications (DingTalk / Feishu / WeCom) + metric alert rules
- **Audit Logs** — Complete operation audit with export and cleanup policies
- **Global Runtime Logging** — Persistent application logs in the app directory with level control (configurable from the panel) and source-location tracing for troubleshooting
- **2FA Authentication** — TOTP two-factor authentication with backup codes
- **QR Login** — Scan-to-login from a mobile device
- **Security Hardening** — SSH hardening wizard, CVE vulnerability scanning (osv.dev), login anomaly detection, IP banning, and file integrity monitoring (FIM)
- **File Sharing** — Secure share links with password and expiry control
- **Port Monitoring** — Real-time listening port viewer

---

## Quick Start

### Deploy the Binary

```bash
# Download the latest release
wget https://github.com/lmy8848/easyserver/releases/latest/download/easyserver-linux-amd64
chmod +x easyserver-linux-amd64

# Generate a config file
cat > config.toml << 'EOF'
[server]
port = 8080
host = "0.0.0.0"

[filemanager]
base_path = "/opt/easyserver/data"
EOF

# Start the panel
./easyserver-linux-amd64 -config config.toml
```

The admin password is randomly generated and printed on first launch. See [config.toml.example](config.toml.example) for all options, and the [Deployment Guide](https://lmy8848.github.io/easyserver/linux-deploy) for systemd setup.

> Note: run without `-dev` so the embedded frontend is served. `-dev` disables the embed and serves API only.

---

## Tech Stack

| Layer    | Technology |
|----------|------------|
| Backend  | Go 1.25 + Gin + SQLite (WAL) + WebSocket + JWT |
| Frontend | React 19 + TypeScript + Ant Design 6 + Vite |
| Deploy   | Single binary + systemd |

---

## Documentation

| Document | Description |
|----------|-------------|
| [Documentation Site](https://lmy8848.github.io/easyserver/) | Complete usage documentation |
| [API Reference](https://lmy8848.github.io/easyserver/api-reference) | Full API documentation |
| [Linux Deployment Guide](https://lmy8848.github.io/easyserver/linux-deploy) | Binary deployment + systemd + Nginx |
| [Contributing Guide](CONTRIBUTING.md) | How to contribute |

---

## System Requirements

| Item   | Minimum       | Recommended                |
|--------|---------------|----------------------------|
| OS     | Linux x86_64  | Ubuntu 22.04+ / Debian 12+ |
| Memory | 512MB         | 1GB+                       |
| Disk   | 1GB           | 5GB+                       |
| Port   | 8080          | Configurable               |

---

## Development

```bash
# Backend (development mode, uses air for hot reload)
make dev

# Or start the API manually (frontend served by Vite separately)
go build -tags dev -o easyserver ./cmd/server
./easyserver -config config.toml -dev

# Frontend (hot reload)
cd web
pnpm install
pnpm dev
# Visit http://localhost:5173
```

See [docs/development.md](docs/development.md) for the full local setup guide.

---

## Security Recommendations

1. **Change** `jwt_secret` and `encryption_key` — generate strong values with `openssl rand -base64 32` (or leave them empty to auto-generate on first start)
2. **Enable HTTPS** in production (via Nginx reverse proxy or the built-in TLS config)
3. **Configure the IP whitelist** to restrict who can reach the panel
4. **Back up the database and config** regularly — data lives under `/opt/easyserver/data`
5. **Watch the runtime log** at `/opt/easyserver/easyserver.log` for troubleshooting and security events

---

## License

Released under the [MIT License](LICENSE).

## Contributing

Issues and Pull Requests are welcome! Please read the [Contributing Guide](CONTRIBUTING.md) first.

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'feat(x): add amazing feature'`)
4. Push the branch (`git push origin feature/amazing-feature`)
5. Submit a Pull Request
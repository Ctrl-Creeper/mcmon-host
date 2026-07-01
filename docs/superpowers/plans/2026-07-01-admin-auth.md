# Admin Auth Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace raw host admin tokens in the host dashboard and desktop app remote integration with single-admin username/password login, optional TOTP 2FA, and session tokens.

**Architecture:** `mcmon-host` stores a singleton admin account plus expiring sessions in SQLite. Host admin APIs accept either session cookie or bearer session token, with legacy `admin_token` kept as a compatibility fallback. `mcmon` desktop app logs into host with credentials and stores the returned session token for its remote proxy.

**Tech Stack:** Go, SQLite via `modernc.org/sqlite`, bcrypt from `golang.org/x/crypto/bcrypt`, TOTP from `github.com/pquerna/otp/totp`, embedded static HTML/JS.

---

## Files

- Create `/Users/lewiswu/Develop/mcmon-host/internal/store/auth_test.go`: store-level tests for admin bootstrap, password checks, sessions, and TOTP secret state.
- Modify `/Users/lewiswu/Develop/mcmon-host/internal/store/store.go`: add admin auth tables and methods.
- Modify `/Users/lewiswu/Develop/mcmon-host/go.mod`: add `github.com/pquerna/otp` and `golang.org/x/crypto` if missing.
- Create `/Users/lewiswu/Develop/mcmon-host/internal/web/auth.go`: auth request/response structs, login/logout/me/password/2FA handlers, session cookie helpers.
- Modify `/Users/lewiswu/Develop/mcmon-host/internal/web/server.go`: mount auth endpoints and update `requireAdmin`.
- Modify `/Users/lewiswu/Develop/mcmon-host/internal/web/server_test.go`: add API auth tests.
- Modify `/Users/lewiswu/Develop/mcmon-host/internal/web/static/index.html`, `agents.html`, `detail.html`: replace admin-token UI with login/logout UI and use cookie-based fetch.
- Modify `/Users/lewiswu/Develop/mcmon-host/internal/web/static/i18n.js`: translate login/2FA labels.
- Modify `/Users/lewiswu/Develop/mcmon-host/README.md`, `README.zh-CN.md`, `install.sh`: document initial admin credentials and session login.
- Modify `/Users/lewiswu/Develop/mc-latency-monitor/internal/app/config.go`: replace remote admin token with remote username/session token.
- Modify `/Users/lewiswu/Develop/mc-latency-monitor/internal/app/server.go`: add remote login endpoint and proxy with session bearer token.
- Modify `/Users/lewiswu/Develop/mc-latency-monitor/internal/app/server_test.go`: update remote config/proxy tests.
- Modify `/Users/lewiswu/Develop/mc-latency-monitor/internal/app/static/remote.html`: switch remote setup UI to login form.
- Modify `/Users/lewiswu/Develop/mc-latency-monitor/README.md`, `README.zh-CN.md`: document remote login.

## Tasks

### Task 1: Host Store Auth Primitives

- [ ] Write failing store tests for admin bootstrap, password verification, sessions, and 2FA secret persistence.
- [ ] Add SQLite tables `admin_auth` and `admin_sessions`.
- [ ] Implement `EnsureAdmin`, `CheckAdminPassword`, `CreateAdminSession`, `AdminSession`, `DeleteAdminSession`, `SetAdminTwoFactor`, and `UpdateAdminCredentials`.
- [ ] Run `go test ./internal/store`.

### Task 2: Host Auth API

- [ ] Write failing web tests for login without 2FA, login with required 2FA, `GET /api/auth/me`, logout, and admin API access with a session bearer token.
- [ ] Add `/api/auth/*` handlers in a new `internal/web/auth.go`.
- [ ] Update `requireAdmin` to accept valid session bearer token or cookie.
- [ ] Keep legacy `admin_token` fallback for current scripts/tests during migration.
- [ ] Run `go test ./internal/web`.

### Task 3: Host Startup and Docs

- [ ] Update `cmd/mcmon-host/main.go` to call `EnsureAdmin("admin", randomPassword)` and print initial credentials when created.
- [ ] Update install output and README to say admin credentials live in the host DB/config context and are printed on first run.
- [ ] Run `go test ./...`.

### Task 4: Host Web Login UI

- [ ] Replace admin token widgets on dashboard pages with username/password/TOTP login state.
- [ ] Add 2FA setup/enable/disable controls on Agents or a small settings section without exposing raw config.
- [ ] Ensure all admin fetches use cookie credentials and no longer read `mcmon-host-admin-token`.
- [ ] Run static grep to confirm `mcmon-host-admin-token` is gone from host static files.

### Task 5: Desktop Remote Login

- [ ] Write failing desktop app tests for remote login storing session token and remote proxy forwarding bearer session token.
- [ ] Update config fields and `/api/remote/config`.
- [ ] Add `/api/remote/login` to call host `/api/auth/login`.
- [ ] Update proxy to use stored session token.
- [ ] Update Remote page UI and docs.
- [ ] Run `go test ./...` in `/Users/lewiswu/Develop/mc-latency-monitor`.

### Task 6: End-to-End Verification

- [ ] Run full tests in both repos.
- [ ] Start a temporary host, login through `/api/auth/login`, call `/api/agents`, and verify session auth.
- [ ] Start the desktop app backend against the temporary host and verify remote login plus proxy.
- [ ] Review diffs for accidental personal paths or old admin-token UI text.

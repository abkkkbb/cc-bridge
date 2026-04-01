# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

CC2API - Claude Code to API Gateway. Routes requests to Anthropic API with anti-detection, account pooling, and load balancing. Supports both Claude Code client (replace mode) and pure API calls (inject mode).

## Tech Stack

- **Backend:** Go 1.25, net/http, SQLite (default) / PostgreSQL
- **Frontend:** Vue 3 + Vite + TailwindCSS
- **TLS:** utls (refraction-networking/utls) for Node.js 24.x fingerprint spoofing
- **Cache:** Redis (optional, default in-memory)
- **Deploy:** Docker (ghcr.io/mamoworks/cc2api)

## Build & Run

```bash
# Backend
go build -o cc2api ./cmd/server/
./cc2api  # reads config.json, SQLite + in-memory cache, zero deps

# Frontend dev
cd web && npm ci && npm run dev  # localhost:3000, proxies to :8080

# Frontend build (production: served by backend from web/dist/)
cd web && npm run build
```

## Architecture

```
Client (Claude Code or API)
  → x-api-key auth (middleware/auth.go)
  → DetectClientType (rewriter.go)
  → session hash → sticky session → account selection (account.go)
  → header whitelist + wire casing restore (rewriter.go)
  → TLS fingerprint spoofing (tlsfp/)
  → forward to api.anthropic.com (gateway.go)
  → async usage recording (usage.go)
```

## Key Files

| File | Purpose |
|------|---------|
| `cmd/server/main.go` | Entry point |
| `internal/service/rewriter.go` | Anti-detection engine (header whitelist, wire casing, beta merge, body rewriting) |
| `internal/service/gateway.go` | Request forwarding + proxy + SSE streaming |
| `internal/service/account.go` | Account pool + sticky session + concurrency |
| `internal/service/oauth.go` | OAuth token testing |
| `internal/tlsfp/tlsfp.go` | uTLS fingerprint (Node.js 24.x) for direct/HTTP/SOCKS5 |
| `internal/handler/router.go` | Routes + Admin API + /v1/models + SPA static |
| `internal/store/db.go` | SQLite/PostgreSQL schema + migration |
| `internal/logger/logger.go` | Level-based logging (debug/info/warn/error) |
| `internal/config/config.go` | Config loading, defaults |

## Data Directory

SQLite database stored in `data/cc2api.db` (default). The `data/` directory is auto-created on startup. Docker deployments must mount `-v ./data:/app/data` to persist data.

## Anti-Detection Layers

- Header wire casing restoration (headerWireCasing map, matching real Claude CLI traffic)
- Header whitelist (CC mode: 19 allowed headers; API mode: fixed standard set)
- Model-aware anthropic-beta (Haiku excludes claude-code beta)
- TLS fingerprint spoofing (utls, Node.js 24.x JA3/JA4)
- User-Agent normalization
- Billing header rewriting (cc_version hash → .000)
- System prompt environment rewriting (Platform, Shell, OS, working dir, home path)
- Device identity unification (device_id, email)
- Hardware fingerprint spoofing (memory, heap, RSS)
- Proxy trace deletion (baseUrl, gateway fields)
- Base64 additional_metadata sanitization
- Empty text block stripping (prevents upstream 400)

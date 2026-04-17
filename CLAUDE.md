# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Claude Code Gateway (cc-bridge) is a Rust reverse proxy for the Anthropic API that pools multiple Claude accounts with load balancing, rate limit handling, sticky sessions, TLS fingerprint spoofing, and request/response rewriting. It includes a Vue 3 management dashboard embedded into the single binary via `rust-embed`.

## Build & Run Commands

```bash
# Development (frontend + backend together)
cp .env.example .env
./scripts/dev.sh

# Or run separately:
cd web && npm ci && npm run dev    # Frontend dev server on :3000
cargo run                           # Backend on :5674 (frontend proxies to it)

# Production build (embeds frontend into binary)
./scripts/build.sh                  # Native
./scripts/build.sh linux-amd64     # Cross-compile

# Docker
docker build -f docker/Dockerfile -t cc-bridge:latest .
```

No integration test suite exists. 120+ unit tests live in `src/service/limit.rs` and `src/service/gateway.rs` tests modules — run with `cargo test --lib`. Account validity is tested via the UI's "test" button.

## Architecture

### Request Flow

Auth middleware → API token lookup → account selection (sticky session + priority + concurrency + availability check via `LimitStore`) → request rewriting (headers, body, telemetry, identity) → TLS fingerprint spoofing via custom rustls (`craftls/`) → forward to api.anthropic.com → absorb rate-limit response headers into `LimitStore` (async DB flush on thresholds) → response header/body filtering → return to client.

### Key Modules

- **`src/handler/router.rs`** — All HTTP endpoints: SPA routes, `/admin/*` management API (password-protected), and the catch-all gateway proxy.
- **`src/service/gateway.rs`** — Core forwarding orchestration: account selection, slot acquisition with `SlotHolder` + `SlotHeldStream`, upstream request, 403 permanent disable, **sticky 429 passthrough** (no cross-account retry), **5xx body-wrap with tracking-header stripping**.
- **`src/service/limit.rs`** — Rate-limit state machine. `LimitStore` holds per-account in-memory `LimitState` (hot state, nanosecond reads). `absorb_headers(account_id, status, headers)` parses both `anthropic-ratelimit-unified-*` (OAuth: 5h/7d windows, representative-claim, overage) and `anthropic-ratelimit-{requests,tokens,input-tokens,output-tokens}-*` (SetupToken RPM/TPM). DB flushed async on first-fill / 5-min TTL / threshold crossing / 429-short-ban / rpm-tpm-preempted. `judge_availability` gates scheduler. Sonnet carve-out: 429 + `representative_claim=seven_day_sonnet` does NOT mark the account globally rejected (keeps Opus usable).
- **`src/service/rewriter.rs`** — Request/response transformation: header normalization, body patching (session hash, version, telemetry paths like event_logging/GrowthBook), system prompt env var injection, AI Gateway fingerprint header stripping.
- **`src/service/account.rs`** — Account selection logic: sticky sessions (SHA256 of UA+body, 24h TTL), OAuth token refresh with locking, concurrency slot management, `refresh_usage` for Admin-manual `/api/oauth/usage` queries (OAuth accounts only; SetupToken returns a user-friendly error).
- **`src/tlsfp/tlsfp.rs`** — Custom TLS ClientHello builder that mimics Node.js fingerprint, using the forked rustls in `craftls/`.
- **`src/middleware/auth.rs`** — Extracts API key from `x-api-key` or `Authorization: Bearer` header, validates against token store.
- **`src/store/`** — SQLx-based persistence (SQLite default, PostgreSQL optional) with `CacheStore` trait implemented by `MemoryStore` and `RedisStore`.
- **`src/model/identity.rs`** — Generates canonical device identity (20+ env vars, process fingerprints) for upstream requests.

### Frontend

Vue 3 + Vite + TypeScript in `web/`. Components: Login, Dashboard, Accounts, Tokens. API client in `web/src/api.ts`. Uses shadcn-style UI components in `web/src/components/ui/`.

### Database

SQLite (WAL mode) or PostgreSQL, selected via `DATABASE_DRIVER` env var. Auto-migration on startup in `src/store/db.rs`. Incremental ALTER TABLE migrations for backward compatibility.

### Custom Rustls Fork

`craftls/` contains a patched rustls that exposes low-level TLS ClientHello construction for fingerprint spoofing. This is a workspace dependency, not published.

## Configuration

All via environment variables (see `.env.example`). Key ones: `SERVER_HOST`/`SERVER_PORT` (default 0.0.0.0:5674), `DATABASE_DRIVER`/`DATABASE_DSN`, `REDIS_HOST` (optional, falls back to in-memory), `ADMIN_PASSWORD` (default "admin"), `LOG_LEVEL`.

## Dual Auth Modes

Accounts support two auth types: **SetupToken** (classic API key) and **OAuth** (with automatic access token refresh via stored refresh tokens). Both flows converge in `AccountService::select_account`. They differ in the rate-limit header contract received from Anthropic:

- **OAuth** — `anthropic-ratelimit-unified-*` headers (5h / 7d aggregate windows, `representative-claim` string hint for Opus/Sonnet sub-windows, overage status). Per-model Opus/Sonnet utilization only surfaces via Admin-triggered `/api/oauth/usage` JSON.
- **SetupToken** — `anthropic-ratelimit-{requests,tokens,input-tokens,output-tokens}-{limit,remaining,reset}` headers (RPM + per-minute TPM). Preemption threshold: `remaining / limit < 3%`.

Both header sets are parsed by `LimitStore::absorb_headers`; they can coexist. `judge_availability` checks, in order: short-term 429 ban → RPM/TPM preemption → 5h/7d 97% hard cap → global `status=rejected`.

## 429 / 5xx Policy

- **429**: the account stays selected for THIS request (sticky). The gateway wraps the body as generic `rate_limit_error` JSON (hides Sonnet/Opus/utilization details), status preserved. `absorb_headers` updates `state.status`/`rate_limited_until` so subsequent requests' `select_account` naturally avoids the account. No cross-account retry — avoiding prompt-cache bust is the priority.
- **5xx**: body replaced with generic `api_error` JSON, status preserved (500/502/503/504/529), tracking headers stripped (`x-request-id`, `cf-ray`, `server`, `via`).
- **Sonnet 429**: `representative_claim == seven_day_sonnet` specifically does NOT set `state.status = Rejected`; Opus requests on the same account continue to schedule.

## KEY: EVERY TIME YOU WANT TO CHANGE STH, PLEASE REFER cc-gateway

# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

Spindle is an RSS feed reader built with Fermyon Spin and WebAssembly, using Go compiled via TinyGo.

## Toolchain Requirements

**Critical: exact versions matter.** TinyGo and Go must be version-compatible, and TinyGo must match the Spin runtime's WASI support.

- **Go 1.23.x** (not 1.24+) — `$(brew --prefix go@1.23)/libexec/bin`
- **TinyGo 0.35.0** (not 0.40+) — `~/.local/tinygo/bin`
- **Spin CLI 3.6.x** — `~/.local/bin`
- **binaryen** (provides `wasm-opt`) — installed via Homebrew

Set PATH before any build/run commands:
```bash
export PATH="$HOME/.local/tinygo/bin:$(brew --prefix go@1.23)/libexec/bin:$HOME/.local/bin:$PATH"
```

## Build & Run

```bash
spin build              # Compiles Go → WASM via TinyGo
spin up                 # Runs the app at http://127.0.0.1:3000
spin build --up         # Build + run in one step
spin watch              # Build + run with hot reload on file changes
```

The build command in spin.toml:
```
tinygo build -target=wasip1 -gc=leaking -buildmode=c-shared -no-debug -o main.wasm .
```

## Architecture

Two Spin components routed by URL prefix:
- **`spindle`** (Go/TinyGo WASM) — handles `/api/...` for JSON API and HTMX HTML fragment endpoints
- **`ui`** (pre-built fileserver WASM) — handles `/...` (everything else), serves static files from `static/`

All routing within the API component is done manually in Go (`router` function in `main.go`) because third-party routers like httprouter can cause WASM traps at init time.

- `main.go` — HTTP handler entry point + routing (JSON API handlers)
- `ui.go` — HTMX HTML fragment handlers (return HTML, not JSON)
- `models.go` — Data types: XML parsing structs (RSS/Atom) and normalized Feed/Article types
- `feeds.go` — Feed fetching via outbound HTTP + XML parsing
- `store.go` — SQLite data access layer (feeds and articles CRUD)
- `migrations.go` — SQL schema as Go string constants (run with IF NOT EXISTS on every request)
- `static/` — HTML, CSS served by the fileserver component (HTMX frontend)
- `spin.toml` — Application manifest (triggers, capability grants, build config)

Spin components are **stateless**: each HTTP request creates a fresh WASM instance. No in-memory state persists between requests. SQLite databases (granted via spin.toml) do persist.

## Key Constraints

- **No goroutines** — WASM is single-threaded
- **No ambient network access** — outbound hosts must be listed in `allowed_outbound_hosts` in spin.toml
- **No persistent memory** — every request starts fresh; use SQLite/KV for state
- **TinyGo stdlib limitations** — avoid packages that require OS features not in WASI; keep dependencies minimal
- **`-gc=leaking` is intentional** — WASM instances are torn down after each request, so GC is unnecessary
- **No LastInsertId/RowsAffected** — Spin's SQLite driver doesn't support these; query back after inserts
- **No transactions** — each SQL statement is independent

## API Endpoints

```
GET    /api/health
POST   /api/feeds                              Subscribe (body: {"url": "..."})
GET    /api/feeds                              List subscriptions
GET    /api/feeds/:id                          Get feed
DELETE /api/feeds/:id                          Unsubscribe
GET    /api/feeds/preview?url=<rss-url>        Preview feed before subscribing
POST   /api/feeds/:id/refresh                  Refresh one feed
POST   /api/feeds/refresh-all                  Refresh all feeds
GET    /api/articles?feed_id=&is_read=&limit=&offset=  List articles
GET    /api/articles/:id                       Get article
PATCH  /api/articles/:id                       Mark read/unread (body: {"is_read": true})
POST   /api/articles/mark-all-read?feed_id=    Bulk mark read
```

## Beginner Guides

Three educational guides live in `guides/`. They trace RSS, Go, and WebAssembly from first principles through the codebase.

- `guides/rss-guide.md` — RSS/Atom format, XML parsing, deduplication
- `guides/go-guide.md` — Go language patterns as used in this project
- `guides/wasm-guide.md` — WebAssembly, WASI, Spin, and WASM-driven constraints

**Keeping guides current:** Each guide embeds file:line references and code snippets. When source files change in a way that affects the concepts explained (e.g. the routing pattern, the DB open-per-request pattern, the build flags), update the relevant guide snippets and line numbers to match. The guides have a header note that tells readers snippets are point-in-time, but accurate references are better than stale ones.

### HTMX UI Endpoints (return HTML fragments)

```
GET    /api/ui/feeds                           Feed sidebar list
POST   /api/ui/feeds                           Subscribe (form-encoded)
DELETE /api/ui/feeds/:id                       Unsubscribe + return updated list
POST   /api/ui/feeds/:id/refresh               Refresh feed + return articles
POST   /api/ui/feeds/refresh-all               Refresh all + return articles
GET    /api/ui/articles?feed_id=               Article list
POST   /api/ui/articles/:id/toggle-read        Toggle read/unread
POST   /api/ui/articles/mark-all-read?feed_id= Mark all read + return articles
```

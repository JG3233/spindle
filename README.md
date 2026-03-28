# Spindle

An RSS feed reader built with [Fermyon Spin](https://spinframework.dev) and WebAssembly, using Go (TinyGo).

## Features

- Subscribe to RSS 2.0 and Atom feeds
- Automatic article deduplication on refresh
- Read/unread tracking with mark-all-read
- HTMX-powered web UI (no JavaScript framework)
- JSON API for programmatic access
- ~930KB WASM binary

## Prerequisites

- [Go 1.23.x](https://go.dev/dl/)
- [TinyGo 0.35.0](https://tinygo.org/getting-started/install/)
- [Spin CLI 3.6.x](https://spinframework.dev/install)
- [binaryen](https://github.com/WebAssembly/binaryen) (`brew install binaryen`)

## Quick Start

```bash
spin build --up
```

Open http://127.0.0.1:3000 in your browser.

## Architecture

Two Spin components:
- **spindle** (Go/TinyGo WASM) — `/api/...` JSON API + HTMX HTML fragment endpoints
- **ui** (static fileserver) — serves HTML/CSS from `static/`

Each HTTP request creates a fresh WASM instance. SQLite persists across requests.

## License

Apache 2.0

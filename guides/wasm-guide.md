# WebAssembly From First Principles
### *A guided tour through Spindle — no prior knowledge assumed*

---

```
┌────────────────────────────────────────────────────────────────┐
│                                                                │
│   "WebAssembly is not a language. It's a virtual machine      │
│    instruction set — like x86 or ARM, but for a computer      │
│    that doesn't exist in hardware."                           │
│                                                                │
└────────────────────────────────────────────────────────────────┘
```

## Part 1: Why Does WebAssembly Exist?

### The Problem With Shipping Code

When you write a program in C, you compile it for a specific CPU architecture. A binary compiled for an Intel x86 processor won't run on an ARM chip. It also won't run in a browser.

The web solved this with JavaScript: ship source code, not binaries, and let every browser compile it on the fly. That works, but JavaScript has tradeoffs: it's dynamically typed, it requires a garbage collector, and historically it's slow for compute-heavy tasks.

Then there's the security problem. When you run arbitrary code from the internet, it can access your files, your network, your processes. Traditional native code is hard to sandbox safely.

WebAssembly (WASM) was designed to solve both problems:

```
The Two Problems WebAssembly Solves
────────────────────────────────────

1. PORTABILITY
   Write once, run anywhere — without shipping source code.
   A WASM binary runs on any platform with a WASM runtime.

2. SECURITY
   WASM code runs in a sandbox. It CAN'T access the filesystem,
   network, or system calls unless the host explicitly grants them.
```

### A Brief Timeline

```
1995 — JavaScript invented to run in browsers
2008 — Chrome's V8 engine makes JS fast
2013 — asm.js: compile C to a subset of JS for speed
2015 — WebAssembly announced as a better target than asm.js
2017 — WASM ships in all major browsers
2019 — WASI proposed: WASM for servers, not just browsers
2022 — Fermyon Spin uses WASM+WASI for serverless apps
```

---

## Part 2: What IS a WASM Binary?

A WASM binary (`.wasm` file) is machine code for a **virtual machine that doesn't exist in hardware**. Think of it like Java's bytecode, or .NET's IL — but standardized and language-agnostic.

The instruction set is simple: integers, floats, stack operations, memory reads/writes. No file I/O, no networking, no threads — just computation.

```
WASM virtual machine model:
────────────────────────────

┌─────────────────────────────────────────────┐
│              WASM Module                    │
│                                             │
│  ┌─────────────┐    ┌────────────────────┐  │
│  │  Functions  │    │  Linear Memory     │  │
│  │  (bytecode) │    │  (one big byte     │  │
│  │             │    │   array — no heap  │  │
│  │  add, sub,  │    │   pointers, no     │  │
│  │  load, store│    │   OS malloc)       │  │
│  └─────────────┘    └────────────────────┘  │
│                                             │
│  ┌─────────────────────────────────────┐   │
│  │  Imports & Exports                  │   │
│  │  (the only way in and out)          │   │
│  │  • host provides: print, read_file  │   │
│  │  • module exports: handle_request   │   │
│  └─────────────────────────────────────┘   │
└─────────────────────────────────────────────┘
```

The only way WASM code can do anything beyond pure computation is through **imports** — functions the host environment provides. Want to write to stdout? The host gives you a `fd_write` function. Want to open a file? Only if the host gives you an `fd_open` function. This is what makes WASM fundamentally secure: **capabilities are granted, not assumed.**

---

## Part 3: WASI — WebAssembly Outside the Browser

WASM was designed for browsers, but it's a useful execution model for servers too: fast startup, strong isolation, portable. The missing piece was a standardized API for system operations.

**WASI** (WebAssembly System Interface) is that standard. It defines what a host should provide to WASM modules that need to:
- Read/write files (via file descriptors)
- Print to stdout/stderr
- Get environment variables
- Make network connections (in newer versions)

Think of WASI as a minimal operating system API — similar to POSIX, but designed for secure sandboxing.

```
Traditional Program         WASM/WASI Program
───────────────────         ─────────────────────────────

OS kernel                   WASM runtime (Spin/Node/wasmtime)
    │                                │
    │ syscalls                       │ WASI imports (controlled)
    │                                │
    ▼                                ▼
Your program (trusted)         WASM module (untrusted/sandboxed)
Can access anything            Can only access what host grants
```

---

## Part 4: TinyGo — Compiling Go to WASM

Normal Go can't easily compile to WASM for WASI. The standard Go runtime has assumptions about OS threads, garbage collection, and memory management that don't fit the WASM model well.

**TinyGo** is a Go compiler designed for constrained environments — microcontrollers and WASM. It produces smaller binaries and supports WASI targets.

The build command in `spin.toml:21`:

```toml
command = "tinygo build -target=wasip1 -gc=leaking -buildmode=c-shared -no-debug -o main.wasm ."
```

Let's decode every flag:

```
Flag breakdown:
───────────────────────────────────────────────────────────────

-target=wasip1
  Compile for WASI Preview 1. This sets the target architecture
  to the WASM instruction set AND configures WASI imports for
  system calls. wasip1 is the stable version supported by Spin.

-gc=leaking
  "Leaking" garbage collector: never actually free memory.
  Why? Each HTTP request creates a fresh WASM instance that
  is destroyed after the response. GC overhead without GC
  benefit = waste. Leaking allocator is faster and simpler.

-buildmode=c-shared
  Produce a shared library, not a standalone executable.
  This is how WASM modules expose functions to the host.
  Spin calls into the exported _start function.

-no-debug
  Strip debug symbols. Makes the binary smaller without
  affecting runtime behavior. Important for WASM size.

-o main.wasm
  Output filename. Spin looks for this in spin.toml.
```

---

## Part 5: Spin — The WASM Runtime

**Fermyon Spin** is the runtime that hosts Spindle's WASM modules. Think of it as an HTTP server that, instead of running a traditional process per request, spawns a fresh WASM instance per request.

```
Traditional HTTP Server           Spin (WASM server)
───────────────────────           ──────────────────────────────────

Process starts once               ┌──────────────────────────────┐
    │                             │  spin up                     │
    │  stays running              │  └─ loads spin.toml          │
    │  in memory forever          │  └─ downloads WASM binaries  │
    │                             └──────────────────────────────┘
    │                                            │
    ▼                                            ▼
Request 1 → handled by           Request 1 → fresh WASM instance
    same process                                 │
Request 2 → handled by                    handle request
    same process                                 │
    (shared memory!)                      WASM instance destroyed
                                                 │
                                 Request 2 → fresh WASM instance
                                            ...and so on
```

**Fresh instance per request** is the key architectural fact. It means:

1. No shared in-memory state between requests
2. No goroutine leaks or connection pool management
3. Perfect isolation — one request's crash can't affect another
4. But also: no warming up, no caching in memory

This is why `store.go` opens a new database connection on every request (`openDB()` called at the start of every handler), and why `migrations.go` uses `IF NOT EXISTS` — the schema check runs every request.

---

## Part 6: The `spin.toml` — Capability Grants

The `spin.toml` file is where WASM's security model becomes visible. Open it:

```toml
# spin.toml:14-21
[component.spindle]
source = "main.wasm"
allowed_outbound_hosts = ["https://*:*", "http://*:*"]
sqlite_databases = ["default"]
key_value_stores = ["default"]
```

These three lines are **capability grants**. By default, a WASM module in Spin can't do anything except compute. These lines grant:

- `allowed_outbound_hosts` — the module may make outbound HTTP requests. The `*` wildcards allow any hostname. You could restrict this to specific domains.
- `sqlite_databases` — the module may access a named SQLite database.
- `key_value_stores` — the module may access a key-value store.

The `ui` component (the static file server) has none of these:

```toml
# spin.toml:29-35
[component.ui]
source = { url = "...", digest = "sha256:..." }
files = [{ source = "static", destination = "/" }]
```

Only `files` — it can read from the `static/` directory and nothing else. No network, no database. Even though both components run in the same Spin process, their capabilities are completely separate.

---

## Part 7: The Capability Model in Code

The capability grants in `spin.toml` aren't just policy documents — they're enforced at the host boundary.

When `feeds.go` calls `http.Get(url)`:

```go
// feeds.go:21
resp, err := http.Get(url)
```

The comment in `feeds.go:13-19` explains what happens:

```
Your Go code → Spin SDK → WASM host boundary → Spin runtime → internet
```

The Spin SDK has replaced Go's `http.DefaultClient` with one that routes through WASI. The call doesn't go directly to the OS network stack — it crosses the WASM/host boundary, where Spin checks: "Is this URL in `allowed_outbound_hosts`?" If not, the request is rejected before leaving the machine.

This is the power of capability-based security: the policy is enforced structurally, not by trust.

```
Without capability model (traditional process):
────────────────────────────────────────────────
  Your code   →   OS network stack   →   internet
  (can call anything, anytime)

With WASM capability model:
───────────────────────────
  Your code   →   WASM boundary   →   Spin runtime   →   internet
                  (checks policy)      (executes if
                                        allowed)
```

---

## Part 8: Two Components, One App

Spindle isn't one WASM module — it's two, composed by Spin's router:

```toml
# spin.toml:8-13
[[trigger.http]]
route = "/api/..."
component = "spindle"     ← our Go code

[[trigger.http]]
route = "/..."
component = "ui"          ← pre-built fileserver
```

When a request comes in, Spin checks routes in order:

```
Request: GET /api/feeds
  └─ matches "/api/..." → spindle component (Go WASM)

Request: GET /style.css
  └─ doesn't match "/api/..."
  └─ matches "/..."      → ui component (fileserver WASM)

Request: GET /index.html
  └─ same as above       → ui component
```

The `ui` component's source is a pre-built WASM binary downloaded from GitHub:

```toml
source = { url = "https://github.com/spinframework/spin-fileserver/...",
           digest = "sha256:ef88..." }
```

The `digest` is a SHA-256 hash of the binary. Spin verifies the download matches before running it — you're trusting the hash, not blindly trusting a URL. This is a security feature: even if the hosting URL were compromised, the hash check would catch a tampered binary.

---

## Part 9: The Constraints That Shape Spindle

Understanding WASM's constraints helps explain several choices in Spindle's code that might otherwise seem strange.

### No Goroutines

```go
// main.go:249-259
// Refresh each feed sequentially. Remember: no goroutines in WASM.
// In a traditional Go server you'd fan out with goroutines.
// Here, it's one feed at a time.
totalAdded := 0
for i := range feeds {
    added, err := refreshFeed(db, &feeds[i])
    ...
    totalAdded += added
}
```

WASM is single-threaded. Go goroutines are multiplexed onto OS threads — but WASM has no OS threads. TinyGo's WASM target simply doesn't support goroutines. Concurrent fan-out is impossible.

In a traditional Go server, `refreshAllHandler` would look like:
```go
var wg sync.WaitGroup
for i := range feeds {
    wg.Add(1)
    go func(f StoreFeed) {  // ← goroutine
        defer wg.Done()
        refreshFeed(db, &f)
    }(feeds[i])
}
wg.Wait()
```

In WASM, it's sequential. The tradeoff: each feed refresh is a network call, so refreshing 20 feeds takes 20× the latency of one.

### No Persistent State

Every request is a fresh WASM instance. That's why:

- `openDB()` is called at the start of every handler
- Migrations run every request (`IF NOT EXISTS` makes them no-ops after first run)
- There's no global state — no connection pools, no caches, no counters

```go
// store.go:20-30
func openDB() (*sql.DB, error) {
    db := sqlite.Open("default")

    for _, migration := range []string{createFeedsTable, createArticlesTable, ...} {
        if _, err := db.Exec(migration); err != nil {
            return nil, fmt.Errorf("running migration: %w", err)
        }
    }

    return db, nil
}
```

The first request ever creates the tables. Every subsequent request, `IF NOT EXISTS` makes those statements instant no-ops.

### No Third-Party Routers

```go
// main.go (comment before router):
// All routing within the API component is done manually in Go
// because third-party routers like httprouter can cause WASM traps at init time.
```

Some Go packages use `init()` functions that do things WASM doesn't support — goroutines, reflection, OS calls. `httprouter` initializes its trie data structures in ways that can trap (crash) at WASM startup. So Spindle uses a hand-written `switch` statement instead. Boring, but reliable.

### The Build Tag Dance

```go
// store.go:1
//go:build tinygo || wasip1
```

`store.go`, `main.go`, and `ui.go` are only compiled when building with TinyGo or for WASI. `models.go` and `helpers.go` don't have this tag — they compile with standard Go too, enabling testing.

```
Files compiled with TinyGo (WASM build):
  ✓ main.go, ui.go, store.go, migrations.go
  ✓ models.go, feeds.go, helpers.go

Files compiled with standard Go (testing):
  ✗ main.go, ui.go, store.go, migrations.go  (build tag excluded)
  ✓ models.go, feeds.go, helpers.go
  ✓ feeds_test.go, routing_test.go
```

---

## Part 10: What Does the WASM Binary Actually Contain?

When `tinygo build` runs, here's what it produces in `main.wasm`:

```
main.wasm contains:
───────────────────

┌─────────────────────────────────────────────────────────┐
│  Compiled bytecode for:                                 │
│    • All your Go functions (router, handlers, etc.)     │
│    • TinyGo standard library (http, json, xml, sql)     │
│    • Spin SDK (http client, sqlite client)              │
│                                                         │
│  WASI imports (what the module needs from the host):    │
│    • fd_write (write to file descriptors)               │
│    • sock_connect (make network connections)            │
│    • clock_time_get (get current time)                  │
│    • TinyGo's WASI shims for Go runtime functions       │
│                                                         │
│  Spin SDK exports (what Spin calls into):               │
│    • _start / wasi_http_incoming_handler                │
│    • handle_http_request                                │
│                                                         │
│  Linear memory: 64KB pages, grows as needed            │
└─────────────────────────────────────────────────────────┘
```

The `-no-debug` flag strips debug symbols. Without it the binary would be larger (debug info, source maps). With it, you can't attach a debugger, but the binary is smaller and faster to load.

The `-gc=leaking` flag replaces the garbage collector with a simple bump allocator — allocate, never free. Normally this would be catastrophic. For a WASM module that lives for exactly one HTTP request, it's perfect: faster allocation, zero GC overhead, and everything gets cleaned up when the instance is destroyed.

---

## Part 11: The Digest-Pinned Fileserver

```toml
source = { url = "https://github.com/spinframework/spin-fileserver/releases/download/v0.3.0/spin_static_fs.wasm",
           digest = "sha256:ef88708817e107bf49985c7cefe4dd1f199bf26f6727819183d5c996baa3d148" }
```

This is a security practice called **content addressing**. Rather than trusting "whatever is at this URL today," you trust "the binary whose SHA-256 hash is exactly this."

If someone compromised the GitHub release and replaced the WASM binary, the hash would change. Spin would refuse to run it. The application would fail rather than run malicious code.

This is the same principle as `go.sum` — Go's module system records the expected hash of every dependency, so a compromised package repository can't silently deliver malicious code.

---

## Part 12: WASM vs. Containers — A Comparison

You might wonder: isn't this what Docker does? Containers also provide isolation and portability. Here's how they compare:

```
                    Containers (Docker)      WebAssembly (Spin)
                    ───────────────────      ──────────────────
Startup time        100ms - 2s               <10ms
Memory footprint    100MB+                   1-10MB
Isolation unit      OS process               Function/module
Security model      Linux namespaces/cgroups WASM sandbox
Portability         Any CPU if image built   Any WASM runtime
Language support    Any language             Languages with WASM target
Shared memory       Possible (risk)          Impossible (by design)
State between reqs  Possible (risk)          Impossible (by design)
```

WASM is more restrictive but also simpler and safer. The impossibility of shared memory between requests means an entire class of concurrency bugs can't happen.

---

## Summary

```
The WebAssembly stack in Spindle:
──────────────────────────────────

  Go source code
       │
       │  tinygo build -target=wasip1 ...
       ▼
  main.wasm  ← portable bytecode, ~1MB
       │
       │  spin up (reads spin.toml)
       ▼
  Spin runtime  ← WASM host, enforces capabilities
       │
       │  HTTP request arrives
       ▼
  Fresh WASM instance created
       │
       │  Executes handlers
       │  DB calls → cross WASM boundary → Spin → SQLite
       │  HTTP calls → cross WASM boundary → Spin → network
       ▼
  Response sent
       │
       ▼
  WASM instance destroyed
       (memory freed, no state persists)
```

**The four pillars of WASM that shape Spindle:**

1. **Sandboxed** — capabilities must be declared in `spin.toml`
2. **Stateless** — fresh instance per request, open DB every time
3. **Single-threaded** — no goroutines, sequential loops
4. **Portable** — same binary runs anywhere with a WASM runtime

**Files to explore:**
- `spin.toml` — capability grants, component definitions
- `store.go:20-30` — `openDB()` and why schema runs every request
- `main.go:249-259` — sequential refresh (no goroutines)
- `feeds.go:13-19` — comment explaining the WASM/host boundary
- `migrations.go` — `IF NOT EXISTS` as the stateless migration strategy

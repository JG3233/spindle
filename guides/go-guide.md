# Go From First Principles
### *A guided tour through Spindle — no prior knowledge assumed*

> **Note:** Code references in this guide point to specific file locations (e.g. `main.go:14`) and include inline snippets. These are accurate as of when the guide was written. If the source files have changed, treat the snippets as illustrative examples and refer to the actual files for the current implementation.

---

```
┌──────────────────────────────────────────────────────────────┐
│                                                              │
│   Go's philosophy in one sentence:                           │
│                                                              │
│   "Make the common case easy, make the wrong thing hard      │
│    to do accidentally, and make the code readable            │
│    by someone who didn't write it."                          │
│                                                              │
└──────────────────────────────────────────────────────────────┘
```

## Part 1: What Is Go, and Why Does It Look Like This?

Go was designed at Google in 2009 by people who were frustrated with C++ — too complex, too slow to compile. They wanted a language that was:

- **Fast to compile** (no waiting)
- **Easy to read** (no clever tricks)
- **Safe by default** (no undefined behavior)
- **Good at networking** (goroutines, HTTP built-in)

The result looks deceptively simple. That's intentional. Go has very few features, by design. You won't find classes, inheritance, exceptions, or operator overloading. What you will find is a small set of orthogonal tools that compose cleanly.

Spindle is a great codebase to learn from because it uses Go idiomatically — no fancy tricks, just the patterns you'll see in every real Go project.

---

## Part 2: The Package System and `main()`

Every Go file starts with a package declaration:

```go
// main.go:3
package main
```

Go organizes code into **packages**. The `main` package is special: it's the entry point for an executable program. Every Go binary must have exactly one `main` package with a `main()` function.

But look at Spindle's `main()`:

```go
// main.go:389
func main() {}
```

Empty! That's because Spin/WASM has a different entry point model. The actual entry point is registered in `init()`:

```go
// main.go:14-16
func init() {
    spinhttp.Handle(router)
}
```

`init()` is a special Go function that runs before `main()`, automatically. Here it registers `router` as the HTTP handler for all incoming requests. When Spin receives an HTTP request, it calls `router`.

---

## Part 3: Imports — Bringing In Other Code

```go
// main.go:5-11
import (
    "encoding/json"
    "net/http"
    "strconv"
    "strings"

    spinhttp "github.com/spinframework/spin-go-sdk/v2/http"
)
```

Imports are grouped by convention: standard library first, then third-party. The `spinhttp` before the URL is an **alias** — it lets you use a shorter name (`spinhttp.Handle`) instead of the full package name.

Go's standard library is extensive and excellent. `net/http`, `encoding/json`, `encoding/xml`, `database/sql` — these are all in the standard library. You don't need to install them.

---

## Part 4: Types — Go Is Statically Typed

Every variable in Go has a type, determined at compile time. There are no runtime type surprises.

### Basic Types

```go
var name string = "Spindle"   // string
var count int = 42            // integer
var ratio float64 = 3.14      // floating point
var active bool = true        // boolean
```

Go can often infer the type:
```go
name := "Spindle"   // Go infers: string
count := 42         // Go infers: int
```

The `:=` operator means "declare and assign." After the first use, use `=` to reassign.

### Structs — Grouping Related Data

Structs are Go's way of bundling data together. You'll see them throughout Spindle:

```go
// models.go:71-77
type Feed struct {
    Title       string    `json:"title"`
    Link        string    `json:"link"`
    Description string    `json:"description"`
    Articles    []Article `json:"articles"`
}
```

`type Feed struct { ... }` defines a new type called `Feed`. The fields inside it have names and types.

Those backtick annotations (`` `json:"title"` ``) are **struct tags** — metadata about the field. The `json` package reads them to know what key to use when converting to/from JSON. The `xml` package uses `xml:"..."` tags similarly.

To create and use a Feed:

```go
f := Feed{
    Title: "My Blog",
    Link:  "https://myblog.com",
}
fmt.Println(f.Title) // "My Blog"
```

### Pointers — The `*` and `&` Symbols

You'll see `*Feed` and `&feed` throughout Spindle. These are pointers.

A **pointer** is a variable that holds the memory address of another variable, rather than the value itself.

```
WITHOUT pointer:          WITH pointer:
─────────────────         ────────────────────────────

 feed1 := Feed{...}       feed1 := Feed{...}
 feed2 := feed1           feed2 := &feed1
 feed2.Title = "X"        feed2.Title = "X"

 // feed1.Title is         // feed1.Title is NOW "X"
 // still original         // because feed2 points to feed1
```

`&feed1` means "give me the address of feed1." `*Feed` as a type means "a pointer to a Feed."

In Spindle, functions return `*Feed` (pointer to Feed) rather than `Feed` (copy of Feed). This is efficient for large structs, and it also allows returning `nil` to signal "nothing here":

```go
// store.go:87-91
func getFeed(db *sql.DB, id int64) (*StoreFeed, error) {
    row := db.QueryRow(...)
    return scanFeed(row)
}
```

If no feed is found, `scanFeed` returns `nil, err`.

---

## Part 5: Functions and Multiple Return Values

Go functions can return multiple values. This is the cornerstone of Go's error handling:

```go
// feeds.go:20
func fetchFeed(url string) (*Feed, error) {
```

This function returns two things: a pointer to a Feed, and an error. The caller must handle both:

```go
feed, err := fetchFeed(url)
if err != nil {
    // something went wrong
    return nil, fmt.Errorf("fetching feed: %w", err)
}
// use feed
```

Compare this to languages that use exceptions. In Go:

```
Python/Java (exceptions)        Go (multiple returns)
────────────────────────        ───────────────────────────────
try:                            feed, err := fetchFeed(url)
    feed = fetchFeed(url)       if err != nil {
except Exception as e:              return nil, err
    handle(e)                   }
                                // proceed with feed
```

Go's approach is more verbose but has a critical benefit: **errors are visible.** Every call site that can fail is explicit about it. You can't accidentally ignore an error — well, you can with `_`, but it's explicit:

```go
_ = db.Exec(...)  // Spindle does this when ignoring an error on purpose
```

### Error Wrapping with `%w`

```go
// feeds.go:22-23
return nil, fmt.Errorf("fetching feed: %w", err)
```

`%w` wraps the original error. The resulting error message becomes `"fetching feed: connection refused"` — preserving context at each layer of the call stack. It's like a chain of "caused by" messages.

---

## Part 6: Slices — Go's Dynamic Arrays

A slice is a view into an underlying array. It's Go's most-used collection type.

```go
var articles []Article         // nil slice (zero value)
articles = make([]Article, 0, len(items))  // empty slice, pre-allocated
articles = append(articles, item)          // add an element
```

`make([]Article, 0, len(items))` creates a slice with length 0 but capacity `len(items)`. Pre-allocating capacity avoids repeated memory allocations during `append`.

Here's the full pattern in `parseRSS` (feeds.go:69-82):

```go
articles := make([]Article, 0, len(rss.Channel.Items))
for _, item := range rss.Channel.Items {
    articles = append(articles, Article{
        GUID:  item.GUID,
        Title: item.Title,
        // ...
    })
}
```

`for _, item := range rss.Channel.Items` iterates over the slice. The `_` discards the index (we don't need `i`). `item` is a copy of each element.

If you need to modify elements in place, range over indices:
```go
for i := range feeds {
    refreshFeed(db, &feeds[i])  // &feeds[i] is a pointer to the actual element
}
```

This pattern appears in `main.go:253` — if you used `for _, f := range feeds`, `f` would be a copy and modifications wouldn't affect the original slice.

---

## Part 7: The `switch` Statement — Spindle's Router

Go's `switch` statement doesn't require `break` between cases (they don't fall through by default). It's more powerful than C's `switch`.

Spindle uses it as a URL router (main.go:22-80):

```go
func router(w http.ResponseWriter, r *http.Request) {
    path := strings.TrimRight(r.URL.Path, "/")
    method := r.Method

    switch {
    case method == http.MethodGet && path == "/api/health":
        healthHandler(w, r)

    case method == http.MethodPost && path == "/api/feeds":
        createFeedHandler(w, r)

    case method == http.MethodGet && strings.HasPrefix(path, "/api/feeds/"):
        getFeedHandler(w, r, path)

    default:
        http.NotFound(w, r)
    }
}
```

`switch { case expr: ... }` without a switch expression is a clean way to express a chain of boolean conditions. Each `case` is evaluated top to bottom; the first true one runs.

The order matters! Notice how `/api/feeds/preview` is checked before `/api/feeds/:id` (main.go:49-63). If you checked for the ID pattern first, `preview` would match as an ID.

---

## Part 8: Interfaces — Behavior Without Inheritance

Go doesn't have classes or inheritance. Instead, it has **interfaces** — a set of methods that a type must implement.

The key HTTP interfaces are `http.ResponseWriter` and `*http.Request`. Every handler in Spindle takes these two parameters:

```go
func healthHandler(w http.ResponseWriter, r *http.Request) {
    writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
```

`w` is the "write to the response" interface. It has a `Write([]byte)` method, a `Header()` method, and a `WriteHeader(int)` method. You don't need to know what concrete type it is — you just call these methods.

`r` is the incoming request. `r.Method`, `r.URL.Path`, `r.Body`, `r.URL.Query()` — all the request data you need.

### The `any` Type

`any` is Go 1.18's alias for `interface{}` — the empty interface that any type satisfies:

```go
// main.go:228
writeJSON(w, http.StatusOK, map[string]any{
    "feed_id":        feed.ID,      // int64
    "articles_added": added,        // int
})
```

`map[string]any` is a map from strings to anything. Useful for ad-hoc JSON responses.

---

## Part 9: The Database Layer — Reading SQL Results

`store.go` shows Go's database patterns clearly. There are two ways to query:

### Single Row: `QueryRow`

```go
// store.go:87-91
func getFeed(db *sql.DB, id int64) (*StoreFeed, error) {
    row := db.QueryRow(
        `SELECT id, url, title, ... FROM feeds WHERE id = ?`, id,
    )
    return scanFeed(row)
}
```

`QueryRow` executes a query and returns a single `*sql.Row`. Call `Scan()` on it to extract values:

```go
// store.go:125-132
func scanFeed(row *sql.Row) (*StoreFeed, error) {
    var f StoreFeed
    err := row.Scan(
        &f.ID, &f.URL, &f.Title, &f.Description,
        &f.SiteLink, &f.LastFetchedAt, &f.CreatedAt,
    )
    ...
}
```

`Scan` takes pointers to variables and fills them with the column values. The `&` takes the address of each field.

### Multiple Rows: `Query`

```go
// store.go:97-116
rows, err := db.Query(`SELECT ... FROM feeds ORDER BY created_at DESC`)
if err != nil {
    return nil, fmt.Errorf("listing feeds: %w", err)
}
defer rows.Close()

var feeds []StoreFeed
for rows.Next() {
    var f StoreFeed
    if err := rows.Scan(&f.ID, &f.URL, ...); err != nil {
        return nil, err
    }
    feeds = append(feeds, f)
}
```

The `defer rows.Close()` is important — it ensures rows are closed even if we return early due to an error. `defer` runs when the surrounding function returns, no matter how.

### Dynamic Query Building

When filters are optional, build the query string dynamically (store.go:213-234):

```go
query := `SELECT ... FROM articles WHERE 1=1`
var args []any

if feedID > 0 {
    query += ` AND feed_id = ?`
    args = append(args, feedID)
}
if isRead >= 0 {
    query += ` AND is_read = ?`
    args = append(args, isRead)
}

query += ` ORDER BY published_at DESC LIMIT ? OFFSET ?`
args = append(args, limit, offset)

rows, err := db.Query(query, args...)
```

`args...` is Go's **spread operator** — it unpacks the slice into individual arguments. Like Python's `*args`.

`WHERE 1=1` is a trick: it's always true, so you can always append `AND ...` without worrying about whether you're adding the first condition.

---

## Part 10: String Building

Spindle's HTMX handlers generate HTML as strings. Go offers `strings.Builder` for efficient concatenation:

```go
// ui.go:38-68
var b strings.Builder
b.WriteString(`<div class="feed-item">`)
fmt.Fprintf(&b, `<span>%s</span>`, escapeHTML(f.Title))
b.WriteString(`</div>`)

writeHTML(w, http.StatusOK, b.String())
```

Why not just concatenate with `+`? In Go, strings are immutable. Each `+` creates a new string. With `strings.Builder`, all the pieces are written to a buffer and converted to a string once at the end.

`fmt.Fprintf(&b, ...)` is like `fmt.Printf` but writes to any `io.Writer` — including `strings.Builder`. The `&b` passes a pointer to `b` because `Fprintf` needs to call `b.Write(...)`.

---

## Part 11: Build Tags — Conditional Compilation

At the top of several files:

```go
//go:build tinygo || wasip1
```

This is a **build tag**. It tells the compiler: only include this file when building with TinyGo OR targeting the `wasip1` platform. Files without this tag (like `models.go`, `helpers.go`) compile with both normal Go and TinyGo.

Why? Because `store.go` and `main.go` use packages (`github.com/spinframework/spin-go-sdk`) that only work in the WASM context. The test files and pure-logic files need to compile with standard Go for testing.

Running `go test ./...` (standard Go) includes `models.go` and `helpers.go` but not `main.go` or `store.go`. The test targets only test the portable code.

---

## Part 12: Putting It All Together

Let's trace a complete request from HTTP to response, touching every layer:

```
HTTP Request: POST /api/feeds
Body: {"url": "https://myblog.com/feed.xml"}

        main.go
        ────────
        router() — switch matches "POST /api/feeds"
        calls createFeedHandler(w, r)
             │
             │  json.NewDecoder(r.Body).Decode(&req)
             │  extracts req.URL = "https://myblog.com/feed.xml"
             │
             │  openDB() — opens SQLite connection
             │
             │  addFeed(db, req.URL)
             ▼
        store.go
        ────────
        addFeed() — orchestrates:
             │
             │  fetchFeed(url)
             ▼
        feeds.go
        ────────
        fetchFeed() — http.Get(url) → bytes
             │
             │  parseFeed(bytes)
             │    parseRSS → xml.Unmarshal → rssXML{}
             │    convert rssXML → Feed{Articles: [...]}
             │
             ▲  returns *Feed
             │
        store.go (continued)
        ────────────────────
        db.Exec("INSERT INTO feeds ...")
        getFeedByURL() — SELECT back to get id
        insertArticles() — INSERT OR IGNORE for each article
             │
             ▲  returns *StoreFeed
             │
        main.go (continued)
        ───────────────────
        writeJSON(w, 201, feed)
             │
             │  json.NewEncoder(w).Encode(feed)
             ▼
HTTP Response: 201 Created
Body: {"id":1,"url":"...","title":"My Blog",...}
```

---

## Summary of Go Patterns You've Seen

| Pattern | Where in Spindle | Key insight |
|---------|-----------------|-------------|
| Multiple returns `(value, error)` | Every function that can fail | Errors are explicit, not exceptions |
| `if err != nil { return }` | After every fallible call | The "error highway" pattern |
| Struct tags `` `json:"..."` `` | `models.go`, `store.go` | Metadata drives serialization |
| `defer rows.Close()` | `store.go` | Cleanup runs at function return |
| `strings.Builder` | `ui.go` | Efficient string concatenation |
| `for i := range slice` | `main.go:253`, `ui.go:164` | When you need to modify elements |
| `args...` spread operator | `store.go:234` | Unpack slice into variadic args |
| `switch { case bool: }` | `main.go:22` | Clean multi-branch dispatch |
| `//go:build` tags | `main.go`, `store.go`, `ui.go` | Conditional compilation |
| `:=` short declaration | Everywhere | Declare + assign in one step |

**Files to explore in order:**
1. `models.go` — types, struct tags, no logic
2. `helpers.go` — pure functions, testable with standard Go
3. `feeds.go` — HTTP, XML parsing, error handling
4. `store.go` — SQL, pointers, the database pattern
5. `main.go` — the router, putting it together
6. `ui.go` — strings.Builder, HTML generation

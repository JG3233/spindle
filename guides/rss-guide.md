# RSS From First Principles
### *A guided tour through Spindle — no prior knowledge assumed*

> **Note:** Code references in this guide point to specific file locations (e.g. `models.go:24`) and include inline snippets. These are accurate as of when the guide was written. If the source files have changed, treat the snippets as illustrative examples and refer to the actual files for the current implementation.

---

```
┌─────────────────────────────────────────────────────────┐
│                                                         │
│   "I just want to read stuff without checking 30        │
│    different websites every morning."                   │
│                                                         │
│   — Every RSS user, ever                                │
│                                                         │
└─────────────────────────────────────────────────────────┘
```

## Part 1: The Problem RSS Solves

Before RSS existed, following your favorite websites meant visiting each one manually. Every. Single. Day. If a site published once a week and you checked daily, you wasted six visits out of seven.

In the late 1990s, developers started asking: what if websites could *publish a machine-readable summary* of their new content, so readers could check one place instead of many?

That's RSS. **Really Simple Syndication.**

Think of it like this:

```
BEFORE RSS                          WITH RSS
──────────                          ────────

You visit 20 websites               Websites publish a "feed"
     │                                   │
     ▼                                   ▼
19 have nothing new             Your reader checks all feeds
     │                                   │
     ▼                                   ▼
1 has a new post                Only shows you new content
     │                                   │
     ▼                                   ▼
😩 40 minutes wasted             ✅ 2 minutes, done
```

Spindle is an RSS reader. It collects these feeds, stores the articles, and lets you read them in one place. Let's trace exactly how it does that.

---

## Part 2: What Does an RSS Feed Actually Look Like?

An RSS feed is just an XML file. Your browser can fetch it like any web page, but instead of HTML for humans, it's structured data for machines.

Here's a minimal RSS feed — something a blog might publish at `https://myblog.com/feed.xml`:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>My Tech Blog</title>
    <link>https://myblog.com</link>
    <description>Writing about programming and other things</description>

    <item>
      <title>Why Go is Great</title>
      <link>https://myblog.com/posts/why-go</link>
      <description>Let me tell you about error handling...</description>
      <guid>https://myblog.com/posts/why-go</guid>
      <pubDate>Mon, 28 Mar 2026 10:00:00 +0000</pubDate>
    </item>

    <item>
      <title>I Finally Understand WebAssembly</title>
      <link>https://myblog.com/posts/wasm</link>
      <description>It's a virtual instruction set, not a language...</description>
      <guid>https://myblog.com/posts/wasm</guid>
      <pubDate>Fri, 24 Mar 2026 09:30:00 +0000</pubDate>
    </item>
  </channel>
</rss>
```

Let's break down the key pieces:

| Tag | What it means |
|-----|---------------|
| `<rss>` | The root element. Tells parsers "this is RSS format". |
| `<channel>` | The feed itself — one blog, one podcast, one news source. |
| `<title>` | The name of the blog/site. |
| `<link>` | The website's homepage URL. |
| `<item>` | One article. A feed can have many items. |
| `<guid>` | **G**lobally **U**nique **Id**entifier — a string that never changes for this article. Usually the URL. |
| `<pubDate>` | When the article was published. RSS uses a specific date format. |

### The GUID Is the Secret to Deduplication

The GUID is how Spindle avoids showing you the same article twice. If you refresh a feed 10 times, each time you'll get the same items back. Without GUIDs, you'd store 10 copies of every article.

With GUIDs, Spindle can say: "Have I seen `https://myblog.com/posts/why-go` before? Yes → skip it."

---

## Part 3: RSS Isn't the Only Format — Meet Atom

RSS got popular fast, but it was also kind of messy. Different versions (0.9, 1.0, 2.0), inconsistent tag names, ambiguous specs. In 2005, the IETF published **Atom** as a cleaner, more standardized alternative.

Spindle supports both. Here's what the same feed looks like in Atom:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <title>My Tech Blog</title>
  <link href="https://myblog.com" rel="alternate"/>

  <entry>
    <title>Why Go is Great</title>
    <link href="https://myblog.com/posts/why-go" rel="alternate"/>
    <id>https://myblog.com/posts/why-go</id>
    <updated>2026-03-28T10:00:00Z</updated>
    <summary>Let me tell you about error handling...</summary>
  </entry>
</feed>
```

Spot the differences?

```
RSS vs Atom — Key Differences
──────────────────────────────

  RSS                         Atom
  ─────────────────           ─────────────────────────────
  <rss>...</rss>              <feed xmlns="...">...</feed>
  <item>...</item>            <entry>...</entry>
  <guid>URL</guid>            <id>URL</id>
  <pubDate>RFC1123</pubDate>  <updated>ISO 8601</updated>
  <link>URL</link>            <link href="URL" rel="alternate"/>
```

Atom's `<link>` is the tricky one. Instead of putting the URL inside the tag like RSS does, Atom puts it in an **attribute**: `href="..."`. And there can be multiple `<link>` elements with different `rel` values:

- `rel="alternate"` → the webpage to read (what you want)
- `rel="self"` → the feed URL itself (what you don't want)

---

## Part 4: How Spindle Models These Formats

Open `models.go`. This file defines Go structs that mirror the XML structure exactly.

```go
// models.go:24-27 — RSS root element
type rssXML struct {
    XMLName xml.Name      `xml:"rss"`
    Channel rssChannelXML `xml:"channel"`
}
```

Those backtick annotations (`` `xml:"rss"` ``) are **struct tags**. They tell Go's XML parser: "when you see an `<rss>` element, decode it into this struct." It's a declaration of the mapping between XML and Go types.

Here's the full RSS hierarchy:

```
XML Structure              Go Struct Mapping
─────────────              ─────────────────

<rss>               →      rssXML
  <channel>         →        .Channel (rssChannelXML)
    <title>         →          .Title (string)
    <link>          →          .Link (string)
    <description>   →          .Description (string)
    <item>          →          .Items ([]rssItemXML)
      <title>       →            [i].Title
      <link>        →            [i].Link
      <guid>        →            [i].GUID
      <pubDate>     →            [i].PubDate
```

And the Atom version (models.go:45-64):

```go
type atomXML struct {
    XMLName xml.Name       `xml:"feed"`
    Title   string         `xml:"title"`
    Links   []atomLinkXML  `xml:"link"`      // ← plural! multiple <link> elements
    Entries []atomEntryXML `xml:"entry"`
}

type atomLinkXML struct {
    Href string `xml:"href,attr"` // ← "attr" means it's an XML attribute, not content
    Rel  string `xml:"rel,attr"`
}
```

The `,attr` suffix in `` `xml:"href,attr"` `` is crucial. Without it, Go would look for a `<href>` child element. With it, Go looks for `href="..."` as an attribute — which is how Atom actually works.

---

## Part 5: Fetching and Parsing a Feed

Now trace what happens when you subscribe to a feed in Spindle. Open `feeds.go`.

### Step 1: HTTP Fetch

```go
// feeds.go:20-37
func fetchFeed(url string) (*Feed, error) {
    resp, err := http.Get(url)
    if err != nil {
        return nil, fmt.Errorf("fetching feed: %w", err)
    }
    defer resp.Body.Close()

    body, err := io.ReadAll(resp.Body)
    ...
    return parseFeed(body)
}
```

`http.Get(url)` downloads the feed XML. `io.ReadAll` reads the entire response into memory as a byte slice. Then we hand it to `parseFeed`.

### Step 2: Auto-detect the Format

```go
// feeds.go:45-58
func parseFeed(data []byte) (*Feed, error) {
    // Try RSS 2.0 first
    feed, err := parseRSS(data)
    if err == nil && feed.Title != "" {
        return feed, nil
    }

    // Try Atom
    feed, err = parseAtom(data)
    if err == nil && feed.Title != "" {
        return feed, nil
    }

    return nil, fmt.Errorf("could not parse feed as RSS or Atom")
}
```

Spindle doesn't look at the `Content-Type` header or examine the root element name. It just tries RSS first. If that succeeds *and* produces a non-empty title, great. If not, try Atom. This is a "duck typing" approach — if it quacks like RSS, it's RSS.

### Step 3: XML Deserialization

```go
// feeds.go:61-90
func parseRSS(data []byte) (*Feed, error) {
    var rss rssXML
    if err := xml.Unmarshal(data, &rss); err != nil {
        return nil, err
    }

    articles := make([]Article, 0, len(rss.Channel.Items))
    for _, item := range rss.Channel.Items {
        guid := item.GUID
        if guid == "" {
            guid = item.Link  // Fall back to link as unique ID
        }
        articles = append(articles, Article{
            GUID:        guid,
            Title:       item.Title,
            Link:        item.Link,
            Description: truncate(item.Description, 500),
            PublishedAt: normalizeDate(item.PubDate),
        })
    }

    return &Feed{
        Title:       rss.Channel.Title,
        Link:        rss.Channel.Link,
        Description: rss.Channel.Description,
        Articles:    articles,
    }, nil
}
```

`xml.Unmarshal(data, &rss)` is the magic line. It reads the byte slice, traverses the XML tree, and fills in the `rssXML` struct fields using those `xml:"..."` tags we saw earlier.

Then we convert from the "raw XML type" (`rssXML`) to the "normalized type" (`Feed`). This separation is important: the XML types exactly mirror the XML format, while `Feed` and `Article` are what the rest of the app uses.

---

## Part 6: The Normalization Layer

Why have two sets of types? Because RSS and Atom represent the same information differently. The app would be full of `if RSS { ... } else { ... }` checks if it worked with raw XML types everywhere.

Instead, both formats funnel into:

```go
// models.go:71-84
type Feed struct {
    Title       string    `json:"title"`
    Link        string    `json:"link"`
    Description string    `json:"description"`
    Articles    []Article `json:"articles"`
}

type Article struct {
    GUID        string `json:"guid"`
    Title       string `json:"title"`
    Link        string `json:"link"`
    Description string `json:"description"`
    PublishedAt string `json:"published_at"`
}
```

Now the rest of the app only knows about `Feed` and `Article`. The parsing complexity is confined to `feeds.go`.

```
        feeds.go                        rest of app
  ┌──────────────────┐                ┌────────────┐
  │  rssXML ─────┐   │                │            │
  │               ├─►│  Feed/Article  │  store.go  │
  │  atomXML ────┘   │                │  ui.go     │
  └──────────────────┘                │  main.go   │
                                      └────────────┘
```

---

## Part 7: The Date Problem

One of the messiest parts of RSS in practice is dates. The spec says use RFC 1123Z format (`Mon, 02 Jan 2006 15:04:05 -0700`), but feeds out in the wild use all sorts of things.

Spindle's `normalizeDate` function handles this:

```go
// feeds.go:139-163
formats := []string{
    time.RFC1123Z,                     // "Mon, 02 Jan 2006 15:04:05 -0700" (most RSS)
    time.RFC1123,                      // "Mon, 02 Jan 2006 15:04:05 MST"
    time.RFC3339,                      // "2006-01-02T15:04:05Z07:00" (Atom)
    "2006-01-02T15:04:05Z",           // Atom without timezone offset
    "2006-01-02 15:04:05",            // Simple datetime
    "Mon, 2 Jan 2006 15:04:05 -0700", // RSS with single-digit day
    "Mon, 2 Jan 2006 15:04:05 MST",
}

for _, format := range formats {
    if t, err := time.Parse(format, s); err == nil {
        return t.UTC().Format(time.RFC3339)
    }
}
```

> **Go's date formatting is unusual.** Most languages use format strings like `%Y-%m-%d`. Go instead uses a *reference time*: January 2, 2006, 15:04:05 UTC-7. Every format string is that exact time written in the format you want. So `"2006-01-02"` means "year-month-day" because the reference time's year is 2006, month is 01 (January), day is 02.

All dates get converted to ISO 8601 (`2026-03-28T10:00:00Z`). Why? Because SQLite can sort ISO 8601 strings alphabetically and get chronological order. If you stored `Mon, 28 Mar 2026 10:00:00 +0000`, sorting alphabetically would give you nonsense.

---

## Part 8: Storing Articles Without Duplicates

Once parsed, articles go into SQLite. Look at `migrations.go`:

```sql
CREATE TABLE IF NOT EXISTS articles (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    feed_id INTEGER NOT NULL REFERENCES feeds(id) ON DELETE CASCADE,
    guid TEXT NOT NULL,
    ...
    UNIQUE(feed_id, guid)  -- ← the deduplication constraint
)
```

And in `store.go`:

```go
// store.go:162-164
result, err := db.Exec(
    `INSERT OR IGNORE INTO articles (feed_id, guid, title, link, description, published_at)
     VALUES (?, ?, ?, ?, ?, ?)`,
    feedID, a.GUID, a.Title, a.Link, a.Description, a.PublishedAt,
)
```

`INSERT OR IGNORE` is SQLite's way of saying: "insert this row, but if a row with the same `(feed_id, guid)` already exists, silently do nothing." Combined with the `UNIQUE(feed_id, guid)` constraint, this means:

```
First refresh:   10 new articles → 10 inserted
Second refresh:  Same 10 articles → 0 inserted (all ignored)
Third refresh:   8 old + 2 new → 2 inserted
```

The GUID from the feed XML becomes the deduplication key in the database.

---

## Part 9: The Complete Flow

Let's trace a subscribe operation from click to database:

```
User pastes feed URL in the form
         │
         │  POST /api/ui/feeds  (form-encoded)
         ▼
   uiCreateFeedHandler (ui.go:74)
         │
         │  calls addFeed(db, url)
         ▼
   addFeed (store.go:50)
         │
         │  calls fetchFeed(url)
         ▼
   fetchFeed (feeds.go:20)
         │  ← HTTP GET to the feed URL
         │  ← Gets back XML bytes
         │
         │  calls parseFeed(body)
         ▼
   parseFeed (feeds.go:45)
         │  tries parseRSS → succeeds or fails
         │  tries parseAtom → succeeds or fails
         │  returns *Feed{Title, Link, Articles...}
         │
         ▲ back in addFeed
         │
         │  INSERT INTO feeds (url, title, ...) VALUES (...)
         │  SELECT back to get the auto-assigned id
         │  calls insertArticles(db, feedID, feed.Articles)
         ▼
   insertArticles (store.go:156)
         │  for each article:
         │    INSERT OR IGNORE INTO articles (feed_id, guid, ...)
         │
         ▲ back in uiCreateFeedHandler
         │
         │  calls uiFeedsListHandler to render updated sidebar
         ▼
   HTML fragment sent back to browser
         │
         ▼  HTMX swaps it into #feed-list
   Sidebar shows new feed  ✓
```

---

## Part 10: What Makes a Good RSS Feed?

Now that you understand the format, here's what separates a great feed from a frustrating one:

| Practice | Good | Bad |
|----------|------|-----|
| GUID | Stable URL that never changes | Timestamp-based or missing |
| Description | Useful summary (not the full article, not empty) | Raw HTML dump or empty |
| pubDate | Correct timestamp in standard format | Missing, wrong timezone |
| Update behavior | New post = new item | Editing old posts changes their GUID |
| Feed URL | Stable, never moves | Changes when site redesigns |

The `truncate(item.Description, 500)` call in `feeds.go:79` is Spindle being defensive about that middle row — some sites stuff entire HTML pages into the description.

---

## Summary

```
RSS in a nutshell:
──────────────────

1. A website publishes an XML file (feed) at a stable URL
2. Each post/article = one <item> (RSS) or <entry> (Atom)
3. Each item has a GUID — a permanent ID for deduplication
4. Dates are messy in practice; normalize them for sorting
5. Your reader checks feeds periodically and shows only new items

Spindle's RSS pipeline:
───────────────────────

  fetch URL          parse XML           normalize           store
  ──────────    →    ──────────    →    ──────────    →    ──────────
  http.Get()        xml.Unmarshal      rssXML→Feed       INSERT OR
  returns bytes     into rssXML/       Article types      IGNORE with
                    atomXML structs    with clean dates   (feed_id,guid)
                                                          UNIQUE key
```

**Files to explore:**
- `models.go` — XML struct definitions and normalized types
- `feeds.go` — Fetching, parsing, date normalization
- `store.go` — `insertArticles()` for deduplication logic
- `migrations.go` — Database schema with `UNIQUE(feed_id, guid)`

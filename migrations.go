//go:build tinygo || wasip1

package main

// SQL schema as Go string constants. These run on every request that
// touches the database (via ensureSchema). This is safe because of
// IF NOT EXISTS — the statements are no-ops after the first run.
//
// Why not a migration system? In a traditional server, you'd run migrations
// once at startup. But WASM components have no startup — each request is a
// fresh instance. So we check the schema on every request. It adds a few
// milliseconds but guarantees the tables exist.

const createFeedsTable = `
CREATE TABLE IF NOT EXISTS feeds (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    url TEXT NOT NULL UNIQUE,
    title TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    site_link TEXT NOT NULL DEFAULT '',
    last_fetched_at TEXT,
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
)`

const createArticlesTable = `
CREATE TABLE IF NOT EXISTS articles (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    feed_id INTEGER NOT NULL REFERENCES feeds(id) ON DELETE CASCADE,
    guid TEXT NOT NULL,
    title TEXT NOT NULL DEFAULT '',
    link TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    published_at TEXT,
    fetched_at TEXT NOT NULL DEFAULT (datetime('now')),
    is_read INTEGER NOT NULL DEFAULT 0,
    UNIQUE(feed_id, guid)
)`

const createArticleIndexes = `
CREATE INDEX IF NOT EXISTS idx_articles_feed_id ON articles(feed_id);
CREATE INDEX IF NOT EXISTS idx_articles_is_read ON articles(is_read)
`

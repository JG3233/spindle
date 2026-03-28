package main

import (
	"database/sql"
	"fmt"

	"github.com/spinframework/spin-go-sdk/v2/sqlite"
)

// openDB opens a connection to Spin's built-in SQLite and ensures
// the schema exists. Called at the start of every handler that needs
// the database.
//
// In a traditional server, you'd open one connection at startup and
// reuse it. In WASM, there IS no startup that persists — each request
// is a fresh instance. So we open + ensure schema every time.
// The "default" name matches sqlite_databases in spin.toml.
func openDB() (*sql.DB, error) {
	db := sqlite.Open("default")

	// Run each migration. IF NOT EXISTS makes these no-ops after first run.
	for _, migration := range []string{createFeedsTable, createArticlesTable, createArticleIndexes} {
		if _, err := db.Exec(migration); err != nil {
			return nil, fmt.Errorf("running migration: %w", err)
		}
	}

	return db, nil
}

// --- Feed (subscription) operations ---

// StoreFeed represents a feed row from the database.
// This is separate from the Feed type in models.go — that one is for
// parsed XML data. This one has database fields like id and created_at.
type StoreFeed struct {
	ID            int64  `json:"id"`
	URL           string `json:"url"`
	Title         string `json:"title"`
	Description   string `json:"description"`
	SiteLink      string `json:"site_link"`
	LastFetchedAt string `json:"last_fetched_at,omitempty"`
	CreatedAt     string `json:"created_at"`
}

// addFeed subscribes to a new feed. It fetches the feed first to get
// its title and description, then inserts it into the database.
func addFeed(db *sql.DB, url string) (*StoreFeed, error) {
	// Fetch and parse the feed to get metadata
	feed, err := fetchFeed(url)
	if err != nil {
		return nil, fmt.Errorf("fetching feed: %w", err)
	}

	// Insert the subscription
	_, err = db.Exec(
		`INSERT INTO feeds (url, title, description, site_link, last_fetched_at)
		 VALUES (?, ?, ?, ?, datetime('now'))`,
		url, feed.Title, feed.Description, feed.Link,
	)
	if err != nil {
		return nil, fmt.Errorf("inserting feed: %w", err)
	}

	// Query it back to get the id and timestamps.
	// We need this because Spin's SQLite driver doesn't support LastInsertId.
	storedFeed, err := getFeedByURL(db, url)
	if err != nil {
		return nil, err
	}

	// Store the initial articles from the feed we already fetched.
	// This avoids requiring a separate refresh after subscribing.
	insertArticles(db, storedFeed.ID, feed.Articles)

	return storedFeed, nil
}

func getFeedByURL(db *sql.DB, url string) (*StoreFeed, error) {
	row := db.QueryRow(`SELECT id, url, title, description, site_link,
		COALESCE(last_fetched_at, ''), created_at FROM feeds WHERE url = ?`, url)
	return scanFeed(row)
}

func getFeed(db *sql.DB, id int64) (*StoreFeed, error) {
	row := db.QueryRow(`SELECT id, url, title, description, site_link,
		COALESCE(last_fetched_at, ''), created_at FROM feeds WHERE id = ?`, id)
	return scanFeed(row)
}

// listFeeds returns all subscribed feeds.
func listFeeds(db *sql.DB) ([]StoreFeed, error) {
	// db.Query returns rows (plural) — you iterate with Next().
	// db.QueryRow returns one row — you call Scan() directly.
	rows, err := db.Query(`SELECT id, url, title, description, site_link,
		COALESCE(last_fetched_at, ''), created_at FROM feeds ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("listing feeds: %w", err)
	}
	defer rows.Close()

	// Build up a slice by iterating. This is the standard Go pattern
	// for reading SQL result sets — there's no ORM magic.
	var feeds []StoreFeed
	for rows.Next() {
		var f StoreFeed
		if err := rows.Scan(&f.ID, &f.URL, &f.Title, &f.Description,
			&f.SiteLink, &f.LastFetchedAt, &f.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning feed: %w", err)
		}
		feeds = append(feeds, f)
	}

	return feeds, nil
}

// deleteFeed removes a subscription and all its articles (via CASCADE).
func deleteFeed(db *sql.DB, id int64) error {
	_, err := db.Exec(`DELETE FROM feeds WHERE id = ?`, id)
	return err
}

func scanFeed(row *sql.Row) (*StoreFeed, error) {
	var f StoreFeed
	err := row.Scan(&f.ID, &f.URL, &f.Title, &f.Description,
		&f.SiteLink, &f.LastFetchedAt, &f.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("scanning feed: %w", err)
	}
	return &f, nil
}

// --- Article operations ---

// StoreArticle represents an article row from the database.
type StoreArticle struct {
	ID          int64  `json:"id"`
	FeedID      int64  `json:"feed_id"`
	GUID        string `json:"guid"`
	Title       string `json:"title"`
	Link        string `json:"link"`
	Description string `json:"description"`
	PublishedAt string `json:"published_at,omitempty"`
	FetchedAt   string `json:"fetched_at"`
	IsRead      bool   `json:"is_read"`
}

// insertArticles stores new articles for a feed, skipping duplicates.
// Returns the number of newly inserted articles.
//
// We use INSERT OR IGNORE — if an article with the same (feed_id, guid)
// already exists, SQLite silently skips it. This is the deduplication:
// refresh a feed 10 times, only new articles get added.
func insertArticles(db *sql.DB, feedID int64, articles []Article) (int, error) {
	added := 0
	for _, a := range articles {
		// INSERT OR IGNORE is SQLite-specific. In Postgres you'd use
		// ON CONFLICT DO NOTHING. Same idea: skip duplicates, don't error.
		result, err := db.Exec(
			`INSERT OR IGNORE INTO articles (feed_id, guid, title, link, description, published_at)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			feedID, a.GUID, a.Title, a.Link, a.Description, a.PublishedAt,
		)
		if err != nil {
			return added, fmt.Errorf("inserting article: %w", err)
		}
		// Spin's driver doesn't support RowsAffected, so we can't check
		// if the row was actually inserted or ignored. We'll count by
		// querying the total before and after instead.
		_ = result
		added++
	}

	return added, nil
}

// refreshFeed fetches a feed and stores any new articles.
// Returns the number of newly added articles.
func refreshFeed(db *sql.DB, feed *StoreFeed) (int, error) {
	parsed, err := fetchFeed(feed.URL)
	if err != nil {
		return 0, fmt.Errorf("fetching feed: %w", err)
	}

	// Count articles before insert so we can report how many were new
	var countBefore int
	row := db.QueryRow(`SELECT COUNT(*) FROM articles WHERE feed_id = ?`, feed.ID)
	if err := row.Scan(&countBefore); err != nil {
		return 0, fmt.Errorf("counting articles: %w", err)
	}

	if _, err := insertArticles(db, feed.ID, parsed.Articles); err != nil {
		return 0, err
	}

	// Update last_fetched_at and feed metadata (title may have changed)
	db.Exec(`UPDATE feeds SET last_fetched_at = datetime('now'), title = ?, description = ?, site_link = ? WHERE id = ?`,
		parsed.Title, parsed.Description, parsed.Link, feed.ID)

	var countAfter int
	row = db.QueryRow(`SELECT COUNT(*) FROM articles WHERE feed_id = ?`, feed.ID)
	if err := row.Scan(&countAfter); err != nil {
		return 0, fmt.Errorf("counting articles: %w", err)
	}

	return countAfter - countBefore, nil
}

// listArticles queries articles with optional filters.
// feedID of 0 means all feeds. isRead: 0=unread, 1=read, -1=all.
func listArticles(db *sql.DB, feedID int64, isRead int, limit, offset int) ([]StoreArticle, error) {
	// Build the query dynamically based on filters.
	// This is a common pattern when you have optional WHERE clauses.
	query := `SELECT id, feed_id, guid, title, link, description,
		COALESCE(published_at, ''), fetched_at, is_read FROM articles WHERE 1=1`
	var args []any

	if feedID > 0 {
		query += ` AND feed_id = ?`
		args = append(args, feedID)
	}
	if isRead >= 0 {
		query += ` AND is_read = ?`
		args = append(args, isRead)
	}

	query += ` ORDER BY published_at DESC, fetched_at DESC, id DESC LIMIT ? OFFSET ?`
	args = append(args, limit, offset)

	// args... is Go's "spread" operator — it unpacks the slice into
	// individual arguments. Like Python's *args.
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing articles: %w", err)
	}
	defer rows.Close()

	var articles []StoreArticle
	for rows.Next() {
		var a StoreArticle
		var isReadInt int
		if err := rows.Scan(&a.ID, &a.FeedID, &a.GUID, &a.Title, &a.Link,
			&a.Description, &a.PublishedAt, &a.FetchedAt, &isReadInt); err != nil {
			return nil, fmt.Errorf("scanning article: %w", err)
		}
		// SQLite stores booleans as integers (0/1).
		// We convert to Go's bool for cleaner JSON output.
		a.IsRead = isReadInt == 1
		articles = append(articles, a)
	}

	return articles, nil
}

// getArticle returns a single article by ID.
func getArticle(db *sql.DB, id int64) (*StoreArticle, error) {
	row := db.QueryRow(`SELECT id, feed_id, guid, title, link, description,
		COALESCE(published_at, ''), fetched_at, is_read FROM articles WHERE id = ?`, id)

	var a StoreArticle
	var isReadInt int
	err := row.Scan(&a.ID, &a.FeedID, &a.GUID, &a.Title, &a.Link,
		&a.Description, &a.PublishedAt, &a.FetchedAt, &isReadInt)
	if err != nil {
		return nil, fmt.Errorf("article not found: %w", err)
	}
	a.IsRead = isReadInt == 1
	return &a, nil
}

// updateArticleRead marks an article as read or unread.
func updateArticleRead(db *sql.DB, id int64, isRead bool) error {
	val := 0
	if isRead {
		val = 1
	}
	_, err := db.Exec(`UPDATE articles SET is_read = ? WHERE id = ?`, val, id)
	return err
}

// markAllRead marks all articles for a feed as read.
// If feedID is 0, marks ALL articles across all feeds as read.
func markAllRead(db *sql.DB, feedID int64) error {
	if feedID > 0 {
		_, err := db.Exec(`UPDATE articles SET is_read = 1 WHERE feed_id = ?`, feedID)
		return err
	}
	_, err := db.Exec(`UPDATE articles SET is_read = 1`)
	return err
}

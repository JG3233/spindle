//go:build tinygo || wasip1

package main

import (
	"database/sql"
	"fmt"
	"strings"

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
	for _, migration := range []string{
		createFeedsTable, createArticlesTable, createArticleIndexes,
		createFoldersTable,
	} {
		if _, err := db.Exec(migration); err != nil {
			return nil, fmt.Errorf("running migration: %w", err)
		}
	}

	// ALTER TABLE doesn't support IF NOT EXISTS, so we ignore "duplicate column" errors
	// from redeployments where the column already exists.
	if _, err := db.Exec(addFolderIDToFeeds); err != nil {
		if !strings.Contains(err.Error(), "duplicate column name") {
			return nil, fmt.Errorf("running migration: %w", err)
		}
	}

	// This index depends on folder_id existing — must run after the ALTER TABLE above.
	if _, err := db.Exec(createFeedFolderIndex); err != nil {
		return nil, fmt.Errorf("running migration: %w", err)
	}

	return db, nil
}

// --- Feed (subscription) operations ---

// StoreFeed, StoreFolder, and StoreArticle types are defined in models.go
// so they can be used in tests without the Spin SDK build tags.

// addFeed subscribes to a new feed. It fetches the feed first to get
// its title and description, then inserts it into the database.
// folderID of 0 means no folder (shown under "General").
func addFeed(db *sql.DB, url string, folderID int64) (*StoreFeed, error) {
	// Fetch and parse the feed to get metadata
	feed, err := fetchFeed(url)
	if err != nil {
		return nil, fmt.Errorf("fetching feed: %w", err)
	}

	// Insert the subscription. Use NULL for folder_id when unassigned.
	var folderVal any
	if folderID > 0 {
		folderVal = folderID
	}
	_, err = db.Exec(
		`INSERT INTO feeds (url, title, description, site_link, last_fetched_at, folder_id)
		 VALUES (?, ?, ?, ?, datetime('now'), ?)`,
		url, feed.Title, feed.Description, feed.Link, folderVal,
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
		COALESCE(folder_id, 0), COALESCE(last_fetched_at, ''), created_at
		FROM feeds WHERE url = ?`, url)
	return scanFeed(row)
}

func getFeed(db *sql.DB, id int64) (*StoreFeed, error) {
	row := db.QueryRow(`SELECT id, url, title, description, site_link,
		COALESCE(folder_id, 0), COALESCE(last_fetched_at, ''), created_at
		FROM feeds WHERE id = ?`, id)
	return scanFeed(row)
}

// listFeeds returns all subscribed feeds, ordered by folder then subscription date.
func listFeeds(db *sql.DB) ([]StoreFeed, error) {
	rows, err := db.Query(`SELECT id, url, title, description, site_link,
		COALESCE(folder_id, 0), COALESCE(last_fetched_at, ''), created_at
		FROM feeds ORDER BY COALESCE(folder_id, 0), created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("listing feeds: %w", err)
	}
	defer rows.Close()

	var feeds []StoreFeed
	for rows.Next() {
		var f StoreFeed
		if err := rows.Scan(&f.ID, &f.URL, &f.Title, &f.Description,
			&f.SiteLink, &f.FolderID, &f.LastFetchedAt, &f.CreatedAt); err != nil {
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
		&f.SiteLink, &f.FolderID, &f.LastFetchedAt, &f.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("scanning feed: %w", err)
	}
	return &f, nil
}

// --- Folder operations ---

// listFolders returns all folders ordered by name.
func listFolders(db *sql.DB) ([]StoreFolder, error) {
	rows, err := db.Query(`SELECT id, name, created_at FROM folders ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("listing folders: %w", err)
	}
	defer rows.Close()

	var folders []StoreFolder
	for rows.Next() {
		var f StoreFolder
		if err := rows.Scan(&f.ID, &f.Name, &f.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning folder: %w", err)
		}
		folders = append(folders, f)
	}
	return folders, nil
}

// getOrCreateFolder returns the ID of a folder by name, creating it if needed.
func getOrCreateFolder(db *sql.DB, name string) (int64, error) {
	// Try to find existing folder first
	var id int64
	err := db.QueryRow(`SELECT id FROM folders WHERE name = ?`, name).Scan(&id)
	if err == nil {
		return id, nil
	}

	// Insert new folder
	_, err = db.Exec(`INSERT INTO folders (name) VALUES (?)`, name)
	if err != nil {
		return 0, fmt.Errorf("creating folder: %w", err)
	}

	// Query back to get the id (Spin SQLite doesn't support LastInsertId)
	err = db.QueryRow(`SELECT id FROM folders WHERE name = ?`, name).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("querying new folder: %w", err)
	}
	return id, nil
}

// deleteFolder removes a folder. Feeds in the folder get folder_id = NULL
// (shown under "General") via the ON DELETE SET NULL foreign key constraint.
func deleteFolder(db *sql.DB, id int64) error {
	_, err := db.Exec(`DELETE FROM folders WHERE id = ?`, id)
	return err
}

// moveFeedToFolder assigns a feed to a folder. folderID of 0 clears the folder.
func moveFeedToFolder(db *sql.DB, feedID, folderID int64) error {
	var folderVal any
	if folderID > 0 {
		folderVal = folderID
	}
	_, err := db.Exec(`UPDATE feeds SET folder_id = ? WHERE id = ?`, folderVal, feedID)
	return err
}

// --- Article operations ---

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

// listArticlesByFolder returns all articles for feeds in a given folder,
// ordered by published date descending.
func listArticlesByFolder(db *sql.DB, folderID int64, limit, offset int) ([]StoreArticle, error) {
	rows, err := db.Query(`
		SELECT a.id, a.feed_id, a.guid, a.title, a.link, a.description,
		       COALESCE(a.published_at, ''), a.fetched_at, a.is_read
		FROM articles a
		JOIN feeds f ON a.feed_id = f.id
		WHERE f.folder_id = ?
		ORDER BY a.published_at DESC, a.fetched_at DESC, a.id DESC
		LIMIT ? OFFSET ?`, folderID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("listing articles by folder: %w", err)
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
		a.IsRead = isReadInt == 1
		articles = append(articles, a)
	}
	return articles, nil
}

// listFeedsByFolder returns all feeds belonging to a folder.
func listFeedsByFolder(db *sql.DB, folderID int64) ([]StoreFeed, error) {
	rows, err := db.Query(`SELECT id, url, title, description, site_link,
		COALESCE(folder_id, 0), COALESCE(last_fetched_at, ''), created_at
		FROM feeds WHERE folder_id = ? ORDER BY created_at DESC`, folderID)
	if err != nil {
		return nil, fmt.Errorf("listing feeds by folder: %w", err)
	}
	defer rows.Close()

	var feeds []StoreFeed
	for rows.Next() {
		var f StoreFeed
		if err := rows.Scan(&f.ID, &f.URL, &f.Title, &f.Description,
			&f.SiteLink, &f.FolderID, &f.LastFetchedAt, &f.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning feed: %w", err)
		}
		feeds = append(feeds, f)
	}
	return feeds, nil
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

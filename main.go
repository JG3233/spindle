//go:build tinygo || wasip1

package main

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	spinhttp "github.com/spinframework/spin-go-sdk/v2/http"
)

func init() {
	spinhttp.Handle(router)
}

func router(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimRight(r.URL.Path, "/")
	method := r.Method

	// --- Public endpoints (no auth required) ---
	switch {
	case method == http.MethodGet && path == "/api/health":
		healthHandler(w, r)
		return
	case method == http.MethodGet && path == "/api/login":
		loginPageHandler(w, r)
		return
	case method == http.MethodPost && path == "/api/login":
		loginHandler(w, r)
		return
	case method == http.MethodPost && path == "/api/logout":
		logoutHandler(w, r)
		return
	}

	// All other routes require authentication
	if !requireAuth(w, r) {
		return
	}

	switch {
	// --- UI (HTMX) endpoints ---
	// These return HTML fragments for HTMX to swap into the page.
	// Must be matched before the JSON API routes since they share /api/feeds prefix.
	case method == http.MethodGet && path == "/api/ui/feeds":
		uiFeedsListHandler(w, r)
	case method == http.MethodPost && path == "/api/ui/feeds":
		uiCreateFeedHandler(w, r)
	case method == http.MethodPost && path == "/api/ui/feeds/refresh-all":
		uiRefreshAllHandler(w, r)
	case method == http.MethodPost && strings.HasSuffix(path, "/refresh") && strings.HasPrefix(path, "/api/ui/feeds/"):
		uiRefreshFeedHandler(w, r, path)
	case method == http.MethodDelete && strings.HasPrefix(path, "/api/ui/feeds/"):
		uiDeleteFeedHandler(w, r, path)
	case method == http.MethodGet && path == "/api/ui/articles":
		uiArticlesHandler(w, r)
	case method == http.MethodPost && path == "/api/ui/articles/mark-all-read":
		uiMarkAllReadHandler(w, r)
	case method == http.MethodPost && strings.HasSuffix(path, "/toggle-read") && strings.HasPrefix(path, "/api/ui/articles/"):
		uiToggleReadHandler(w, r, path)

	// --- JSON API endpoints ---

	// Feed preview
	case method == http.MethodGet && path == "/api/feeds/preview":
		previewHandler(w, r)

	// Feed refresh
	case method == http.MethodPost && path == "/api/feeds/refresh-all":
		refreshAllHandler(w, r)
	case method == http.MethodPost && strings.HasSuffix(path, "/refresh") && strings.HasPrefix(path, "/api/feeds/"):
		refreshFeedHandler(w, r, path)

	// Feed subscription CRUD
	case method == http.MethodPost && path == "/api/feeds":
		createFeedHandler(w, r)
	case method == http.MethodGet && path == "/api/feeds":
		listFeedsHandler(w, r)
	case method == http.MethodGet && strings.HasPrefix(path, "/api/feeds/"):
		getFeedHandler(w, r, path)
	case method == http.MethodDelete && strings.HasPrefix(path, "/api/feeds/"):
		deleteFeedHandler(w, r, path)

	// Article operations
	case method == http.MethodGet && path == "/api/articles":
		listArticlesHandler(w, r)
	case method == http.MethodPost && path == "/api/articles/mark-all-read":
		markAllReadHandler(w, r)
	case method == http.MethodGet && strings.HasPrefix(path, "/api/articles/"):
		getArticleHandler(w, r, path)
	case method == http.MethodPatch && strings.HasPrefix(path, "/api/articles/"):
		updateArticleHandler(w, r, path)

	default:
		http.NotFound(w, r)
	}
}

// --- Handlers ---

func healthHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func previewHandler(w http.ResponseWriter, r *http.Request) {
	url := r.URL.Query().Get("url")
	if url == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "url query parameter is required",
		})
		return
	}

	feed, err := fetchFeed(url)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to fetch feed"})
		return
	}

	writeJSON(w, http.StatusOK, feed)
}

func createFeedHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.URL == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "request body must contain a 'url' field",
		})
		return
	}

	db, err := openDB()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	feed, err := addFeed(db, req.URL)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			writeJSON(w, http.StatusConflict, map[string]string{
				"error": "already subscribed to this feed",
			})
			return
		}
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to fetch feed"})
		return
	}

	writeJSON(w, http.StatusCreated, feed)
}

func listFeedsHandler(w http.ResponseWriter, r *http.Request) {
	db, err := openDB()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	feeds, err := listFeeds(db)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	if feeds == nil {
		feeds = []StoreFeed{}
	}
	writeJSON(w, http.StatusOK, feeds)
}

func getFeedHandler(w http.ResponseWriter, r *http.Request, path string) {
	id := parseFeedID(path)
	if id == -1 {
		http.NotFound(w, r)
		return
	}

	db, err := openDB()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	feed, err := getFeed(db, id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "feed not found"})
		return
	}

	writeJSON(w, http.StatusOK, feed)
}

func deleteFeedHandler(w http.ResponseWriter, r *http.Request, path string) {
	id := parseFeedID(path)
	if id == -1 {
		http.NotFound(w, r)
		return
	}

	db, err := openDB()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	if err := deleteFeed(db, id); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// refreshFeedHandler refreshes a single feed.
// POST /api/feeds/:id/refresh
func refreshFeedHandler(w http.ResponseWriter, r *http.Request, path string) {
	id := parseFeedIDFromRefresh(path)
	if id == -1 {
		http.NotFound(w, r)
		return
	}

	db, err := openDB()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	feed, err := getFeed(db, id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "feed not found"})
		return
	}

	added, err := refreshFeed(db, feed)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "failed to fetch feed"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"feed_id":        feed.ID,
		"articles_added": added,
	})
}

// refreshAllHandler refreshes all subscribed feeds.
// POST /api/feeds/refresh-all
func refreshAllHandler(w http.ResponseWriter, r *http.Request) {
	db, err := openDB()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	feeds, err := listFeeds(db)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	// Refresh each feed sequentially. Remember: no goroutines in WASM.
	// In a traditional Go server you'd fan out with goroutines.
	// Here, it's one feed at a time.
	totalAdded := 0
	for i := range feeds {
		added, err := refreshFeed(db, &feeds[i])
		if err != nil {
			// Log but continue — one failing feed shouldn't block the rest
			continue
		}
		totalAdded += added
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"feeds_refreshed": len(feeds),
		"articles_added":  totalAdded,
	})
}

// listArticlesHandler returns articles with optional filters.
// GET /api/articles?feed_id=1&is_read=0&limit=50&offset=0
func listArticlesHandler(w http.ResponseWriter, r *http.Request) {
	db, err := openDB()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	// Parse query parameters with defaults.
	// strconv.Atoi is like Python's int() but returns an error instead of raising.
	q := r.URL.Query()
	feedID, _ := strconv.ParseInt(q.Get("feed_id"), 10, 64) // 0 if missing = all feeds
	isRead := -1                                              // -1 = all
	if q.Get("is_read") != "" {
		isRead, _ = strconv.Atoi(q.Get("is_read"))
	}
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 {
		limit = 50
	}
	offset, _ := strconv.Atoi(q.Get("offset"))

	articles, err := listArticles(db, feedID, isRead, limit, offset)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	if articles == nil {
		articles = []StoreArticle{}
	}
	writeJSON(w, http.StatusOK, articles)
}

// getArticleHandler returns a single article by ID.
// GET /api/articles/:id
func getArticleHandler(w http.ResponseWriter, r *http.Request, path string) {
	id := parseArticleID(path)
	if id == -1 {
		http.NotFound(w, r)
		return
	}

	db, err := openDB()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	article, err := getArticle(db, id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "article not found"})
		return
	}

	writeJSON(w, http.StatusOK, article)
}

// updateArticleHandler marks an article as read or unread.
// PATCH /api/articles/:id with JSON body: {"is_read": true}
func updateArticleHandler(w http.ResponseWriter, r *http.Request, path string) {
	id := parseArticleID(path)
	if id == -1 {
		http.NotFound(w, r)
		return
	}

	var req struct {
		IsRead bool `json:"is_read"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	db, err := openDB()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	if err := updateArticleRead(db, id, req.IsRead); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	article, err := getArticle(db, id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "article not found"})
		return
	}

	writeJSON(w, http.StatusOK, article)
}

// markAllReadHandler marks all articles as read, optionally filtered by feed.
// POST /api/articles/mark-all-read?feed_id=1
func markAllReadHandler(w http.ResponseWriter, r *http.Request) {
	db, err := openDB()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	feedID, _ := strconv.ParseInt(r.URL.Query().Get("feed_id"), 10, 64)

	if err := markAllRead(db, feedID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func main() {}

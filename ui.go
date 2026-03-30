//go:build tinygo || wasip1

package main

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// --- HTMX fragment endpoints ---
//
// These return HTML fragments, not JSON. HTMX swaps these fragments
// directly into the page. This is "hypermedia-driven" development:
// the server decides what the UI looks like, the browser just displays it.
//
// Each handler writes HTML using fmt.Fprintf. In a larger app you'd use
// Go's html/template package, but for learning, raw string formatting
// makes the HTML generation visible and obvious.

// uiFeedsListHandler returns the feed sidebar HTML.
// GET /api/ui/feeds
func uiFeedsListHandler(w http.ResponseWriter, r *http.Request) {
	db, err := openDB()
	if err != nil {
		writeHTML(w, http.StatusInternalServerError, `<p class="error">Database error</p>`)
		return
	}

	feeds, err := listFeeds(db)
	if err != nil {
		writeHTML(w, http.StatusInternalServerError, `<p class="error">Failed to load feeds</p>`)
		return
	}

	var b strings.Builder

	// "All Articles" link at top
	b.WriteString(`<div class="feed-item all-feeds-item" `)
	b.WriteString(`hx-get="/api/ui/articles" hx-target="#article-list" hx-swap="innerHTML" `)
	b.WriteString(`onclick="document.getElementById('content-title').textContent='All Articles'">`)
	b.WriteString(`<span class="feed-title">All Articles</span>`)
	b.WriteString(`<span class="feed-actions">` +
		`<button class="btn-icon" hx-post="/api/ui/feeds/refresh-all" ` +
		`hx-target="#article-list" hx-swap="innerHTML" ` +
		`hx-disabled-elt="this" ` +
		`onclick="event.stopPropagation()" title="Refresh all">&#8635;</button>` +
		`</span>`)
	b.WriteString(`</div>`)

	if len(feeds) == 0 {
		b.WriteString(`<p class="empty">No feeds yet. Add one above!</p>`)
	}

	for _, f := range feeds {
		fmt.Fprintf(&b, `<div class="feed-item" `+
			`hx-get="/api/ui/articles?feed_id=%d" hx-target="#article-list" hx-swap="innerHTML" `+
			`onclick="document.getElementById('content-title').textContent='%s'">`,
			f.ID, escapeHTML(f.Title))
		fmt.Fprintf(&b, `<span class="feed-title">%s</span>`, escapeHTML(f.Title))
		fmt.Fprintf(&b, `<span class="feed-actions">`+
			`<button class="btn-icon" hx-post="/api/ui/feeds/%d/refresh" `+
			`hx-target="#article-list" hx-swap="innerHTML" `+
			`onclick="event.stopPropagation()" title="Refresh">&#8635;</button>`+
			`<button class="btn-icon" hx-delete="/api/ui/feeds/%d" `+
			`hx-target="#feed-list" hx-swap="innerHTML" `+
			`hx-confirm="Unsubscribe from %s?" `+
			`onclick="event.stopPropagation()" title="Unsubscribe">&#10005;</button>`+
			`</span>`, f.ID, f.ID, escapeHTML(f.Title))
		b.WriteString(`</div>`)
	}

	writeHTML(w, http.StatusOK, b.String())
}

// uiCreateFeedHandler subscribes and returns updated feed list.
// POST /api/ui/feeds (form-encoded — HTMX sends forms naturally)
func uiCreateFeedHandler(w http.ResponseWriter, r *http.Request) {
	url := r.FormValue("url")
	if url == "" {
		writeHTML(w, http.StatusBadRequest, `<p class="error">URL is required</p>`)
		return
	}

	db, err := openDB()
	if err != nil {
		writeHTML(w, http.StatusInternalServerError, `<p class="error">Database error</p>`)
		return
	}

	_, err = addFeed(db, url)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			// Still return the feed list so the UI isn't broken
			uiFeedsListHandler(w, r)
			return
		}
		writeHTML(w, http.StatusBadGateway, `<p class="error">Failed to fetch feed</p>`)
		return
	}

	// Return the updated feed list
	uiFeedsListHandler(w, r)
}

// uiDeleteFeedHandler unsubscribes and returns updated feed list.
// DELETE /api/ui/feeds/:id
func uiDeleteFeedHandler(w http.ResponseWriter, r *http.Request, path string) {
	idStr := strings.TrimPrefix(path, "/api/ui/feeds/")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	db, err := openDB()
	if err != nil {
		writeHTML(w, http.StatusInternalServerError, `<p class="error">Database error</p>`)
		return
	}

	deleteFeed(db, id)
	uiFeedsListHandler(w, r)
}

// uiRefreshFeedHandler refreshes one feed and returns its articles.
// POST /api/ui/feeds/:id/refresh
func uiRefreshFeedHandler(w http.ResponseWriter, r *http.Request, path string) {
	trimmed := strings.TrimPrefix(path, "/api/ui/feeds/")
	trimmed = strings.TrimSuffix(trimmed, "/refresh")
	id, err := strconv.ParseInt(trimmed, 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	db, err := openDB()
	if err != nil {
		writeHTML(w, http.StatusInternalServerError, `<p class="error">Database error</p>`)
		return
	}

	feed, err := getFeed(db, id)
	if err != nil {
		writeHTML(w, http.StatusNotFound, `<p class="error">Feed not found</p>`)
		return
	}

	refreshFeed(db, feed)
	renderArticleList(w, db, id)
}

// uiRefreshAllHandler refreshes all feeds and returns all articles.
// POST /api/ui/feeds/refresh-all
func uiRefreshAllHandler(w http.ResponseWriter, r *http.Request) {
	db, err := openDB()
	if err != nil {
		writeHTML(w, http.StatusInternalServerError, `<p class="error">Database error</p>`)
		return
	}

	feeds, err := listFeeds(db)
	if err != nil {
		writeHTML(w, http.StatusInternalServerError, `<p class="error">Failed to load feeds</p>`)
		return
	}

	totalNew := 0
	for i := range feeds {
		n, _ := refreshFeed(db, &feeds[i])
		totalNew += n
	}

	articles, err := listArticles(db, 0, -1, 100, 0)
	if err != nil {
		writeHTML(w, http.StatusInternalServerError, `<p class="error">Failed to load articles</p>`)
		return
	}

	// Send the toast message via HX-Trigger-After-Swap so the client fires
	// a "refreshDone" event after the article list is swapped in. The JS
	// listener in index.html updates #refresh-toast and restarts its animation.
	var msg string
	if totalNew == 1 {
		msg = "1 new article"
	} else if totalNew > 1 {
		msg = fmt.Sprintf("%d new articles", totalNew)
	} else {
		msg = "Already up to date"
	}
	w.Header().Set("HX-Trigger-After-Swap", fmt.Sprintf(`{"refreshDone":"%s"}`, msg))

	var b strings.Builder
	if len(articles) == 0 {
		b.WriteString(`<p class="empty">No articles yet. Subscribe to a feed and refresh!</p>`)
	} else {
		for i := range articles {
			b.WriteString(renderOneArticle(&articles[i]))
		}
	}

	writeHTML(w, http.StatusOK, b.String())
}

// uiArticlesHandler returns the article list HTML.
// GET /api/ui/articles?feed_id=1
func uiArticlesHandler(w http.ResponseWriter, r *http.Request) {
	db, err := openDB()
	if err != nil {
		writeHTML(w, http.StatusInternalServerError, `<p class="error">Database error</p>`)
		return
	}

	feedID, _ := strconv.ParseInt(r.URL.Query().Get("feed_id"), 10, 64)
	renderArticleList(w, db, feedID)
}

// uiToggleReadHandler toggles read/unread and returns the updated article.
// POST /api/ui/articles/:id/toggle-read
func uiToggleReadHandler(w http.ResponseWriter, r *http.Request, path string) {
	trimmed := strings.TrimPrefix(path, "/api/ui/articles/")
	trimmed = strings.TrimSuffix(trimmed, "/toggle-read")
	id, err := strconv.ParseInt(trimmed, 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	db, err := openDB()
	if err != nil {
		writeHTML(w, http.StatusInternalServerError, `<p class="error">Database error</p>`)
		return
	}

	article, err := getArticle(db, id)
	if err != nil {
		writeHTML(w, http.StatusNotFound, `<p class="error">Article not found</p>`)
		return
	}

	updateArticleRead(db, id, !article.IsRead)
	article, _ = getArticle(db, id)
	writeHTML(w, http.StatusOK, renderOneArticle(article))
}

// uiMarkAllReadHandler marks all articles read and returns updated list.
// POST /api/ui/articles/mark-all-read?feed_id=1
func uiMarkAllReadHandler(w http.ResponseWriter, r *http.Request) {
	db, err := openDB()
	if err != nil {
		writeHTML(w, http.StatusInternalServerError, `<p class="error">Database error</p>`)
		return
	}

	feedID, _ := strconv.ParseInt(r.URL.Query().Get("feed_id"), 10, 64)
	markAllRead(db, feedID)
	renderArticleList(w, db, feedID)
}

// --- HTML rendering helpers ---

func renderArticleList(w http.ResponseWriter, db *sql.DB, feedID int64) {
	articles, err := listArticles(db, feedID, -1, 100, 0)
	if err != nil {
		writeHTML(w, http.StatusInternalServerError, `<p class="error">Failed to load articles</p>`)
		return
	}

	if len(articles) == 0 {
		writeHTML(w, http.StatusOK, `<p class="empty">No articles yet. Subscribe to a feed and refresh!</p>`)
		return
	}

	var b strings.Builder
	for i := range articles {
		b.WriteString(renderOneArticle(&articles[i]))
	}
	writeHTML(w, http.StatusOK, b.String())
}

func renderOneArticle(a *StoreArticle) string {
	var b strings.Builder

	readClass := ""
	if a.IsRead {
		readClass = " read"
	}
	btnLabel := "Mark read"
	if a.IsRead {
		btnLabel = "Mark unread"
	}

	fmt.Fprintf(&b, `<div class="article-item%s" id="article-%d">`, readClass, a.ID)
	fmt.Fprintf(&b, `<div class="article-title"><a href="%s" target="_blank">%s</a></div>`,
		escapeHTML(a.Link), escapeHTML(a.Title))
	if a.PublishedAt != "" {
		fmt.Fprintf(&b, `<div class="article-meta"><span>%s</span></div>`, escapeHTML(a.PublishedAt))
	}
	if a.Description != "" {
		fmt.Fprintf(&b, `<div class="article-description">%s</div>`, escapeHTML(a.Description))
	}
	fmt.Fprintf(&b, `<div class="article-actions">`+
		`<button class="btn-small" hx-post="/api/ui/articles/%d/toggle-read" `+
		`hx-target="#article-%d" hx-swap="outerHTML">%s</button></div>`,
		a.ID, a.ID, btnLabel)
	b.WriteString(`</div>`)

	return b.String()
}

// uiAuthStatusHandler returns the logout button if auth is enabled, empty otherwise.
// GET /api/ui/auth-status
func uiAuthStatusHandler(w http.ResponseWriter, r *http.Request) {
	if getConfiguredPassword() == "" {
		writeHTML(w, http.StatusOK, "")
		return
	}
	writeHTML(w, http.StatusOK, `<form method="POST" action="/api/logout" class="logout-form"><button type="submit" class="btn-secondary">Logout</button></form>`)
}

func writeHTML(w http.ResponseWriter, status int, html string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	fmt.Fprint(w, html)
}


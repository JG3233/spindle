package main

import (
	"fmt"
	"strings"
)

// render.go contains pure HTML rendering functions that take plain data types.
// No build tag — these can be tested with standard `go test`.

// renderFeedList builds the sidebar HTML from feeds and folders.
// Feeds with no folder (FolderID == 0) appear under "General". Named folders follow.
func renderFeedList(feeds []StoreFeed, folders []StoreFolder) string {
	var b strings.Builder

	// "All Articles" link at top
	b.WriteString(`<div class="feed-item all-feeds-item" `)
	b.WriteString(`hx-get="/api/ui/articles" hx-target="#article-list" hx-swap="innerHTML" `)
	b.WriteString(`onclick="document.getElementById('content-title').textContent='All Articles'">`)
	b.WriteString(`<span class="feed-title">All Articles</span>`)
	b.WriteString(`<span class="feed-actions">` +
		`<button class="btn-icon" ` +
		`onclick="event.stopPropagation(); refreshAllFeeds(this)" ` +
		`title="Refresh all">&#8635;</button>` +
		`</span>`)
	b.WriteString(`</div>`)

	if len(feeds) == 0 {
		b.WriteString(`<p class="empty">No feeds yet. Add one above!</p>`)
		return b.String()
	}

	// Separate feeds into General (folder_id=0) and by folder
	var generalFeeds []StoreFeed
	folderFeeds := make(map[int64][]StoreFeed)
	for _, f := range feeds {
		if f.FolderID == 0 {
			generalFeeds = append(generalFeeds, f)
		} else {
			folderFeeds[f.FolderID] = append(folderFeeds[f.FolderID], f)
		}
	}

	// Render General section
	b.WriteString(`<div class="folder-section">`)
	b.WriteString(`<div class="folder-header"><span class="folder-name">General</span></div>`)
	for _, f := range generalFeeds {
		b.WriteString(renderFeedItem(f, folders))
	}
	b.WriteString(`</div>`)

	// Render named folder sections
	for _, folder := range folders {
		ffeeds := folderFeeds[folder.ID]
		fmt.Fprintf(&b, `<div class="folder-section">`)
		fmt.Fprintf(&b, `<div class="folder-header">`+
			`<span class="folder-name folder-link" `+
			`hx-get="/api/ui/articles?folder_id=%d" `+
			`hx-target="#article-list" hx-swap="innerHTML" `+
			`onclick="document.getElementById('content-title').textContent='%s'">%s</span>`+
			`<span class="folder-header-actions">`+
			`<button class="btn-icon" `+
			`hx-post="/api/ui/folders/%d/refresh" `+
			`hx-target="#article-list" hx-swap="innerHTML" `+
			`hx-disabled-elt="this" `+
			`title="Refresh folder">&#8635;</button>`+
			`<button class="btn-icon folder-delete" `+
			`hx-delete="/api/ui/folders/%d" `+
			`hx-target="#feed-list" hx-swap="innerHTML" `+
			`hx-confirm="Delete folder %s? Feeds will move to General." `+
			`title="Delete folder">&#10005;</button>`+
			`</span>`+
			`</div>`,
			folder.ID, escapeHTML(folder.Name), escapeHTML(folder.Name),
			folder.ID, folder.ID, escapeHTML(folder.Name))
		for _, f := range ffeeds {
			b.WriteString(renderFeedItem(f, folders))
		}
		b.WriteString(`</div>`)
	}

	return b.String()
}

// renderFeedItem renders a single feed row, including the folder-move select.
func renderFeedItem(f StoreFeed, folders []StoreFolder) string {
	var b strings.Builder

	fmt.Fprintf(&b, `<div class="feed-item" data-feed-id="%d" `+
		`hx-get="/api/ui/articles?feed_id=%d" hx-target="#article-list" hx-swap="innerHTML" `+
		`onclick="document.getElementById('content-title').textContent='%s'">`,
		f.ID, f.ID, escapeHTML(f.Title))
	fmt.Fprintf(&b, `<span class="feed-title">%s</span>`, escapeHTML(f.Title))

	// Build folder select options
	var sel strings.Builder
	fmt.Fprintf(&sel, `<select class="folder-select" `+
		`hx-post="/api/ui/feeds/%d/folder" `+
		`hx-target="#feed-list" hx-swap="innerHTML" `+
		`hx-trigger="change" `+
		`name="folder_id" `+
		`onclick="event.stopPropagation()" `+
		`title="Move to folder">`, f.ID)
	// "General" option (value 0)
	selected := ""
	if f.FolderID == 0 {
		selected = ` selected`
	}
	fmt.Fprintf(&sel, `<option value="0"%s>General</option>`, selected)
	for _, folder := range folders {
		selected = ""
		if f.FolderID == folder.ID {
			selected = ` selected`
		}
		fmt.Fprintf(&sel, `<option value="%d"%s>%s</option>`,
			folder.ID, selected, escapeHTML(folder.Name))
	}
	sel.WriteString(`</select>`)

	fmt.Fprintf(&b, `<span class="feed-actions">`+
		`<button class="btn-icon" hx-post="/api/ui/feeds/%d/refresh" `+
		`hx-target="#article-list" hx-swap="innerHTML" `+
		`hx-disabled-elt="this" `+
		`onclick="event.stopPropagation()" title="Refresh">&#8635;</button>`+
		`%s`+
		`<button class="btn-icon" hx-delete="/api/ui/feeds/%d" `+
		`hx-target="#feed-list" hx-swap="innerHTML" `+
		`hx-confirm="Unsubscribe from %s?" `+
		`onclick="event.stopPropagation()" title="Unsubscribe">&#10005;</button>`+
		`</span>`, f.ID, sel.String(), f.ID, escapeHTML(f.Title))
	b.WriteString(`</div>`)

	return b.String()
}

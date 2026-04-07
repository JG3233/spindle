package main

import "testing"

// --- Folder path helpers ---

func TestParseFolderID(t *testing.T) {
	tests := []struct {
		name string
		path string
		want int64
	}{
		{"valid", "/api/ui/folders/5", 5},
		{"single digit", "/api/ui/folders/1", 1},
		{"large id", "/api/ui/folders/999", 999},
		{"non-numeric", "/api/ui/folders/abc", -1},
		{"empty id", "/api/ui/folders/", -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseFolderID(tt.path)
			if got != tt.want {
				t.Errorf("parseFolderID(%q) = %d, want %d", tt.path, got, tt.want)
			}
		})
	}
}

func TestParseFeedIDFromUIFolder(t *testing.T) {
	tests := []struct {
		name string
		path string
		want int64
	}{
		{"valid", "/api/ui/feeds/42/folder", 42},
		{"single digit", "/api/ui/feeds/1/folder", 1},
		{"non-numeric", "/api/ui/feeds/abc/folder", -1},
		{"empty id", "/api/ui/feeds//folder", -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseFeedIDFromUIFolder(tt.path)
			if got != tt.want {
				t.Errorf("parseFeedIDFromUIFolder(%q) = %d, want %d", tt.path, got, tt.want)
			}
		})
	}
}

func TestParseFolderIDFromRefresh(t *testing.T) {
	tests := []struct {
		name string
		path string
		want int64
	}{
		{"valid", "/api/ui/folders/5/refresh", 5},
		{"single digit", "/api/ui/folders/1/refresh", 1},
		{"non-numeric", "/api/ui/folders/abc/refresh", -1},
		{"empty id", "/api/ui/folders//refresh", -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseFolderIDFromRefresh(tt.path)
			if got != tt.want {
				t.Errorf("parseFolderIDFromRefresh(%q) = %d, want %d", tt.path, got, tt.want)
			}
		})
	}
}

// --- renderFeedList ---

func TestRenderFeedList_Empty(t *testing.T) {
	html := renderFeedList(nil, nil)
	if html == "" {
		t.Error("expected non-empty HTML for empty feed list")
	}
	if !containsStr(html, "All Articles") {
		t.Error("expected 'All Articles' link at top")
	}
	if !containsStr(html, "No feeds yet") {
		t.Error("expected empty state message")
	}
}

func TestRenderFeedList_GeneralOnly(t *testing.T) {
	feeds := []StoreFeed{
		{ID: 1, Title: "Tech Blog", FolderID: 0},
		{ID: 2, Title: "News Site", FolderID: 0},
	}
	html := renderFeedList(feeds, nil)

	if !containsStr(html, "General") {
		t.Error("expected General section header")
	}
	if !containsStr(html, "Tech Blog") {
		t.Error("expected feed title in output")
	}
	if !containsStr(html, "News Site") {
		t.Error("expected second feed title in output")
	}
	if !containsStr(html, `data-feed-id="1"`) {
		t.Error("expected data-feed-id attribute")
	}
}

func TestRenderFeedList_WithFolders(t *testing.T) {
	folders := []StoreFolder{
		{ID: 10, Name: "Tech"},
		{ID: 20, Name: "News"},
	}
	feeds := []StoreFeed{
		{ID: 1, Title: "General Feed", FolderID: 0},
		{ID: 2, Title: "Tech Blog", FolderID: 10},
		{ID: 3, Title: "BBC", FolderID: 20},
	}
	html := renderFeedList(feeds, folders)

	if !containsStr(html, "General") {
		t.Error("expected General section")
	}
	if !containsStr(html, "Tech") {
		t.Error("expected Tech folder section")
	}
	if !containsStr(html, "News") {
		t.Error("expected News folder section")
	}
	if !containsStr(html, "General Feed") {
		t.Error("expected general feed title")
	}
	if !containsStr(html, "Tech Blog") {
		t.Error("expected tech feed title")
	}
	if !containsStr(html, "BBC") {
		t.Error("expected news feed title")
	}
}

func TestRenderFeedList_FolderClickable(t *testing.T) {
	folders := []StoreFolder{{ID: 7, Name: "Science"}}
	feeds := []StoreFeed{{ID: 1, Title: "Nature", FolderID: 7}}
	html := renderFeedList(feeds, folders)

	if !containsStr(html, `/api/ui/articles?folder_id=7`) {
		t.Error("expected folder to link to folder articles endpoint")
	}
	if !containsStr(html, "folder-link") {
		t.Error("expected folder-link CSS class on folder name")
	}
}

func TestRenderFeedList_FolderRefreshButton(t *testing.T) {
	folders := []StoreFolder{{ID: 7, Name: "Science"}}
	feeds := []StoreFeed{{ID: 1, Title: "Nature", FolderID: 7}}
	html := renderFeedList(feeds, folders)

	if !containsStr(html, `refreshFolderFeeds(this, 7)`) {
		t.Error("expected refreshFolderFeeds JS call in folder header refresh button")
	}
}

func TestRenderFeedList_FolderDeleteButton(t *testing.T) {
	folders := []StoreFolder{{ID: 5, Name: "Sports"}}
	feeds := []StoreFeed{{ID: 1, Title: "ESPN", FolderID: 5}}
	html := renderFeedList(feeds, folders)

	if !containsStr(html, `/api/ui/folders/5`) {
		t.Error("expected delete folder endpoint in folder header")
	}
	if !containsStr(html, "folder-delete") {
		t.Error("expected folder-delete CSS class on delete button")
	}
}

func TestRenderFeedList_FolderSelectInFeedItem(t *testing.T) {
	folders := []StoreFolder{{ID: 3, Name: "Science"}}
	feeds := []StoreFeed{{ID: 7, Title: "Nature", FolderID: 3}}
	html := renderFeedList(feeds, folders)

	if !containsStr(html, `/api/ui/feeds/7/folder`) {
		t.Error("expected folder move endpoint in feed item select")
	}
	if !containsStr(html, "folder-select") {
		t.Error("expected folder-select CSS class")
	}
	// The feed is in folder 3, so option value="3" should be selected
	if !containsStr(html, `value="3" selected`) {
		t.Error("expected current folder pre-selected in select")
	}
}

func TestRenderFeedList_GeneralSelectedWhenNoFolder(t *testing.T) {
	folders := []StoreFolder{{ID: 1, Name: "Work"}}
	feeds := []StoreFeed{{ID: 5, Title: "Unfiled Feed", FolderID: 0}}
	html := renderFeedList(feeds, folders)

	if !containsStr(html, `value="0" selected`) {
		t.Error("expected General option pre-selected for unfoldered feed")
	}
}

func TestRenderFeedList_HTMLEscaping(t *testing.T) {
	folders := []StoreFolder{{ID: 1, Name: `<script>alert("xss")</script>`}}
	feeds := []StoreFeed{{ID: 1, Title: `<b>Bold & "Quoted"</b>`, FolderID: 0}}
	html := renderFeedList(feeds, folders)

	if containsStr(html, `<script>`) {
		t.Error("folder name must be HTML-escaped")
	}
	if containsStr(html, `<b>Bold`) {
		t.Error("feed title must be HTML-escaped")
	}
}

// containsStr is a small helper used only in these tests.
func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		}())
}

package main

import (
	"testing"
)

// --- RSS parsing ---

const testRSS = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Test Blog</title>
    <link>https://example.com</link>
    <description>A test feed</description>
    <item>
      <title>First Post</title>
      <link>https://example.com/first</link>
      <guid>guid-1</guid>
      <description>The first post</description>
      <pubDate>Fri, 27 Mar 2026 22:09:00 +0000</pubDate>
    </item>
    <item>
      <title>Second Post</title>
      <link>https://example.com/second</link>
      <guid>guid-2</guid>
      <description>The second post</description>
      <pubDate>Thu, 26 Mar 2026 10:00:00 +0000</pubDate>
    </item>
  </channel>
</rss>`

func TestParseRSS(t *testing.T) {
	feed, err := parseRSS([]byte(testRSS))
	if err != nil {
		t.Fatalf("parseRSS failed: %v", err)
	}

	if feed.Title != "Test Blog" {
		t.Errorf("title = %q, want %q", feed.Title, "Test Blog")
	}
	if feed.Link != "https://example.com" {
		t.Errorf("link = %q, want %q", feed.Link, "https://example.com")
	}
	if feed.Description != "A test feed" {
		t.Errorf("description = %q, want %q", feed.Description, "A test feed")
	}
	if len(feed.Articles) != 2 {
		t.Fatalf("got %d articles, want 2", len(feed.Articles))
	}

	a := feed.Articles[0]
	if a.GUID != "guid-1" {
		t.Errorf("article[0].GUID = %q, want %q", a.GUID, "guid-1")
	}
	if a.Title != "First Post" {
		t.Errorf("article[0].Title = %q, want %q", a.Title, "First Post")
	}
	if a.PublishedAt != "2026-03-27T22:09:00Z" {
		t.Errorf("article[0].PublishedAt = %q, want normalized ISO", a.PublishedAt)
	}
}

func TestParseRSS_MissingGUID(t *testing.T) {
	xml := `<?xml version="1.0"?>
<rss><channel><title>Test</title>
  <item>
    <title>No GUID</title>
    <link>https://example.com/no-guid</link>
  </item>
</channel></rss>`

	feed, err := parseRSS([]byte(xml))
	if err != nil {
		t.Fatalf("parseRSS failed: %v", err)
	}

	if feed.Articles[0].GUID != "https://example.com/no-guid" {
		t.Errorf("expected link as fallback GUID, got %q", feed.Articles[0].GUID)
	}
}

// --- Atom parsing ---

const testAtom = `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <title>Atom Blog</title>
  <link href="https://example.com" rel="alternate"/>
  <link href="https://example.com/feed.xml" rel="self"/>
  <entry>
    <title>Atom Post</title>
    <id>tag:example.com,2026:post-1</id>
    <link href="https://example.com/atom-post" rel="alternate"/>
    <updated>2026-03-27T15:30:00Z</updated>
    <summary>An atom summary</summary>
    <content>Full atom content here</content>
  </entry>
  <entry>
    <title>No Summary</title>
    <id>tag:example.com,2026:post-2</id>
    <link href="https://example.com/no-summary" rel="alternate"/>
    <updated>2026-03-26T12:00:00Z</updated>
    <content>Only content, no summary</content>
  </entry>
</feed>`

func TestParseAtom(t *testing.T) {
	feed, err := parseAtom([]byte(testAtom))
	if err != nil {
		t.Fatalf("parseAtom failed: %v", err)
	}

	if feed.Title != "Atom Blog" {
		t.Errorf("title = %q, want %q", feed.Title, "Atom Blog")
	}
	// Should pick the "alternate" link, not "self"
	if feed.Link != "https://example.com" {
		t.Errorf("link = %q, want %q", feed.Link, "https://example.com")
	}
	if len(feed.Articles) != 2 {
		t.Fatalf("got %d articles, want 2", len(feed.Articles))
	}

	// First entry: has summary, should use summary not content
	a := feed.Articles[0]
	if a.Description != "An atom summary" {
		t.Errorf("article[0].Description = %q, want summary", a.Description)
	}

	// Second entry: no summary, should fall back to content
	a2 := feed.Articles[1]
	if a2.Description != "Only content, no summary" {
		t.Errorf("article[1].Description = %q, want content fallback", a2.Description)
	}
}

// --- parseFeed auto-detection ---

func TestParseFeed_DetectsRSS(t *testing.T) {
	feed, err := parseFeed([]byte(testRSS))
	if err != nil {
		t.Fatalf("parseFeed failed: %v", err)
	}
	if feed.Title != "Test Blog" {
		t.Errorf("expected RSS feed title, got %q", feed.Title)
	}
}

func TestParseFeed_DetectsAtom(t *testing.T) {
	feed, err := parseFeed([]byte(testAtom))
	if err != nil {
		t.Fatalf("parseFeed failed: %v", err)
	}
	if feed.Title != "Atom Blog" {
		t.Errorf("expected Atom feed title, got %q", feed.Title)
	}
}

func TestParseFeed_InvalidXML(t *testing.T) {
	_, err := parseFeed([]byte("not xml at all"))
	if err == nil {
		t.Error("expected error for invalid XML")
	}
}

func TestParseFeed_EmptyFeed(t *testing.T) {
	_, err := parseFeed([]byte(`<html><body>Not a feed</body></html>`))
	if err == nil {
		t.Error("expected error for HTML that isn't a feed")
	}
}

// --- Date normalization ---

func TestNormalizeDate(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"RFC1123Z (typical RSS)", "Fri, 27 Mar 2026 22:09:00 +0000", "2026-03-27T22:09:00Z"},
		{"RFC1123Z with offset", "Fri, 27 Mar 2026 17:09:00 -0500", "2026-03-27T22:09:00Z"},
		{"RFC3339 (Atom)", "2026-03-27T15:30:00Z", "2026-03-27T15:30:00Z"},
		{"RFC3339 with offset", "2026-03-27T10:30:00-05:00", "2026-03-27T15:30:00Z"},
		{"single-digit day", "Mon, 2 Jan 2026 08:00:00 +0000", "2026-01-02T08:00:00Z"},
		{"empty string", "", ""},
		{"unparseable", "not a date", "not a date"},
		{"whitespace", "  2026-03-27T15:30:00Z  ", "2026-03-27T15:30:00Z"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeDate(tt.input)
			if got != tt.want {
				t.Errorf("normalizeDate(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// --- Truncation ---

func TestTruncate(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		maxLen int
		want   string
	}{
		{"under limit", "short", 10, "short"},
		{"exact limit", "exact", 5, "exact"},
		{"over limit", "this is too long", 7, "this is..."},
		{"empty", "", 10, ""},
		{"whitespace trimmed", "  padded  ", 10, "padded"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncate(tt.input, tt.maxLen)
			if got != tt.want {
				t.Errorf("truncate(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
			}
		})
	}
}

// --- atomLink ---

func TestAtomLink(t *testing.T) {
	tests := []struct {
		name  string
		links []atomLinkXML
		want  string
	}{
		{"alternate link", []atomLinkXML{{Href: "https://example.com", Rel: "alternate"}}, "https://example.com"},
		{"empty rel defaults", []atomLinkXML{{Href: "https://example.com", Rel: ""}}, "https://example.com"},
		{"prefers alternate", []atomLinkXML{
			{Href: "https://self.com", Rel: "self"},
			{Href: "https://alt.com", Rel: "alternate"},
		}, "https://alt.com"},
		{"falls back to first", []atomLinkXML{
			{Href: "https://self.com", Rel: "self"},
		}, "https://self.com"},
		{"no links", []atomLinkXML{}, ""},
		{"nil links", nil, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := atomLink(tt.links)
			if got != tt.want {
				t.Errorf("atomLink() = %q, want %q", got, tt.want)
			}
		})
	}
}

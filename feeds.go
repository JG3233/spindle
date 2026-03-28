package main

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// fetchFeed downloads a URL and parses it as an RSS or Atom feed.
//
// This is where the WASM sandbox gets interesting:
//   - http.Get() looks like normal Go, but under the hood the Spin SDK
//     replaced http.DefaultClient with one that routes through WASI.
//   - The request goes: your Go code → Spin SDK → WASM host boundary →
//     Spin runtime → actual internet. If the URL isn't in allowed_outbound_hosts,
//     the Spin runtime rejects it before it ever leaves the machine.
func fetchFeed(url string) (*Feed, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("fetching feed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("feed returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading feed body: %w", err)
	}

	return parseFeed(body)
}

// parseFeed tries RSS first, then Atom. This is a simple heuristic:
// try to unmarshal as RSS — if the channel title is empty, try Atom.
//
// fmt.Errorf("...: %w", err) is Go's error wrapping. The %w verb wraps
// the original error so callers can unwrap it later. Think of it like
// Python's `raise NewError("context") from original_error`.
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

func parseRSS(data []byte) (*Feed, error) {
	var rss rssXML
	if err := xml.Unmarshal(data, &rss); err != nil {
		return nil, err
	}

	// Convert from RSS XML types to our normalized Feed type.
	// This is a common Go pattern: loop + append to build a slice.
	articles := make([]Article, 0, len(rss.Channel.Items))
	for _, item := range rss.Channel.Items {
		guid := item.GUID
		if guid == "" {
			guid = item.Link // Fall back to link as unique ID
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

func parseAtom(data []byte) (*Feed, error) {
	var atom atomXML
	if err := xml.Unmarshal(data, &atom); err != nil {
		return nil, err
	}

	articles := make([]Article, 0, len(atom.Entries))
	for _, entry := range atom.Entries {
		link := atomLink(entry.Links)
		desc := entry.Summary
		if desc == "" {
			desc = entry.Content
		}
		articles = append(articles, Article{
			GUID:        entry.ID,
			Title:       entry.Title,
			Link:        link,
			Description: truncate(desc, 500),
			PublishedAt: normalizeDate(entry.Updated),
		})
	}

	return &Feed{
		Title:       atom.Title,
		Link:        atomLink(atom.Links),
		Description: "",
		Articles:    articles,
	}, nil
}

// atomLink extracts the best URL from Atom's link elements.
// Atom uses <link rel="alternate" href="..."/> instead of just <link>URL</link>.
func atomLink(links []atomLinkXML) string {
	for _, l := range links {
		if l.Rel == "alternate" || l.Rel == "" {
			return l.Href
		}
	}
	if len(links) > 0 {
		return links[0].Href
	}
	return ""
}

// normalizeDate tries to parse various RSS/Atom date formats and returns
// a consistent ISO 8601 string that SQLite can sort correctly.
// If parsing fails, returns the original string as-is.
func normalizeDate(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}

	// Common date formats found in RSS and Atom feeds.
	// Go uses a reference time (Mon Jan 2 15:04:05 MST 2006) as the format template.
	formats := []string{
		time.RFC1123Z,                    // "Mon, 02 Jan 2006 15:04:05 -0700" (most RSS)
		time.RFC1123,                     // "Mon, 02 Jan 2006 15:04:05 MST"
		time.RFC3339,                     // "2006-01-02T15:04:05Z07:00" (Atom)
		"2006-01-02T15:04:05Z",          // Atom without timezone offset
		"2006-01-02 15:04:05",           // Simple datetime
		"Mon, 2 Jan 2006 15:04:05 -0700", // RSS with single-digit day
		"Mon, 2 Jan 2006 15:04:05 MST",
	}

	for _, format := range formats {
		if t, err := time.Parse(format, s); err == nil {
			return t.UTC().Format(time.RFC3339)
		}
	}

	return s // Can't parse — return as-is
}

// truncate cuts a string to maxLen, adding "..." if truncated.
// RSS descriptions can contain entire HTML pages — we just want a preview.
func truncate(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

package main

import "encoding/xml"

// --- XML parsing structs ---
//
// These structs map directly to RSS 2.0 and Atom XML elements.
// The `xml:"..."` tags tell Go's XML decoder which elements to extract.
// This is similar to Python's dataclasses, but the tags drive deserialization.
//
// Example RSS XML:
//   <rss><channel>
//     <title>My Blog</title>
//     <item><title>Post 1</title><link>https://...</link></item>
//   </channel></rss>
//
// Example Atom XML:
//   <feed xmlns="http://www.w3.org/2005/Atom">
//     <title>My Blog</title>
//     <entry><title>Post 1</title><link href="https://..."/></entry>
//   </feed>

// RSS 2.0 format
type rssXML struct {
	XMLName xml.Name      `xml:"rss"`
	Channel rssChannelXML `xml:"channel"`
}

type rssChannelXML struct {
	Title       string       `xml:"title"`
	Link        string       `xml:"link"`
	Description string       `xml:"description"`
	Items       []rssItemXML `xml:"item"`
}

type rssItemXML struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Description string `xml:"description"`
	GUID        string `xml:"guid"`
	PubDate     string `xml:"pubDate"`
}

// Atom format
type atomXML struct {
	XMLName xml.Name       `xml:"feed"`
	Title   string         `xml:"title"`
	Links   []atomLinkXML  `xml:"link"`
	Entries []atomEntryXML `xml:"entry"`
}

type atomEntryXML struct {
	Title   string        `xml:"title"`
	Links   []atomLinkXML `xml:"link"`
	ID      string        `xml:"id"`
	Updated string        `xml:"updated"`
	Summary string        `xml:"summary"`
	Content string        `xml:"content"`
}

type atomLinkXML struct {
	Href string `xml:"href,attr"` // Atom puts URLs in attributes: <link href="..."/>
	Rel  string `xml:"rel,attr"`  // "alternate", "self", etc.
}

// --- Normalized types ---
//
// Both RSS and Atom get converted into these common types.
// This way the rest of the app doesn't care which format the feed uses.

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

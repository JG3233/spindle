package main

import "testing"

// --- Path parsing helpers ---

func TestParseFeedID(t *testing.T) {
	tests := []struct {
		name string
		path string
		want int64
	}{
		{"valid", "/api/feeds/42", 42},
		{"single digit", "/api/feeds/1", 1},
		{"non-numeric", "/api/feeds/abc", -1},
		{"empty id", "/api/feeds/", -1},
		{"nested path", "/api/feeds/42/refresh", -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseFeedID(tt.path)
			if got != tt.want {
				t.Errorf("parseFeedID(%q) = %d, want %d", tt.path, got, tt.want)
			}
		})
	}
}

func TestParseFeedIDFromRefresh(t *testing.T) {
	tests := []struct {
		name string
		path string
		want int64
	}{
		{"valid", "/api/feeds/42/refresh", 42},
		{"single digit", "/api/feeds/1/refresh", 1},
		{"non-numeric", "/api/feeds/abc/refresh", -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseFeedIDFromRefresh(tt.path)
			if got != tt.want {
				t.Errorf("parseFeedIDFromRefresh(%q) = %d, want %d", tt.path, got, tt.want)
			}
		})
	}
}

func TestParseArticleID(t *testing.T) {
	tests := []struct {
		name string
		path string
		want int64
	}{
		{"valid", "/api/articles/7", 7},
		{"non-numeric", "/api/articles/xyz", -1},
		{"empty", "/api/articles/", -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseArticleID(tt.path)
			if got != tt.want {
				t.Errorf("parseArticleID(%q) = %d, want %d", tt.path, got, tt.want)
			}
		})
	}
}

// --- HTML escaping ---

func TestEscapeHTML(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"ampersand", "A & B", "A &amp; B"},
		{"less than", "a < b", "a &lt; b"},
		{"greater than", "a > b", "a &gt; b"},
		{"double quote", `say "hello"`, "say &quot;hello&quot;"},
		{"all together", `<a href="x">&</a>`, "&lt;a href=&quot;x&quot;&gt;&amp;&lt;/a&gt;"},
		{"no escaping needed", "plain text", "plain text"},
		{"empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := escapeHTML(tt.input)
			if got != tt.want {
				t.Errorf("escapeHTML(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

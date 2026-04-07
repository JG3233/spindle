package main

import (
	"strconv"
	"strings"
)

// --- Path helpers ---
//
// These extract IDs from URL paths. They live here (no build tag)
// so they can be tested with standard `go test`.

func parseFeedID(path string) int64 {
	idStr := strings.TrimPrefix(path, "/api/feeds/")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return -1
	}
	return id
}

// parseFeedIDFromRefresh handles "/api/feeds/42/refresh" → 42
func parseFeedIDFromRefresh(path string) int64 {
	trimmed := strings.TrimPrefix(path, "/api/feeds/")
	trimmed = strings.TrimSuffix(trimmed, "/refresh")
	id, err := strconv.ParseInt(trimmed, 10, 64)
	if err != nil {
		return -1
	}
	return id
}

func parseArticleID(path string) int64 {
	idStr := strings.TrimPrefix(path, "/api/articles/")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return -1
	}
	return id
}

// parseFolderID extracts the folder ID from "/api/ui/folders/:id"
func parseFolderID(path string) int64 {
	idStr := strings.TrimPrefix(path, "/api/ui/folders/")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return -1
	}
	return id
}

// parseFeedIDFromUIFolder handles "/api/ui/feeds/42/folder" → 42
func parseFeedIDFromUIFolder(path string) int64 {
	trimmed := strings.TrimPrefix(path, "/api/ui/feeds/")
	trimmed = strings.TrimSuffix(trimmed, "/folder")
	id, err := strconv.ParseInt(trimmed, 10, 64)
	if err != nil {
		return -1
	}
	return id
}

// parseFolderIDFromRefresh handles "/api/ui/folders/5/refresh" → 5
func parseFolderIDFromRefresh(path string) int64 {
	trimmed := strings.TrimPrefix(path, "/api/ui/folders/")
	trimmed = strings.TrimSuffix(trimmed, "/refresh")
	id, err := strconv.ParseInt(trimmed, 10, 64)
	if err != nil {
		return -1
	}
	return id
}

// --- HTML helpers ---

func escapeHTML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, `"`, "&quot;")
	return s
}

#!/usr/bin/env bash
#
# Integration tests for Spindle — builds the app, starts it, hits every
# API endpoint with curl, and verifies responses.
#
# Usage: bash test_integration.sh
#
# Requires: spin CLI, tinygo, go 1.23 on PATH

set -euo pipefail

# --- Setup ---

export PATH="$HOME/.local/tinygo/bin:$(brew --prefix go@1.23)/libexec/bin:$HOME/.local/bin:$PATH"

PORT=3199
BASE="http://127.0.0.1:${PORT}"
PASS=0
FAIL=0

cleanup() {
    if [[ -n "${SPIN_PID:-}" ]]; then
        kill "$SPIN_PID" 2>/dev/null || true
        wait "$SPIN_PID" 2>/dev/null || true
    fi
    # Remove the SQLite data created during tests
    rm -rf .spin/
}
trap cleanup EXIT

# --- Helpers ---

assert_status() {
    local desc="$1" expected="$2" actual="$3"
    if [[ "$actual" == "$expected" ]]; then
        echo "  ✓ $desc"
        ((PASS++))
    else
        echo "  ✗ $desc (expected $expected, got $actual)"
        ((FAIL++))
    fi
}

assert_contains() {
    local desc="$1" body="$2" pattern="$3"
    if echo "$body" | grep -q "$pattern"; then
        echo "  ✓ $desc"
        ((PASS++))
    else
        echo "  ✗ $desc (response missing: $pattern)"
        ((FAIL++))
    fi
}

# Curl wrapper that returns "status body" separated by newline.
# -s: silent, -w: append status code, -o-: output body to stdout
api() {
    local method="$1" path="$2"
    shift 2
    local response
    response=$(curl -s -w "\n%{http_code}" -X "$method" "${BASE}${path}" "$@")
    echo "$response"
}

get_status() { echo "$1" | tail -n1; }
get_body()   { echo "$1" | sed '$d'; }

# --- Build ---

echo "Building..."
spin build 2>&1 | tail -1

# --- Start ---

echo "Starting spin on port ${PORT}..."
spin up --listen "127.0.0.1:${PORT}" &
SPIN_PID=$!

# Wait for the server to be ready
for i in $(seq 1 20); do
    if curl -s -o /dev/null "${BASE}/api/health" 2>/dev/null; then
        break
    fi
    sleep 0.5
done

# Verify it's actually up
if ! curl -s -o /dev/null "${BASE}/api/health"; then
    echo "ERROR: spin did not start within 10 seconds"
    exit 1
fi

echo "Server ready."
echo ""

# --- Tests ---

echo "=== Health ==="
resp=$(api GET /api/health)
assert_status "GET /api/health → 200" "200" "$(get_status "$resp")"
assert_contains "response has status ok" "$(get_body "$resp")" '"status"'

echo ""
echo "=== Subscribe to a feed ==="
# Use a well-known, stable RSS feed for testing
FEED_URL="https://www.reddit.com/r/technology/.rss"
resp=$(api POST /api/feeds -H "Content-Type: application/json" -d "{\"url\": \"${FEED_URL}\"}")
status=$(get_status "$resp")
body=$(get_body "$resp")
assert_status "POST /api/feeds → 201" "201" "$status"
assert_contains "response has feed id" "$body" '"id"'
assert_contains "response has url" "$body" '"url"'

# Extract the feed ID for later use
FEED_ID=$(echo "$body" | grep -o '"id":[0-9]*' | head -1 | cut -d: -f2)
echo "  (feed_id=${FEED_ID})"

echo ""
echo "=== List feeds ==="
resp=$(api GET /api/feeds)
assert_status "GET /api/feeds → 200" "200" "$(get_status "$resp")"
assert_contains "response is array with feed" "$(get_body "$resp")" "$FEED_URL"

echo ""
echo "=== Get single feed ==="
resp=$(api GET "/api/feeds/${FEED_ID}")
assert_status "GET /api/feeds/:id → 200" "200" "$(get_status "$resp")"
assert_contains "response has feed url" "$(get_body "$resp")" "$FEED_URL"

echo ""
echo "=== Duplicate subscribe → 409 ==="
resp=$(api POST /api/feeds -H "Content-Type: application/json" -d "{\"url\": \"${FEED_URL}\"}")
assert_status "POST /api/feeds (duplicate) → 409" "409" "$(get_status "$resp")"

echo ""
echo "=== Refresh feed ==="
resp=$(api POST "/api/feeds/${FEED_ID}/refresh")
assert_status "POST /api/feeds/:id/refresh → 200" "200" "$(get_status "$resp")"
assert_contains "response has articles_added" "$(get_body "$resp")" '"articles_added"'

echo ""
echo "=== Refresh all ==="
resp=$(api POST /api/feeds/refresh-all)
assert_status "POST /api/feeds/refresh-all → 200" "200" "$(get_status "$resp")"
assert_contains "response has feeds_refreshed" "$(get_body "$resp")" '"feeds_refreshed"'

echo ""
echo "=== List articles ==="
resp=$(api GET "/api/articles?feed_id=${FEED_ID}")
status=$(get_status "$resp")
body=$(get_body "$resp")
assert_status "GET /api/articles → 200" "200" "$status"
assert_contains "articles array is non-empty" "$body" '"id"'

# Extract an article ID
ARTICLE_ID=$(echo "$body" | grep -o '"id":[0-9]*' | head -1 | cut -d: -f2)
echo "  (article_id=${ARTICLE_ID})"

echo ""
echo "=== Get single article ==="
resp=$(api GET "/api/articles/${ARTICLE_ID}")
assert_status "GET /api/articles/:id → 200" "200" "$(get_status "$resp")"
assert_contains "response has article title" "$(get_body "$resp")" '"title"'

echo ""
echo "=== Mark article read ==="
resp=$(api PATCH "/api/articles/${ARTICLE_ID}" -H "Content-Type: application/json" -d '{"is_read": true}')
assert_status "PATCH /api/articles/:id → 200" "200" "$(get_status "$resp")"
assert_contains "article is now read" "$(get_body "$resp")" '"is_read":true'

echo ""
echo "=== Mark article unread ==="
resp=$(api PATCH "/api/articles/${ARTICLE_ID}" -H "Content-Type: application/json" -d '{"is_read": false}')
assert_status "PATCH /api/articles/:id → 200" "200" "$(get_status "$resp")"
assert_contains "article is now unread" "$(get_body "$resp")" '"is_read":false'

echo ""
echo "=== Mark all read ==="
resp=$(api POST "/api/articles/mark-all-read?feed_id=${FEED_ID}")
assert_status "POST /api/articles/mark-all-read → 200" "200" "$(get_status "$resp")"

echo ""
echo "=== Preview feed ==="
resp=$(api GET "/api/feeds/preview?url=${FEED_URL}")
assert_status "GET /api/feeds/preview → 200" "200" "$(get_status "$resp")"
assert_contains "preview has title" "$(get_body "$resp")" '"title"'

echo ""
echo "=== Delete feed ==="
resp=$(api DELETE "/api/feeds/${FEED_ID}")
assert_status "DELETE /api/feeds/:id → 204" "204" "$(get_status "$resp")"

echo ""
echo "=== Verify feed deleted ==="
resp=$(api GET "/api/feeds/${FEED_ID}")
assert_status "GET deleted feed → 404" "404" "$(get_status "$resp")"

echo ""
echo "=== 404 for unknown route ==="
resp=$(api GET /api/nonexistent)
assert_status "GET /api/nonexistent → 404" "404" "$(get_status "$resp")"

echo ""
echo "=== HTMX: feed list ==="
resp=$(api GET /api/ui/feeds)
assert_status "GET /api/ui/feeds → 200" "200" "$(get_status "$resp")"
assert_contains "returns HTML" "$(get_body "$resp")" "All Articles"

# --- Summary ---

echo ""
echo "================================"
echo "Results: ${PASS} passed, ${FAIL} failed"
echo "================================"

if [[ $FAIL -gt 0 ]]; then
    exit 1
fi

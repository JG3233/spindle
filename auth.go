//go:build tinygo || wasip1

package main

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/spinframework/spin-go-sdk/v2/kv"
	"github.com/spinframework/spin-go-sdk/v2/variables"
)

const (
	sessionCookieName = "spindle_session"
	sessionKVPrefix   = "session:"
	sessionMaxAgeS    = 7 * 24 * 3600 // 7 days in seconds

	// Rate limiting: lock out after maxLoginAttempts failures within the window.
	rateLimitKVKey   = "login_attempts"
	maxLoginAttempts = 5
	lockoutDurationS = 900 // 15 minutes in seconds
)

// getConfiguredPassword returns the password from Spin variables.
// Returns empty string if not configured (auth disabled).
func getConfiguredPassword() string {
	pw, err := variables.Get("password")
	if err != nil || pw == "" {
		return ""
	}
	return pw
}

// generateSessionToken creates a cryptographically random token.
func generateSessionToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		// Fallback should never happen — crypto/rand uses OS entropy.
		panic("crypto/rand failed: " + err.Error())
	}
	return hex.EncodeToString(b)
}

// storeSession saves a session token in the KV store with a creation timestamp.
func storeSession(token string) error {
	store, err := kv.OpenStore("default")
	if err != nil {
		return fmt.Errorf("opening kv store: %w", err)
	}
	defer store.Close()

	// Store the creation time so we can enforce expiration.
	createdAt := strconv.FormatInt(time.Now().Unix(), 10)
	return store.Set(sessionKVPrefix+token, []byte(createdAt))
}

// validateSession checks if a session token exists and hasn't expired.
func validateSession(token string) bool {
	store, err := kv.OpenStore("default")
	if err != nil {
		return false
	}
	defer store.Close()

	data, err := store.Get(sessionKVPrefix + token)
	if err != nil {
		return false
	}

	createdAt, err := strconv.ParseInt(string(data), 10, 64)
	if err != nil {
		return false
	}

	if time.Now().Unix()-createdAt > int64(sessionMaxAgeS) {
		// Expired — clean up
		store.Delete(sessionKVPrefix + token)
		return false
	}
	return true
}

// --- Rate limiting ---
// Tracks failed login attempts in KV as "count:unix_timestamp".
// After maxLoginAttempts failures, further logins are rejected until
// lockoutDurationS seconds have elapsed since the last failure.

func getLoginAttempts() (int, int64) {
	store, err := kv.OpenStore("default")
	if err != nil {
		return 0, 0
	}
	defer store.Close()

	data, err := store.Get(rateLimitKVKey)
	if err != nil {
		return 0, 0
	}

	parts := strings.SplitN(string(data), ":", 2)
	if len(parts) != 2 {
		return 0, 0
	}
	count, _ := strconv.Atoi(parts[0])
	ts, _ := strconv.ParseInt(parts[1], 10, 64)
	return count, ts
}

func recordFailedLogin() {
	store, err := kv.OpenStore("default")
	if err != nil {
		return
	}
	defer store.Close()

	count, _ := getLoginAttempts()
	now := time.Now().Unix()
	store.Set(rateLimitKVKey, []byte(fmt.Sprintf("%d:%d", count+1, now)))
}

func resetLoginAttempts() {
	store, err := kv.OpenStore("default")
	if err != nil {
		return
	}
	defer store.Close()

	store.Delete(rateLimitKVKey)
}

func isLockedOut() bool {
	count, lastFailure := getLoginAttempts()
	if count < maxLoginAttempts {
		return false
	}

	now := time.Now().Unix()
	if now-lastFailure > int64(lockoutDurationS) {
		// Lockout expired — reset
		resetLoginAttempts()
		return false
	}

	return true
}

// requireAuth checks if the request is authenticated.
// Returns true if the request should proceed, false if it was rejected
// (and a response was already written).
func requireAuth(w http.ResponseWriter, r *http.Request) bool {
	password := getConfiguredPassword()
	if password == "" {
		// No password configured — auth disabled
		return true
	}

	// Check session cookie
	cookie, err := r.Cookie(sessionCookieName)
	if err == nil && cookie.Value != "" {
		if validateSession(cookie.Value) {
			return true
		}
	}

	// Not authenticated — check if this is an HTMX request or API call
	if r.Header.Get("HX-Request") == "true" {
		// HTMX request — return a redirect header so HTMX navigates to login
		w.Header().Set("HX-Redirect", "/api/login")
		w.WriteHeader(http.StatusUnauthorized)
		return false
	}

	if strings.HasPrefix(r.URL.Path, "/api/ui/") {
		// Browser request to UI endpoint — redirect to login
		http.Redirect(w, r, "/api/login", http.StatusSeeOther)
		return false
	}

	// JSON API request — return 401
	writeJSON(w, http.StatusUnauthorized, map[string]string{
		"error": "authentication required",
	})
	return false
}

// loginPageHandler serves the login form.
// GET /api/login
func loginPageHandler(w http.ResponseWriter, r *http.Request) {
	password := getConfiguredPassword()
	if password == "" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	// If already authenticated, redirect to app
	cookie, err := r.Cookie(sessionCookieName)
	if err == nil && cookie.Value != "" && validateSession(cookie.Value) {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	msg := r.URL.Query().Get("error")
	errorHTML := ""
	if msg != "" {
		errorHTML = fmt.Sprintf(`<p class="login-error">%s</p>`, escapeHTML(msg))
	}

	writeHTML(w, http.StatusOK, fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Spindle — Login</title>
    <link rel="stylesheet" href="/style.css">
</head>
<body>
    <div class="login-container">
        <h1>Spindle</h1>
        <p class="subtitle">Spin + WASM RSS Reader</p>
        %s
        <form method="POST" action="/api/login" class="login-form">
            <input type="password" name="password" placeholder="Password" required autofocus>
            <button type="submit">Log in</button>
        </form>
    </div>
</body>
</html>`, errorHTML))
}

// loginHandler processes the login form submission.
// POST /api/login
func loginHandler(w http.ResponseWriter, r *http.Request) {
	password := getConfiguredPassword()
	if password == "" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	// Check rate limit before doing anything else
	if isLockedOut() {
		http.Redirect(w, r, "/api/login?error=Too+many+attempts.+Try+again+later.", http.StatusSeeOther)
		return
	}

	submitted := r.FormValue("password")
	if subtle.ConstantTimeCompare([]byte(submitted), []byte(password)) != 1 {
		recordFailedLogin()
		http.Redirect(w, r, "/api/login?error=Invalid+password", http.StatusSeeOther)
		return
	}

	// Successful login — reset rate limiter
	resetLoginAttempts()

	// Create session
	token := generateSessionToken()
	if err := storeSession(token); err != nil {
		writeHTML(w, http.StatusInternalServerError, "Failed to create session")
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
		SameSite: http.SameSiteLaxMode,
	})

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// logoutHandler clears the session.
// POST /api/logout
func logoutHandler(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(sessionCookieName)
	if err == nil && cookie.Value != "" {
		// Remove from KV store
		store, err := kv.OpenStore("default")
		if err == nil {
			store.Delete(sessionKVPrefix + cookie.Value)
			store.Close()
		}
	}

	// Clear the cookie
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	})

	http.Redirect(w, r, "/api/login", http.StatusSeeOther)
}

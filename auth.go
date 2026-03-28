//go:build tinygo || wasip1

package main

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"

	"github.com/spinframework/spin-go-sdk/v2/kv"
	"github.com/spinframework/spin-go-sdk/v2/variables"
)

const (
	sessionCookieName = "spindle_session"
	sessionKVPrefix   = "session:"
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

// hashPassword returns a hex-encoded SHA-256 hash of the password.
func hashPassword(password string) string {
	h := sha256.Sum256([]byte(password))
	return hex.EncodeToString(h[:])
}

// generateSessionToken creates a deterministic but unique-enough token
// from the password. Since we're single-user, we hash the password with
// a session-specific salt.
func generateSessionToken(password string) string {
	// Use the password hash as the session token base.
	// This is acceptable for a single-user app — the token changes
	// whenever the password changes.
	h := sha256.Sum256([]byte("spindle-session:" + password))
	return hex.EncodeToString(h[:])
}

// storeSession saves a session token in the KV store.
func storeSession(token string) error {
	store, err := kv.OpenStore("default")
	if err != nil {
		return fmt.Errorf("opening kv store: %w", err)
	}
	defer store.Close()

	return store.Set(sessionKVPrefix+token, []byte("1"))
}

// validateSession checks if a session token exists in the KV store.
func validateSession(token string) bool {
	store, err := kv.OpenStore("default")
	if err != nil {
		return false
	}
	defer store.Close()

	exists, err := store.Exists(sessionKVPrefix + token)
	if err != nil {
		return false
	}
	return exists
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

	submitted := r.FormValue("password")
	if subtle.ConstantTimeCompare([]byte(submitted), []byte(password)) != 1 {
		http.Redirect(w, r, "/api/login?error=Invalid+password", http.StatusSeeOther)
		return
	}

	// Create session
	token := generateSessionToken(password)
	if err := storeSession(token); err != nil {
		writeHTML(w, http.StatusInternalServerError, "Failed to create session")
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
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

package main

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/swm8023/wheelmaker/internal/shared"
)

const (
	monitorSessionCookieName = "wm_monitor_session"
	monitorCSRFCookieName    = "wm_monitor_csrf"
	monitorCSRFHeaderName    = "X-WheelMaker-Monitor-CSRF"
	monitorSessionTTL        = 7 * 24 * time.Hour
)

var errMonitorTokenRequired = errors.New("registry.token is required for monitor authentication")

type monitorAuthenticator struct {
	token string
}

func loadMonitorRuntimeConfig(baseDir string) (*shared.AppConfig, error) {
	cfgPath := filepath.Join(baseDir, "config.json")
	cfg, err := shared.LoadConfig(cfgPath)
	if err != nil {
		return nil, fmt.Errorf("cannot load config.json at %s: %w", cfgPath, err)
	}
	if strings.TrimSpace(cfg.Registry.Token) == "" {
		return nil, errMonitorTokenRequired
	}
	return cfg, nil
}

func newMonitorAuthenticator(token string) *monitorAuthenticator {
	return &monitorAuthenticator{token: strings.TrimSpace(token)}
}

func (a *monitorAuthenticator) protectAPI(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ok, bearer := a.authenticated(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		if !bearer && requiresMonitorCSRF(r.Method) && !a.validCSRF(r) {
			writeError(w, http.StatusForbidden, "csrf token mismatch")
			return
		}
		next(w, r)
	}
}

func (a *monitorAuthenticator) protectPage(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ok, _ := a.authenticated(r)
		if !ok {
			handleMonitorLoginPage().ServeHTTP(w, r)
			return
		}
		next(w, r)
	}
}

func (a *monitorAuthenticator) authenticated(r *http.Request) (bool, bool) {
	if a == nil || a.token == "" {
		return false, false
	}
	if a.validBearer(r) {
		return true, true
	}
	cookie, err := r.Cookie(monitorSessionCookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		return false, false
	}
	if !a.validSessionCookie(cookie.Value, time.Now()) {
		return false, false
	}
	return true, false
}

func (a *monitorAuthenticator) validBearer(r *http.Request) bool {
	raw := strings.TrimSpace(r.Header.Get("Authorization"))
	if raw == "" {
		return false
	}
	const prefix = "Bearer "
	if len(raw) < len(prefix) || !strings.EqualFold(raw[:len(prefix)], prefix) {
		return false
	}
	return a.tokenMatches(strings.TrimSpace(raw[len(prefix):]))
}

func (a *monitorAuthenticator) tokenMatches(candidate string) bool {
	if a == nil || a.token == "" || candidate == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(candidate), []byte(a.token)) == 1
}

func (a *monitorAuthenticator) validSessionCookie(value string, now time.Time) bool {
	parts := strings.Split(value, ".")
	if len(parts) != 3 {
		return false
	}
	expiryUnix, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return false
	}
	if now.Unix() > expiryUnix {
		return false
	}
	nonce := parts[1]
	if nonce == "" || parts[2] == "" {
		return false
	}
	want := a.signSession(parts[0], nonce)
	return subtle.ConstantTimeCompare([]byte(parts[2]), []byte(want)) == 1
}

func (a *monitorAuthenticator) newSessionCookieValue(expires time.Time) (string, error) {
	nonce, err := randomHex(16)
	if err != nil {
		return "", err
	}
	expiry := strconv.FormatInt(expires.Unix(), 10)
	return expiry + "." + nonce + "." + a.signSession(expiry, nonce), nil
}

func (a *monitorAuthenticator) signSession(expiry string, nonce string) string {
	key := sha256.Sum256([]byte("wheelmaker-monitor-session\x00" + a.token))
	mac := hmac.New(sha256.New, key[:])
	mac.Write([]byte(expiry))
	mac.Write([]byte{0})
	mac.Write([]byte(nonce))
	return hex.EncodeToString(mac.Sum(nil))
}

func (a *monitorAuthenticator) validCSRF(r *http.Request) bool {
	header := strings.TrimSpace(r.Header.Get(monitorCSRFHeaderName))
	if header == "" {
		return false
	}
	cookie, err := r.Cookie(monitorCSRFCookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(header), []byte(cookie.Value)) == 1
}

func requiresMonitorCSRF(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return false
	default:
		return true
	}
}

func (a *monitorAuthenticator) handleLogin() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if a == nil || a.token == "" {
			writeError(w, http.StatusInternalServerError, errMonitorTokenRequired.Error())
			return
		}
		var payload struct {
			Token string `json:"token"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			writeError(w, http.StatusBadRequest, "invalid login payload")
			return
		}
		if !a.tokenMatches(strings.TrimSpace(payload.Token)) {
			writeError(w, http.StatusUnauthorized, "invalid token")
			return
		}

		expires := time.Now().Add(monitorSessionTTL)
		sessionValue, err := a.newSessionCookieValue(expires)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "create session: "+err.Error())
			return
		}
		csrfValue, err := randomHex(16)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "create csrf token: "+err.Error())
			return
		}
		secure := isHTTPSRequest(r)
		http.SetCookie(w, monitorCookie(monitorSessionCookieName, sessionValue, expires, true, secure))
		http.SetCookie(w, monitorCookie(monitorCSRFCookieName, csrfValue, expires, false, secure))
		writeJSON(w, map[string]string{"status": "ok"})
	}
}

func (a *monitorAuthenticator) handleLogout() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		secure := isHTTPSRequest(r)
		http.SetCookie(w, expiredMonitorCookie(monitorSessionCookieName, true, secure))
		http.SetCookie(w, expiredMonitorCookie(monitorCSRFCookieName, false, secure))
		writeJSON(w, map[string]string{"status": "ok"})
	}
}

func (a *monitorAuthenticator) handleStatus() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ok, _ := a.authenticated(r)
		writeJSON(w, map[string]bool{"authenticated": ok})
	}
}

func monitorCookie(name string, value string, expires time.Time, httpOnly bool, secure bool) *http.Cookie {
	return &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		Expires:  expires,
		MaxAge:   int(monitorSessionTTL.Seconds()),
		HttpOnly: httpOnly,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	}
}

func expiredMonitorCookie(name string, httpOnly bool, secure bool) *http.Cookie {
	return &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0).UTC(),
		MaxAge:   -1,
		HttpOnly: httpOnly,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	}
}

func isHTTPSRequest(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	for _, part := range strings.Split(r.Header.Get("X-Forwarded-Proto"), ",") {
		if strings.EqualFold(strings.TrimSpace(part), "https") {
			return true
		}
	}
	return false
}

func randomHex(bytesLen int) (string, error) {
	buf := make([]byte, bytesLen)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

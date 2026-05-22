package portrelay

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (c *Controller) setAuthCookie(w http.ResponseWriter, slot relaySlot) {
	expires := time.Now().Add(12 * time.Hour)
	payload := fmt.Sprintf("%s:%d:%d", slot.RelayID, slot.AccessCodeGeneration, expires.Unix())
	signature := c.signCookiePayload(payload)
	value := base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." + signature
	http.SetCookie(w, &http.Cookie{
		Name:     relayCookieName,
		Value:    value,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func (c *Controller) clearAuthCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     relayCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func (c *Controller) authenticated(r *http.Request, slot relaySlot) bool {
	cookie, err := r.Cookie(relayCookieName)
	if err != nil || cookie.Value == "" {
		return false
	}
	parts := strings.Split(cookie.Value, ".")
	if len(parts) != 2 {
		return false
	}
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return false
	}
	payload := string(payloadBytes)
	if !hmac.Equal([]byte(parts[1]), []byte(c.signCookiePayload(payload))) {
		return false
	}
	fields := strings.Split(payload, ":")
	if len(fields) != 3 {
		return false
	}
	if fields[0] != slot.RelayID {
		return false
	}
	generation, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil || generation != slot.AccessCodeGeneration {
		return false
	}
	expires, err := strconv.ParseInt(fields[2], 10, 64)
	if err != nil || time.Now().Unix() > expires {
		return false
	}
	return true
}

func (c *Controller) signCookiePayload(payload string) string {
	mac := hmac.New(sha256.New, c.secret)
	_, _ = mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

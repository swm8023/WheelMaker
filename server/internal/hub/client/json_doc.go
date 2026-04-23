package client

import (
	"encoding/json"
	"strings"
)

func normalizeJSONDoc(raw string, fallback string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback
	}
	var v any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return fallback
	}
	return raw
}

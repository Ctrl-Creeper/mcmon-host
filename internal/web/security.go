package web

import (
	"net/http"
	"net/url"
	"strings"
)

func bearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
	}
	return ""
}

func originAllowed(origin, host, rawAllowlist string) bool {
	normalizedOrigin, originHost, ok := normalizeOrigin(origin)
	if !ok {
		return false
	}
	if strings.EqualFold(originHost, strings.ToLower(host)) {
		return true
	}
	for _, entry := range splitAllowlist(rawAllowlist) {
		if entry == "*" {
			return true
		}
		if strings.Contains(entry, "://") {
			normalizedEntry, _, ok := normalizeOrigin(entry)
			if ok && strings.EqualFold(normalizedEntry, normalizedOrigin) {
				return true
			}
			continue
		}
		if strings.EqualFold(entry, originHost) {
			return true
		}
	}
	return false
}

func splitAllowlist(raw string) []string {
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r'
	})
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if entry := strings.TrimSpace(part); entry != "" {
			out = append(out, strings.ToLower(entry))
		}
	}
	return out
}

func normalizeOrigin(raw string) (string, string, bool) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", "", false
	}
	host := strings.ToLower(parsed.Host)
	return strings.ToLower(parsed.Scheme) + "://" + host, host, true
}

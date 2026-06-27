package web

import "testing"

func TestOriginMatchesHostAndAllowlist(t *testing.T) {
	if !originAllowed("https://mcmon.example", "mcmon.example", "") {
		t.Fatal("same host origin should be allowed")
	}
	if !originAllowed("https://dash.example", "mcmon.example", "https://dash.example,other.example") {
		t.Fatal("configured origin should be allowed")
	}
	if originAllowed("https://evil.example", "mcmon.example", "https://dash.example") {
		t.Fatal("unlisted origin should be rejected")
	}
	if originAllowed("not-a-url", "mcmon.example", "*") {
		t.Fatal("invalid origin should be rejected")
	}
}

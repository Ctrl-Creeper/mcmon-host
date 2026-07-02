package web

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readStaticFile(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("static", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestStaticPagesUseSharedAuthReadiness(t *testing.T) {
	auth := readStaticFile(t, "auth.js")
	for _, want := range []string{"isAuthenticated", "onReady", "onChange"} {
		if !strings.Contains(auth, want) {
			t.Fatalf("auth.js missing shared auth readiness marker %q", want)
		}
	}
	for _, unwanted := range []string{"mcmon-auth-ready", "whenReady"} {
		if strings.Contains(auth, unwanted) {
			t.Fatalf("auth.js still contains redundant auth readiness artifact %q", unwanted)
		}
	}

	for _, page := range []string{"index.html", "agents.html", "detail.html", "settings.html"} {
		body := readStaticFile(t, page)
		if !strings.Contains(body, "onAuthReady(") {
			t.Fatalf("%s does not wait for shared auth readiness before protected API work", page)
		}
	}
}

func TestAccountSecurityLivesOnSettingsPage(t *testing.T) {
	agents := readStaticFile(t, "agents.html")
	if strings.Contains(agents, "Account security") || strings.Contains(agents, "securityCard") {
		t.Fatalf("agents.html should not contain account security controls")
	}

	settings := readStaticFile(t, "settings.html")
	if !strings.Contains(settings, "Account security") || !strings.Contains(settings, "securityCard") {
		t.Fatalf("settings.html should contain account security controls")
	}
}

func TestAgentTargetEditorIncludesPublicVisibilityControl(t *testing.T) {
	agents := readStaticFile(t, "agents.html")
	for _, want := range []string{"public_visible", "Publicly visible"} {
		if !strings.Contains(agents, want) {
			t.Fatalf("agents.html missing target visibility control marker %q", want)
		}
	}
}

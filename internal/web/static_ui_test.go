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

func TestStaticPagesUseSiteBrandingScript(t *testing.T) {
	site := readStaticFile(t, "site.js")
	for _, want := range []string{"/api/site-settings", "data-site-brand", "favicon"} {
		if !strings.Contains(site, want) {
			t.Fatalf("site.js missing branding marker %q", want)
		}
	}

	for _, page := range []string{"index.html", "agents.html", "detail.html", "settings.html"} {
		body := readStaticFile(t, page)
		if !strings.Contains(body, `src="/site.js"`) || !strings.Contains(body, "data-site-brand") {
			t.Fatalf("%s does not load shared site branding", page)
		}
	}
}

func TestStaticPagesUseCleanRoutesAndSettingsAppearanceControls(t *testing.T) {
	for _, page := range []string{"index.html", "agents.html", "detail.html", "settings.html"} {
		body := readStaticFile(t, page)
		for _, unwanted := range []string{`href="/agents.html"`, `href="/settings.html"`, "`/detail.html?"} {
			if strings.Contains(body, unwanted) {
				t.Fatalf("%s still references legacy route %q", page, unwanted)
			}
		}
	}

	settings := readStaticFile(t, "settings.html")
	for _, want := range []string{"appearanceCard", "siteTitle", "brandName", "iconUrl", "siteIconFile"} {
		if !strings.Contains(settings, want) {
			t.Fatalf("settings.html missing appearance control marker %q", want)
		}
	}
}

func TestAgentTargetEditorIncludesPublicVisibilityControl(t *testing.T) {
	agents := readStaticFile(t, "agents.html")
	for _, want := range []string{"public_visible", "Publicly visible"} {
		if !strings.Contains(agents, want) {
			t.Fatalf("agents.html missing target visibility control marker %q", want)
		}
	}

	style := readStaticFile(t, "style.css")
	for _, want := range []string{".visibility-toggle", ".visibility-toggle input[type=\"checkbox\"]"} {
		if !strings.Contains(style, want) {
			t.Fatalf("style.css missing compact visibility toggle style %q", want)
		}
	}
}

func TestPublicDashboardUsesPublicAPIsForGuests(t *testing.T) {
	index := readStaticFile(t, "index.html")
	for _, want := range []string{"/api/public/targets", "/api/public/series"} {
		if !strings.Contains(index, want) {
			t.Fatalf("index.html missing guest public API marker %q", want)
		}
	}
	for _, unwanted := range []string{"Login to view host data.", "else stopDashboard()"} {
		if strings.Contains(index, unwanted) {
			t.Fatalf("index.html still blocks guest dashboard with %q", unwanted)
		}
	}
}

func TestPublicDetailUsesPublicAPIsForGuests(t *testing.T) {
	detail := readStaticFile(t, "detail.html")
	for _, want := range []string{"/api/public/targets", "/api/public/series"} {
		if !strings.Contains(detail, want) {
			t.Fatalf("detail.html missing guest public API marker %q", want)
		}
	}
	for _, unwanted := range []string{"Login required", "if (!window.mcmonAuth?.isAuthenticated) return", "else stopDetail()"} {
		if strings.Contains(detail, unwanted) {
			t.Fatalf("detail.html still blocks guest detail with %q", unwanted)
		}
	}
}

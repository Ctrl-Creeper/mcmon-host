package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestInstallScriptUsesInstallTokenAndImmutableConfig(t *testing.T) {
	st, mux := newTestServer(t, Options{DiscoveryKey: "discover", AdminToken: "admin-token"})

	agent, ok, err := st.AgentByToken("agent-token")
	if err != nil || !ok {
		t.Fatalf("seed agent lookup ok=%v err=%v", ok, err)
	}
	agent.InstallToken = "install-token"
	if err := st.UpdateAgent(agent); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "http://internal.local/api/agents/agent-1/install.sh?token=install-token", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Host", "host.example.com")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("install script status = %d, want 200: %s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	for _, want := range []string{
		"Ctrl-Creeper/mcmon-agent",
		"--host-url 'https://host.example.com'",
		"--token 'agent-token'",
		"--config-base64 '",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("install script missing %q:\n%s", want, body)
		}
	}
}

func TestInstallPowerShellUsesInstallTokenAndImmutableConfig(t *testing.T) {
	st, mux := newTestServer(t, Options{DiscoveryKey: "discover", AdminToken: "admin-token"})

	agent, ok, err := st.AgentByToken("agent-token")
	if err != nil || !ok {
		t.Fatalf("seed agent lookup ok=%v err=%v", ok, err)
	}
	agent.InstallToken = "install-token"
	if err := st.UpdateAgent(agent); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "http://internal.local/api/agents/agent-1/install.ps1?token=install-token", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Host", "host.example.com")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("install script status = %d, want 200: %s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	for _, want := range []string{
		"mcmon-agent/main/install.ps1",
		"-HostUrl 'https://host.example.com'",
		"-Token 'agent-token'",
		"-ConfigBase64 '",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("install script missing %q:\n%s", want, body)
		}
	}
}

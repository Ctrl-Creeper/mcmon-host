package web

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Ctrl-Creeper/mcmon-host/internal/hub"
	"github.com/Ctrl-Creeper/mcmon-host/internal/store"
	"github.com/gorilla/websocket"
	"github.com/pquerna/otp/totp"
)

func newTestServer(t *testing.T, opts Options) (*store.Store, *http.ServeMux) {
	t.Helper()
	st, err := store.Open(t.TempDir() + "/mcmon-host.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.CreateAgent(store.Agent{ID: "agent-1", Name: "Agent One", Token: "agent-token"}); err != nil {
		t.Fatal(err)
	}
	return st, NewMux(st, hub.New(st), opts)
}

func TestAdminAPIsRequireAdminTokenWhenConfigured(t *testing.T) {
	_, mux := newTestServer(t, Options{DiscoveryKey: "discover", AdminToken: "admin-token"})

	req := httptest.NewRequest(http.MethodGet, "/api/agents", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("GET /api/agents without token = %d, want %d", rr.Code, http.StatusUnauthorized)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/agents", nil)
	req.Header.Set("Authorization", "Bearer admin-token")
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /api/agents with admin token = %d, want %d: %s", rr.Code, http.StatusOK, rr.Body.String())
	}
}

func TestCleanStaticRoutesAndLegacyHTMLRoutes(t *testing.T) {
	_, mux := newTestServer(t, Options{DiscoveryKey: "discover", AdminToken: "admin-token"})
	for _, tc := range []struct {
		path string
		want string
	}{
		{"/", "MCMon Host - Dashboard"},
		{"/agents", "Agents - MCMon Host"},
		{"/agents.html", "Agents - MCMon Host"},
		{"/settings", "Settings - MCMon Host"},
		{"/settings.html", "Settings - MCMon Host"},
		{"/detail", "Server Detail - MCMon Host"},
		{"/detail.html", "Server Detail - MCMon Host"},
	} {
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), tc.want) {
			t.Fatalf("GET %s = %d body has %q? %v", tc.path, rr.Code, tc.want, strings.Contains(rr.Body.String(), tc.want))
		}
	}
}

func TestSiteSettingsAPIAndFavicon(t *testing.T) {
	_, mux := newTestServer(t, Options{DiscoveryKey: "discover", AdminToken: "admin-token"})

	req := httptest.NewRequest(http.MethodGet, "/api/site-settings", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /api/site-settings = %d, want 200: %s", rr.Code, rr.Body.String())
	}
	var settings store.SiteSettings
	if err := json.NewDecoder(rr.Body).Decode(&settings); err != nil {
		t.Fatal(err)
	}
	if settings.SiteTitle != "MCMon Host" || settings.BrandName != "MCMon Host" {
		t.Fatalf("default settings = %#v", settings)
	}

	req = httptest.NewRequest(http.MethodPut, "/api/site-settings", bytes.NewReader([]byte(`{"site_title":"My Monitor","brand_name":"My Brand","icon_url":"https://example.com/icon.png"}`)))
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("PUT /api/site-settings without auth = %d, want 401", rr.Code)
	}

	req = httptest.NewRequest(http.MethodPut, "/api/site-settings", bytes.NewReader([]byte(`{"site_title":"My Monitor","brand_name":"My Brand","icon_url":"https://example.com/icon.png"}`)))
	req.Header.Set("Authorization", "Bearer admin-token")
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("PUT /api/site-settings = %d, want 200: %s", rr.Code, rr.Body.String())
	}
	settings = store.SiteSettings{}
	if err := json.NewDecoder(rr.Body).Decode(&settings); err != nil {
		t.Fatal(err)
	}
	if settings.SiteTitle != "My Monitor" || settings.BrandName != "My Brand" || settings.IconURL != "https://example.com/icon.png" {
		t.Fatalf("updated settings = %#v", settings)
	}

	req = httptest.NewRequest(http.MethodPut, "/api/site-icon", bytes.NewReader([]byte("icon-bytes")))
	req.Header.Set("Authorization", "Bearer admin-token")
	req.Header.Set("Content-Type", "image/png")
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("PUT /api/site-icon = %d, want 200: %s", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/favicon.ico", nil)
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK || rr.Body.String() != "icon-bytes" || rr.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("GET /favicon.ico status=%d content-type=%q body=%q", rr.Code, rr.Header().Get("Content-Type"), rr.Body.String())
	}
}

func TestAuthLoginReturnsSessionAndSessionAuthorizesAdminAPI(t *testing.T) {
	st, mux := newTestServer(t, Options{DiscoveryKey: "discover", AdminToken: "admin-token"})
	if _, _, _, err := st.EnsureAdmin("admin", "secret-password"); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader([]byte(`{"username":"admin","password":"secret-password"}`)))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("login = %d, want 200: %s", rr.Code, rr.Body.String())
	}
	var login struct {
		SessionToken string `json:"session_token"`
		Username     string `json:"username"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&login); err != nil {
		t.Fatal(err)
	}
	if login.SessionToken == "" || login.Username != "admin" {
		t.Fatalf("login response = %#v", login)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/agents", nil)
	req.Header.Set("Authorization", "Bearer "+login.SessionToken)
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /api/agents with session = %d, want 200: %s", rr.Code, rr.Body.String())
	}
}

func TestAuthLoginRequiresTotpWhenEnabled(t *testing.T) {
	st, mux := newTestServer(t, Options{DiscoveryKey: "discover", AdminToken: "admin-token"})
	if _, _, _, err := st.EnsureAdmin("admin", "secret-password"); err != nil {
		t.Fatal(err)
	}
	key, err := totp.Generate(totp.GenerateOpts{Issuer: "MCMon", AccountName: "admin"})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetAdminTwoFactor(key.Secret()); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader([]byte(`{"username":"admin","password":"secret-password"}`)))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("login without totp = %d, want 401", rr.Code)
	}

	code, err := totp.GenerateCode(key.Secret(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	body := fmt.Sprintf(`{"username":"admin","password":"secret-password","totp_code":%q}`, code)
	req = httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("login with totp = %d, want 200: %s", rr.Code, rr.Body.String())
	}
}

func TestAuthTwoFactorSetupAndEnable(t *testing.T) {
	st, mux := newTestServer(t, Options{DiscoveryKey: "discover", AdminToken: "admin-token"})
	if _, _, _, err := st.EnsureAdmin("admin", "secret-password"); err != nil {
		t.Fatal(err)
	}
	session, err := st.CreateAdminSession("agent", "127.0.0.1", 3600)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/auth/2fa/setup", nil)
	req.Header.Set("Authorization", "Bearer "+session.Token)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("setup = %d, want 200: %s", rr.Code, rr.Body.String())
	}
	var setup struct {
		Secret          string `json:"secret"`
		ProvisioningURI string `json:"provisioning_uri"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&setup); err != nil {
		t.Fatal(err)
	}
	if setup.Secret == "" || setup.ProvisioningURI == "" {
		t.Fatalf("setup response = %#v", setup)
	}
	code, err := totp.GenerateCode(setup.Secret, time.Now())
	if err != nil {
		t.Fatal(err)
	}

	body := fmt.Sprintf(`{"secret":%q,"totp_code":%q}`, setup.Secret, code)
	req = httptest.NewRequest(http.MethodPost, "/api/auth/2fa/enable", bytes.NewReader([]byte(body)))
	req.Header.Set("Authorization", "Bearer "+session.Token)
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("enable = %d, want 200: %s", rr.Code, rr.Body.String())
	}
	admin, ok, err := st.Admin()
	if err != nil {
		t.Fatal(err)
	}
	if !ok || admin.TwoFactorSecret != setup.Secret {
		t.Fatalf("admin after enable ok=%v admin=%#v", ok, admin)
	}
}

func TestAuthPasswordUpdateSyncsConfigCallback(t *testing.T) {
	st, err := store.Open(t.TempDir() + "/mcmon-host.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.CreateAgent(store.Agent{ID: "agent-1", Name: "Agent One", Token: "agent-token"}); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := st.EnsureAdmin("admin", "old-password"); err != nil {
		t.Fatal(err)
	}
	var syncedUsername, syncedPassword string
	mux := NewMux(st, hub.New(st), Options{
		DiscoveryKey: "discover",
		AdminToken:   "admin-token",
		UpdateAdminCredentials: func(username, password string) error {
			syncedUsername = username
			syncedPassword = password
			return nil
		},
	})
	session, err := st.CreateAdminSession("agent", "127.0.0.1", 3600)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/auth/password", bytes.NewReader([]byte(`{"username":"admin2","current_password":"old-password","new_password":"new-password"}`)))
	req.Header.Set("Authorization", "Bearer "+session.Token)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("password update = %d, want 200: %s", rr.Code, rr.Body.String())
	}
	if syncedUsername != "admin2" || syncedPassword != "new-password" {
		t.Fatalf("synced credentials username=%q password=%q", syncedUsername, syncedPassword)
	}
	if _, ok, err := st.CheckAdminPassword("admin2", "new-password"); err != nil || !ok {
		t.Fatalf("new credentials ok=%v err=%v", ok, err)
	}
}

func TestDeleteAgentRemovesTargetsAndSamples(t *testing.T) {
	st, mux := newTestServer(t, Options{DiscoveryKey: "discover", AdminToken: "admin-token"})
	targets := []store.AgentTarget{{
		AgentID: "agent-1", TargetID: "target-1", Name: "Target One", Host: "mc.example.com", Port: 25565,
		TimeoutMs: 1500,
		Monitors: store.Monitors{
			Online: store.SimpleMonitor{Enabled: true, IntervalSec: 60},
		},
	}}
	if err := st.UpsertTargets("agent-1", targets); err != nil {
		t.Fatal(err)
	}
	value := 1.0
	if err := st.InsertMetricSample(store.MetricSample{AgentID: "agent-1", TargetID: "target-1", Metric: "online", Ts: 123, Value: &value}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/agents/agent-1", nil)
	req.Header.Set("Authorization", "Bearer admin-token")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("DELETE /api/agents/agent-1 = %d, want 200: %s", rr.Code, rr.Body.String())
	}

	agents, err := st.ListAgents()
	if err != nil {
		t.Fatal(err)
	}
	if len(agents) != 0 {
		t.Fatalf("agents after delete = %#v, want none", agents)
	}
	gotTargets, err := st.TargetsForAgent("agent-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(gotTargets) != 0 {
		t.Fatalf("targets after delete = %#v, want none", gotTargets)
	}
	series, err := st.MetricSeries("agent-1", "target-1", "online", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(series) != 0 {
		t.Fatalf("metric series after delete = %#v, want none", series)
	}
}

func TestPublicTargetsOnlyReturnPublicVisibleTargets(t *testing.T) {
	st, mux := newTestServer(t, Options{DiscoveryKey: "discover", AdminToken: "admin-token"})
	targets := []store.AgentTarget{
		{AgentID: "agent-1", TargetID: "public-1", Name: "Public", Host: "public.example.com", Port: 25565},
		{AgentID: "agent-1", TargetID: "hidden-1", Name: "Hidden", Host: "hidden.example.com", Port: 25565, PublicVisible: false, PublicVisibleSet: true},
	}
	if err := st.UpsertTargets("agent-1", targets); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/public/targets", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /api/public/targets = %d, want 200: %s", rr.Code, rr.Body.String())
	}
	var got []store.AgentTarget
	if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].TargetID != "public-1" {
		t.Fatalf("public targets = %#v, want only public-1", got)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/targets", nil)
	req.Header.Set("Authorization", "Bearer admin-token")
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /api/targets as admin = %d, want 200: %s", rr.Code, rr.Body.String())
	}
	got = nil
	if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("admin targets = %#v, want both targets", got)
	}
}

func TestPublicSeriesRejectsHiddenTargets(t *testing.T) {
	st, mux := newTestServer(t, Options{DiscoveryKey: "discover", AdminToken: "admin-token"})
	targets := []store.AgentTarget{
		{AgentID: "agent-1", TargetID: "public-1", Name: "Public", Host: "public.example.com", Port: 25565},
		{AgentID: "agent-1", TargetID: "hidden-1", Name: "Hidden", Host: "hidden.example.com", Port: 25565, PublicVisible: false, PublicVisibleSet: true},
	}
	if err := st.UpsertTargets("agent-1", targets); err != nil {
		t.Fatal(err)
	}
	now := time.Now().Unix()
	value := 1.0
	if err := st.InsertSample(store.Sample{AgentID: "agent-1", TargetID: "public-1", Ts: now, LossPct: 0}); err != nil {
		t.Fatal(err)
	}
	if err := st.InsertMetricSample(store.MetricSample{AgentID: "agent-1", TargetID: "public-1", Metric: "online", Ts: now, Value: &value}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/public/series?agent=agent-1&target=public-1&range=1h", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("public visible series = %d, want 200: %s", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/public/series?agent=agent-1&target=hidden-1&range=1h", nil)
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("hidden public series = %d, want 404", rr.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/public/metrics?agent=agent-1&target=hidden-1&metric=online&range=1h", nil)
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("hidden public metrics = %d, want 404", rr.Code)
	}
}

func TestDiscoverStillUsesDiscoveryKey(t *testing.T) {
	_, mux := newTestServer(t, Options{DiscoveryKey: "discover", AdminToken: "admin-token"})

	req := httptest.NewRequest(http.MethodPost, "/api/discover?name=new-agent", nil)
	req.Header.Set("Authorization", "Bearer admin-token")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("discover with admin token = %d, want %d", rr.Code, http.StatusUnauthorized)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/discover?name=new-agent", nil)
	req.Header.Set("Authorization", "Bearer discover")
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("discover with discovery key = %d, want %d: %s", rr.Code, http.StatusOK, rr.Body.String())
	}
}

func TestAgentV2RPCAcceptsBearerTokenAndStoresPingResult(t *testing.T) {
	st, mux := newTestServer(t, Options{DiscoveryKey: "discover", AdminToken: "admin-token"})
	ts := time.Now().Unix()
	body := []byte(fmt.Sprintf(`{"jsonrpc":"2.0","id":"1","method":"agent.pingResult","params":{"target_id":"target-1","ts":%d,"min_ms":10,"p50_ms":12,"max_ms":15,"loss_pct":0}}`, ts))
	req := httptest.NewRequest(http.MethodPost, "/api/agents/v2/rpc", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer agent-token")
	rr := httptest.NewRecorder()

	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("v2 rpc status = %d, want %d: %s", rr.Code, http.StatusOK, rr.Body.String())
	}

	series, err := st.Series("agent-1", "target-1", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(series) != 1 || series[0].P50Ms == nil || *series[0].P50Ms != 12 {
		t.Fatalf("stored series = %#v, want one p50=12 sample", series)
	}
}

func TestAgentV2RPCAcceptsMetricResult(t *testing.T) {
	st, mux := newTestServer(t, Options{DiscoveryKey: "discover", AdminToken: "admin-token"})
	ts := time.Now().Unix()
	body := []byte(fmt.Sprintf(`{"jsonrpc":"2.0","id":"1","method":"agent.metricResult","params":{"target_id":"target-1","metric":"players","ts":%d,"value":12,"extra":"{\"max\":40}"}}`, ts))
	req := httptest.NewRequest(http.MethodPost, "/api/agents/v2/rpc", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer agent-token")
	rr := httptest.NewRecorder()

	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("v2 metric rpc status = %d, want %d: %s", rr.Code, http.StatusOK, rr.Body.String())
	}

	series, err := st.MetricSeries("agent-1", "target-1", "players", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(series) != 1 || series[0].Value == nil || *series[0].Value != 12 {
		t.Fatalf("stored metric series = %#v, want one value=12 sample", series)
	}
}

func TestAgentV2RPCRejectsInvalidPingPayload(t *testing.T) {
	_, mux := newTestServer(t, Options{DiscoveryKey: "discover", AdminToken: "admin-token"})
	ts := time.Now().Unix()
	body := []byte(fmt.Sprintf(`{"jsonrpc":"2.0","id":"1","method":"agent.pingResult","params":{"target_id":"","ts":%d,"loss_pct":2}}`, ts))
	req := httptest.NewRequest(http.MethodPost, "/api/agents/v2/rpc", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer agent-token")
	rr := httptest.NewRecorder()

	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("invalid ping payload status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestWebSocketOriginPolicy(t *testing.T) {
	_, mux := newTestServer(t, Options{DiscoveryKey: "discover", AdminToken: "admin-token"})
	server := httptest.NewServer(mux)
	defer server.Close()

	wsURL := "ws" + server.URL[len("http"):] + "/api/agents/v2/rpc"
	headers := http.Header{}
	headers.Set("Authorization", "Bearer agent-token")
	headers.Set("Origin", "https://evil.example")
	_, resp, err := websocket.DefaultDialer.Dial(wsURL, headers)
	if err == nil {
		t.Fatal("cross-origin websocket unexpectedly connected")
	}
	if resp == nil || resp.StatusCode != http.StatusForbidden {
		t.Fatalf("cross-origin websocket status = %#v, want 403", resp)
	}

	headers.Set("Origin", server.URL)
	conn, resp, err := websocket.DefaultDialer.Dial(wsURL, headers)
	if err != nil {
		t.Fatalf("same-origin websocket failed, resp=%#v err=%v", resp, err)
	}
	defer conn.Close()

	msg, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      "hello",
		"method":  "agent.hello",
		"params": map[string]any{
			"version": "test",
			"targets": []map[string]any{{
				"id": "target-1", "name": "Target One", "host": "mc.example.com", "port": 25565,
			}},
		},
	})
	if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
		t.Fatal(err)
	}
	var respMsg map[string]any
	if err := conn.ReadJSON(&respMsg); err != nil {
		t.Fatal(err)
	}
	if respMsg["error"] != nil {
		t.Fatalf("hello response has error: %#v", respMsg)
	}
}

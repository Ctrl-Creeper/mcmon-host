package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/YOUR_PATH/mcmon-host/internal/hub"
	"github.com/YOUR_PATH/mcmon-host/internal/store"
	"github.com/gorilla/websocket"
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
	body := []byte(`{"jsonrpc":"2.0","id":"1","method":"agent.pingResult","params":{"target_id":"target-1","ts":12345,"min_ms":10,"p50_ms":12,"max_ms":15,"loss_pct":0}}`)
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
	body := []byte(`{"jsonrpc":"2.0","id":"1","method":"agent.metricResult","params":{"target_id":"target-1","metric":"players","ts":12345,"value":12,"extra":"{\"max\":40}"}}`)
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
	body := []byte(`{"jsonrpc":"2.0","id":"1","method":"agent.pingResult","params":{"target_id":"","ts":12345,"loss_pct":2}}`)
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

package web

import (
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/lewiswu/mcmon-host/internal/hub"
	"github.com/lewiswu/mcmon-host/internal/store"
)

//go:embed static
var staticFS embed.FS

type Options struct {
	DiscoveryKey     string
	AdminToken       string
	WSAllowedOrigins string
}

var rangeToSeconds = map[string]int64{
	"1h": 3600, "6h": 6 * 3600, "12h": 12 * 3600,
	"1d": 86400, "7d": 7 * 86400, "30d": 30 * 86400,
}

func NewMux(st *store.Store, h *hub.Hub, opts Options) *http.ServeMux {
	mux := http.NewServeMux()
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			origin := r.Header.Get("Origin")
			if origin == "" {
				return true
			}
			return originAllowed(origin, r.Host, opts.WSAllowedOrigins)
		},
	}
	upgrader.EnableCompression = true

	// --- Static files (dashboard) ---
	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		panic(err)
	}
	mux.Handle("/", http.FileServer(http.FS(sub)))

	// --- Agent WebSocket ---
	mux.HandleFunc("/api/ws", func(w http.ResponseWriter, r *http.Request) {
		token := agentTokenFromRequest(r)
		if token == "" {
			http.Error(w, "missing token", http.StatusUnauthorized)
			return
		}
		agent, ok, err := st.AgentByToken(token)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if !ok {
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("ws upgrade: %v", err)
			return
		}
		h.HandleConn(ws, agent)
	})

	mux.HandleFunc("/api/agents/v2/rpc", func(w http.ResponseWriter, r *http.Request) {
		token := agentTokenFromRequest(r)
		if token == "" {
			http.Error(w, "missing token", http.StatusUnauthorized)
			return
		}
		agent, ok, err := st.AgentByToken(token)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if !ok {
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}
		switch r.Method {
		case http.MethodGet:
			ws, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				log.Printf("ws upgrade: %v", err)
				return
			}
			h.HandleConn(ws, agent)
		case http.MethodPost:
			r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
			var raw json.RawMessage
			if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
				http.Error(w, "invalid JSON body: "+err.Error(), http.StatusBadRequest)
				return
			}
			resp, status := h.HandleRPC(agent, raw)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			w.Write(resp)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// --- Auto-discovery ---
	mux.HandleFunc("/api/discover", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") || strings.TrimPrefix(auth, "Bearer ") != opts.DiscoveryKey {
			http.Error(w, "invalid discovery key", http.StatusUnauthorized)
			return
		}
		name := r.URL.Query().Get("name")
		if name == "" {
			name = "agent-" + randHex(3)
		}
		agentID := randHex(8)
		token := randHex(16)
		if err := st.CreateAgent(store.Agent{ID: agentID, Name: name, Token: token}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		log.Printf("auto-discovered new agent: id=%s name=%s", agentID, name)
		writeJSON(w, map[string]string{"agent_id": agentID, "token": token})
	})

	// --- REST API for dashboard / app ---

	mux.HandleFunc("/api/agents", func(w http.ResponseWriter, r *http.Request) {
		if !requireAdmin(w, r, opts.AdminToken) {
			return
		}
		agents, err := st.ListAgents()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		type agentInfo struct {
			store.Agent
			Online bool `json:"online"`
		}
		out := make([]agentInfo, len(agents))
		for i, a := range agents {
			out[i] = agentInfo{Agent: a, Online: h.IsOnline(a.ID)}
		}
		writeJSON(w, out)
	})

	mux.HandleFunc("/api/agents/", func(w http.ResponseWriter, r *http.Request) {
		if !requireAdmin(w, r, opts.AdminToken) {
			return
		}
		// /api/agents/{agentID}/targets
		path := strings.TrimPrefix(r.URL.Path, "/api/agents/")
		parts := strings.SplitN(path, "/", 2)
		agentID := parts[0]
		sub := ""
		if len(parts) > 1 {
			sub = parts[1]
		}

		switch sub {
		case "targets":
			targets, err := st.TargetsForAgent(agentID)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			writeJSON(w, targets)
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	})

	mux.HandleFunc("/api/targets", func(w http.ResponseWriter, r *http.Request) {
		if !requireAdmin(w, r, opts.AdminToken) {
			return
		}
		targets, err := st.AllTargets()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, targets)
	})

	mux.HandleFunc("/api/series", func(w http.ResponseWriter, r *http.Request) {
		if !requireAdmin(w, r, opts.AdminToken) {
			return
		}
		agentID := r.URL.Query().Get("agent")
		targetID := r.URL.Query().Get("target")
		rangeKey := r.URL.Query().Get("range")
		secs, ok := rangeToSeconds[rangeKey]
		if !ok {
			secs = 3600
		}
		since := time.Now().Unix() - secs
		series, err := st.Series(agentID, targetID, since)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, series)
	})

	return mux
}

func requireAdmin(w http.ResponseWriter, r *http.Request, adminToken string) bool {
	if adminToken == "" {
		return true
	}
	if bearerToken(r) == adminToken {
		return true
	}
	http.Error(w, "unauthorized", http.StatusUnauthorized)
	return false
}

func agentTokenFromRequest(r *http.Request) string {
	if token := bearerToken(r); token != "" {
		return token
	}
	return r.URL.Query().Get("token")
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	json.NewEncoder(w).Encode(v)
}

func randHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}

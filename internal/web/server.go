package web

import (
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/Ctrl-Creeper/mcmon-host/internal/hub"
	"github.com/Ctrl-Creeper/mcmon-host/internal/store"
	"github.com/gorilla/websocket"
)

// secureEqual compares two strings in constant time relative to length,
// preventing token timing attacks. Returns false if either side is empty.
func secureEqual(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

//go:embed static
var staticFS embed.FS

type Options struct {
	DiscoveryKey     string
	AdminToken       string
	WSAllowedOrigins string
	PublicURL        string
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
		if !secureEqual(bearerToken(r), opts.DiscoveryKey) {
			http.Error(w, "invalid discovery key", http.StatusUnauthorized)
			return
		}
		name := r.URL.Query().Get("name")
		if name == "" {
			name = "agent-" + randHex(3)
		}
		agentID := randHex(8)
		token := randHex(16)
		if err := st.CreateAgent(store.Agent{ID: agentID, Name: name, Token: token, InstallToken: randHex(16)}); err != nil {
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
		switch r.Method {
		case http.MethodGet:
			agents, err := st.ListAgents()
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			type agentInfo struct {
				store.Agent
				Online       bool   `json:"online"`
				InstallToken string `json:"install_token"`
			}
			out := make([]agentInfo, len(agents))
			for i, a := range agents {
				out[i] = agentInfo{Agent: a, Online: h.IsOnline(a.ID), InstallToken: a.InstallToken}
			}
			writeJSON(w, out)
		case http.MethodPost:
			var body struct {
				Name    string              `json:"name"`
				Targets []store.AgentTarget `json:"targets"`
			}
			if !decodeJSON(w, r, &body) {
				return
			}
			if strings.TrimSpace(body.Name) == "" {
				http.Error(w, "name is required", http.StatusBadRequest)
				return
			}
			agent := store.Agent{ID: randHex(8), Name: body.Name, Token: randHex(16), InstallToken: randHex(16)}
			if err := st.CreateAgent(agent); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if len(body.Targets) > 0 {
				for i := range body.Targets {
					body.Targets[i].AgentID = agent.ID
					if body.Targets[i].TargetID == "" {
						body.Targets[i].TargetID = randHex(8)
					}
					if !hub.IsSafeID(body.Targets[i].TargetID) {
						http.Error(w, "target_id must be [A-Za-z0-9_-]{1,128}", http.StatusBadRequest)
						return
					}
				}
				if err := st.UpsertTargets(agent.ID, body.Targets); err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
			}
			writeJSON(w, agent)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/agents/", func(w http.ResponseWriter, r *http.Request) {
		// /api/agents/{agentID}/targets
		path := strings.TrimPrefix(r.URL.Path, "/api/agents/")
		parts := strings.SplitN(path, "/", 2)
		agentID := parts[0]
		sub := ""
		if len(parts) > 1 {
			sub = parts[1]
		}

		switch sub {
		case "install.sh":
			if r.Method != http.MethodGet {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			writeInstallScript(w, r, st, agentID, opts)
		case "install.ps1":
			if r.Method != http.MethodGet {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			writeInstallPowerShell(w, r, st, agentID, opts)
		case "install-token":
			if !requireAdmin(w, r, opts.AdminToken) {
				return
			}
			if r.Method != http.MethodPost {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			newToken := randHex(16)
			if err := st.RotateInstallToken(agentID, newToken); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			writeJSON(w, map[string]string{"install_token": newToken})
		case "targets":
			if !requireAdmin(w, r, opts.AdminToken) {
				return
			}
			switch r.Method {
			case http.MethodGet:
				targets, err := st.TargetsForAgent(agentID)
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				writeJSON(w, targets)
			case http.MethodPut:
				var targets []store.AgentTarget
				if !decodeJSON(w, r, &targets) {
					return
				}
				for i := range targets {
					targets[i].AgentID = agentID
					if targets[i].TargetID == "" {
						targets[i].TargetID = randHex(8)
					}
					if !hub.IsSafeID(targets[i].TargetID) {
						http.Error(w, "target_id must be [A-Za-z0-9_-]{1,128}", http.StatusBadRequest)
						return
					}
				}
				if err := st.UpsertTargets(agentID, targets); err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				writeJSON(w, targets)
			default:
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			}
		default:
			if !requireAdmin(w, r, opts.AdminToken) {
				return
			}
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

	mux.HandleFunc("/api/metrics", func(w http.ResponseWriter, r *http.Request) {
		if !requireAdmin(w, r, opts.AdminToken) {
			return
		}
		agentID := r.URL.Query().Get("agent")
		targetID := r.URL.Query().Get("target")
		metric := r.URL.Query().Get("metric")
		if agentID == "" || targetID == "" || metric == "" {
			http.Error(w, "agent, target and metric are required", http.StatusBadRequest)
			return
		}
		rangeKey := r.URL.Query().Get("range")
		secs, ok := rangeToSeconds[rangeKey]
		if !ok {
			secs = 3600
		}
		since := time.Now().Unix() - secs
		series, err := st.MetricSeries(agentID, targetID, metric, since)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, series)
	})

	return mux
}

func decodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	defer r.Body.Close()
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		http.Error(w, "invalid JSON body: "+err.Error(), http.StatusBadRequest)
		return false
	}
	return true
}

func requireAdmin(w http.ResponseWriter, r *http.Request, adminToken string) bool {
	if adminToken == "" {
		return true
	}
	if secureEqual(bearerToken(r), adminToken) {
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

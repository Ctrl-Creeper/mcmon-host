package web

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/lewiswu/mcmon-host/internal/store"
)

type agentInstallConfig struct {
	HostURL string              `json:"host_url"`
	AgentID string              `json:"agent_id"`
	Token   string              `json:"token"`
	Targets []store.AgentTarget `json:"targets"`
}

func writeInstallScript(w http.ResponseWriter, r *http.Request, st *store.Store, agentID string, opts Options) {
	cfg, ok := buildAgentInstallPayload(w, r, st, agentID, opts)
	if !ok {
		return
	}

	w.Header().Set("Content-Type", "text/x-shellscript; charset=utf-8")
	fmt.Fprintf(w, `#!/bin/sh
set -eu
curl -fsSL https://raw.githubusercontent.com/Ctrl-Creeper/mcmon-agent/main/install.sh | sudo sh -s -- \
  --host-url '%s' \
  --agent-id '%s' \
  --token '%s' \
  --config-base64 '%s'
`, shellQuote(cfg.HostURL), shellQuote(cfg.AgentID), shellQuote(cfg.Token), shellQuote(cfg.ConfigBase64))
}

func writeInstallPowerShell(w http.ResponseWriter, r *http.Request, st *store.Store, agentID string, opts Options) {
	cfg, ok := buildAgentInstallPayload(w, r, st, agentID, opts)
	if !ok {
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintf(w, `$ErrorActionPreference = 'Stop'
$installer = Join-Path $env:TEMP 'mcmon-agent-install.ps1'
Invoke-WebRequest -UseBasicParsing 'https://raw.githubusercontent.com/Ctrl-Creeper/mcmon-agent/main/install.ps1' -OutFile $installer
& $installer -HostUrl '%s' -AgentId '%s' -Token '%s' -ConfigBase64 '%s'
`, psQuote(cfg.HostURL), psQuote(cfg.AgentID), psQuote(cfg.Token), psQuote(cfg.ConfigBase64))
}

type agentInstallPayload struct {
	HostURL      string
	AgentID      string
	Token        string
	ConfigBase64 string
}

func buildAgentInstallPayload(w http.ResponseWriter, r *http.Request, st *store.Store, agentID string, opts Options) (agentInstallPayload, bool) {
	token := r.URL.Query().Get("token")
	agent, ok, err := st.AgentByInstallToken(token)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return agentInstallPayload{}, false
	}
	if !ok || agent.ID != agentID {
		http.Error(w, "invalid install token", http.StatusUnauthorized)
		return agentInstallPayload{}, false
	}
	targets, err := st.TargetsForAgent(agent.ID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return agentInstallPayload{}, false
	}
	hostURL := strings.TrimRight(opts.PublicURL, "/")
	if hostURL == "" {
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		hostURL = scheme + "://" + r.Host
	}
	cfg := agentInstallConfig{HostURL: hostURL, AgentID: agent.ID, Token: agent.Token, Targets: targets}
	raw, err := json.Marshal(cfg)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return agentInstallPayload{}, false
	}
	configB64 := base64.StdEncoding.EncodeToString(raw)
	return agentInstallPayload{HostURL: hostURL, AgentID: agent.ID, Token: agent.Token, ConfigBase64: configB64}, true
}

func shellQuote(s string) string {
	return strings.ReplaceAll(s, `'`, `'\''`)
}

func psQuote(s string) string {
	return strings.ReplaceAll(s, `'`, `''`)
}

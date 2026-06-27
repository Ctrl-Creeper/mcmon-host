// Package rpc defines the JSON-RPC 2.0 message types used between
// mcmon-agent and mcmon-host over a WebSocket connection.
package rpc

import "encoding/json"

// --- JSON-RPC envelope ---

type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func NewRequest(id any, method string, params any) ([]byte, error) {
	p, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	return json.Marshal(Request{JSONRPC: "2.0", ID: id, Method: method, Params: p})
}

func OKResponse(id any, result any) ([]byte, error) {
	r, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	return json.Marshal(Response{JSONRPC: "2.0", ID: id, Result: r})
}

func ErrResponse(id any, code int, msg string) ([]byte, error) {
	return json.Marshal(Response{JSONRPC: "2.0", ID: id, Error: &RPCError{Code: code, Message: msg}})
}

// --- Payloads ---

// AgentHello is sent right after WebSocket connect so the host knows
// what this agent is and what MC servers it monitors.
type AgentHello struct {
	Version string       `json:"version"`
	Targets []AgentTarget `json:"targets"`
}

type AgentTarget struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Host string `json:"host"`
	Port int    `json:"port"`
}

// PingResult is sent periodically by the agent — one per target per burst.
type PingResult struct {
	TargetID string   `json:"target_id"`
	Ts       int64    `json:"ts"`
	MinMs    *float64 `json:"min_ms"`
	P50Ms    *float64 `json:"p50_ms"`
	MaxMs    *float64 `json:"max_ms"`
	LossPct  float64  `json:"loss_pct"`
}

// AutoDiscoverResp is returned by the host when an agent registers.
type AutoDiscoverResp struct {
	AgentID string `json:"agent_id"`
	Token   string `json:"token"`
}

// Package hub manages connected agents over WebSocket, dispatching
// incoming JSON-RPC messages and tracking online/offline state.
package hub

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/lewiswu/mcmon-host/internal/rpc"
	"github.com/lewiswu/mcmon-host/internal/store"
)

const (
	heartbeatInterval = 30 * time.Second
	readDeadline      = 90 * time.Second
)

type ConnectedAgent struct {
	Agent     store.Agent
	ConnSince time.Time
}

type Hub struct {
	mu      sync.RWMutex
	st      *store.Store
	clients map[string]*conn // agentID -> conn
}

type conn struct {
	ws    *websocket.Conn
	agent store.Agent
	since time.Time
}

func New(st *store.Store) *Hub {
	return &Hub{st: st, clients: make(map[string]*conn)}
}

// Online returns all currently connected agents.
func (h *Hub) Online() []ConnectedAgent {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]ConnectedAgent, 0, len(h.clients))
	for _, c := range h.clients {
		out = append(out, ConnectedAgent{Agent: c.agent, ConnSince: c.since})
	}
	return out
}

func (h *Hub) IsOnline(agentID string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	_, ok := h.clients[agentID]
	return ok
}

// HandleConn runs the read loop for one authenticated agent WebSocket.
// It blocks until the connection closes.
func (h *Hub) HandleConn(ws *websocket.Conn, agent store.Agent) {
	c := &conn{ws: ws, agent: agent, since: time.Now()}

	h.mu.Lock()
	if old, ok := h.clients[agent.ID]; ok {
		old.ws.Close()
	}
	h.clients[agent.ID] = c
	h.mu.Unlock()

	log.Printf("agent %s (%s) connected", agent.ID, agent.Name)

	// Heartbeat writer
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(heartbeatInterval)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				if err := ws.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second)); err != nil {
					return
				}
			}
		}
	}()

	defer func() {
		close(done)
		h.mu.Lock()
		if h.clients[agent.ID] == c {
			delete(h.clients, agent.ID)
		}
		h.mu.Unlock()
		ws.Close()
		log.Printf("agent %s (%s) disconnected", agent.ID, agent.Name)
	}()

	ws.SetReadDeadline(time.Now().Add(readDeadline))
	ws.SetReadLimit(1 << 20)
	ws.SetPongHandler(func(string) error {
		ws.SetReadDeadline(time.Now().Add(readDeadline))
		return nil
	})

	for {
		_, msg, err := ws.ReadMessage()
		if err != nil {
			return
		}
		ws.SetReadDeadline(time.Now().Add(readDeadline))
		h.handleMessage(c, msg)
	}
}

func (h *Hub) handleMessage(c *conn, raw []byte) {
	var req rpc.Request
	if err := json.Unmarshal(raw, &req); err != nil {
		log.Printf("agent %s: bad rpc: %v", c.agent.ID, err)
		return
	}

	switch req.Method {
	case "agent.hello":
		h.onHello(c, req)
	case "agent.pingResult":
		h.onPingResult(c, req)
	case "agent.metricResult":
		h.onMetricResult(c, req)
	default:
		resp, _ := rpc.ErrResponse(req.ID, -32601, "unknown method")
		c.ws.WriteMessage(websocket.TextMessage, resp)
	}
}

func (h *Hub) HandleRPC(agent store.Agent, raw []byte) ([]byte, int) {
	var req rpc.Request
	if err := json.Unmarshal(raw, &req); err != nil {
		resp, _ := rpc.ErrResponse(nil, -32700, "parse error")
		return resp, 400
	}
	switch req.Method {
	case "agent.hello":
		resp := h.onHelloRequest(&conn{agent: agent}, req)
		return resp, statusFromRPC(resp)
	case "agent.pingResult":
		resp := h.onPingResultRequest(&conn{agent: agent}, req)
		return resp, statusFromRPC(resp)
	case "agent.metricResult":
		resp := h.onMetricResultRequest(&conn{agent: agent}, req)
		return resp, statusFromRPC(resp)
	default:
		resp, _ := rpc.ErrResponse(req.ID, -32601, "unknown method")
		return resp, 400
	}
}

func (h *Hub) onMetricResult(c *conn, req rpc.Request) {
	resp := h.onMetricResultRequest(c, req)
	if c.ws != nil && req.ID != nil {
		c.ws.WriteMessage(websocket.TextMessage, resp)
	}
}

func (h *Hub) onMetricResultRequest(c *conn, req rpc.Request) []byte {
	var mr rpc.MetricResult
	if err := json.Unmarshal(req.Params, &mr); err != nil {
		log.Printf("agent %s: bad metricResult: %v", c.agent.ID, err)
		resp, _ := rpc.ErrResponse(req.ID, -32602, "invalid metricResult params")
		return resp
	}
	if err := validateMetricResult(mr); err != nil {
		resp, _ := rpc.ErrResponse(req.ID, -32602, err.Error())
		return resp
	}
	sm := store.MetricSample{
		AgentID: c.agent.ID, TargetID: mr.TargetID, Metric: mr.Metric, Ts: mr.Ts, Value: mr.Value, Extra: mr.Extra,
	}
	if err := h.st.InsertMetricSample(sm); err != nil {
		log.Printf("agent %s: insert metric sample: %v", c.agent.ID, err)
		resp, _ := rpc.ErrResponse(req.ID, -32000, "failed to save metric sample")
		return resp
	}
	_ = h.st.TouchAgent(c.agent.ID, c.agent.Version, time.Now().Unix())
	resp, _ := rpc.OKResponse(req.ID, "ok")
	return resp
}

func (h *Hub) onHello(c *conn, req rpc.Request) {
	resp := h.onHelloRequest(c, req)
	if c.ws != nil {
		c.ws.WriteMessage(websocket.TextMessage, resp)
	}
}

func (h *Hub) onHelloRequest(c *conn, req rpc.Request) []byte {
	var hello rpc.AgentHello
	if err := json.Unmarshal(req.Params, &hello); err != nil {
		log.Printf("agent %s: bad hello: %v", c.agent.ID, err)
		resp, _ := rpc.ErrResponse(req.ID, -32602, "invalid hello params")
		return resp
	}
	if err := validateHello(hello); err != nil {
		resp, _ := rpc.ErrResponse(req.ID, -32602, err.Error())
		return resp
	}

	targets := make([]store.AgentTarget, len(hello.Targets))
	for i, t := range hello.Targets {
		targets[i] = store.AgentTarget{
			AgentID: c.agent.ID, TargetID: t.ID,
			Name: t.Name, Host: t.Host, Port: t.Port,
		}
	}
	if err := h.st.UpsertTargets(c.agent.ID, targets); err != nil {
		log.Printf("agent %s: upsert targets: %v", c.agent.ID, err)
		resp, _ := rpc.ErrResponse(req.ID, -32000, "failed to save targets")
		return resp
	}
	if err := h.st.TouchAgent(c.agent.ID, hello.Version, time.Now().Unix()); err != nil {
		log.Printf("agent %s: touch: %v", c.agent.ID, err)
	}
	log.Printf("agent %s: registered %d targets", c.agent.ID, len(targets))

	resp, _ := rpc.OKResponse(req.ID, "ok")
	return resp
}

func (h *Hub) onPingResult(c *conn, req rpc.Request) {
	resp := h.onPingResultRequest(c, req)
	if c.ws != nil && req.ID != nil {
		c.ws.WriteMessage(websocket.TextMessage, resp)
	}
}

func (h *Hub) onPingResultRequest(c *conn, req rpc.Request) []byte {
	var pr rpc.PingResult
	if err := json.Unmarshal(req.Params, &pr); err != nil {
		log.Printf("agent %s: bad pingResult: %v", c.agent.ID, err)
		resp, _ := rpc.ErrResponse(req.ID, -32602, "invalid pingResult params")
		return resp
	}
	if err := validatePingResult(pr); err != nil {
		resp, _ := rpc.ErrResponse(req.ID, -32602, err.Error())
		return resp
	}

	sm := store.Sample{
		AgentID:  c.agent.ID,
		TargetID: pr.TargetID,
		Ts:       pr.Ts,
		MinMs:    pr.MinMs,
		P50Ms:    pr.P50Ms,
		MaxMs:    pr.MaxMs,
		LossPct:  pr.LossPct,
	}
	if err := h.st.InsertSample(sm); err != nil {
		log.Printf("agent %s: insert sample: %v", c.agent.ID, err)
		resp, _ := rpc.ErrResponse(req.ID, -32000, "failed to save sample")
		return resp
	}
	_ = h.st.TouchAgent(c.agent.ID, c.agent.Version, time.Now().Unix())
	resp, _ := rpc.OKResponse(req.ID, "ok")
	return resp
}

func validateHello(hello rpc.AgentHello) error {
	if len(hello.Version) > 64 {
		return errors.New("version is too long")
	}
	if len(hello.Targets) > 200 {
		return errors.New("too many targets")
	}
	for _, t := range hello.Targets {
		if t.ID == "" || len(t.ID) > 128 {
			return errors.New("target id is required")
		}
		if t.Host == "" || len(t.Host) > 255 {
			return fmt.Errorf("target %s host is required", t.ID)
		}
		if t.Port <= 0 || t.Port > 65535 {
			return fmt.Errorf("target %s port is invalid", t.ID)
		}
	}
	return nil
}

func validatePingResult(pr rpc.PingResult) error {
	if pr.TargetID == "" || len(pr.TargetID) > 128 {
		return errors.New("target_id is required")
	}
	if pr.Ts <= 0 {
		return errors.New("ts is required")
	}
	if pr.LossPct < 0 || pr.LossPct > 1 || math.IsNaN(pr.LossPct) || math.IsInf(pr.LossPct, 0) {
		return errors.New("loss_pct must be between 0 and 1")
	}
	for _, v := range []*float64{pr.MinMs, pr.P50Ms, pr.MaxMs} {
		if v != nil && (*v < 0 || math.IsNaN(*v) || math.IsInf(*v, 0)) {
			return errors.New("latency values must be finite and non-negative")
		}
	}
	return nil
}

func validateMetricResult(mr rpc.MetricResult) error {
	if mr.TargetID == "" || len(mr.TargetID) > 128 {
		return errors.New("target_id is required")
	}
	switch mr.Metric {
	case "online", "players", "latency", "loss":
	default:
		return errors.New("metric is invalid")
	}
	if mr.Ts <= 0 {
		return errors.New("ts is required")
	}
	if mr.Value != nil && (*mr.Value < 0 || math.IsNaN(*mr.Value) || math.IsInf(*mr.Value, 0)) {
		return errors.New("value must be finite and non-negative")
	}
	if len(mr.Extra) > 4096 {
		return errors.New("extra is too large")
	}
	return nil
}

func statusFromRPC(resp []byte) int {
	var r rpc.Response
	if err := json.Unmarshal(resp, &r); err != nil || r.Error != nil {
		return 400
	}
	return 200
}

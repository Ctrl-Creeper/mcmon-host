// Package store manages SQLite persistence for agents and their ping data.
package store

import (
	"database/sql"
	"encoding/json"
	"fmt"

	_ "modernc.org/sqlite"
)

const defaultProtocolVersion = 760

type Agent struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Token        string `json:"-"`
	InstallToken string `json:"-"`
	Version      string `json:"version,omitempty"`
	LastSeen     int64  `json:"last_seen,omitempty"`
	Disabled     bool   `json:"disabled,omitempty"`
}

type AgentTarget struct {
	AgentID   string   `json:"agent_id"`
	TargetID  string   `json:"target_id"`
	Name      string   `json:"name"`
	Host      string   `json:"host"`
	Port      int      `json:"port"`
	TimeoutMs int      `json:"timeout_ms"`
	Monitors  Monitors `json:"monitors"`
}

type Monitors struct {
	Online  SimpleMonitor `json:"online"`
	Players SimpleMonitor `json:"players"`
	Latency ProbeMonitor  `json:"latency"`
	Loss    ProbeMonitor  `json:"loss"`
}

type SimpleMonitor struct {
	Enabled     bool `json:"enabled"`
	IntervalSec int  `json:"interval_sec"`
}

type ProbeMonitor struct {
	Enabled         bool `json:"enabled"`
	IntervalSec     int  `json:"interval_sec"`
	ProbesPerBurst  int  `json:"probes_per_burst"`
	ProbeGapMs      int  `json:"probe_gap_ms"`
	ProtocolVersion int  `json:"protocol_version,omitempty"`
}

type Sample struct {
	AgentID  string   `json:"agent_id"`
	TargetID string   `json:"target_id"`
	Ts       int64    `json:"ts"`
	MinMs    *float64 `json:"min_ms"`
	P50Ms    *float64 `json:"p50_ms"`
	MaxMs    *float64 `json:"max_ms"`
	LossPct  float64  `json:"loss_pct"`
}

type MetricSample struct {
	AgentID  string   `json:"agent_id"`
	TargetID string   `json:"target_id"`
	Metric   string   `json:"metric"`
	Ts       int64    `json:"ts"`
	Value    *float64 `json:"value"`
	Extra    string   `json:"extra"`
}

type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS agents (
			id        TEXT PRIMARY KEY,
			name      TEXT NOT NULL,
			token     TEXT NOT NULL UNIQUE,
			install_token TEXT NOT NULL DEFAULT '',
			version   TEXT NOT NULL DEFAULT '',
			last_seen INTEGER NOT NULL DEFAULT 0,
			disabled  INTEGER NOT NULL DEFAULT 0
		);
		CREATE TABLE IF NOT EXISTS agent_targets (
			agent_id  TEXT NOT NULL,
			target_id TEXT NOT NULL,
			name      TEXT NOT NULL,
			host      TEXT NOT NULL,
			port      INTEGER NOT NULL,
			timeout_ms INTEGER NOT NULL DEFAULT 1500,
			monitors_json TEXT NOT NULL DEFAULT '',
			PRIMARY KEY (agent_id, target_id)
		);
		CREATE TABLE IF NOT EXISTS samples (
			agent_id  TEXT NOT NULL,
			target_id TEXT NOT NULL,
			ts        INTEGER NOT NULL,
			min_ms    REAL,
			p50_ms    REAL,
			max_ms    REAL,
			loss_pct  REAL NOT NULL,
			PRIMARY KEY (agent_id, target_id, ts)
		);
		CREATE INDEX IF NOT EXISTS idx_samples_lookup ON samples(agent_id, target_id, ts);
		CREATE TABLE IF NOT EXISTS metric_samples (
			agent_id  TEXT NOT NULL,
			target_id TEXT NOT NULL,
			metric    TEXT NOT NULL,
			ts        INTEGER NOT NULL,
			value     REAL,
			extra     TEXT NOT NULL DEFAULT '',
			PRIMARY KEY (agent_id, target_id, metric, ts)
		);
		CREATE INDEX IF NOT EXISTS idx_metric_samples_lookup ON metric_samples(agent_id, target_id, metric, ts);
	`); err != nil {
		db.Close()
		return nil, err
	}
	for _, stmt := range []string{
		`ALTER TABLE agents ADD COLUMN install_token TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE agents ADD COLUMN version TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE agents ADD COLUMN last_seen INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE agents ADD COLUMN disabled INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE agent_targets ADD COLUMN timeout_ms INTEGER NOT NULL DEFAULT 1500`,
		`ALTER TABLE agent_targets ADD COLUMN monitors_json TEXT NOT NULL DEFAULT ''`,
	} {
		_, _ = db.Exec(stmt)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

func defaultMonitors() Monitors {
	return Monitors{
		Online:  SimpleMonitor{Enabled: true, IntervalSec: 60},
		Players: SimpleMonitor{Enabled: true, IntervalSec: 60},
		Latency: ProbeMonitor{Enabled: true, IntervalSec: 60, ProbesPerBurst: 5, ProbeGapMs: 1500, ProtocolVersion: defaultProtocolVersion},
		Loss:    ProbeMonitor{Enabled: true, IntervalSec: 60, ProbesPerBurst: 5, ProbeGapMs: 1500, ProtocolVersion: defaultProtocolVersion},
	}
}

func encodeMonitors(m Monitors) (string, error) {
	m = normalizeMonitors(m)
	b, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func decodeMonitors(raw string) Monitors {
	m := defaultMonitors()
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &m)
	}
	return normalizeMonitors(m)
}

func normalizeMonitors(m Monitors) Monitors {
	if m.Online.IntervalSec <= 0 {
		m.Online.IntervalSec = 60
	}
	if m.Players.IntervalSec <= 0 {
		m.Players.IntervalSec = 60
	}
	if m.Latency.IntervalSec <= 0 {
		m.Latency.IntervalSec = 60
	}
	if m.Latency.ProbesPerBurst <= 0 {
		m.Latency.ProbesPerBurst = 5
	}
	if m.Latency.ProbeGapMs <= 0 {
		m.Latency.ProbeGapMs = 1500
	}
	if m.Latency.ProtocolVersion <= 0 {
		m.Latency.ProtocolVersion = defaultProtocolVersion
	}
	if m.Loss.IntervalSec <= 0 {
		m.Loss.IntervalSec = 60
	}
	if m.Loss.ProbesPerBurst <= 0 {
		m.Loss.ProbesPerBurst = 5
	}
	if m.Loss.ProbeGapMs <= 0 {
		m.Loss.ProbeGapMs = 1500
	}
	return m
}

func normalizedTimeout(timeoutMs int) int {
	if timeoutMs <= 0 {
		return 1500
	}
	return timeoutMs
}

// --- Agents ---

func (s *Store) CreateAgent(a Agent) error {
	_, err := s.db.Exec(`INSERT INTO agents (id, name, token, install_token, version, last_seen, disabled) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		a.ID, a.Name, a.Token, a.InstallToken, a.Version, a.LastSeen, a.Disabled)
	return err
}

func (s *Store) UpdateAgent(a Agent) error {
	_, err := s.db.Exec(`UPDATE agents SET name=?, token=?, install_token=?, version=?, last_seen=?, disabled=? WHERE id=?`,
		a.Name, a.Token, a.InstallToken, a.Version, a.LastSeen, a.Disabled, a.ID)
	return err
}

func (s *Store) AgentByToken(token string) (Agent, bool, error) {
	var a Agent
	err := s.db.QueryRow(`SELECT id, name, token, install_token, version, last_seen, disabled FROM agents WHERE token=? AND disabled=0`, token).
		Scan(&a.ID, &a.Name, &a.Token, &a.InstallToken, &a.Version, &a.LastSeen, &a.Disabled)
	if err == sql.ErrNoRows {
		return Agent{}, false, nil
	}
	return a, err == nil, err
}

func (s *Store) AgentByInstallToken(token string) (Agent, bool, error) {
	var a Agent
	err := s.db.QueryRow(`SELECT id, name, token, install_token, version, last_seen, disabled FROM agents WHERE install_token=? AND install_token<>'' AND disabled=0`, token).
		Scan(&a.ID, &a.Name, &a.Token, &a.InstallToken, &a.Version, &a.LastSeen, &a.Disabled)
	if err == sql.ErrNoRows {
		return Agent{}, false, nil
	}
	return a, err == nil, err
}

func (s *Store) ListAgents() ([]Agent, error) {
	rows, err := s.db.Query(`SELECT id, name, token, install_token, version, last_seen, disabled FROM agents ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Agent
	for rows.Next() {
		var a Agent
		if err := rows.Scan(&a.ID, &a.Name, &a.Token, &a.InstallToken, &a.Version, &a.LastSeen, &a.Disabled); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) TouchAgent(agentID, version string, ts int64) error {
	_, err := s.db.Exec(`UPDATE agents SET version=?, last_seen=? WHERE id=?`, version, ts, agentID)
	return err
}

// --- Agent targets ---

func (s *Store) UpsertTargets(agentID string, targets []AgentTarget) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM agent_targets WHERE agent_id=?`, agentID); err != nil {
		return err
	}
	for _, t := range targets {
		monitorsJSON, err := encodeMonitors(t.Monitors)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(
			`INSERT INTO agent_targets (agent_id, target_id, name, host, port, timeout_ms, monitors_json) VALUES (?,?,?,?,?,?,?)`,
			agentID, t.TargetID, t.Name, t.Host, t.Port, normalizedTimeout(t.TimeoutMs), monitorsJSON,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) TargetsForAgent(agentID string) ([]AgentTarget, error) {
	rows, err := s.db.Query(`SELECT agent_id, target_id, name, host, port, timeout_ms, monitors_json FROM agent_targets WHERE agent_id=?`, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AgentTarget
	for rows.Next() {
		var t AgentTarget
		var monitorsJSON string
		if err := rows.Scan(&t.AgentID, &t.TargetID, &t.Name, &t.Host, &t.Port, &t.TimeoutMs, &monitorsJSON); err != nil {
			return nil, err
		}
		t.Monitors = decodeMonitors(monitorsJSON)
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) AllTargets() ([]AgentTarget, error) {
	rows, err := s.db.Query(`SELECT agent_id, target_id, name, host, port, timeout_ms, monitors_json FROM agent_targets ORDER BY agent_id, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AgentTarget
	for rows.Next() {
		var t AgentTarget
		var monitorsJSON string
		if err := rows.Scan(&t.AgentID, &t.TargetID, &t.Name, &t.Host, &t.Port, &t.TimeoutMs, &monitorsJSON); err != nil {
			return nil, err
		}
		t.Monitors = decodeMonitors(monitorsJSON)
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) InsertMetricSample(sm MetricSample) error {
	_, err := s.db.Exec(
		`INSERT OR REPLACE INTO metric_samples (agent_id, target_id, metric, ts, value, extra) VALUES (?,?,?,?,?,?)`,
		sm.AgentID, sm.TargetID, sm.Metric, sm.Ts, sm.Value, sm.Extra,
	)
	return err
}

func (s *Store) MetricSeries(agentID, targetID, metric string, sinceTs int64) ([]MetricSample, error) {
	rows, err := s.db.Query(
		`SELECT agent_id, target_id, metric, ts, value, extra FROM metric_samples WHERE agent_id=? AND target_id=? AND metric=? AND ts>=? ORDER BY ts`,
		agentID, targetID, metric, sinceTs,
	)
	if err != nil {
		return nil, fmt.Errorf("query metric series: %w", err)
	}
	defer rows.Close()
	var out []MetricSample
	for rows.Next() {
		var sm MetricSample
		if err := rows.Scan(&sm.AgentID, &sm.TargetID, &sm.Metric, &sm.Ts, &sm.Value, &sm.Extra); err != nil {
			return nil, err
		}
		out = append(out, sm)
	}
	return out, rows.Err()
}

// --- Samples ---

func (s *Store) InsertSample(sm Sample) error {
	_, err := s.db.Exec(
		`INSERT OR REPLACE INTO samples (agent_id, target_id, ts, min_ms, p50_ms, max_ms, loss_pct) VALUES (?,?,?,?,?,?,?)`,
		sm.AgentID, sm.TargetID, sm.Ts, sm.MinMs, sm.P50Ms, sm.MaxMs, sm.LossPct,
	)
	return err
}

func (s *Store) Series(agentID, targetID string, sinceTs int64) ([]Sample, error) {
	rows, err := s.db.Query(
		`SELECT agent_id, target_id, ts, min_ms, p50_ms, max_ms, loss_pct FROM samples WHERE agent_id=? AND target_id=? AND ts>=? ORDER BY ts`,
		agentID, targetID, sinceTs,
	)
	if err != nil {
		return nil, fmt.Errorf("query series: %w", err)
	}
	defer rows.Close()
	var out []Sample
	for rows.Next() {
		var sm Sample
		if err := rows.Scan(&sm.AgentID, &sm.TargetID, &sm.Ts, &sm.MinMs, &sm.P50Ms, &sm.MaxMs, &sm.LossPct); err != nil {
			return nil, err
		}
		out = append(out, sm)
	}
	return out, rows.Err()
}

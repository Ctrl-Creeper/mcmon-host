// Package store manages SQLite persistence for agents and their ping data.
package store

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

type Agent struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Token    string `json:"-"`
	Version  string `json:"version,omitempty"`
	LastSeen int64  `json:"last_seen,omitempty"`
	Disabled bool   `json:"disabled,omitempty"`
}

type AgentTarget struct {
	AgentID  string `json:"agent_id"`
	TargetID string `json:"target_id"`
	Name     string `json:"name"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
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
	`); err != nil {
		db.Close()
		return nil, err
	}
	for _, stmt := range []string{
		`ALTER TABLE agents ADD COLUMN version TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE agents ADD COLUMN last_seen INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE agents ADD COLUMN disabled INTEGER NOT NULL DEFAULT 0`,
	} {
		_, _ = db.Exec(stmt)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

// --- Agents ---

func (s *Store) CreateAgent(a Agent) error {
	_, err := s.db.Exec(`INSERT INTO agents (id, name, token, version, last_seen, disabled) VALUES (?, ?, ?, ?, ?, ?)`,
		a.ID, a.Name, a.Token, a.Version, a.LastSeen, a.Disabled)
	return err
}

func (s *Store) AgentByToken(token string) (Agent, bool, error) {
	var a Agent
	err := s.db.QueryRow(`SELECT id, name, token, version, last_seen, disabled FROM agents WHERE token=? AND disabled=0`, token).
		Scan(&a.ID, &a.Name, &a.Token, &a.Version, &a.LastSeen, &a.Disabled)
	if err == sql.ErrNoRows {
		return Agent{}, false, nil
	}
	return a, err == nil, err
}

func (s *Store) ListAgents() ([]Agent, error) {
	rows, err := s.db.Query(`SELECT id, name, version, last_seen, disabled FROM agents ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Agent
	for rows.Next() {
		var a Agent
		if err := rows.Scan(&a.ID, &a.Name, &a.Version, &a.LastSeen, &a.Disabled); err != nil {
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
		if _, err := tx.Exec(
			`INSERT INTO agent_targets (agent_id, target_id, name, host, port) VALUES (?,?,?,?,?)`,
			agentID, t.TargetID, t.Name, t.Host, t.Port,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) TargetsForAgent(agentID string) ([]AgentTarget, error) {
	rows, err := s.db.Query(`SELECT agent_id, target_id, name, host, port FROM agent_targets WHERE agent_id=?`, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AgentTarget
	for rows.Next() {
		var t AgentTarget
		if err := rows.Scan(&t.AgentID, &t.TargetID, &t.Name, &t.Host, &t.Port); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) AllTargets() ([]AgentTarget, error) {
	rows, err := s.db.Query(`SELECT agent_id, target_id, name, host, port FROM agent_targets ORDER BY agent_id, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AgentTarget
	for rows.Next() {
		var t AgentTarget
		if err := rows.Scan(&t.AgentID, &t.TargetID, &t.Name, &t.Host, &t.Port); err != nil {
			return nil, err
		}
		out = append(out, t)
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

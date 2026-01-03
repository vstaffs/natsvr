package cloud

import (
	"database/sql"
	"time"

	_ "modernc.org/sqlite"
)

// ForwardRule represents a port forwarding rule
type ForwardRule struct {
	ID            string
	Name          string
	Type          string // "local", "remote", "p2p", "cloud-self"
	Protocol      string // "tcp", "udp"
	SourceAgentID string
	ListenPort    int
	TargetAgentID string
	TargetHost    string
	TargetPort    int
	Enabled       bool
	RateLimit     int64 // bytes per second, 0 = unlimited
	TrafficLimit  int64 // max total bytes, 0 = unlimited
	TrafficUsed   int64 // current traffic used
	CreatedAt     time.Time
}

// Token represents an authentication token
type Token struct {
	ID         string
	Name       string
	Token      string
	UsageCount int
	CreatedAt  time.Time
}

// Store handles database operations
type Store struct {
	db *sql.DB
}

// NewStore creates a new store
func NewStore(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}

	store := &Store{db: db}
	if err := store.migrate(); err != nil {
		return nil, err
	}

	return store, nil
}

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS forward_rules (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			type TEXT NOT NULL,
			protocol TEXT NOT NULL,
			source_agent_id TEXT,
			listen_port INTEGER NOT NULL,
			target_agent_id TEXT,
			target_host TEXT NOT NULL,
			target_port INTEGER NOT NULL,
			enabled INTEGER NOT NULL DEFAULT 1,
			rate_limit INTEGER NOT NULL DEFAULT 0,
			traffic_limit INTEGER NOT NULL DEFAULT 0,
			traffic_used INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);

		CREATE TABLE IF NOT EXISTS tokens (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			token TEXT NOT NULL UNIQUE,
			usage_count INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);
	`)
	if err != nil {
		return err
	}

	// Migration: add new columns if they don't exist
	s.db.Exec("ALTER TABLE forward_rules ADD COLUMN rate_limit INTEGER NOT NULL DEFAULT 0")
	s.db.Exec("ALTER TABLE forward_rules ADD COLUMN traffic_limit INTEGER NOT NULL DEFAULT 0")
	s.db.Exec("ALTER TABLE forward_rules ADD COLUMN traffic_used INTEGER NOT NULL DEFAULT 0")

	return nil
}

// Close closes the database
func (s *Store) Close() error {
	return s.db.Close()
}

// Forward Rules

func (s *Store) GetForwardRules() ([]*ForwardRule, error) {
	rows, err := s.db.Query(`
		SELECT id, name, type, protocol, source_agent_id, listen_port, 
		       target_agent_id, target_host, target_port, enabled,
		       rate_limit, traffic_limit, traffic_used, created_at
		FROM forward_rules
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []*ForwardRule
	for rows.Next() {
		r := &ForwardRule{}
		var sourceAgentID, targetAgentID sql.NullString
		err := rows.Scan(
			&r.ID, &r.Name, &r.Type, &r.Protocol, &sourceAgentID,
			&r.ListenPort, &targetAgentID, &r.TargetHost, &r.TargetPort,
			&r.Enabled, &r.RateLimit, &r.TrafficLimit, &r.TrafficUsed, &r.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		if sourceAgentID.Valid {
			r.SourceAgentID = sourceAgentID.String
		}
		if targetAgentID.Valid {
			r.TargetAgentID = targetAgentID.String
		}
		rules = append(rules, r)
	}

	return rules, nil
}

func (s *Store) GetForwardRule(id string) (*ForwardRule, error) {
	r := &ForwardRule{}
	var sourceAgentID, targetAgentID sql.NullString
	err := s.db.QueryRow(`
		SELECT id, name, type, protocol, source_agent_id, listen_port, 
		       target_agent_id, target_host, target_port, enabled,
		       rate_limit, traffic_limit, traffic_used, created_at
		FROM forward_rules WHERE id = ?
	`, id).Scan(
		&r.ID, &r.Name, &r.Type, &r.Protocol, &sourceAgentID,
		&r.ListenPort, &targetAgentID, &r.TargetHost, &r.TargetPort,
		&r.Enabled, &r.RateLimit, &r.TrafficLimit, &r.TrafficUsed, &r.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	if sourceAgentID.Valid {
		r.SourceAgentID = sourceAgentID.String
	}
	if targetAgentID.Valid {
		r.TargetAgentID = targetAgentID.String
	}
	return r, nil
}

func (s *Store) CreateForwardRule(r *ForwardRule) error {
	r.CreatedAt = time.Now()
	_, err := s.db.Exec(`
		INSERT INTO forward_rules (id, name, type, protocol, source_agent_id, 
		                           listen_port, target_agent_id, target_host, 
		                           target_port, enabled, rate_limit, traffic_limit,
		                           traffic_used, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, r.ID, r.Name, r.Type, r.Protocol, r.SourceAgentID,
		r.ListenPort, r.TargetAgentID, r.TargetHost, r.TargetPort,
		r.Enabled, r.RateLimit, r.TrafficLimit, r.TrafficUsed, r.CreatedAt)
	return err
}

func (s *Store) UpdateForwardRule(r *ForwardRule) error {
	_, err := s.db.Exec(`
		UPDATE forward_rules 
		SET name = ?, type = ?, protocol = ?, source_agent_id = ?,
		    listen_port = ?, target_agent_id = ?, target_host = ?,
		    target_port = ?, enabled = ?, rate_limit = ?, traffic_limit = ?,
		    traffic_used = ?
		WHERE id = ?
	`, r.Name, r.Type, r.Protocol, r.SourceAgentID,
		r.ListenPort, r.TargetAgentID, r.TargetHost, r.TargetPort,
		r.Enabled, r.RateLimit, r.TrafficLimit, r.TrafficUsed, r.ID)
	return err
}

// UpdateTrafficUsed updates only the traffic_used field
func (s *Store) UpdateTrafficUsed(id string, trafficUsed int64) error {
	_, err := s.db.Exec(`UPDATE forward_rules SET traffic_used = ? WHERE id = ?`, trafficUsed, id)
	return err
}

func (s *Store) DeleteForwardRule(id string) error {
	_, err := s.db.Exec("DELETE FROM forward_rules WHERE id = ?", id)
	return err
}

// Tokens

func (s *Store) GetTokens() ([]*Token, error) {
	rows, err := s.db.Query(`
		SELECT id, name, token, usage_count, created_at
		FROM tokens
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tokens []*Token
	for rows.Next() {
		t := &Token{}
		err := rows.Scan(&t.ID, &t.Name, &t.Token, &t.UsageCount, &t.CreatedAt)
		if err != nil {
			return nil, err
		}
		tokens = append(tokens, t)
	}

	return tokens, nil
}

func (s *Store) CreateToken(t *Token) error {
	t.CreatedAt = time.Now()
	_, err := s.db.Exec(`
		INSERT INTO tokens (id, name, token, usage_count, created_at)
		VALUES (?, ?, ?, ?, ?)
	`, t.ID, t.Name, t.Token, t.UsageCount, t.CreatedAt)
	return err
}

func (s *Store) DeleteToken(id string) error {
	_, err := s.db.Exec("DELETE FROM tokens WHERE id = ?", id)
	return err
}

func (s *Store) IncrementTokenUsage(id string) error {
	_, err := s.db.Exec("UPDATE tokens SET usage_count = usage_count + 1 WHERE id = ?", id)
	return err
}

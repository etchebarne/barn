package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/etchebarne/barn/internal/ids"
)

// ConnectorAccount is one authenticated account on an external service.
type ConnectorAccount struct {
	ID          string
	Type        string
	Name        string
	Credentials string // encrypted JSON
	Config      json.RawMessage
	CreatedAt   int64
}

const accountColumns = `id, type, name, credentials, config, created_at`

func scanAccount(row interface{ Scan(...any) error }) (ConnectorAccount, error) {
	var a ConnectorAccount
	var config string
	err := row.Scan(&a.ID, &a.Type, &a.Name, &a.Credentials, &config, &a.CreatedAt)
	a.Config = json.RawMessage(config)
	return a, err
}

func (s *Store) CreateAccount(ctx context.Context, a ConnectorAccount) (ConnectorAccount, error) {
	a.ID, a.CreatedAt = ids.New(), now()
	if len(a.Config) == 0 {
		a.Config = json.RawMessage(`{}`)
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO connector_accounts (`+accountColumns+`) VALUES (?, ?, ?, ?, ?, ?)`,
		a.ID, a.Type, a.Name, a.Credentials, string(a.Config), a.CreatedAt)
	return a, err
}

func (s *Store) UpdateAccount(ctx context.Context, a ConnectorAccount) error {
	res, err := s.db.ExecContext(ctx, `UPDATE connector_accounts SET name = ?, credentials = ?, config = ? WHERE id = ?`,
		a.Name, a.Credentials, string(a.Config), a.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) GetAccount(ctx context.Context, id string) (ConnectorAccount, error) {
	a, err := scanAccount(s.db.QueryRowContext(ctx, `SELECT `+accountColumns+` FROM connector_accounts WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return a, ErrNotFound
	}
	return a, err
}

// DeleteAccount removes an account with its grants, signals and the signal tasks waiting on it.
func (s *Store) DeleteAccount(ctx context.Context, id string) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `DELETE FROM connector_accounts WHERE id = ?`, id)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return ErrNotFound
		}
		_, err = tx.ExecContext(ctx, `DELETE FROM tasks WHERE kind = 'signal' AND json_extract(spec, '$.accountId') = ?`, id)
		return err
	})
}

func (s *Store) Accounts(ctx context.Context) ([]ConnectorAccount, error) {
	return s.queryAccounts(ctx, `SELECT `+accountColumns+` FROM connector_accounts ORDER BY id`)
}

// AgentAccounts returns the accounts an agent has been granted.
func (s *Store) AgentAccounts(ctx context.Context, agentID string) ([]ConnectorAccount, error) {
	return s.queryAccounts(ctx, `SELECT `+prefixed("c.", accountColumns)+` FROM connector_accounts c
		JOIN grants g ON g.account_id = c.id WHERE g.agent_id = ? ORDER BY c.id`, agentID)
}

func (s *Store) queryAccounts(ctx context.Context, q string, args ...any) ([]ConnectorAccount, error) {
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ConnectorAccount
	for rows.Next() {
		a, err := scanAccount(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// SetGrants replaces which agents may use an account.
func (s *Store) SetGrants(ctx context.Context, accountID string, agentIDs []string) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM grants WHERE account_id = ?`, accountID); err != nil {
			return err
		}
		for _, id := range agentIDs {
			if _, err := tx.ExecContext(ctx, `INSERT INTO grants (agent_id, account_id) VALUES (?, ?)`, id, accountID); err != nil {
				return err
			}
		}
		return nil
	})
}

// Grants returns the agents granted an account.
func (s *Store) Grants(ctx context.Context, accountID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT agent_id FROM grants WHERE account_id = ? ORDER BY agent_id`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// Signal is an event received from a connector.
type Signal struct {
	ID         string
	AccountID  string
	Type       string
	Payload    json.RawMessage
	ReceivedAt int64
}

func (s *Store) InsertSignal(ctx context.Context, sig Signal) (Signal, error) {
	sig.ID, sig.ReceivedAt = ids.New(), now()
	_, err := s.db.ExecContext(ctx, `INSERT INTO signals (id, account_id, type, payload, received_at) VALUES (?, ?, ?, ?, ?)`,
		sig.ID, sig.AccountID, sig.Type, string(sig.Payload), sig.ReceivedAt)
	return sig, err
}

func (s *Store) GetSignal(ctx context.Context, id string) (Signal, error) {
	var sig Signal
	var payload string
	err := s.db.QueryRowContext(ctx, `SELECT id, account_id, type, payload, received_at FROM signals WHERE id = ?`, id).
		Scan(&sig.ID, &sig.AccountID, &sig.Type, &payload, &sig.ReceivedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return sig, ErrNotFound
	}
	sig.Payload = json.RawMessage(payload)
	return sig, err
}

// SignalTasks returns enabled signal tasks of active agents granted the account.
func (s *Store) SignalTasks(ctx context.Context, accountID string) ([]Task, error) {
	return s.queryTasks(ctx, `SELECT `+prefixed("t.", taskColumns)+` FROM tasks t
		JOIN grants g ON g.agent_id = t.agent_id AND g.account_id = ?
		JOIN agents a ON a.id = t.agent_id AND a.archived_at IS NULL
		WHERE t.kind = 'signal' AND t.enabled = 1 AND json_extract(t.spec, '$.accountId') = ?
		ORDER BY t.id`, accountID, accountID)
}

func prefixed(prefix, columns string) string {
	var out []byte
	for i, part := range splitComma(columns) {
		if i > 0 {
			out = append(out, ", "...)
		}
		out = append(out, prefix+part...)
	}
	return string(out)
}

func splitComma(s string) []string {
	var parts []string
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == ',' {
			p := s[start:i]
			for len(p) > 0 && p[0] == ' ' {
				p = p[1:]
			}
			parts = append(parts, p)
			start = i + 1
		}
	}
	return parts
}

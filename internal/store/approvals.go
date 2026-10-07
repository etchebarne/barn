package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/etchebarne/openbot/internal/ids"
)

// ApprovalRule is an action the user always allows an agent to take without asking.
type ApprovalRule struct {
	ID        string
	AgentID   string
	Action    string            // see the approval_rules migration
	Match     map[string]string // argument values the call must have; empty = any
	Label     string            // what the user sees, e.g. "Slack message as you · Slack"
	CreatedAt int64
}

// AllowAction saves (or keeps) a standing approval, optionally limited to argument values.
func (s *Store) AllowAction(ctx context.Context, agentID, action string, match map[string]string, label string) error {
	if match == nil {
		match = map[string]string{}
	}
	m, _ := json.Marshal(match) // map keys marshal sorted, so equal matches are equal strings
	_, err := s.db.ExecContext(ctx, `INSERT INTO approval_rules (id, agent_id, action, match, label, created_at)
		VALUES (?, ?, ?, ?, ?, ?) ON CONFLICT (agent_id, action, match) DO UPDATE SET label = excluded.label`,
		ids.New(), agentID, action, string(m), label, now())
	return err
}

// ActionAllowed reports whether a standing approval covers this call: same action, and every
// value in the rule's match equal to the call's argument (ignoring case, spaces and a leading
// "#", so "#Alerts" matches "alerts").
func (s *Store) ActionAllowed(ctx context.Context, agentID, action string, args json.RawMessage) (bool, error) {
	rules, err := s.rulesFor(ctx, agentID, action)
	if err != nil {
		return false, err
	}
	var got map[string]any
	_ = json.Unmarshal(args, &got)
	for _, r := range rules {
		ok := true
		for k, want := range r.Match {
			v, present := got[k]
			if !present || normalizeMatch(fmt.Sprint(v)) != normalizeMatch(want) {
				ok = false
				break
			}
		}
		if ok {
			return true, nil
		}
	}
	return false, nil
}

func normalizeMatch(s string) string {
	return strings.ToLower(strings.TrimPrefix(strings.TrimSpace(s), "#"))
}

func (s *Store) rulesFor(ctx context.Context, agentID, action string) ([]ApprovalRule, error) {
	return s.queryRules(ctx, `SELECT id, agent_id, action, match, label, created_at FROM approval_rules
		WHERE agent_id = ? AND action = ?`, agentID, action)
}

func (s *Store) ApprovalRules(ctx context.Context, agentID string) ([]ApprovalRule, error) {
	return s.queryRules(ctx, `SELECT id, agent_id, action, match, label, created_at FROM approval_rules
		WHERE agent_id = ? ORDER BY created_at, id`, agentID)
}

func (s *Store) queryRules(ctx context.Context, q string, args ...any) ([]ApprovalRule, error) {
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ApprovalRule
	for rows.Next() {
		var r ApprovalRule
		var match string
		if err := rows.Scan(&r.ID, &r.AgentID, &r.Action, &match, &r.Label, &r.CreatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(match), &r.Match)
		out = append(out, r)
	}
	return out, rows.Err()
}

// RevokeApproval removes a standing approval, so the agent asks again.
func (s *Store) RevokeApproval(ctx context.Context, agentID, ruleID string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM approval_rules WHERE agent_id = ? AND id = ?`, agentID, ruleID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

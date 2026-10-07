package store

import (
	"context"
	"database/sql"
	"errors"

	"github.com/etchebarne/barn/internal/ids"
)

// SidebarCategory groups chats in the user's sidebar.
type SidebarCategory struct {
	ID        string
	Name      string
	Collapsed bool
}

// ErrBadLayout means a layout named an unknown chat or category.
var ErrBadLayout = errors.New("unknown chat or category in the layout")

func (s *Store) SidebarCategories(ctx context.Context) ([]SidebarCategory, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, collapsed FROM sidebar_categories ORDER BY position, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SidebarCategory{}
	for rows.Next() {
		var c SidebarCategory
		if err := rows.Scan(&c.ID, &c.Name, &c.Collapsed); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// CreateSidebarCategory adds a category at the end.
func (s *Store) CreateSidebarCategory(ctx context.Context, name string) (SidebarCategory, error) {
	c := SidebarCategory{ID: ids.New(), Name: name}
	_, err := s.db.ExecContext(ctx, `INSERT INTO sidebar_categories (id, name, position, created_at)
		VALUES (?, ?, (SELECT COALESCE(MAX(position), -1) + 1 FROM sidebar_categories), ?)`, c.ID, name, now())
	return c, err
}

// UpdateSidebarCategory renames and/or collapses a category (nil leaves a field as it is).
func (s *Store) UpdateSidebarCategory(ctx context.Context, id string, name *string, collapsed *bool) (SidebarCategory, error) {
	var c SidebarCategory
	err := s.tx(ctx, func(tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx, `SELECT id, name, collapsed FROM sidebar_categories WHERE id = ?`, id).
			Scan(&c.ID, &c.Name, &c.Collapsed); errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		} else if err != nil {
			return err
		}
		if name != nil {
			c.Name = *name
		}
		if collapsed != nil {
			c.Collapsed = *collapsed
		}
		_, err := tx.ExecContext(ctx, `UPDATE sidebar_categories SET name = ?, collapsed = ? WHERE id = ?`, c.Name, c.Collapsed, id)
		return err
	})
	return c, err
}

// DeleteSidebarCategory removes a category; its chats go back to Unassigned, unarranged.
func (s *Store) DeleteSidebarCategory(ctx context.Context, id string) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `UPDATE chats SET category_id = NULL, position = NULL WHERE category_id = ?`, id); err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx, `DELETE FROM sidebar_categories WHERE id = ?`, id)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return ErrNotFound
		}
		return nil
	})
}

// SidebarSection is one section of a layout: a category (nil = Unassigned) and its chats.
type SidebarSection struct {
	CategoryID *string
	ChatIDs    []string
}

// SetSidebarLayout orders the categories and places the chats of the given sections, in one
// transaction. categoryOrder must list every category.
func (s *Store) SetSidebarLayout(ctx context.Context, categoryOrder []string, sections []SidebarSection) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		var total int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM sidebar_categories`).Scan(&total); err != nil {
			return err
		}
		if total != len(categoryOrder) {
			return ErrBadLayout
		}
		for i, id := range categoryOrder {
			res, err := tx.ExecContext(ctx, `UPDATE sidebar_categories SET position = ? WHERE id = ?`, i, id)
			if err != nil {
				return err
			}
			if n, _ := res.RowsAffected(); n != 1 {
				return ErrBadLayout
			}
		}
		seen := map[string]bool{}
		for _, sec := range sections {
			for i, chatID := range sec.ChatIDs {
				if seen[chatID] {
					return ErrBadLayout
				}
				seen[chatID] = true
				res, err := tx.ExecContext(ctx, `UPDATE chats SET category_id = ?, position = ? WHERE id = ?`, sec.CategoryID, i, chatID)
				if err != nil {
					return ErrBadLayout // e.g. an unknown category (foreign key)
				}
				if n, _ := res.RowsAffected(); n != 1 {
					return ErrBadLayout
				}
			}
		}
		return nil
	})
}

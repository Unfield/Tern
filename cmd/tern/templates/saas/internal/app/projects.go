package app

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// Project is the example of the app's own data: each belongs to a team,
// whose members see and change it. Replace it with what your app keeps.
type Project struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// Projects stores projects in the projects table (migrations/).
type Projects struct{ db *sql.DB }

// ErrNotFound is a project that doesn't exist, or isn't the user's.
var ErrNotFound = errors.New("project not found")

// List returns the team's projects, the most recently changed first.
func (p *Projects) List(ctx context.Context, teamID int64) ([]Project, error) {
	rows, err := p.db.QueryContext(ctx, `SELECT id, name, description, created_at, updated_at FROM projects WHERE team_id = ? ORDER BY updated_at DESC, id DESC`, teamID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Project
	for rows.Next() {
		var pr Project
		var created, updated int64
		if err := rows.Scan(&pr.ID, &pr.Name, &pr.Description, &created, &updated); err != nil {
			return nil, err
		}
		pr.CreatedAt, pr.UpdatedAt = time.UnixMilli(created).UTC(), time.UnixMilli(updated).UTC()
		out = append(out, pr)
	}
	return out, rows.Err()
}

// Count is how many projects the team has.
func (p *Projects) Count(ctx context.Context, teamID int64) (int, error) {
	var n int
	err := p.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM projects WHERE team_id = ?`, teamID).Scan(&n)
	return n, err
}

// Create adds a project to the team, made by the user.
func (p *Projects) Create(ctx context.Context, teamID, userID int64, name, description string) (Project, error) {
	now := time.Now()
	res, err := p.db.ExecContext(ctx, `INSERT INTO projects (team_id, created_by, name, description, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		teamID, userID, strings.TrimSpace(name), strings.TrimSpace(description), now.UnixMilli(), now.UnixMilli())
	if err != nil {
		return Project{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Project{}, err
	}
	return Project{ID: id, Name: strings.TrimSpace(name), Description: strings.TrimSpace(description), CreatedAt: now.UTC(), UpdatedAt: now.UTC()}, nil
}

// Update renames a project and changes its description.
func (p *Projects) Update(ctx context.Context, teamID, id int64, name, description string) error {
	res, err := p.db.ExecContext(ctx, `UPDATE projects SET name = ?, description = ?, updated_at = ? WHERE id = ? AND team_id = ?`,
		strings.TrimSpace(name), strings.TrimSpace(description), time.Now().UnixMilli(), id, teamID)
	return found(res, err)
}

// Delete removes a project.
func (p *Projects) Delete(ctx context.Context, teamID, id int64) error {
	res, err := p.db.ExecContext(ctx, `DELETE FROM projects WHERE id = ? AND team_id = ?`, id, teamID)
	return found(res, err)
}

// DeleteAll removes the team's projects, when the team is deleted.
func (p *Projects) DeleteAll(ctx context.Context, teamID int64) error {
	_, err := p.db.ExecContext(ctx, `DELETE FROM projects WHERE team_id = ?`, teamID)
	return err
}

func found(res sql.Result, err error) error {
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

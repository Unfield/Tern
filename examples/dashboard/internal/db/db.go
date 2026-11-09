// Package db stores projects and tasks in SQLite (modernc.org/sqlite, pure
// Go), so the whole app still builds into one static binary.
package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// ErrNotFound is returned when a project or task does not exist.
var ErrNotFound = errors.New("not found")

type Project struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Status      string    `json:"status"` // active, paused, done
	Created     time.Time `json:"created"`
	TaskCount   int       `json:"taskCount"`
	DoneCount   int       `json:"doneCount"`
}

type Task struct {
	ID        int64     `json:"id"`
	ProjectID int64     `json:"projectId"`
	Title     string    `json:"title"`
	Notes     string    `json:"notes,omitempty"`
	Priority  string    `json:"priority"` // low, medium, high
	Done      bool      `json:"done"`
	Created   time.Time `json:"created"`
}

type Activity struct {
	Project string    `json:"project"`
	Task    string    `json:"task"`
	Done    bool      `json:"done"`
	At      time.Time `json:"at"`
}

type Stats struct {
	Projects       int        `json:"projects"`
	ActiveProjects int        `json:"activeProjects"`
	OpenTasks      int        `json:"openTasks"`
	DoneTasks      int        `json:"doneTasks"`
	PerDay         []DayCount `json:"perDay"`
}

type DayCount struct {
	Day     string `json:"day"` // YYYY-MM-DD
	Created int    `json:"created"`
}

// Store is a user's view of the database.
type Store struct{ db *sql.DB }

var (
	once     sync.Once
	instance *Store
	openErr  error
)

// Default opens the database named by DATABASE_PATH (default
// dashboard.db) on first use.
func Default() (*Store, error) {
	once.Do(func() {
		path := os.Getenv("DATABASE_PATH")
		if path == "" {
			path = "dashboard.db"
		}
		instance, openErr = Open(path)
	})
	return instance, openErr
}

// Open opens or creates a database.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // SQLite allows one writer at a time
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS projects (
			id INTEGER PRIMARY KEY,
			owner TEXT NOT NULL,
			name TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'active',
			created TIMESTAMP NOT NULL
		);
		CREATE TABLE IF NOT EXISTS tasks (
			id INTEGER PRIMARY KEY,
			project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
			title TEXT NOT NULL,
			notes TEXT NOT NULL DEFAULT '',
			priority TEXT NOT NULL DEFAULT 'medium',
			done INTEGER NOT NULL DEFAULT 0,
			created TIMESTAMP NOT NULL,
			updated TIMESTAMP NOT NULL
		);
		CREATE TABLE IF NOT EXISTS profiles (
			owner TEXT PRIMARY KEY,
			display_name TEXT NOT NULL,
			email TEXT NOT NULL,
			weekly_digest INTEGER NOT NULL DEFAULT 1
		);`)
	if err != nil {
		return nil, fmt.Errorf("db: creating schema: %w", err)
	}
	return &Store{db: db}, nil
}

// Ping checks that the database is reachable, for health checks.
func (s *Store) Ping(ctx context.Context) error { return s.db.PingContext(ctx) }

// SeedIfEmpty gives a new user sample projects so the dashboard has data.
func (s *Store) SeedIfEmpty(ctx context.Context, owner string) error {
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM projects WHERE owner = ?`, owner).Scan(&n); err != nil || n > 0 {
		return err
	}
	samples := []struct {
		name, desc, status string
		tasks              []string
	}{
		{"Website redesign", "New marketing site with the updated brand.", "active", []string{"Audit current pages", "Wireframes", "Pick a font pairing", "Build the pricing page"}},
		{"Mobile app", "iOS and Android companion app.", "active", []string{"Set up CI", "Login screen", "Push notifications"}},
		{"Q3 planning", "Goals and budget for next quarter.", "paused", []string{"Collect team input", "Draft budget"}},
	}
	for i, p := range samples {
		created := time.Now().Add(-time.Duration(len(samples)-i) * 72 * time.Hour)
		id, err := s.createProject(ctx, owner, p.name, p.desc, p.status, created)
		if err != nil {
			return err
		}
		for j, t := range p.tasks {
			at := created.Add(time.Duration(j) * 20 * time.Hour)
			if _, err := s.db.ExecContext(ctx, `INSERT INTO tasks (project_id, title, priority, done, created, updated) VALUES (?, ?, ?, ?, ?, ?)`,
				id, t, []string{"high", "medium", "low"}[j%3], j%2, at, at); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Store) createProject(ctx context.Context, owner, name, desc, status string, created time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx, `INSERT INTO projects (owner, name, description, status, created) VALUES (?, ?, ?, ?, ?)`,
		owner, name, desc, status, created)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// CreateProject adds a project and returns it.
func (s *Store) CreateProject(ctx context.Context, owner, name, desc string) (Project, error) {
	id, err := s.createProject(ctx, owner, name, desc, "active", time.Now())
	if err != nil {
		return Project{}, err
	}
	return s.Project(ctx, owner, id)
}

const projectColumns = `p.id, p.name, p.description, p.status, p.created,
	(SELECT count(*) FROM tasks t WHERE t.project_id = p.id),
	(SELECT count(*) FROM tasks t WHERE t.project_id = p.id AND t.done = 1)`

func scanProject(row interface{ Scan(...any) error }) (Project, error) {
	var p Project
	err := row.Scan(&p.ID, &p.Name, &p.Description, &p.Status, &p.Created, &p.TaskCount, &p.DoneCount)
	return p, err
}

// Projects lists projects whose name matches query (all when empty).
func (s *Store) Projects(ctx context.Context, owner, query, status string) ([]Project, error) {
	q := `SELECT ` + projectColumns + ` FROM projects p WHERE p.owner = ? AND p.name LIKE ?`
	args := []any{owner, "%" + strings.ReplaceAll(query, "%", `\%`) + "%"}
	if status != "" {
		q += ` AND p.status = ?`
		args = append(args, status)
	}
	rows, err := s.db.QueryContext(ctx, q+` ORDER BY p.created DESC`, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	projects := []Project{}
	for rows.Next() {
		p, err := scanProject(rows)
		if err != nil {
			return nil, err
		}
		projects = append(projects, p)
	}
	return projects, rows.Err()
}

// Project returns one project.
func (s *Store) Project(ctx context.Context, owner string, id int64) (Project, error) {
	p, err := scanProject(s.db.QueryRowContext(ctx, `SELECT `+projectColumns+` FROM projects p WHERE p.owner = ? AND p.id = ?`, owner, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Project{}, ErrNotFound
	}
	return p, err
}

// UpdateProject changes a project's name, description and status.
func (s *Store) UpdateProject(ctx context.Context, owner string, id int64, name, desc, status string) error {
	return expectOne(s.db.ExecContext(ctx, `UPDATE projects SET name = ?, description = ?, status = ? WHERE owner = ? AND id = ?`, name, desc, status, owner, id))
}

// DeleteProject removes a project and its tasks.
func (s *Store) DeleteProject(ctx context.Context, owner string, id int64) error {
	return expectOne(s.db.ExecContext(ctx, `DELETE FROM projects WHERE owner = ? AND id = ?`, owner, id))
}

// Tasks lists a project's tasks, open first.
func (s *Store) Tasks(ctx context.Context, projectID int64) ([]Task, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, project_id, title, notes, priority, done, created FROM tasks WHERE project_id = ? ORDER BY done, CASE priority WHEN 'high' THEN 0 WHEN 'medium' THEN 1 ELSE 2 END, id`, projectID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	tasks := []Task{}
	for rows.Next() {
		var t Task
		if err := rows.Scan(&t.ID, &t.ProjectID, &t.Title, &t.Notes, &t.Priority, &t.Done, &t.Created); err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}

// AddTask adds a task to a project the owner has.
func (s *Store) AddTask(ctx context.Context, owner string, projectID int64, title, notes, priority string) (Task, error) {
	if _, err := s.Project(ctx, owner, projectID); err != nil {
		return Task{}, err
	}
	now := time.Now()
	res, err := s.db.ExecContext(ctx, `INSERT INTO tasks (project_id, title, notes, priority, created, updated) VALUES (?, ?, ?, ?, ?, ?)`,
		projectID, title, notes, priority, now, now)
	if err != nil {
		return Task{}, err
	}
	id, _ := res.LastInsertId()
	return Task{ID: id, ProjectID: projectID, Title: title, Notes: notes, Priority: priority, Created: now}, nil
}

// SetTaskDone marks a task done or open.
func (s *Store) SetTaskDone(ctx context.Context, owner string, taskID int64, done bool) error {
	return expectOne(s.db.ExecContext(ctx, `UPDATE tasks SET done = ?, updated = ? WHERE id = ? AND project_id IN (SELECT id FROM projects WHERE owner = ?)`,
		done, time.Now(), taskID, owner))
}

// DeleteTask removes a task.
func (s *Store) DeleteTask(ctx context.Context, owner string, taskID int64) error {
	return expectOne(s.db.ExecContext(ctx, `DELETE FROM tasks WHERE id = ? AND project_id IN (SELECT id FROM projects WHERE owner = ?)`, taskID, owner))
}

// Stats summarizes a user's projects, with tasks created per day for the
// last 14 days.
func (s *Store) Stats(ctx context.Context, owner string) (Stats, error) {
	var st Stats
	err := s.db.QueryRowContext(ctx, `SELECT
		(SELECT count(*) FROM projects WHERE owner = ?),
		(SELECT count(*) FROM projects WHERE owner = ? AND status = 'active'),
		(SELECT count(*) FROM tasks t JOIN projects p ON p.id = t.project_id WHERE p.owner = ? AND t.done = 0),
		(SELECT count(*) FROM tasks t JOIN projects p ON p.id = t.project_id WHERE p.owner = ? AND t.done = 1)`,
		owner, owner, owner, owner).Scan(&st.Projects, &st.ActiveProjects, &st.OpenTasks, &st.DoneTasks)
	if err != nil {
		return st, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT t.created FROM tasks t JOIN projects p ON p.id = t.project_id WHERE p.owner = ? AND t.created >= ?`,
		owner, time.Now().AddDate(0, 0, -13).Truncate(24*time.Hour))
	if err != nil {
		return st, err
	}
	defer func() { _ = rows.Close() }()
	counts := map[string]int{}
	for rows.Next() {
		var at time.Time
		if err := rows.Scan(&at); err != nil {
			return st, err
		}
		counts[at.Format(time.DateOnly)]++
	}
	for i := 13; i >= 0; i-- {
		day := time.Now().AddDate(0, 0, -i).Format(time.DateOnly)
		st.PerDay = append(st.PerDay, DayCount{Day: day, Created: counts[day]})
	}
	return st, rows.Err()
}

// RecentActivity returns the most recently changed tasks.
func (s *Store) RecentActivity(ctx context.Context, owner string, limit int) ([]Activity, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT p.name, t.title, t.done, t.updated FROM tasks t JOIN projects p ON p.id = t.project_id
		WHERE p.owner = ? ORDER BY t.updated DESC LIMIT ?`, owner, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []Activity{}
	for rows.Next() {
		var a Activity
		if err := rows.Scan(&a.Project, &a.Task, &a.Done, &a.At); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// Profile is a user's settings.
type Profile struct {
	DisplayName  string `json:"displayName"`
	Email        string `json:"email"`
	WeeklyDigest bool   `json:"weeklyDigest"`
}

// Profile returns the user's settings, with defaults for new users.
func (s *Store) Profile(ctx context.Context, owner string) (Profile, error) {
	p := Profile{DisplayName: owner, WeeklyDigest: true}
	err := s.db.QueryRowContext(ctx, `SELECT display_name, email, weekly_digest FROM profiles WHERE owner = ?`, owner).
		Scan(&p.DisplayName, &p.Email, &p.WeeklyDigest)
	if errors.Is(err, sql.ErrNoRows) {
		return p, nil
	}
	return p, err
}

// SaveProfile stores the user's settings.
func (s *Store) SaveProfile(ctx context.Context, owner string, p Profile) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO profiles (owner, display_name, email, weekly_digest) VALUES (?, ?, ?, ?)
		ON CONFLICT(owner) DO UPDATE SET display_name = excluded.display_name, email = excluded.email, weekly_digest = excluded.weekly_digest`,
		owner, p.DisplayName, p.Email, p.WeeklyDigest)
	return err
}

func expectOne(res sql.Result, err error) error {
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

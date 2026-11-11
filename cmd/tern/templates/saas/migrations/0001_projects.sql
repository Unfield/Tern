CREATE TABLE projects (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	team_id INTEGER NOT NULL,
	created_by INTEGER NOT NULL,
	name TEXT NOT NULL,
	description TEXT NOT NULL DEFAULT '',
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL
);
CREATE INDEX projects_team ON projects (team_id, updated_at);

-- +down
DROP TABLE projects;

-- The queries the app runs, typed from the schema in migrations/: tern
-- generates queries.gen.go from them (see docs/sql.md in Tern).

-- The team's projects, the most recently changed first.
-- name: ProjectsOf :many
-- row ProjectRow
SELECT id, name, description, created_at, updated_at FROM projects
WHERE team_id = :team_id
ORDER BY updated_at DESC, id DESC;

-- name: CountProjects :one
SELECT COUNT(*) AS n FROM projects WHERE team_id = :team_id;

-- name: CreateProject :one
-- row ProjectRow
INSERT INTO projects (team_id, created_by, name, description, created_at, updated_at)
VALUES (:team_id, :created_by, :name, :description, :now, :now)
RETURNING id, name, description, created_at, updated_at;

-- name: UpdateProject :execrows
UPDATE projects SET name = :name, description = :description, updated_at = :now
WHERE id = :id AND team_id = :team_id;

-- name: DeleteProject :execrows
DELETE FROM projects WHERE id = :id AND team_id = :team_id;

-- name: DeleteProjects :exec
DELETE FROM projects WHERE team_id = :team_id;

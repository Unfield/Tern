-- name: AddLink :one
-- row Link
INSERT INTO links (id, user_id, url, title, note) VALUES (:id, :user_id, :url, :title, :note)
RETURNING id, url, title, note, read, added_at;

-- A user's links, newest first; show is "read", "unread" or "all".
-- name: LinksOf :many
-- row Link
SELECT id, url, title, note, read, added_at FROM links
WHERE user_id = :user_id AND (:show = 'all' OR read = (:show = 'read'))
ORDER BY id DESC
LIMIT :limit;

-- name: CountLinks :one
SELECT COUNT(*) AS n FROM links WHERE user_id = :user_id;

-- name: Titles :many
SELECT title FROM links WHERE user_id = :user_id AND title LIKE :pattern ORDER BY title;

-- name: MarkRead :execrows
UPDATE links SET read = :read WHERE id = :id AND user_id = :user_id;

-- name: DeleteLinks :exec
DELETE FROM links WHERE user_id = :user_id;

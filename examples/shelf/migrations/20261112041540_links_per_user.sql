-- Links saved before accounts belong to no one.
ALTER TABLE links ADD COLUMN user_id INTEGER NOT NULL DEFAULT 0;
CREATE INDEX links_by_user ON links (user_id, id);

-- +down
DROP INDEX links_by_user;
ALTER TABLE links DROP COLUMN user_id;

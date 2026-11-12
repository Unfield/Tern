-- Explicit ids, so the same schema runs on SQLite and PostgreSQL.
CREATE TABLE users (
	id    BIGINT NOT NULL PRIMARY KEY,
	email TEXT NOT NULL UNIQUE,
	name  TEXT
);

CREATE TABLE links (
	id       BIGINT NOT NULL PRIMARY KEY,
	user_id  BIGINT NOT NULL REFERENCES users (id),
	url      TEXT NOT NULL,
	title    TEXT NOT NULL,
	note     TEXT,
	read     BOOLEAN NOT NULL DEFAULT FALSE,
	added_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- +down
DROP TABLE links;
DROP TABLE users;

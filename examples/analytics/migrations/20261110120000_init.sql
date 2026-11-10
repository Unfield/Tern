CREATE TABLE users (
	id INTEGER PRIMARY KEY,
	name TEXT NOT NULL,
	email TEXT NOT NULL UNIQUE,
	password_hash BLOB NOT NULL,
	weekly_report INTEGER NOT NULL DEFAULT 1,
	created_at INTEGER NOT NULL
);

CREATE TABLE sites (
	id INTEGER PRIMARY KEY,
	domain TEXT NOT NULL UNIQUE,
	name TEXT NOT NULL,
	created_at INTEGER NOT NULL
);

-- One row per page view. visitor is a hash that changes every day, so
-- nobody can be followed across days, and no cookie is set.
CREATE TABLE events (
	id INTEGER PRIMARY KEY,
	site_id INTEGER NOT NULL REFERENCES sites (id) ON DELETE CASCADE,
	ts INTEGER NOT NULL,
	visitor TEXT NOT NULL,
	path TEXT NOT NULL,
	referrer TEXT NOT NULL DEFAULT '',
	browser TEXT NOT NULL,
	os TEXT NOT NULL,
	device TEXT NOT NULL
);
CREATE INDEX events_site_ts ON events (site_id, ts);

-- Totals per day, kept after old events are deleted.
CREATE TABLE daily (
	site_id INTEGER NOT NULL REFERENCES sites (id) ON DELETE CASCADE,
	day TEXT NOT NULL,
	visitors INTEGER NOT NULL,
	pageviews INTEGER NOT NULL,
	PRIMARY KEY (site_id, day)
);

-- The day's secret for visitor hashes; yesterday's is deleted.
CREATE TABLE salts (
	day TEXT PRIMARY KEY,
	salt BLOB NOT NULL
);

-- +down
DROP TABLE salts;
DROP TABLE daily;
DROP TABLE events;
DROP TABLE sites;
DROP TABLE users;

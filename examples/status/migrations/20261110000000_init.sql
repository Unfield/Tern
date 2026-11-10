-- The status page's schema. Timestamps are Unix milliseconds.

CREATE TABLE users (
	id INTEGER PRIMARY KEY,
	username TEXT NOT NULL UNIQUE,
	password_hash TEXT NOT NULL,
	created_at BIGINT NOT NULL
);

CREATE TABLE monitors (
	id INTEGER PRIMARY KEY,
	name TEXT NOT NULL,
	type TEXT NOT NULL,                      -- http, tcp, dns, tls
	target TEXT NOT NULL,                    -- a URL, host:port or hostname
	method TEXT NOT NULL DEFAULT 'GET',
	expected_status TEXT NOT NULL DEFAULT '200-399',
	keyword TEXT NOT NULL DEFAULT '',        -- http: the body must contain it
	interval_seconds INTEGER NOT NULL DEFAULT 60,
	timeout_seconds INTEGER NOT NULL DEFAULT 10,
	retries INTEGER NOT NULL DEFAULT 1,      -- failed checks tolerated before "down"
	cert_expiry_days INTEGER NOT NULL DEFAULT 14,
	group_name TEXT NOT NULL DEFAULT 'Services',
	public INTEGER NOT NULL DEFAULT 1,
	paused INTEGER NOT NULL DEFAULT 0,
	status TEXT NOT NULL DEFAULT 'pending',  -- up, down, pending, paused
	last_check BIGINT,
	last_latency INTEGER,
	last_message TEXT NOT NULL DEFAULT '',
	cert_expires BIGINT,
	created_at BIGINT NOT NULL
);

CREATE TABLE heartbeats (
	id INTEGER PRIMARY KEY,
	monitor_id INTEGER NOT NULL REFERENCES monitors (id) ON DELETE CASCADE,
	at BIGINT NOT NULL,
	status TEXT NOT NULL,
	latency_ms INTEGER NOT NULL,
	message TEXT NOT NULL DEFAULT ''
);
CREATE INDEX heartbeats_monitor_at ON heartbeats (monitor_id, at);

CREATE TABLE incidents (
	id INTEGER PRIMARY KEY,
	title TEXT NOT NULL,
	status TEXT NOT NULL,                    -- investigating, identified, monitoring, resolved
	impact TEXT NOT NULL DEFAULT 'minor',    -- minor, major, maintenance
	monitor_id INTEGER REFERENCES monitors (id) ON DELETE SET NULL,
	auto INTEGER NOT NULL DEFAULT 0,         -- opened by a monitor going down
	created_at BIGINT NOT NULL,
	resolved_at BIGINT
);

CREATE TABLE incident_updates (
	id INTEGER PRIMARY KEY,
	incident_id INTEGER NOT NULL REFERENCES incidents (id) ON DELETE CASCADE,
	status TEXT NOT NULL,
	body TEXT NOT NULL,                      -- Markdown
	created_at BIGINT NOT NULL
);

CREATE TABLE channels (
	id INTEGER PRIMARY KEY,
	name TEXT NOT NULL,
	type TEXT NOT NULL,                      -- webhook, slack, discord, email
	config TEXT NOT NULL,                    -- JSON
	enabled INTEGER NOT NULL DEFAULT 1,
	created_at BIGINT NOT NULL
);

CREATE TABLE settings (
	key TEXT PRIMARY KEY,
	value TEXT NOT NULL
);

-- +down
DROP TABLE settings;
DROP TABLE channels;
DROP TABLE incident_updates;
DROP TABLE incidents;
DROP TABLE heartbeats;
DROP TABLE monitors;
DROP TABLE users;

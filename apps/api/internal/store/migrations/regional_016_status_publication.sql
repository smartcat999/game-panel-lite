CREATE TABLE regional_status_sequence (
 id smallint PRIMARY KEY CHECK(id = 1),
 sequence bigint NOT NULL CHECK(sequence >= 0)
);
INSERT INTO regional_status_sequence(id,sequence) VALUES(1,0);

CREATE TABLE regional_status_outbox (
 id text PRIMARY KEY,
 sequence bigint NOT NULL UNIQUE CHECK(sequence > 0),
 event_type text NOT NULL CHECK(event_type = 'region.status.observed'),
 payload text NOT NULL,
 lease_token text NOT NULL DEFAULT '',
 lease_until_ms bigint NOT NULL DEFAULT 0,
 next_attempt_ms bigint NOT NULL DEFAULT 0,
 attempts bigint NOT NULL DEFAULT 0,
 published_at_ms bigint NOT NULL DEFAULT 0,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE INDEX idx_regional_status_publication ON regional_status_outbox(next_attempt_ms,created_at,id) WHERE published_at_ms = 0;

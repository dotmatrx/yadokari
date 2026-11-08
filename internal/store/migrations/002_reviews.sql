-- Completed reviews waiting to be posted. A row is removed only when the API
-- accepts it, or kept with an error when the API definitively rejects it.
-- One pending review per assignment: a double submit is impossible here.
CREATE TABLE outbox (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    assignment_id INTEGER NOT NULL UNIQUE,
    subject_id    INTEGER NOT NULL,
    payload       TEXT    NOT NULL,   -- JSON wk.ReviewInput
    created_at    INTEGER NOT NULL,
    attempts      INTEGER NOT NULL DEFAULT 0,
    failed        TEXT    NOT NULL DEFAULT ''  -- non-empty: rejected, won't retry
);

-- The review session in progress, so a crash or quitting mid-session loses
-- nothing.
CREATE TABLE session_state (
    name        TEXT PRIMARY KEY,
    updated_at  INTEGER NOT NULL,
    raw         TEXT NOT NULL
);

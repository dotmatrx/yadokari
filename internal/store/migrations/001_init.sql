-- Every table keeps the API resource exactly as received in `raw` (the whole
-- {id, object, url, data_updated_at, data} envelope) plus the columns we
-- query on. Timestamps are integer Unix microseconds (NULL when absent).

CREATE TABLE subjects (
    id              INTEGER PRIMARY KEY,
    object          TEXT    NOT NULL,   -- radical, kanji, vocabulary, kana_vocabulary
    level           INTEGER NOT NULL,
    hidden          INTEGER NOT NULL,   -- hidden_at IS NOT NULL
    data_updated_at INTEGER NOT NULL,
    raw             TEXT    NOT NULL
);
CREATE INDEX subjects_level ON subjects (level, object);

CREATE TABLE assignments (
    id              INTEGER PRIMARY KEY,
    subject_id      INTEGER NOT NULL,
    subject_type    TEXT    NOT NULL,
    srs_stage       INTEGER NOT NULL,
    unlocked_at     INTEGER,
    started_at      INTEGER,
    available_at    INTEGER,
    passed_at       INTEGER,
    burned_at       INTEGER,
    hidden          INTEGER NOT NULL,
    data_updated_at INTEGER NOT NULL,
    raw             TEXT    NOT NULL
);
CREATE UNIQUE INDEX assignments_subject ON assignments (subject_id);
CREATE INDEX assignments_available ON assignments (available_at);

CREATE TABLE study_materials (
    id              INTEGER PRIMARY KEY,
    subject_id      INTEGER NOT NULL,
    data_updated_at INTEGER NOT NULL,
    raw             TEXT    NOT NULL
);
CREATE INDEX study_materials_subject ON study_materials (subject_id);

CREATE TABLE spaced_repetition_systems (
    id              INTEGER PRIMARY KEY,
    data_updated_at INTEGER NOT NULL,
    raw             TEXT    NOT NULL
);

CREATE TABLE review_statistics (
    id              INTEGER PRIMARY KEY,
    subject_id      INTEGER NOT NULL,
    data_updated_at INTEGER NOT NULL,
    raw             TEXT    NOT NULL
);
CREATE INDEX review_statistics_subject ON review_statistics (subject_id);

-- Single resources (user), with validators for conditional GETs.
CREATE TABLE singletons (
    name            TEXT PRIMARY KEY,
    etag            TEXT NOT NULL DEFAULT '',
    last_modified   TEXT NOT NULL DEFAULT '',
    fetched_at      INTEGER NOT NULL,
    raw             TEXT NOT NULL
);

-- Per-collection sync bookmark: the collection's data_updated_at at the last
-- successful sync, used as updated_after next time.
CREATE TABLE sync_state (
    collection      TEXT PRIMARY KEY,
    updated_after   INTEGER,
    synced_at       INTEGER NOT NULL
);

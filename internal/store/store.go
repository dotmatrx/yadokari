// Package store is yadokari's local SQLite cache of the user's WaniKani data.
//
// Resources are stored exactly as the API sent them (raw JSON) alongside the
// handful of columns the app queries, so new fields never require a re-sync
// and decoding happens on read with internal/wk types.
package store

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite" // registers the "sqlite" driver

	"github.com/dotmatrx/yadokari/internal/wk"
)

//go:embed migrations/*.sql
var migrations embed.FS

// Store is an open cache database.
type Store struct {
	db *sql.DB
}

// Open opens (creating if needed) the database at path and migrates it.
// Use ":memory:" for a throwaway database in tests.
func Open(ctx context.Context, path string) (*Store, error) {
	dsn := "file:" + path + "?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"
	if path != ":memory:" {
		dsn += "&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)"
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	if path == ":memory:" {
		db.SetMaxOpenConns(1) // each connection would get its own empty database
	}
	s := &Store{db: db}
	if err := s.migrate(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate(ctx context.Context) error {
	var version int
	if err := s.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	names, err := fs.Glob(migrations, "migrations/*.sql")
	if err != nil {
		return err
	}
	sort.Strings(names)
	for _, name := range names {
		n, err := strconv.Atoi(strings.SplitN(strings.TrimPrefix(name, "migrations/"), "_", 2)[0])
		if err != nil {
			return fmt.Errorf("store: bad migration name %s", name)
		}
		if n <= version {
			continue
		}
		sqlText, err := migrations.ReadFile(name)
		if err != nil {
			return err
		}
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, string(sqlText)); err != nil {
			tx.Rollback()
			return fmt.Errorf("store: migration %s: %w", name, err)
		}
		if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", n)); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

// micros converts a time to the stored representation.
func micros(t time.Time) int64 { return t.UnixMicro() }

func nullMicros(t *time.Time) sql.NullInt64 {
	if t == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: t.UnixMicro(), Valid: true}
}

func fromMicros(v sql.NullInt64) *time.Time {
	if !v.Valid {
		return nil
	}
	t := time.UnixMicro(v.Int64).UTC()
	return &t
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// RawResource is a resource as received, with its data undecoded.
type RawResource = wk.Resource[json.RawMessage]

// Collection names, used for sync bookmarks.
const (
	Subjects                = "subjects"
	Assignments             = "assignments"
	StudyMaterials          = "study_materials"
	SpacedRepetitionSystems = "spaced_repetition_systems"
	ReviewStatistics        = "review_statistics"
)

// Collections lists every collection the store can hold.
var Collections = []string{Subjects, Assignments, StudyMaterials, SpacedRepetitionSystems, ReviewStatistics}

// Upsert stores a page of resources from the named collection, in one
// transaction. Existing rows with the same id are replaced.
func (s *Store) Upsert(ctx context.Context, collection string, page []RawResource) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, r := range page {
		raw, err := json.Marshal(r)
		if err != nil {
			return err
		}
		if err := upsertOne(ctx, tx, collection, r, string(raw)); err != nil {
			return fmt.Errorf("store: %s %d: %w", collection, r.ID, err)
		}
	}
	return tx.Commit()
}

func upsertOne(ctx context.Context, tx *sql.Tx, collection string, r RawResource, raw string) error {
	updated := micros(r.DataUpdatedAt)
	switch collection {
	case Subjects:
		var d wk.Subject
		if err := json.Unmarshal(r.Data, &d); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO subjects
			(id, object, level, hidden, data_updated_at, raw) VALUES (?, ?, ?, ?, ?, ?)`,
			r.ID, r.Object, d.Level, boolInt(d.HiddenAt != nil), updated, raw)
		return err
	case Assignments:
		var d wk.Assignment
		if err := json.Unmarshal(r.Data, &d); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO assignments
			(id, subject_id, subject_type, srs_stage, unlocked_at, started_at, available_at,
			 passed_at, burned_at, hidden, data_updated_at, raw)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			r.ID, d.SubjectID, d.SubjectType, d.SRSStage, nullMicros(d.UnlockedAt), nullMicros(d.StartedAt),
			nullMicros(d.AvailableAt), nullMicros(d.PassedAt), nullMicros(d.BurnedAt), boolInt(d.Hidden), updated, raw)
		return err
	case StudyMaterials:
		var d wk.StudyMaterial
		if err := json.Unmarshal(r.Data, &d); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO study_materials
			(id, subject_id, data_updated_at, raw) VALUES (?, ?, ?, ?)`, r.ID, d.SubjectID, updated, raw)
		return err
	case SpacedRepetitionSystems:
		_, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO spaced_repetition_systems
			(id, data_updated_at, raw) VALUES (?, ?, ?)`, r.ID, updated, raw)
		return err
	case ReviewStatistics:
		var d wk.ReviewStatistic
		if err := json.Unmarshal(r.Data, &d); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO review_statistics
			(id, subject_id, data_updated_at, raw) VALUES (?, ?, ?, ?)`, r.ID, d.SubjectID, updated, raw)
		return err
	}
	return fmt.Errorf("unknown collection %q", collection)
}

// Count returns the number of rows in a collection.
func (s *Store) Count(ctx context.Context, collection string) (int, error) {
	if !validCollection(collection) {
		return 0, fmt.Errorf("store: unknown collection %q", collection)
	}
	var n int
	err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM "+collection).Scan(&n)
	return n, err
}

func validCollection(c string) bool {
	for _, x := range Collections {
		if x == c {
			return true
		}
	}
	return false
}

// SyncState is a collection's sync bookmark.
type SyncState struct {
	UpdatedAfter *time.Time // nil: never synced, do a full fetch
	SyncedAt     time.Time
}

// SyncState returns the bookmark for a collection (zero value if never synced).
func (s *Store) SyncState(ctx context.Context, collection string) (SyncState, error) {
	var after sql.NullInt64
	var synced int64
	err := s.db.QueryRowContext(ctx, `SELECT updated_after, synced_at FROM sync_state WHERE collection = ?`,
		collection).Scan(&after, &synced)
	if errors.Is(err, sql.ErrNoRows) {
		return SyncState{}, nil
	}
	if err != nil {
		return SyncState{}, err
	}
	return SyncState{UpdatedAfter: fromMicros(after), SyncedAt: time.UnixMicro(synced).UTC()}, nil
}

// SetSyncState records a successful sync of a collection.
func (s *Store) SetSyncState(ctx context.Context, collection string, st SyncState) error {
	_, err := s.db.ExecContext(ctx, `INSERT OR REPLACE INTO sync_state (collection, updated_after, synced_at)
		VALUES (?, ?, ?)`, collection, nullMicros(st.UpdatedAfter), micros(st.SyncedAt))
	return err
}

// Singleton is a stored single resource with its HTTP validators.
type Singleton struct {
	Raw       json.RawMessage
	Validator wk.Conditional
	FetchedAt time.Time
}

// PutSingleton stores a single resource (e.g. "user").
func (s *Store) PutSingleton(ctx context.Context, name string, v Singleton) error {
	_, err := s.db.ExecContext(ctx, `INSERT OR REPLACE INTO singletons (name, etag, last_modified, fetched_at, raw)
		VALUES (?, ?, ?, ?, ?)`, name, v.Validator.ETag, v.Validator.LastModified, micros(v.FetchedAt), string(v.Raw))
	return err
}

// Singleton returns a stored single resource; ok is false if absent.
func (s *Store) Singleton(ctx context.Context, name string) (v Singleton, ok bool, err error) {
	var raw string
	var fetched int64
	err = s.db.QueryRowContext(ctx, `SELECT etag, last_modified, fetched_at, raw FROM singletons WHERE name = ?`,
		name).Scan(&v.Validator.ETag, &v.Validator.LastModified, &fetched, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return Singleton{}, false, nil
	}
	if err != nil {
		return Singleton{}, false, err
	}
	v.Raw, v.FetchedAt = json.RawMessage(raw), time.UnixMicro(fetched).UTC()
	return v, true, nil
}

// User decodes the stored user, if any.
func (s *Store) User(ctx context.Context) (*wk.Resource[wk.User], error) {
	v, ok, err := s.Singleton(ctx, "user")
	if err != nil || !ok {
		return nil, err
	}
	var u wk.Resource[wk.User]
	if err := json.Unmarshal(v.Raw, &u); err != nil {
		return nil, fmt.Errorf("store: decode user: %w", err)
	}
	return &u, nil
}

// Subject decodes one stored subject.
func (s *Store) Subject(ctx context.Context, id int) (*wk.Resource[wk.Subject], error) {
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT raw FROM subjects WHERE id = ?`, id).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var r wk.Resource[wk.Subject]
	if err := json.Unmarshal([]byte(raw), &r); err != nil {
		return nil, err
	}
	return &r, nil
}

// Subjects decodes the stored subjects with the given IDs, in the order
// given; IDs not in the store are skipped.
func (s *Store) Subjects(ctx context.Context, ids []int) ([]wk.Resource[wk.Subject], error) {
	out := make([]wk.Resource[wk.Subject], 0, len(ids))
	for _, id := range ids {
		r, err := s.Subject(ctx, id)
		if err != nil {
			return nil, err
		}
		if r != nil {
			out = append(out, *r)
		}
	}
	return out, nil
}

// StudyMaterial returns the user's synonyms and notes for a subject, or nil
// if there are none (or they are hidden).
func (s *Store) StudyMaterial(ctx context.Context, subjectID int) (*wk.StudyMaterial, error) {
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT raw FROM study_materials WHERE subject_id = ?`, subjectID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var r wk.Resource[wk.StudyMaterial]
	if err := json.Unmarshal([]byte(raw), &r); err != nil {
		return nil, err
	}
	if r.Data.Hidden {
		return nil, nil
	}
	return &r.Data, nil
}

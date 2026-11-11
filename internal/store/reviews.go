package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jtharris/yadokari/internal/wk"
)

// ReviewItem is a due review with everything needed to ask and check it.
type ReviewItem struct {
	Assignment wk.Resource[wk.Assignment]
	Subject    wk.Resource[wk.Subject]
	Synonyms   []string // the user's meaning synonyms
}

// ReviewItems returns the assignments available for review at now, joined
// with their subjects and the user's synonyms. Same rules as Queue.
func (s *Store) ReviewItems(ctx context.Context, now time.Time, maxLevel int) ([]ReviewItem, error) {
	if maxLevel <= 0 {
		maxLevel = 1 << 30
	}
	return s.items(ctx, `
		SELECT a.raw, s.raw, sm.raw
		FROM assignments a
		JOIN subjects s ON s.id = a.subject_id
		LEFT JOIN study_materials sm ON sm.subject_id = a.subject_id
		WHERE a.available_at IS NOT NULL AND a.available_at <= ? AND a.started_at IS NOT NULL
		  AND a.hidden = 0 AND s.hidden = 0 AND s.level <= ?
		  AND a.id NOT IN (SELECT assignment_id FROM outbox WHERE failed = '')
		ORDER BY a.id`, micros(now), maxLevel)
}

// items runs a query returning (assignment, subject, study material) raws.
func (s *Store) items(ctx context.Context, query string, args ...any) ([]ReviewItem, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ReviewItem
	for rows.Next() {
		var aRaw, sRaw string
		var smRaw sql.NullString
		if err := rows.Scan(&aRaw, &sRaw, &smRaw); err != nil {
			return nil, err
		}
		var it ReviewItem
		if err := json.Unmarshal([]byte(aRaw), &it.Assignment); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(sRaw), &it.Subject); err != nil {
			return nil, err
		}
		if smRaw.Valid {
			var sm wk.Resource[wk.StudyMaterial]
			if err := json.Unmarshal([]byte(smRaw.String), &sm); err == nil && !sm.Data.Hidden {
				it.Synonyms = sm.Data.MeaningSynonyms
			}
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// LessonItems returns the lessons available now (unlocked, not started, not
// already queued to start), in WaniKani's lesson order: level, then lesson
// position. Same rules as Queue's lesson count.
func (s *Store) LessonItems(ctx context.Context, maxLevel int) ([]ReviewItem, error) {
	if maxLevel <= 0 {
		maxLevel = 1 << 30
	}
	return s.items(ctx, `
		SELECT a.raw, s.raw, sm.raw
		FROM assignments a
		JOIN subjects s ON s.id = a.subject_id
		LEFT JOIN study_materials sm ON sm.subject_id = a.subject_id
		WHERE a.unlocked_at IS NOT NULL AND a.started_at IS NULL AND a.hidden = 0 AND s.hidden = 0 AND s.level <= ?
		  AND a.id NOT IN (SELECT assignment_id FROM outbox WHERE failed = '')
		ORDER BY s.level, json_extract(s.raw, '$.data.lesson_position'), s.id`, maxLevel)
}

// ErrAlreadyQueued means a review for that assignment is already waiting.
var ErrAlreadyQueued = errors.New("store: a review for this assignment is already queued")

// Outbox row kinds.
const (
	KindReview = "review" // POST /reviews
	KindStart  = "start"  // PUT /assignments/:id/start (a finished lesson)
)

// OutboxItem is a queued write: a review, or a lesson start.
type OutboxItem struct {
	ID           int64
	Kind         string
	AssignmentID int
	SubjectID    int
	Review       wk.ReviewInput // KindReview
	StartedAt    time.Time      // KindStart
	Created      time.Time
	Attempts     int
	Failed       string
}

type startPayload struct {
	StartedAt time.Time `json:"started_at"`
}

// QueueStart adds a finished lesson to the outbox, to be started at
// startedAt. Like QueueReview, a pending row for the assignment is never
// replaced; a rejected one is.
func (s *Store) QueueStart(ctx context.Context, assignmentID, subjectID int, startedAt time.Time) error {
	payload, err := json.Marshal(startPayload{StartedAt: startedAt.UTC()})
	if err != nil {
		return err
	}
	return s.queue(ctx, KindStart, assignmentID, subjectID, string(payload), startedAt)
}

// QueueReview adds a completed review to the outbox.
func (s *Store) QueueReview(ctx context.Context, subjectID int, r wk.ReviewInput) error {
	payload, err := json.Marshal(r)
	if err != nil {
		return err
	}
	created := time.Now()
	if r.CreatedAt != nil {
		created = *r.CreatedAt
	}
	return s.queue(ctx, KindReview, r.AssignmentID, subjectID, string(payload), created)
}

func (s *Store) queue(ctx context.Context, kind string, assignmentID, subjectID int, payload string, created time.Time) error {
	// A previously rejected write for the same assignment is replaced; a
	// pending one is not (that would be a double submit).
	res, err := s.db.ExecContext(ctx, `INSERT INTO outbox (kind, assignment_id, subject_id, payload, created_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (assignment_id) DO UPDATE SET
			kind = excluded.kind, subject_id = excluded.subject_id, payload = excluded.payload,
			created_at = excluded.created_at, attempts = 0, failed = ''
		WHERE outbox.failed != ''`, kind, assignmentID, subjectID, payload, micros(created))
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrAlreadyQueued
	}
	return nil
}

// Outbox returns queued reviews, oldest first. Failed ones are included only
// if withFailed is set.
func (s *Store) Outbox(ctx context.Context, withFailed bool) ([]OutboxItem, error) {
	q := `SELECT id, kind, assignment_id, subject_id, payload, created_at, attempts, failed FROM outbox`
	if !withFailed {
		q += ` WHERE failed = ''`
	}
	rows, err := s.db.QueryContext(ctx, q+` ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []OutboxItem
	for rows.Next() {
		var it OutboxItem
		var payload string
		var created int64
		if err := rows.Scan(&it.ID, &it.Kind, &it.AssignmentID, &it.SubjectID, &payload, &created, &it.Attempts, &it.Failed); err != nil {
			return nil, err
		}
		var err error
		if it.Kind == KindStart {
			var p startPayload
			err = json.Unmarshal([]byte(payload), &p)
			it.StartedAt = p.StartedAt
		} else {
			err = json.Unmarshal([]byte(payload), &it.Review)
		}
		if err != nil {
			return nil, fmt.Errorf("store: outbox %d: %w", it.ID, err)
		}
		it.Created = time.UnixMicro(created).UTC()
		out = append(out, it)
	}
	return out, rows.Err()
}

// OutboxCounts returns how many reviews are waiting and how many failed.
func (s *Store) OutboxCounts(ctx context.Context) (pending, failed int, err error) {
	err = s.db.QueryRowContext(ctx, `SELECT count(*) FILTER (WHERE failed = ''), count(*) FILTER (WHERE failed != '') FROM outbox`).
		Scan(&pending, &failed)
	return
}

// OutboxAttempted records a failed attempt that will be retried.
func (s *Store) OutboxAttempted(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE outbox SET attempts = attempts + 1 WHERE id = ?`, id)
	return err
}

// OutboxFailed marks a review as definitively rejected; it is kept for the
// user to see but never retried.
func (s *Store) OutboxFailed(ctx context.Context, id int64, reason string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE outbox SET attempts = attempts + 1, failed = ? WHERE id = ?`, reason, id)
	return err
}

// Rejected returns only the reviews the API definitively refused, oldest first.
func (s *Store) Rejected(ctx context.Context) ([]OutboxItem, error) {
	all, err := s.Outbox(ctx, true)
	var out []OutboxItem
	for _, it := range all {
		if it.Failed != "" {
			out = append(out, it)
		}
	}
	return out, err
}

// DismissRejected deletes every rejected review, returning how many. The
// assignments become reviewable again if they are still due.
func (s *Store) DismissRejected(ctx context.Context) (int, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM outbox WHERE failed != ''`)
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	return int(n), err
}

// ReviewOutcome is where a reviewed assignment stands: still queued,
// rejected, or posted (then SRSStage is the stage the API returned).
type ReviewOutcome struct {
	SRSStage int
	Queued   bool   // still in the outbox (pending or rejected)
	Failed   string // non-empty: rejected
}

// ReviewOutcomes looks up the given assignments. Missing ones are left out.
func (s *Store) ReviewOutcomes(ctx context.Context, assignmentIDs []int) (map[int]ReviewOutcome, error) {
	out := map[int]ReviewOutcome{}
	if len(assignmentIDs) == 0 {
		return out, nil
	}
	ids, err := json.Marshal(assignmentIDs)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT a.id, a.srs_stage, o.id IS NOT NULL, coalesce(o.failed, '')
		FROM assignments a LEFT JOIN outbox o ON o.assignment_id = a.id
		WHERE a.id IN (SELECT value FROM json_each(?))`, string(ids))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int
		var o ReviewOutcome
		if err := rows.Scan(&id, &o.SRSStage, &o.Queued, &o.Failed); err != nil {
			return nil, err
		}
		out[id] = o
	}
	return out, rows.Err()
}

// OutboxDone removes a posted review and stores the resources the API
// returned (the updated assignment and review statistic) in one transaction.
func (s *Store) OutboxDone(ctx context.Context, id int64, updated map[string][]RawResource) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for collection, page := range updated {
		for _, r := range page {
			raw, err := json.Marshal(r)
			if err != nil {
				return err
			}
			if err := upsertOne(ctx, tx, collection, r, string(raw)); err != nil {
				return err
			}
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM outbox WHERE id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// SaveSession stores a session snapshot (any JSON-marshalable value).
func (s *Store) SaveSession(ctx context.Context, name string, v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT OR REPLACE INTO session_state (name, updated_at, raw) VALUES (?, ?, ?)`,
		name, micros(time.Now()), string(raw))
	return err
}

// LoadSession decodes a stored snapshot into v; ok is false if none exists.
func (s *Store) LoadSession(ctx context.Context, name string, v any) (ok bool, err error) {
	var raw string
	err = s.db.QueryRowContext(ctx, `SELECT raw FROM session_state WHERE name = ?`, name).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, json.Unmarshal([]byte(raw), v)
}

// ClearSession removes a stored snapshot.
func (s *Store) ClearSession(ctx context.Context, name string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM session_state WHERE name = ?`, name)
	return err
}

package store

import (
	"context"
	"database/sql"
	"time"
)

// Queue summarizes what the user can study, computed from the local cache so
// the dashboard needs no network. maxLevel is the subscription's
// max_level_granted (subjects above it are excluded); 0 means no cap.
type Queue struct {
	Lessons  int
	Reviews  int       // available now
	Forecast [24]int   // reviews becoming available in each of the next 24 hours (hour 0 = rest of this hour)
	Start    time.Time // top of the current hour
	Next     *time.Time
}

// Queue computes lesson/review counts and the 24-hour forecast at now.
//
// Availability is decided by available_at alone: burned items have none, and a
// resurrected item keeps its burned_at but is reviewable again.
func (s *Store) Queue(ctx context.Context, now time.Time, maxLevel int) (Queue, error) {
	if maxLevel <= 0 {
		maxLevel = 1 << 30
	}
	q := Queue{Start: now.UTC().Truncate(time.Hour)}
	nowUS := micros(now)

	err := s.db.QueryRowContext(ctx, `
		SELECT count(*) FROM assignments a JOIN subjects s ON s.id = a.subject_id
		WHERE a.unlocked_at IS NOT NULL AND a.started_at IS NULL AND a.hidden = 0 AND s.level <= ?`,
		maxLevel).Scan(&q.Lessons)
	if err != nil {
		return q, err
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT a.available_at FROM assignments a JOIN subjects s ON s.id = a.subject_id
		WHERE a.available_at IS NOT NULL AND a.started_at IS NOT NULL
		  AND a.hidden = 0 AND s.level <= ?
		ORDER BY a.available_at`, maxLevel)
	if err != nil {
		return q, err
	}
	defer rows.Close()
	end := micros(q.Start.Add(24 * time.Hour))
	for rows.Next() {
		var at sql.NullInt64
		if err := rows.Scan(&at); err != nil {
			return q, err
		}
		switch {
		case at.Int64 <= nowUS:
			q.Reviews++
		case at.Int64 < end:
			if q.Next == nil {
				q.Next = fromMicros(at)
			}
			q.Forecast[int((at.Int64-micros(q.Start))/int64(time.Hour/time.Microsecond))]++
		default:
			if q.Next == nil {
				q.Next = fromMicros(at)
			}
		}
	}
	return q, rows.Err()
}

// LevelProgress is the user's kanji progress on a level. WaniKani levels up
// when 90% of the level's kanji have been passed (reached Guru).
type LevelProgress struct {
	Level  int
	Kanji  int // non-hidden kanji on the level
	Passed int
	Needed int // kanji to pass for level-up: ceil(0.9 × Kanji)
}

// LevelProgress returns kanji progress for a level.
func (s *Store) LevelProgress(ctx context.Context, level int) (LevelProgress, error) {
	p := LevelProgress{Level: level}
	err := s.db.QueryRowContext(ctx, `
		SELECT count(*), count(a.passed_at)
		FROM subjects s LEFT JOIN assignments a ON a.subject_id = s.id
		WHERE s.object = 'kanji' AND s.level = ? AND s.hidden = 0`, level).Scan(&p.Kanji, &p.Passed)
	p.Needed = (p.Kanji*9 + 9) / 10
	return p, err
}

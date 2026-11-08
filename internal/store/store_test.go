package store

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jtharris/yadokari/internal/wk"
	"github.com/jtharris/yadokari/internal/wkmock"
)

var now = time.Date(2026, 11, 7, 12, 30, 0, 0, time.UTC)

func toRaw[T any](t *testing.T, rs []wk.Resource[T]) []RawResource {
	t.Helper()
	out := make([]RawResource, len(rs))
	for i, r := range rs {
		data, err := json.Marshal(r.Data)
		if err != nil {
			t.Fatal(err)
		}
		out[i] = RawResource{ID: r.ID, Object: r.Object, URL: r.URL, DataUpdatedAt: r.DataUpdatedAt, Data: data}
	}
	return out
}

func openSample(t *testing.T) *Store {
	t.Helper()
	ctx := context.Background()
	s, err := Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	d := wkmock.SampleData(now)
	for _, c := range []struct {
		name string
		rs   []RawResource
	}{
		{Subjects, toRaw(t, d.Subjects)},
		{Assignments, toRaw(t, d.Assignments)},
		{StudyMaterials, toRaw(t, d.StudyMaterials)},
		{SpacedRepetitionSystems, toRaw(t, d.SRS)},
	} {
		if err := s.Upsert(ctx, c.name, c.rs); err != nil {
			t.Fatal(err)
		}
	}
	return s
}

func TestQueueAndLevelProgress(t *testing.T) {
	s := openSample(t)
	ctx := context.Background()

	q, err := s.Queue(ctx, now, 3)
	if err != nil {
		t.Fatal(err)
	}
	if q.Lessons != 2 || q.Reviews != 4 {
		t.Errorf("lessons %d reviews %d, want 2 and 4", q.Lessons, q.Reviews)
	}
	// Kanji 102 is due at 15:00, three hours after the 12:00 bucket start.
	want := [24]int{}
	want[3] = 1
	if q.Forecast != want {
		t.Errorf("forecast %v", q.Forecast)
	}
	if q.Next == nil || !q.Next.Equal(time.Date(2026, 11, 7, 15, 0, 0, 0, time.UTC)) {
		t.Errorf("next %v", q.Next)
	}

	// maxLevel 0 means no subscription cap.
	if q, _ := s.Queue(ctx, now, 0); q.Reviews != 4 {
		t.Errorf("maxLevel 0 means no cap: reviews %d", q.Reviews)
	}

	p, err := s.LevelProgress(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if p != (LevelProgress{Level: 1, Kanji: 2, Passed: 1, Needed: 2}) {
		t.Errorf("progress %+v", p)
	}
}

func TestUpsertReplaces(t *testing.T) {
	s := openSample(t)
	ctx := context.Background()
	d := wkmock.SampleData(now)

	// Assignment 1006 (kana vocab, due now) is reviewed and moves to tomorrow.
	a := d.Assignments[6]
	later := now.Add(26 * time.Hour)
	a.Data.AvailableAt = &later
	a.DataUpdatedAt = now
	if err := s.Upsert(ctx, Assignments, toRaw(t, []wk.Resource[wk.Assignment]{a})); err != nil {
		t.Fatal(err)
	}
	if n, _ := s.Count(ctx, Assignments); n != len(d.Assignments) {
		t.Errorf("count %d after replace", n)
	}
	if q, _ := s.Queue(ctx, now, 3); q.Reviews != 3 {
		t.Errorf("reviews %d, want 3", q.Reviews)
	}
}

func TestSingletonAndSyncState(t *testing.T) {
	s := openSample(t)
	ctx := context.Background()

	if u, err := s.User(ctx); err != nil || u != nil {
		t.Fatalf("no user yet: %v %v", u, err)
	}
	raw, _ := json.Marshal(wkmock.SampleData(now).User)
	if err := s.PutSingleton(ctx, "user", Singleton{Raw: raw, Validator: wk.Conditional{ETag: `W/"x"`}, FetchedAt: now}); err != nil {
		t.Fatal(err)
	}
	u, err := s.User(ctx)
	if err != nil || u.Data.Username != "mock_learner" {
		t.Fatalf("user %+v %v", u, err)
	}
	v, ok, _ := s.Singleton(ctx, "user")
	if !ok || v.Validator.ETag != `W/"x"` || !v.FetchedAt.Equal(now) {
		t.Fatalf("singleton %+v", v)
	}

	st, _ := s.SyncState(ctx, Subjects)
	if st.UpdatedAfter != nil {
		t.Fatal("fresh store should have no bookmark")
	}
	after := now.Add(-time.Hour).Add(123 * time.Microsecond)
	if err := s.SetSyncState(ctx, Subjects, SyncState{UpdatedAfter: &after, SyncedAt: now}); err != nil {
		t.Fatal(err)
	}
	st, _ = s.SyncState(ctx, Subjects)
	if st.UpdatedAfter == nil || !st.UpdatedAfter.Equal(after) || !st.SyncedAt.Equal(now) {
		t.Fatalf("bookmark %+v", st)
	}

	sub, err := s.Subject(ctx, 101)
	if err != nil || sub == nil || *sub.Data.Characters != "一" {
		t.Fatalf("subject %+v %v", sub, err)
	}
}

func TestReopenKeepsDataAndSkipsMigrations(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "cache.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	d := wkmock.SampleData(now)
	if err := s.Upsert(ctx, Subjects, toRaw(t, d.Subjects)); err != nil {
		t.Fatal(err)
	}
	s.Close()

	s, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if n, _ := s.Count(ctx, Subjects); n != len(d.Subjects) {
		t.Fatalf("subjects after reopen: %d", n)
	}
}

// TestRecordedAccount loads a local recording (gitignored) and checks the
// counts against what the recording's summary showed. Skipped without one.
func TestRecordedAccount(t *testing.T) {
	dirs, _ := filepath.Glob("../../testdata/recordings/*")
	if len(dirs) == 0 {
		t.Skip("no recording (run `just probe-live`)")
	}
	dir := dirs[len(dirs)-1]
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "cache.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	files := map[string]string{
		Subjects: wkmock.FileSubjects, Assignments: wkmock.FileAssignments, StudyMaterials: wkmock.FileStudyMaterials,
		SpacedRepetitionSystems: wkmock.FileSRS, ReviewStatistics: wkmock.FileReviewStatistics,
	}
	start := time.Now()
	for coll, name := range files {
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		var rs []RawResource
		if err := json.Unmarshal(raw, &rs); err != nil {
			t.Fatal(err)
		}
		if err := s.Upsert(ctx, coll, rs); err != nil {
			t.Fatal(err)
		}
		if n, _ := s.Count(ctx, coll); n != len(rs) {
			t.Errorf("%s: stored %d of %d", coll, n, len(rs))
		}
	}
	t.Logf("loaded %s in %v", filepath.Base(dir), time.Since(start).Round(time.Millisecond))

	info, err := os.Stat(filepath.Join(dir, wkmock.FileAssignments))
	if err != nil {
		t.Fatal(err)
	}
	t0 := time.Now()
	q, err := s.Queue(ctx, info.ModTime(), 60)
	if err != nil {
		t.Fatal(err)
	}
	p, _ := s.LevelProgress(ctx, 60)
	t.Logf("at %s: lessons %d, reviews %d, next 24h %v (queried in %v); level 60 kanji %d/%d passed, %d needed",
		info.ModTime().Format(time.Kitchen), q.Lessons, q.Reviews, q.Forecast, time.Since(t0).Round(time.Microsecond),
		p.Passed, p.Kanji, p.Needed)

	// The mock computes /summary independently from the same assignments.
	data, err := wkmock.LoadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	m := wkmock.New(data)
	m.Now = func() time.Time { return info.ModTime() }
	srv := m.Start()
	defer srv.Close()
	c, _ := wk.New("x", wk.WithBaseURL(srv.URL+"/v2"))
	sum, _, err := c.Summary(ctx, wk.Conditional{})
	if err != nil {
		t.Fatal(err)
	}
	if got := len(sum.Data.Lessons[0].SubjectIDs); got != q.Lessons {
		t.Errorf("lessons: store %d, mock summary %d", q.Lessons, got)
	}
	if got := len(sum.Data.Reviews[0].SubjectIDs); got != q.Reviews {
		t.Errorf("reviews now: store %d, mock summary %d", q.Reviews, got)
	}
}

func TestResurrectedItemIsReviewable(t *testing.T) {
	s := openSample(t)
	ctx := context.Background()
	d := wkmock.SampleData(now)
	a := d.Assignments[7] // radical 2, due in 30h
	burned := now.Add(-100 * 24 * time.Hour)
	resurrected := now.Add(-2 * time.Hour)
	due := now.Add(-time.Hour)
	a.Data.SRSStage, a.Data.BurnedAt, a.Data.ResurrectedAt, a.Data.AvailableAt = 1, &burned, &resurrected, &due
	if err := s.Upsert(ctx, Assignments, toRaw(t, []wk.Resource[wk.Assignment]{a})); err != nil {
		t.Fatal(err)
	}
	if q, _ := s.Queue(ctx, now, 3); q.Reviews != 5 {
		t.Errorf("reviews %d, want 5 (resurrected item counts)", q.Reviews)
	}
}

func TestRejectedAndOutcomes(t *testing.T) {
	ctx := context.Background()
	s := openSample(t)
	// 1000 (stage 4) stays pending, 1001 (stage 1) is rejected, 1002 (stage 2)
	// is never queued.
	for _, q := range []struct{ subject, assignment int }{{1, 1000}, {3, 1001}} {
		if err := s.QueueReview(ctx, q.subject, wk.ReviewInput{AssignmentID: q.assignment}); err != nil {
			t.Fatal(err)
		}
	}
	items, _ := s.Outbox(ctx, false)
	if len(items) != 2 || items[1].SubjectID != 3 {
		t.Fatalf("outbox %+v", items)
	}
	if err := s.OutboxFailed(ctx, items[1].ID, "HTTP 422: not due"); err != nil {
		t.Fatal(err)
	}

	rej, err := s.Rejected(ctx)
	if err != nil || len(rej) != 1 || rej[0].Review.AssignmentID != 1001 || rej[0].Failed != "HTTP 422: not due" {
		t.Fatalf("rejected %+v %v", rej, err)
	}

	got, err := s.ReviewOutcomes(ctx, []int{1000, 1001, 1002, 9999})
	if err != nil {
		t.Fatal(err)
	}
	want := map[int]ReviewOutcome{
		1000: {SRSStage: 4, Queued: true},
		1001: {SRSStage: 1, Queued: true, Failed: "HTTP 422: not due"},
		1002: {SRSStage: 2},
	}
	if len(got) != len(want) {
		t.Errorf("outcomes %+v", got)
	}
	for id, w := range want {
		if got[id] != w {
			t.Errorf("outcome %d = %+v, want %+v", id, got[id], w)
		}
	}

	if n, err := s.DismissRejected(ctx); err != nil || n != 1 {
		t.Fatalf("dismiss: %d %v", n, err)
	}
	if p, f, _ := s.OutboxCounts(ctx); p != 1 || f != 0 {
		t.Errorf("after dismiss: %d pending, %d failed", p, f)
	}
}

func TestSRSCounts(t *testing.T) {
	s := openSample(t)
	got, err := s.SRSCounts(context.Background(), 3)
	if err != nil {
		t.Fatal(err)
	}
	// Sample stages: 4, 1, 2, 5, 3, 6 started; two lessons (stage 0) excluded.
	if got != [10]int{0, 1, 1, 1, 1, 1, 1, 0, 0, 0} {
		t.Errorf("SRSCounts = %v", got)
	}
}

package wksync

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/dotmatrx/yadokari/internal/store"
	"github.com/dotmatrx/yadokari/internal/wk"
	"github.com/dotmatrx/yadokari/internal/wkmock"
)

var now = time.Date(2026, 11, 7, 12, 30, 0, 0, time.UTC)

type env struct {
	mock   *wkmock.Server
	client *wk.Client
	store  *store.Store
	syncer *Syncer
	events []Event
}

func setup(t *testing.T, data wkmock.Data, clock *time.Time) *env {
	t.Helper()
	e := &env{mock: wkmock.New(data)}
	e.mock.Now = func() time.Time { return *clock }
	srv := e.mock.Start()
	t.Cleanup(srv.Close)
	var err error
	e.client, err = wk.New("tok", wk.WithBaseURL(srv.URL+"/v2"), wk.WithRateLimit(60000, 1000))
	if err != nil {
		t.Fatal(err)
	}
	e.store, err = store.Open(context.Background(), filepath.Join(t.TempDir(), "cache.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { e.store.Close() })
	e.syncer = &Syncer{Client: e.client, Store: e.store, Now: func() time.Time { return *clock },
		Progress: func(ev Event) { e.events = append(e.events, ev) }}
	return e
}

func all() []string { return append(append([]string{}, Foreground...), Background...) }

func TestFullThenIncremental(t *testing.T) {
	clock := now
	e := setup(t, wkmock.SampleData(now), &clock)
	ctx := context.Background()

	res, err := e.syncer.Run(ctx, all())
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range res {
		if !r.Full {
			t.Errorf("%s: first sync should be full", r.Collection)
		}
	}
	if got := e.client.Requests(); got != 6 {
		t.Errorf("first sync: %d requests, want 6 (user + 5 collections)", got)
	}
	for c, want := range map[string]int{store.Subjects: 8, store.Assignments: 8, store.StudyMaterials: 1, store.SpacedRepetitionSystems: 1} {
		if n, _ := e.store.Count(ctx, c); n != want {
			t.Errorf("%s: %d stored, want %d", c, n, want)
		}
	}
	if u, _ := e.store.User(ctx); u == nil || u.Data.Username != "mock_learner" {
		t.Fatalf("user not stored: %+v", u)
	}

	// Nothing changed: one request per collection plus a 304 for the user.
	e.events = nil
	before := e.client.Requests()
	res, err = e.syncer.Run(ctx, all())
	if err != nil {
		t.Fatal(err)
	}
	if got := e.client.Requests() - before; got != 6 {
		t.Errorf("second sync: %d requests, want 6", got)
	}
	for _, r := range res {
		// An empty collection has no data_updated_at to bookmark, so it is
		// fetched "in full" again: still a single, empty request.
		n, _ := e.store.Count(ctx, r.Collection)
		if r.Fetched != 0 || (r.Full && n > 0) {
			t.Errorf("%s: second sync fetched %d (full=%v), want 0 incremental", r.Collection, r.Fetched, r.Full)
		}
	}

	// A review on the server updates one assignment and creates a statistic.
	clock = now.Add(time.Minute)
	if _, err := e.client.CreateReview(ctx, wk.ReviewInput{AssignmentID: 1002}); err != nil {
		t.Fatal(err)
	}
	clock = now.Add(2 * time.Minute)
	res, err = e.syncer.Collections(ctx, all())
	if err != nil {
		t.Fatal(err)
	}
	fetched := map[string]int{}
	for _, r := range res {
		fetched[r.Collection] = r.Fetched
	}
	if fetched[store.Assignments] != 1 || fetched[store.ReviewStatistics] != 1 || fetched[store.Subjects] != 0 {
		t.Errorf("after a review: fetched %v", fetched)
	}
	q, _ := e.store.Queue(ctx, clock, 3)
	if q.Reviews != 3 {
		t.Errorf("reviews after one was done: %d, want 3", q.Reviews)
	}
}

func TestProgressEvents(t *testing.T) {
	clock := now
	data := wkmock.SampleData(now)
	e := setup(t, data, &clock)
	e.mock.PerPage = 3
	if _, err := e.syncer.Collection(context.Background(), store.Subjects); err != nil {
		t.Fatal(err)
	}
	var fetched []int
	for _, ev := range e.events {
		if ev.Total != 8 {
			t.Errorf("event total %d, want 8", ev.Total)
		}
		fetched = append(fetched, ev.Fetched)
	}
	if len(fetched) != 4 || fetched[0] != 3 || fetched[2] != 8 || !e.events[3].Done {
		t.Errorf("events %+v", e.events)
	}
}

func TestFailedSyncKeepsBookmark(t *testing.T) {
	clock := now
	e := setup(t, wkmock.SampleData(now), &clock)
	ctx := context.Background()
	if _, err := e.syncer.Collection(ctx, store.Subjects); err != nil {
		t.Fatal(err)
	}
	before, _ := e.store.SyncState(ctx, store.Subjects)

	e.mock.Token = "other" // every request now fails with 401
	if _, err := e.syncer.Collection(ctx, store.Subjects); !wk.IsStatus(err, 401) {
		t.Fatalf("want 401, got %v", err)
	}
	after, _ := e.store.SyncState(ctx, store.Subjects)
	if !after.SyncedAt.Equal(before.SyncedAt) {
		t.Error("a failed sync must not move the bookmark")
	}
}

// TestRecordedAccount syncs a whole recorded account through the mock and
// checks request counts. Skipped without a local recording.
func TestRecordedAccount(t *testing.T) {
	dirs, _ := filepath.Glob("../../testdata/recordings/*")
	if len(dirs) == 0 {
		t.Skip("no recording (run `just probe-live`)")
	}
	data, err := wkmock.LoadDir(dirs[len(dirs)-1])
	if err != nil {
		t.Fatal(err)
	}
	clock := time.Now()
	e := setup(t, data, &clock)
	ctx := context.Background()

	t0 := time.Now()
	if _, err := e.syncer.Run(ctx, Foreground); err != nil {
		t.Fatal(err)
	}
	fg := e.client.Requests()
	if _, err := e.syncer.Collections(ctx, Background); err != nil {
		t.Fatal(err)
	}
	full := e.client.Requests()
	elapsed := time.Since(t0)

	before := e.client.Requests()
	if _, err := e.syncer.Run(ctx, all()); err != nil {
		t.Fatal(err)
	}
	again := e.client.Requests() - before
	t.Logf("first sync: %d foreground + %d background requests (%v without rate limiting); repeat sync: %d requests",
		fg, full-fg, elapsed.Round(time.Millisecond), again)
	if again != 6 {
		t.Errorf("repeat sync should be 6 requests, got %d", again)
	}
	for _, c := range all() {
		n, _ := e.store.Count(ctx, c)
		var want int
		switch c {
		case store.Subjects:
			want = len(data.Subjects)
		case store.Assignments:
			want = len(data.Assignments)
		case store.StudyMaterials:
			want = len(data.StudyMaterials)
		case store.SpacedRepetitionSystems:
			want = len(data.SRS)
		case store.ReviewStatistics:
			want = len(data.ReviewStatistics)
		}
		if n != want {
			t.Errorf("%s: %d stored, want %d", c, n, want)
		}
	}
}

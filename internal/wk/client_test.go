package wk_test

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"testing"
	"time"

	"yadokari/internal/wk"
	"yadokari/internal/wkmock"
)

const token = "test-token"

var now = time.Date(2026, 11, 7, 12, 30, 0, 0, time.UTC)

func setup(t *testing.T, configure func(*wkmock.Server)) (*wk.Client, *wkmock.Server) {
	t.Helper()
	m := wkmock.New(wkmock.SampleData(now))
	m.Token = token
	m.Now = func() time.Time { return now }
	if configure != nil {
		configure(m)
	}
	srv := m.Start()
	t.Cleanup(srv.Close)
	c, err := wk.New(token, wk.WithBaseURL(srv.URL+"/v2"), wk.WithRateLimit(6000, 100))
	if err != nil {
		t.Fatal(err)
	}
	return c, m
}

func TestUserAndConditionalGet(t *testing.T) {
	c, _ := setup(t, nil)
	ctx := context.Background()

	u, meta, err := c.User(ctx, wk.Conditional{})
	if err != nil {
		t.Fatal(err)
	}
	if u.Data.Username != "mock_learner" || u.Data.Subscription.MaxLevelGranted != 3 {
		t.Fatalf("unexpected user: %+v", u.Data)
	}
	if meta.ETag == "" {
		t.Fatal("expected an ETag")
	}

	u2, meta2, err := c.User(ctx, wk.Conditional{ETag: meta.ETag})
	if err != nil {
		t.Fatal(err)
	}
	if !meta2.NotModified || u2 != nil {
		t.Fatalf("expected 304, got status %d", meta2.Status)
	}
}

func TestSummary(t *testing.T) {
	c, _ := setup(t, nil)
	s, _, err := c.Summary(context.Background(), wk.Conditional{})
	if err != nil {
		t.Fatal(err)
	}
	if got := len(s.Data.Lessons[0].SubjectIDs); got != 2 {
		t.Errorf("lessons: got %d, want 2", got)
	}
	if got := len(s.Data.Reviews); got != 25 {
		t.Errorf("review buckets: got %d, want 25", got)
	}
	if got := len(s.Data.Reviews[0].SubjectIDs); got != 4 {
		t.Errorf("reviews available now: got %d, want 4", got)
	}
}

func TestPaginationFollowsNextURL(t *testing.T) {
	c, m := setup(t, func(m *wkmock.Server) { m.PerPage = 3 })
	var ids []int
	pages := 0
	err := c.Subjects(context.Background(), nil, func(p *wk.Collection[wk.Subject]) error {
		pages++
		if p.TotalCount != 8 {
			t.Errorf("total_count = %d, want 8", p.TotalCount)
		}
		for _, r := range p.Data {
			ids = append(ids, r.ID)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 8 || pages != 3 {
		t.Fatalf("got %d subjects over %d pages, want 8 over 3", len(ids), pages)
	}
	if int(c.Requests()) != m.Requests() || m.Requests() != 3 {
		t.Fatalf("requests: client %d, server %d, want 3", c.Requests(), m.Requests())
	}
}

func TestFilters(t *testing.T) {
	c, _ := setup(t, nil)
	ctx := context.Background()
	count := func(q url.Values) int {
		n := 0
		if err := c.Assignments(ctx, q, func(p *wk.Collection[wk.Assignment]) error {
			n += len(p.Data)
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if got := count(url.Values{"immediately_available_for_review": {""}}); got != 4 {
		t.Errorf("available for review: %d, want 4", got)
	}
	if got := count(url.Values{"immediately_available_for_lessons": {""}}); got != 2 {
		t.Errorf("available for lessons: %d, want 2", got)
	}
	if got := count(url.Values{"subject_types": {"kanji"}}); got != 2 {
		t.Errorf("kanji: %d, want 2", got)
	}

	subjects := 0
	err := c.Subjects(ctx, url.Values{"types": {"radical"}, "updated_after": {wk.Time(now.Add(-time.Hour))}}, func(p *wk.Collection[wk.Subject]) error {
		subjects += len(p.Data)
		return nil
	})
	if err != nil || subjects != 0 {
		t.Errorf("updated_after should exclude old subjects: n=%d err=%v", subjects, err)
	}
}

func TestUnauthorized(t *testing.T) {
	_, m := setup(t, nil)
	srv := m.Start()
	defer srv.Close()
	c, _ := wk.New("wrong", wk.WithBaseURL(srv.URL+"/v2"))
	_, _, err := c.User(context.Background(), wk.Conditional{})
	if !wk.IsStatus(err, http.StatusUnauthorized) {
		t.Fatalf("want 401, got %v", err)
	}
}

func TestCreateReviewAgainstMock(t *testing.T) {
	c, m := setup(t, nil)
	ctx := context.Background()

	// Kanji 101 (assignment 1002) is due at stage 2; two reading misses → stage 1.
	out, err := c.CreateReview(ctx, wk.ReviewInput{AssignmentID: 1002, IncorrectReadingAnswers: 2})
	if err != nil {
		t.Fatal(err)
	}
	if out.Data.StartingSRSStage != 2 || out.Data.EndingSRSStage != 1 {
		t.Errorf("stages %d→%d, want 2→1", out.Data.StartingSRSStage, out.Data.EndingSRSStage)
	}
	if out.ResourcesUpdated.Assignment.Data.SRSStage != 1 {
		t.Errorf("resources_updated assignment stage = %d", out.ResourcesUpdated.Assignment.Data.SRSStage)
	}
	if a, _ := m.Assignment(1002); a.Data.AvailableAt == nil || !a.Data.AvailableAt.After(now) {
		t.Errorf("assignment should be rescheduled into the future: %+v", a.Data.AvailableAt)
	}

	// The same assignment is no longer due, so a double submit is rejected.
	_, err = c.CreateReview(ctx, wk.ReviewInput{AssignmentID: 1002})
	if !wk.IsStatus(err, http.StatusUnprocessableEntity) {
		t.Fatalf("double submit: want 422, got %v", err)
	}

	w := m.Writes()
	if len(w) != 2 || w[0].Status != http.StatusCreated || w[1].Status != http.StatusUnprocessableEntity {
		t.Fatalf("write log: %+v", w)
	}
}

func TestStartAssignmentAgainstMock(t *testing.T) {
	c, _ := setup(t, nil)
	ctx := context.Background()
	a, err := c.StartAssignment(ctx, 1004, nil) // vocabulary 201 lesson
	if err != nil {
		t.Fatal(err)
	}
	if a.Data.SRSStage != 1 || a.Data.StartedAt == nil || a.Data.AvailableAt == nil {
		t.Fatalf("not started: %+v", a.Data)
	}
	if _, err := c.StartAssignment(ctx, 1004, nil); !wk.IsStatus(err, http.StatusUnprocessableEntity) {
		t.Fatalf("second start: want 422, got %v", err)
	}
}

// failingTransport fails the test if any request reaches the network.
type failingTransport struct{ t *testing.T }

func (f failingTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	f.t.Errorf("unexpected request to %s", r.URL)
	return nil, errors.New("no network in tests")
}

func TestLiveWritesBlockedByDefault(t *testing.T) {
	c, err := wk.New(token, wk.WithHTTPClient(&http.Client{Transport: failingTransport{t}}))
	if err != nil {
		t.Fatal(err)
	}
	if !c.IsLive() {
		t.Fatal("default client should target the live API")
	}
	ctx := context.Background()
	if _, err := c.CreateReview(ctx, wk.ReviewInput{AssignmentID: 1}); !errors.Is(err, wk.ErrLiveWritesDisabled) {
		t.Errorf("CreateReview: got %v", err)
	}
	if _, err := c.StartAssignment(ctx, 1, nil); !errors.Is(err, wk.ErrLiveWritesDisabled) {
		t.Errorf("StartAssignment: got %v", err)
	}
}

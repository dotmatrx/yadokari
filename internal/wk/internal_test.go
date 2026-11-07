package wk

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

func testClient(t *testing.T, url string) (*Client, *[]time.Duration) {
	t.Helper()
	c, err := New("tok", WithBaseURL(url+"/v2"), WithRateLimit(6000, 100))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_800_000_000, 0)
	var slept []time.Duration
	c.now = func() time.Time { return now }
	c.sleep = func(_ context.Context, d time.Duration) error {
		slept = append(slept, d)
		now = now.Add(d)
		return nil
	}
	return c, &slept
}

func TestRetryAfter429UsesReset(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.Header().Set("RateLimit-Limit", "60")
			w.Header().Set("RateLimit-Remaining", "0")
			w.Header().Set("RateLimit-Reset", strconv.FormatInt(1_800_000_000+20, 10))
			w.WriteHeader(http.StatusTooManyRequests)
			fmt.Fprint(w, "Rate Limit Exceeded")
			return
		}
		fmt.Fprint(w, `{"object":"user","data":{"username":"x"}}`)
	}))
	defer srv.Close()

	c, slept := testClient(t, srv.URL)
	u, _, err := c.User(context.Background(), Conditional{})
	if err != nil {
		t.Fatal(err)
	}
	if u.Data.Username != "x" || calls.Load() != 2 {
		t.Fatalf("user=%q calls=%d", u.Data.Username, calls.Load())
	}
	if len(*slept) != 1 || (*slept)[0] < 20*time.Second || (*slept)[0] > 21*time.Second {
		t.Fatalf("slept %v, want ~20s until reset", *slept)
	}
}

func TestWritesNotRetriedOn5xx(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	c, _ := testClient(t, srv.URL)
	if _, err := c.CreateReview(context.Background(), ReviewInput{AssignmentID: 1}); !IsStatus(err, http.StatusBadGateway) {
		t.Fatalf("got %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("POST was sent %d times; an ambiguous write must not be retried", calls.Load())
	}

	calls.Store(0)
	if _, _, err := c.User(context.Background(), Conditional{}); err == nil {
		t.Fatal("expected error")
	}
	if calls.Load() != 4 {
		t.Fatalf("GET attempts = %d, want 4 (1 + 3 retries)", calls.Load())
	}
}

func TestNextURLOnForeignHostIsRefused(t *testing.T) {
	var foreignHits atomic.Int32
	foreign := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		foreignHits.Add(1)
	}))
	defer foreign.Close()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"object":"collection","pages":{"next_url":%q},"data":[{"id":1,"object":"kanji","data":{}}]}`,
			foreign.URL+"/v2/subjects?page_after_id=1")
	}))
	defer srv.Close()

	c, _ := testClient(t, srv.URL)
	err := c.Subjects(context.Background(), nil, func(*Collection[Subject]) error { return nil })
	if err == nil {
		t.Fatal("expected refusal to follow next_url to another host")
	}
	if foreignHits.Load() != 0 {
		t.Fatal("token was sent to a foreign host")
	}
}

func TestRemainingZeroPausesNextRequest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("RateLimit-Limit", "60")
		w.Header().Set("RateLimit-Remaining", "0")
		w.Header().Set("RateLimit-Reset", strconv.FormatInt(1_800_000_000+30, 10))
		fmt.Fprint(w, `{"object":"user","data":{}}`)
	}))
	defer srv.Close()

	c, slept := testClient(t, srv.URL)
	ctx := context.Background()
	if _, _, err := c.User(ctx, Conditional{}); err != nil {
		t.Fatal(err)
	}
	if len(*slept) != 0 {
		t.Fatalf("first request should not wait: %v", *slept)
	}
	if _, _, err := c.User(ctx, Conditional{}); err != nil {
		t.Fatal(err)
	}
	if len(*slept) != 1 || (*slept)[0] != 30*time.Second {
		t.Fatalf("second request should wait for reset: %v", *slept)
	}
}

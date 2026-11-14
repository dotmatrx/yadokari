package wksync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"

	"github.com/jtharris/yadokari/internal/store"
	"github.com/jtharris/yadokari/internal/wk"
)

// Uploader sends queued writes from the store's outbox, oldest first: reviews
// (POST /reviews) and lesson starts (PUT /assignments/:id/start).
//
// WaniKani has no idempotency keys, but its own rule gives the same safety:
// a review is only accepted while the assignment is due, and accepting it
// reschedules the assignment, so a second identical POST is refused (422).
// An outbox row is deleted only after a 2xx; a write whose outcome is
// unknown (network error, 5xx) is never retried automatically by the client,
// and is retried here on the next flush, where a 422 then means it was
// recorded the first time.
//
// Flushes are serialized: the dashboard and the review screen both flush,
// and two concurrent flushes would post the same row twice.
type Uploader struct {
	Client *wk.Client
	Store  *store.Store

	mu sync.Mutex
}

// UploadResult summarizes a flush.
type UploadResult struct {
	Sent     int  // accepted by the API
	Rejected int  // definitively refused; kept and shown, never retried
	Held     int  // not attempted: a development build against the live API
	Live     bool // the client points at the live API
}

// ErrHeld is returned when reviews are being held because writes to the
// live API are disabled.
var ErrHeld = errors.New("uploads held: development builds don't write to the live WaniKani API")

// Flush posts every pending review. It stops at the first transient failure
// (network, rate limit, server error) so reviews go out in order; the
// returned error describes it and the rest stay queued.
func (u *Uploader) Flush(ctx context.Context) (UploadResult, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	res := UploadResult{Live: u.Client.IsLive()}
	items, err := u.Store.Outbox(ctx, false)
	if err != nil {
		return res, err
	}
	for i, it := range items {
		updated, err := u.send(ctx, it)
		var apiErr *wk.APIError
		switch {
		case err == nil:
			if err := u.Store.OutboxDone(ctx, it.ID, updated); err != nil {
				return res, err
			}
			res.Sent++

		case errors.Is(err, wk.ErrLiveWritesDisabled):
			res.Held = len(items) - i
			return res, ErrHeld

		case errors.As(err, &apiErr) && definitive(apiErr.Status):
			reason := fmt.Sprintf("HTTP %d: %s", apiErr.Status, apiErr.Message)
			if apiErr.Status == http.StatusUnprocessableEntity && it.Attempts > 0 {
				reason += " (probably recorded by an earlier attempt whose reply was lost)"
			}
			if err := u.Store.OutboxFailed(ctx, it.ID, reason); err != nil {
				return res, err
			}
			res.Rejected++

		default:
			_ = u.Store.OutboxAttempted(ctx, it.ID)
			return res, fmt.Errorf("uploading %s for assignment %d: %w", what(it.Kind), it.AssignmentID, err)
		}
	}
	return res, nil
}

// send performs one outbox row's write and returns the resources the API
// sent back, for the store.
func (u *Uploader) send(ctx context.Context, it store.OutboxItem) (map[string][]store.RawResource, error) {
	if it.Kind == store.KindStart {
		at := it.StartedAt
		a, err := u.Client.StartAssignment(ctx, it.AssignmentID, &at)
		if err != nil {
			return nil, err
		}
		raw, err := rawOf(*a)
		if err != nil {
			return nil, err
		}
		return map[string][]store.RawResource{store.Assignments: {raw}}, nil
	}
	out, err := u.Client.CreateReview(ctx, it.Review)
	if err != nil {
		return nil, err
	}
	return resourcesUpdated(out)
}

func what(kind string) string {
	if kind == store.KindStart {
		return "lesson start"
	}
	return "review"
}

// definitive reports whether an HTTP status means "this review will never be
// accepted" rather than "try again later". Auth failures are not: the token
// may be fixed, and the review is still valid.
func definitive(status int) bool {
	switch status {
	case http.StatusNotFound, http.StatusConflict, http.StatusUnprocessableEntity:
		return true
	}
	return false
}

func resourcesUpdated(out *wk.ReviewCreated) (map[string][]store.RawResource, error) {
	a, rs := out.ResourcesUpdated.Assignment, out.ResourcesUpdated.ReviewStatistic
	aRaw, err := rawOf(a)
	if err != nil {
		return nil, err
	}
	updated := map[string][]store.RawResource{store.Assignments: {aRaw}}
	if rs.ID != 0 {
		rsRaw, err := rawOf(rs)
		if err != nil {
			return nil, err
		}
		updated[store.ReviewStatistics] = []store.RawResource{rsRaw}
	}
	return updated, nil
}

func rawOf[T any](r wk.Resource[T]) (store.RawResource, error) {
	data, err := json.Marshal(r.Data)
	if err != nil {
		return store.RawResource{}, err
	}
	return store.RawResource{ID: r.ID, Object: r.Object, URL: r.URL, DataUpdatedAt: r.DataUpdatedAt, Data: data}, nil
}

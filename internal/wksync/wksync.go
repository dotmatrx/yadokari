// Package wksync copies the user's WaniKani data into the local store.
//
// The first sync of a collection fetches everything; later syncs pass the
// collection's last data_updated_at as updated_after, so an unchanged
// collection costs one request. The user resource uses its ETag. Sync only
// reads from the API.
package wksync

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	"github.com/jtharris/yadokari/internal/store"
	"github.com/jtharris/yadokari/internal/wk"
)

// Foreground collections are needed before the dashboard is accurate.
// Subjects come before assignments so dashboard joins always resolve.
var Foreground = []string{
	store.SpacedRepetitionSystems,
	store.Subjects,
	store.Assignments,
	store.StudyMaterials,
}

// Background collections can arrive after the dashboard is shown.
var Background = []string{store.ReviewStatistics}

// Event reports sync progress.
type Event struct {
	Collection string // "user" or a collection name
	Fetched    int    // resources received so far in this sync
	Total      int    // total_count reported by the API (0 if unknown)
	Done       bool
}

// Syncer runs syncs. Progress may be nil.
type Syncer struct {
	Client   *wk.Client
	Store    *store.Store
	Now      func() time.Time
	Progress func(Event)
}

func (s *Syncer) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *Syncer) report(e Event) {
	if s.Progress != nil {
		s.Progress(e)
	}
}

// Result summarizes one sync of a collection.
type Result struct {
	Collection string
	Fetched    int
	Full       bool // no bookmark existed: everything was fetched
}

// User refreshes the stored user with a conditional GET.
func (s *Syncer) User(ctx context.Context) (*wk.Resource[wk.User], error) {
	prev, ok, err := s.Store.Singleton(ctx, "user")
	if err != nil {
		return nil, err
	}
	cond := wk.Conditional{}
	if ok {
		cond = prev.Validator
	}
	r, meta, err := s.Client.GetRawConditional(ctx, "user", cond)
	if err != nil {
		return nil, fmt.Errorf("sync user: %w", err)
	}
	if !meta.NotModified {
		raw, err := json.Marshal(r)
		if err != nil {
			return nil, err
		}
		err = s.Store.PutSingleton(ctx, "user", store.Singleton{
			Raw:       raw,
			Validator: wk.Conditional{ETag: meta.ETag, LastModified: meta.LastModified},
			FetchedAt: s.now(),
		})
		if err != nil {
			return nil, err
		}
	}
	s.report(Event{Collection: "user", Fetched: 1, Total: 1, Done: true})
	return s.Store.User(ctx)
}

// Collection syncs one collection, page by page.
func (s *Syncer) Collection(ctx context.Context, name string) (Result, error) {
	res := Result{Collection: name}
	st, err := s.Store.SyncState(ctx, name)
	if err != nil {
		return res, err
	}
	q := url.Values{}
	if st.UpdatedAfter != nil {
		q.Set("updated_after", wk.Time(*st.UpdatedAfter))
	} else {
		res.Full = true
	}

	started := s.now()
	var bookmark *time.Time
	first := true
	err = s.Client.EachRaw(ctx, name, q, func(p *wk.Collection[json.RawMessage]) error {
		if first {
			// The collection's data_updated_at covers the whole filtered
			// scope, not just this page; null when nothing matched.
			bookmark = p.DataUpdatedAt
			first = false
		}
		if err := s.Store.Upsert(ctx, name, p.Data); err != nil {
			return err
		}
		res.Fetched += len(p.Data)
		s.report(Event{Collection: name, Fetched: res.Fetched, Total: p.TotalCount})
		return nil
	})
	if err != nil {
		return res, fmt.Errorf("sync %s: %w", name, err)
	}
	if bookmark == nil {
		// Nothing new: keep the old bookmark. An empty collection stays
		// unbookmarked (the local clock is not a safe substitute for the
		// server's), so it is re-fetched in full: one empty request.
		bookmark = st.UpdatedAfter
	}
	if err := s.Store.SetSyncState(ctx, name, store.SyncState{UpdatedAfter: bookmark, SyncedAt: started}); err != nil {
		return res, err
	}
	s.report(Event{Collection: name, Fetched: res.Fetched, Total: res.Fetched, Done: true})
	return res, nil
}

// Run syncs the user and then each named collection in order.
func (s *Syncer) Run(ctx context.Context, collections []string) ([]Result, error) {
	if _, err := s.User(ctx); err != nil {
		return nil, err
	}
	return s.Collections(ctx, collections)
}

// Collections syncs each named collection in order, stopping at the first error.
func (s *Syncer) Collections(ctx context.Context, collections []string) ([]Result, error) {
	var out []Result
	for _, c := range collections {
		r, err := s.Collection(ctx, c)
		if err != nil {
			return out, err
		}
		out = append(out, r)
	}
	return out, nil
}

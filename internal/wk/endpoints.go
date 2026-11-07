package wk

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Ints formats ids for an array filter (comma-separated).
func Ints(ids []int) string {
	s := make([]string, len(ids))
	for i, id := range ids {
		s[i] = strconv.Itoa(id)
	}
	return strings.Join(s, ",")
}

// Time formats a timestamp for filters such as updated_after.
func Time(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

func getOne[T any](ctx context.Context, c *Client, path string, cond Conditional) (*Resource[T], Meta, error) {
	u, err := c.resolve(path, nil)
	if err != nil {
		return nil, Meta{}, err
	}
	var r Resource[T]
	meta, err := c.do(ctx, http.MethodGet, u, nil, cond, &r)
	if err != nil || meta.NotModified {
		return nil, meta, err
	}
	return &r, meta, nil
}

// each walks every page of a collection, following pages.next_url.
func each[T any](ctx context.Context, c *Client, path string, q url.Values, fn func(*Collection[T]) error) error {
	u, err := c.resolve(path, q)
	if err != nil {
		return err
	}
	for u != nil {
		var page Collection[T]
		if _, err := c.do(ctx, http.MethodGet, u, nil, Conditional{}, &page); err != nil {
			return err
		}
		if err := fn(&page); err != nil {
			return err
		}
		u = nil
		if page.Pages.NextURL != nil && *page.Pages.NextURL != "" && len(page.Data) > 0 {
			if u, err = c.resolve(*page.Pages.NextURL, nil); err != nil {
				return err
			}
		}
	}
	return nil
}

// User fetches GET /user. On 304 the resource is nil and meta.NotModified is set.
func (c *Client) User(ctx context.Context, cond Conditional) (*Resource[User], Meta, error) {
	return getOne[User](ctx, c, "user", cond)
}

// Summary fetches GET /summary.
func (c *Client) Summary(ctx context.Context, cond Conditional) (*Resource[Summary], Meta, error) {
	return getOne[Summary](ctx, c, "summary", cond)
}

// Subject fetches one subject.
func (c *Client) Subject(ctx context.Context, id int) (*Resource[Subject], error) {
	r, _, err := getOne[Subject](ctx, c, "subjects/"+strconv.Itoa(id), Conditional{})
	return r, err
}

// Subjects walks GET /subjects. Filters: ids, types, slugs, levels, hidden, updated_after.
func (c *Client) Subjects(ctx context.Context, q url.Values, fn func(*Collection[Subject]) error) error {
	return each(ctx, c, "subjects", q, fn)
}

// Assignments walks GET /assignments.
func (c *Client) Assignments(ctx context.Context, q url.Values, fn func(*Collection[Assignment]) error) error {
	return each(ctx, c, "assignments", q, fn)
}

// StudyMaterials walks GET /study_materials.
func (c *Client) StudyMaterials(ctx context.Context, q url.Values, fn func(*Collection[StudyMaterial]) error) error {
	return each(ctx, c, "study_materials", q, fn)
}

// SpacedRepetitionSystems walks GET /spaced_repetition_systems.
func (c *Client) SpacedRepetitionSystems(ctx context.Context, q url.Values, fn func(*Collection[SpacedRepetitionSystem]) error) error {
	return each(ctx, c, "spaced_repetition_systems", q, fn)
}

// ReviewStatistics walks GET /review_statistics.
func (c *Client) ReviewStatistics(ctx context.Context, q url.Values, fn func(*Collection[ReviewStatistic]) error) error {
	return each(ctx, c, "review_statistics", q, fn)
}

// CreateReview records a completed review. IRREVERSIBLE on the live API.
func (c *Client) CreateReview(ctx context.Context, in ReviewInput) (*ReviewCreated, error) {
	u, err := c.resolve("reviews", nil)
	if err != nil {
		return nil, err
	}
	var out ReviewCreated
	if _, err := c.do(ctx, http.MethodPost, u, map[string]ReviewInput{"review": in}, Conditional{}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// StartAssignment moves a finished lesson into reviews. IRREVERSIBLE on the live API.
func (c *Client) StartAssignment(ctx context.Context, id int, startedAt *time.Time) (*Resource[Assignment], error) {
	u, err := c.resolve("assignments/"+strconv.Itoa(id)+"/start", nil)
	if err != nil {
		return nil, err
	}
	body := map[string]any{"assignment": map[string]any{}}
	if startedAt != nil {
		body["assignment"] = map[string]any{"started_at": startedAt.UTC()}
	}
	var out Resource[Assignment]
	if _, err := c.do(ctx, http.MethodPut, u, body, Conditional{}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetRaw fetches a single resource without decoding its data, e.g. for
// recording fixtures exactly as the API sent them.
func (c *Client) GetRaw(ctx context.Context, path string) (*Resource[json.RawMessage], error) {
	r, _, err := getOne[json.RawMessage](ctx, c, path, Conditional{})
	return r, err
}

// EachRaw walks a collection without decoding resource data.
func (c *Client) EachRaw(ctx context.Context, path string, q url.Values, fn func(*Collection[json.RawMessage]) error) error {
	return each(ctx, c, path, q, fn)
}

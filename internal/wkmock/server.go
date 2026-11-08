// Package wkmock is an in-memory fake of the WaniKani API v2 for development
// and tests. It serves fixture data, paginates like the real API, honours
// conditional requests, can simulate the rate limit, and records every write
// so tests (and humans) can inspect what the client would have submitted.
//
// SRS behaviour on POST /reviews is a plausible imitation of WaniKani's, not
// a reference implementation.
package wkmock

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jtharris/yadokari/internal/wk"
)

// Write is a logged non-GET request.
type Write struct {
	Method string
	Path   string
	Body   json.RawMessage
	Status int
	At     time.Time
}

// Server is the fake API. Configure the exported fields before serving.
type Server struct {
	// Token is the accepted bearer token. Empty accepts any non-empty token.
	Token string
	// PerPage overrides page sizes (default: 1000 subjects, 500 others).
	PerPage int
	// RateLimit enables a fixed per-minute window when > 0.
	RateLimit int
	// Now is the server clock (default time.Now).
	Now func() time.Time

	mu          sync.Mutex
	data        Data
	writes      []Write
	requests    int
	windowStart time.Time
	windowCount int
}

// New returns a server holding a copy of data.
func New(data Data) *Server {
	s := &Server{Now: time.Now}
	s.data = data.clone()
	return s
}

// Start serves s on a local httptest server; the caller must Close it.
// The client base URL is srv.URL + "/v2/".
func (s *Server) Start() *httptest.Server { return httptest.NewServer(s.Handler()) }

// Writes returns the writes received so far.
func (s *Server) Writes() []Write {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.writes)
}

// Requests returns the number of requests served.
func (s *Server) Requests() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.requests
}

// Assignment returns the current state of an assignment.
func (s *Server) Assignment(id int) (wk.Resource[wk.Assignment], bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := slices.IndexFunc(s.data.Assignments, func(r wk.Resource[wk.Assignment]) bool { return r.ID == id })
	if i < 0 {
		return wk.Resource[wk.Assignment]{}, false
	}
	return s.data.Assignments[i], true
}

// Handler returns the HTTP handler.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v2/user", s.getUser)
	mux.HandleFunc("GET /v2/summary", s.getSummary)
	mux.HandleFunc("GET /v2/subjects", listHandler(s, "subjects", func(d *Data) []wk.Resource[wk.Subject] { return d.Subjects }, subjectFilter))
	mux.HandleFunc("GET /v2/subjects/{id}", oneHandler(s, func(d *Data) []wk.Resource[wk.Subject] { return d.Subjects }))
	mux.HandleFunc("GET /v2/assignments", listHandler(s, "assignments", func(d *Data) []wk.Resource[wk.Assignment] { return d.Assignments }, assignmentFilter))
	mux.HandleFunc("GET /v2/assignments/{id}", oneHandler(s, func(d *Data) []wk.Resource[wk.Assignment] { return d.Assignments }))
	mux.HandleFunc("GET /v2/study_materials", listHandler(s, "study_materials", func(d *Data) []wk.Resource[wk.StudyMaterial] { return d.StudyMaterials }, studyMaterialFilter))
	mux.HandleFunc("GET /v2/spaced_repetition_systems", listHandler(s, "spaced_repetition_systems", func(d *Data) []wk.Resource[wk.SpacedRepetitionSystem] { return d.SRS }, nil))
	mux.HandleFunc("GET /v2/review_statistics", listHandler(s, "review_statistics", func(d *Data) []wk.Resource[wk.ReviewStatistic] { return d.ReviewStatistics }, reviewStatisticFilter))
	mux.HandleFunc("POST /v2/reviews", s.postReview)
	mux.HandleFunc("POST /v2/reviews/", s.postReview)
	mux.HandleFunc("PUT /v2/assignments/{id}/start", s.startAssignment)
	return s.middleware(mux)
}

func (s *Server) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.requests++
		limited, rlHeaders := s.rateLimitLocked()
		s.mu.Unlock()
		for k, v := range rlHeaders {
			w.Header().Set(k, v)
		}

		tok, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok || tok == "" || (s.Token != "" && tok != s.Token) {
			writeError(w, http.StatusUnauthorized, "Unauthorized. Nice try.")
			return
		}
		if limited {
			w.WriteHeader(http.StatusTooManyRequests)
			fmt.Fprint(w, "Rate Limit Exceeded")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) rateLimitLocked() (limited bool, headers map[string]string) {
	if s.RateLimit <= 0 {
		return false, nil
	}
	now := s.Now()
	if now.Sub(s.windowStart) >= time.Minute {
		s.windowStart = now.Truncate(time.Minute)
		s.windowCount = 0
	}
	s.windowCount++
	remaining := max(s.RateLimit-s.windowCount, 0)
	headers = map[string]string{
		"RateLimit-Limit":     strconv.Itoa(s.RateLimit),
		"RateLimit-Remaining": strconv.Itoa(remaining),
		"RateLimit-Reset":     strconv.FormatInt(s.windowStart.Add(time.Minute).Unix(), 10),
	}
	return s.windowCount > s.RateLimit, headers
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]any{"error": msg, "code": status})
}

// writeJSON writes v with an ETag, answering 304 when the client's matches.
func writeJSON(w http.ResponseWriter, r *http.Request, status int, v any) {
	raw, err := json.Marshal(v)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	sum := sha256.Sum256(raw)
	etag := `W/"` + hex.EncodeToString(sum[:8]) + `"`
	w.Header().Set("ETag", etag)
	if r.Method == http.MethodGet && r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	w.Write(raw)
}

func baseURL(r *http.Request) string { return "http://" + r.Host + "/v2/" }

func (s *Server) getUser(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	u := s.data.User
	s.mu.Unlock()
	u.URL = baseURL(r) + "user"
	writeJSON(w, r, http.StatusOK, u)
}

// getSummary derives the report from assignments, as the real API does.
func (s *Server) getSummary(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.Now().UTC()
	hour := now.Truncate(time.Hour)

	lessons := wk.SummaryBucket{AvailableAt: hour, SubjectIDs: []int{}}
	reviews := make([]wk.SummaryBucket, 25)
	for i := range reviews {
		reviews[i] = wk.SummaryBucket{AvailableAt: hour.Add(time.Duration(i) * time.Hour), SubjectIDs: []int{}}
	}
	var next *time.Time
	for _, a := range s.data.Assignments {
		d := a.Data
		if d.Hidden || !s.levelGrantedLocked(s.assignmentLevelLocked(d)) {
			continue
		}
		if d.UnlockedAt != nil && d.StartedAt == nil {
			lessons.SubjectIDs = append(lessons.SubjectIDs, d.SubjectID)
			continue
		}
		if d.AvailableAt == nil { // burned items have none; resurrected ones keep burned_at
			continue
		}
		at := *d.AvailableAt
		if next == nil || at.Before(*next) {
			t := at
			next = &t
		}
		idx := 0
		if at.After(now) {
			idx = int(at.Sub(hour) / time.Hour)
		}
		if idx < len(reviews) {
			reviews[idx].SubjectIDs = append(reviews[idx].SubjectIDs, d.SubjectID)
		}
	}
	writeJSON(w, r, http.StatusOK, map[string]any{
		"object":          "report",
		"url":             baseURL(r) + "summary",
		"data_updated_at": hour,
		"data": wk.Summary{
			Lessons:       []wk.SummaryBucket{lessons},
			NextReviewsAt: next,
			Reviews:       reviews,
		},
	})
}

// assignmentLevelLocked returns the subject's level; real assignment payloads
// don't carry one.
func (s *Server) assignmentLevelLocked(a wk.Assignment) int {
	if a.Level != 0 {
		return a.Level
	}
	i, ok := slices.BinarySearchFunc(s.data.Subjects, a.SubjectID, func(r wk.Resource[wk.Subject], id int) int { return r.ID - id })
	if !ok {
		return 0
	}
	return s.data.Subjects[i].Data.Level
}

func (s *Server) levelGrantedLocked(level int) bool {
	granted := s.data.User.Data.Subscription.MaxLevelGranted
	return granted == 0 || level == 0 || level <= granted
}

type filterFunc[T any] func(s *Server, q url.Values, r wk.Resource[T]) (bool, error)

func listHandler[T any](s *Server, path string, get func(*Data) []wk.Resource[T], filter filterFunc[T]) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		s.mu.Lock()
		defer s.mu.Unlock()

		ids, err := intSet(q, "ids")
		if err != nil {
			writeError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		var updatedAfter time.Time
		if v := q.Get("updated_after"); v != "" {
			if updatedAfter, err = time.Parse(time.RFC3339Nano, v); err != nil {
				writeError(w, http.StatusUnprocessableEntity, "invalid updated_after")
				return
			}
		}
		after := 0
		if v := q.Get("page_after_id"); v != "" {
			if after, err = strconv.Atoi(v); err != nil {
				writeError(w, http.StatusUnprocessableEntity, "invalid page_after_id")
				return
			}
		}

		var matched []wk.Resource[T]
		var updated *time.Time
		for _, res := range get(&s.data) {
			if ids != nil && !ids[res.ID] {
				continue
			}
			if !updatedAfter.IsZero() && !res.DataUpdatedAt.After(updatedAfter) {
				continue
			}
			if filter != nil {
				ok, err := filter(s, q, res)
				if err != nil {
					writeError(w, http.StatusUnprocessableEntity, err.Error())
					return
				}
				if !ok {
					continue
				}
			}
			matched = append(matched, res)
			if updated == nil || res.DataUpdatedAt.After(*updated) {
				t := res.DataUpdatedAt
				updated = &t
			}
		}

		perPage := s.PerPage
		if perPage <= 0 {
			perPage = 500
			if path == "subjects" {
				perPage = 1000
			}
		}
		start, _ := slices.BinarySearchFunc(matched, after, func(r wk.Resource[T], id int) int { return r.ID - id })
		if start < len(matched) && matched[start].ID == after {
			start++
		}
		end := min(start+perPage, len(matched))
		page := matched[start:end]
		if page == nil {
			page = []wk.Resource[T]{}
		}

		var next *string
		if end < len(matched) {
			nq := cloneValues(q)
			nq.Set("page_after_id", strconv.Itoa(page[len(page)-1].ID))
			u := baseURL(r) + path + "?" + nq.Encode()
			next = &u
		}
		self := baseURL(r) + path
		if r.URL.RawQuery != "" {
			self += "?" + r.URL.RawQuery
		}
		writeJSON(w, r, http.StatusOK, wk.Collection[T]{
			Object:        "collection",
			URL:           self,
			Pages:         wk.Pages{NextURL: next, PerPage: perPage},
			TotalCount:    len(matched),
			DataUpdatedAt: updated,
			Data:          page,
		})
	}
}

func oneHandler[T any](s *Server, get func(*Data) []wk.Resource[T]) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(r.PathValue("id"))
		if err != nil {
			writeError(w, http.StatusNotFound, "Not found")
			return
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		items := get(&s.data)
		i := slices.IndexFunc(items, func(res wk.Resource[T]) bool { return res.ID == id })
		if i < 0 {
			writeError(w, http.StatusNotFound, "Not found")
			return
		}
		writeJSON(w, r, http.StatusOK, items[i])
	}
}

func cloneValues(q url.Values) url.Values {
	out := make(url.Values, len(q))
	for k, v := range q {
		out[k] = slices.Clone(v)
	}
	return out
}

package wkmock

import (
	"bytes"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"slices"
	"strconv"
	"time"

	"yadokari/internal/wk"
)

// defaultIntervals are the default SRS's stage intervals in seconds, used when
// no spaced_repetition_systems fixture covers the subject.
var defaultIntervals = map[int]int64{
	1: 14400, 2: 28800, 3: 82800, 4: 169200, 5: 601200, 6: 1206000, 7: 2588400, 8: 10364400,
}

const (
	passingStage = 5
	burnedStage  = 9
)

// recordingWriter captures the status for the write log.
type recordingWriter struct {
	http.ResponseWriter
	status int
}

func (w *recordingWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

// logged wraps a write handler so the request is recorded whatever the outcome.
func (s *Server) logged(w http.ResponseWriter, r *http.Request, h func(http.ResponseWriter, *http.Request, []byte)) {
	body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	rw := &recordingWriter{ResponseWriter: w, status: http.StatusOK}
	h(rw, r, body)
	s.mu.Lock()
	s.writes = append(s.writes, Write{
		Method: r.Method,
		Path:   r.URL.Path,
		Body:   json.RawMessage(bytes.Clone(body)),
		Status: rw.status,
		At:     s.Now(),
	})
	s.mu.Unlock()
}

func (s *Server) postReview(w http.ResponseWriter, r *http.Request) {
	s.logged(w, r, func(w http.ResponseWriter, r *http.Request, body []byte) {
		var req struct {
			Review *wk.ReviewInput `json:"review"`
		}
		if err := json.Unmarshal(body, &req); err != nil || req.Review == nil {
			writeError(w, http.StatusUnprocessableEntity, "review is missing or malformed")
			return
		}
		in := *req.Review
		if in.IncorrectMeaningAnswers < 0 || in.IncorrectReadingAnswers < 0 {
			writeError(w, http.StatusUnprocessableEntity, "incorrect answer counts must be >= 0")
			return
		}

		s.mu.Lock()
		defer s.mu.Unlock()
		now := s.Now().UTC()

		i := slices.IndexFunc(s.data.Assignments, func(a wk.Resource[wk.Assignment]) bool {
			return (in.AssignmentID != 0 && a.ID == in.AssignmentID) ||
				(in.AssignmentID == 0 && in.SubjectID != 0 && a.Data.SubjectID == in.SubjectID)
		})
		if i < 0 {
			writeError(w, http.StatusNotFound, "Not found")
			return
		}
		a := &s.data.Assignments[i]
		if a.Data.AvailableAt == nil || a.Data.AvailableAt.After(now) || a.Data.BurnedAt != nil {
			writeError(w, http.StatusUnprocessableEntity, "assignment is not available for review")
			return
		}
		createdAt := now
		if in.CreatedAt != nil {
			createdAt = in.CreatedAt.UTC()
			if createdAt.After(now) || createdAt.Before(*a.Data.AvailableAt) {
				writeError(w, http.StatusUnprocessableEntity, "created_at must be in the past and after available_at")
				return
			}
		}

		srsID := s.subjectSRSLocked(a.Data.SubjectID)
		start := a.Data.SRSStage
		end := nextStage(start, in.IncorrectMeaningAnswers+in.IncorrectReadingAnswers)

		a.Data.SRSStage = end
		if end >= burnedStage {
			a.Data.BurnedAt = &createdAt
			a.Data.AvailableAt = nil
		} else {
			at := createdAt.Add(time.Duration(s.intervalLocked(srsID, end)) * time.Second).Truncate(time.Hour)
			a.Data.AvailableAt = &at
		}
		if end >= passingStage && a.Data.PassedAt == nil {
			a.Data.PassedAt = &createdAt
		}
		a.DataUpdatedAt = now

		stat := s.updateStatLocked(a.Data, in, now)

		resp := map[string]any{
			"id":              0,
			"object":          "review",
			"url":             baseURL(r) + "reviews/0",
			"data_updated_at": now,
			"data": wk.Review{
				AssignmentID:             a.ID,
				SubjectID:                a.Data.SubjectID,
				SpacedRepetitionSystemID: srsID,
				StartingSRSStage:         start,
				EndingSRSStage:           end,
				IncorrectMeaningAnswers:  in.IncorrectMeaningAnswers,
				IncorrectReadingAnswers:  in.IncorrectReadingAnswers,
				CreatedAt:                createdAt,
			},
			"resources_updated": map[string]any{
				"assignment":       *a,
				"review_statistic": stat,
			},
		}
		writeJSON(w, r, http.StatusCreated, resp)
	})
}

// nextStage approximates WaniKani's SRS rule: no misses moves up one stage;
// otherwise drop ceil(misses/2) stages, doubled from Guru up, never below 1.
func nextStage(stage, misses int) int {
	if misses == 0 {
		return min(stage+1, burnedStage)
	}
	penalty := 1
	if stage >= passingStage {
		penalty = 2
	}
	adj := int(math.Ceil(float64(misses) / 2))
	return max(stage-adj*penalty, 1)
}

func (s *Server) subjectSRSLocked(subjectID int) int {
	i := slices.IndexFunc(s.data.Subjects, func(r wk.Resource[wk.Subject]) bool { return r.ID == subjectID })
	if i < 0 || s.data.Subjects[i].Data.SpacedRepetitionSystemID == 0 {
		return 1
	}
	return s.data.Subjects[i].Data.SpacedRepetitionSystemID
}

func (s *Server) intervalLocked(srsID, stage int) int64 {
	for _, sys := range s.data.SRS {
		if sys.ID != srsID {
			continue
		}
		for _, st := range sys.Data.Stages {
			if st.Position == stage && st.Interval != nil {
				return *st.Interval
			}
		}
	}
	return defaultIntervals[stage]
}

func (s *Server) updateStatLocked(a wk.Assignment, in wk.ReviewInput, now time.Time) wk.Resource[wk.ReviewStatistic] {
	i := slices.IndexFunc(s.data.ReviewStatistics, func(r wk.Resource[wk.ReviewStatistic]) bool {
		return r.Data.SubjectID == a.SubjectID
	})
	if i < 0 {
		id := 1
		for _, r := range s.data.ReviewStatistics {
			id = max(id, r.ID+1)
		}
		s.data.ReviewStatistics = append(s.data.ReviewStatistics, wk.Resource[wk.ReviewStatistic]{
			ID: id, Object: "review_statistic",
			Data: wk.ReviewStatistic{SubjectID: a.SubjectID, SubjectType: a.SubjectType, CreatedAt: now},
		})
		i = len(s.data.ReviewStatistics) - 1
	}
	st := &s.data.ReviewStatistics[i]
	d := &st.Data
	bump := func(misses int, correct, incorrect, streak, maxStreak *int) {
		*correct++
		*incorrect += misses
		if misses == 0 {
			*streak++
		} else {
			*streak = 1
		}
		*maxStreak = max(*maxStreak, *streak)
	}
	bump(in.IncorrectMeaningAnswers, &d.MeaningCorrect, &d.MeaningIncorrect, &d.MeaningCurrentStreak, &d.MeaningMaxStreak)
	if a.SubjectType == wk.ObjectKanji || a.SubjectType == wk.ObjectVocabulary {
		bump(in.IncorrectReadingAnswers, &d.ReadingCorrect, &d.ReadingIncorrect, &d.ReadingCurrentStreak, &d.ReadingMaxStreak)
	}
	total := d.MeaningCorrect + d.ReadingCorrect + d.MeaningIncorrect + d.ReadingIncorrect
	if total > 0 {
		d.PercentageCorrect = int(math.Round(float64(d.MeaningCorrect+d.ReadingCorrect) / float64(total) * 100))
	}
	st.DataUpdatedAt = now
	return *st
}

func (s *Server) startAssignment(w http.ResponseWriter, r *http.Request) {
	s.logged(w, r, func(w http.ResponseWriter, r *http.Request, body []byte) {
		id, err := strconv.Atoi(r.PathValue("id"))
		if err != nil {
			writeError(w, http.StatusNotFound, "Not found")
			return
		}
		var req struct {
			Assignment struct {
				StartedAt *time.Time `json:"started_at"`
			} `json:"assignment"`
		}
		if len(body) > 0 {
			if err := json.Unmarshal(body, &req); err != nil {
				writeError(w, http.StatusUnprocessableEntity, "malformed body")
				return
			}
		}

		s.mu.Lock()
		defer s.mu.Unlock()
		now := s.Now().UTC()
		i := slices.IndexFunc(s.data.Assignments, func(a wk.Resource[wk.Assignment]) bool { return a.ID == id })
		if i < 0 {
			writeError(w, http.StatusNotFound, "Not found")
			return
		}
		a := &s.data.Assignments[i]
		if a.Data.SRSStage != 0 || a.Data.StartedAt != nil || a.Data.UnlockedAt == nil || !s.levelGrantedLocked(s.assignmentLevelLocked(a.Data)) {
			writeError(w, http.StatusUnprocessableEntity, "assignment cannot be started")
			return
		}
		started := now
		if req.Assignment.StartedAt != nil {
			started = req.Assignment.StartedAt.UTC()
			if started.Before(*a.Data.UnlockedAt) || started.After(now) {
				writeError(w, http.StatusUnprocessableEntity, "started_at must be after unlocked_at and not in the future")
				return
			}
		}
		at := started.Add(time.Duration(s.intervalLocked(s.subjectSRSLocked(a.Data.SubjectID), 1)) * time.Second).Truncate(time.Hour)
		a.Data.SRSStage = 1
		a.Data.StartedAt = &started
		a.Data.AvailableAt = &at
		a.DataUpdatedAt = now
		writeJSON(w, r, http.StatusOK, *a)
	})
}

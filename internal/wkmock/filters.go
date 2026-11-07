package wkmock

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jtharris/yadokari/internal/wk"
)

func intSet(q url.Values, key string) (map[int]bool, error) {
	v := q.Get(key)
	if v == "" {
		return nil, nil
	}
	set := map[int]bool{}
	for _, p := range strings.Split(v, ",") {
		n, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil {
			return nil, fmt.Errorf("invalid %s", key)
		}
		set[n] = true
	}
	return set, nil
}

func strSet(q url.Values, key string) map[string]bool {
	v := q.Get(key)
	if v == "" {
		return nil
	}
	set := map[string]bool{}
	for _, p := range strings.Split(v, ",") {
		set[strings.TrimSpace(p)] = true
	}
	return set
}

// boolParam returns (value, present). The API treats a present-but-empty
// immediately_available_* flag as true.
func boolParam(q url.Values, key string) (bool, bool, error) {
	if !q.Has(key) {
		return false, false, nil
	}
	v := q.Get(key)
	if v == "" {
		return true, true, nil
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false, false, fmt.Errorf("invalid %s", key)
	}
	return b, true, nil
}

func timeParam(q url.Values, key string) (time.Time, bool, error) {
	v := q.Get(key)
	if v == "" {
		return time.Time{}, false, nil
	}
	t, err := time.Parse(time.RFC3339Nano, v)
	if err != nil {
		return time.Time{}, false, fmt.Errorf("invalid %s", key)
	}
	return t, true, nil
}

func inSet[K comparable](set map[K]bool, k K) bool { return set == nil || set[k] }

func subjectFilter(_ *Server, q url.Values, r wk.Resource[wk.Subject]) (bool, error) {
	levels, err := intSet(q, "levels")
	if err != nil {
		return false, err
	}
	hidden, hasHidden, err := boolParam(q, "hidden")
	if err != nil {
		return false, err
	}
	d := r.Data
	return inSet(strSet(q, "types"), r.Object) &&
		inSet(strSet(q, "slugs"), d.Slug) &&
		inSet(levels, d.Level) &&
		(!hasHidden || hidden == (d.HiddenAt != nil)), nil
}

func assignmentFilter(s *Server, q url.Values, r wk.Resource[wk.Assignment]) (bool, error) {
	d := r.Data
	now := s.Now()
	level := s.assignmentLevelLocked(d)

	for _, key := range []string{"subject_ids", "levels", "srs_stages"} {
		set, err := intSet(q, key)
		if err != nil {
			return false, err
		}
		v := map[string]int{"subject_ids": d.SubjectID, "levels": level, "srs_stages": d.SRSStage}[key]
		if !inSet(set, v) {
			return false, nil
		}
	}
	if !inSet(strSet(q, "subject_types"), d.SubjectType) {
		return false, nil
	}

	checks := []struct {
		key  string
		have bool
	}{
		{"hidden", d.Hidden},
		{"started", d.StartedAt != nil},
		{"unlocked", d.UnlockedAt != nil},
		{"burned", d.BurnedAt != nil},
		{"in_review", d.StartedAt != nil && d.BurnedAt == nil},
		{"immediately_available_for_lessons", d.UnlockedAt != nil && d.StartedAt == nil && s.levelGrantedLocked(level)},
		{"immediately_available_for_review", d.AvailableAt != nil && !d.AvailableAt.After(now) && d.BurnedAt == nil && s.levelGrantedLocked(level)},
	}
	for _, c := range checks {
		want, present, err := boolParam(q, c.key)
		if err != nil {
			return false, err
		}
		if present && want != c.have {
			return false, nil
		}
	}

	if t, ok, err := timeParam(q, "available_before"); err != nil {
		return false, err
	} else if ok && (d.AvailableAt == nil || !d.AvailableAt.Before(t)) {
		return false, nil
	}
	if t, ok, err := timeParam(q, "available_after"); err != nil {
		return false, err
	} else if ok && (d.AvailableAt == nil || !d.AvailableAt.After(t)) {
		return false, nil
	}
	return true, nil
}

func studyMaterialFilter(_ *Server, q url.Values, r wk.Resource[wk.StudyMaterial]) (bool, error) {
	ids, err := intSet(q, "subject_ids")
	if err != nil {
		return false, err
	}
	return inSet(ids, r.Data.SubjectID) && inSet(strSet(q, "subject_types"), r.Data.SubjectType), nil
}

func reviewStatisticFilter(_ *Server, q url.Values, r wk.Resource[wk.ReviewStatistic]) (bool, error) {
	ids, err := intSet(q, "subject_ids")
	if err != nil {
		return false, err
	}
	return inSet(ids, r.Data.SubjectID) && inSet(strSet(q, "subject_types"), r.Data.SubjectType), nil
}

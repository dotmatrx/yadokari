package wkmock

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNextStage(t *testing.T) {
	tests := []struct{ stage, misses, want int }{
		{1, 0, 2},
		{8, 0, 9},
		{4, 1, 3},  // apprentice: -ceil(1/2)*1
		{4, 3, 2},  // -2
		{2, 5, 1},  // floor at 1
		{5, 1, 3},  // guru: -ceil(1/2)*2
		{8, 2, 6},  // -1*2
		{7, 4, 3},  // -2*2
		{6, 10, 1}, // floor at 1
	}
	for _, tt := range tests {
		if got := nextStage(tt.stage, tt.misses); got != tt.want {
			t.Errorf("nextStage(%d, %d) = %d, want %d", tt.stage, tt.misses, got, tt.want)
		}
	}
}

func TestRateLimitWindow(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 10, 0, time.UTC)
	s := New(SampleData(now))
	s.RateLimit = 2
	s.Now = func() time.Time { return now }
	h := s.Handler()

	get := func() *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, "/v2/user", nil)
		r.Header.Set("Authorization", "Bearer x")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	for i, want := range []int{200, 200, 429} {
		if w := get(); w.Code != want {
			t.Fatalf("request %d: %d, want %d", i, w.Code, want)
		}
	}
	now = now.Add(time.Minute)
	if w := get(); w.Code != 200 || w.Header().Get("RateLimit-Remaining") != "1" {
		t.Fatalf("after window: %d remaining=%s", w.Code, w.Header().Get("RateLimit-Remaining"))
	}
}

func TestLoadDirRoundTrip(t *testing.T) {
	data := SampleData(time.Now())
	dir := t.TempDir()
	write := func(name string, v any) {
		raw, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), raw, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(FileUser, data.User)
	write(FileSubjects, data.Subjects)
	write(FileAssignments, data.Assignments)

	got, err := LoadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.User.Data.Username != data.User.Data.Username || len(got.Subjects) != len(data.Subjects) || len(got.Assignments) != len(data.Assignments) {
		t.Fatalf("round trip mismatch")
	}
	if len(got.StudyMaterials) != 0 {
		t.Fatalf("missing file should give an empty collection")
	}
}

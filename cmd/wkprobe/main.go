// Command wkprobe runs the full read-only sync against the API (or the mock),
// reports request count and timing, checks the payloads against our types
// (reporting fields we don't model), and can record the raw responses as
// fixtures for the mock. It never writes to the API.
//
// The token is read from WANIKANI_API_TOKEN (just loads .env automatically).
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"time"

	"yadokari/internal/wk"
	"yadokari/internal/wkmock"
)

func main() {
	base := flag.String("base", wk.DefaultBaseURL, "API base URL (e.g. http://127.0.0.1:8642/v2/ for the mock)")
	record := flag.String("record", "", "write raw fixtures (user, subjects, assignments, …) to this directory")
	rpm := flag.Int("rpm", 55, "client-side requests per minute")
	burst := flag.Int("burst", 5, "client-side burst")
	flag.Parse()

	token := os.Getenv("WANIKANI_API_TOKEN")
	if token == "" {
		log.Fatal("set WANIKANI_API_TOKEN (a READ-ONLY token) in the environment or .env")
	}
	// No WithLiveWrites: writes to the live API are impossible from this tool.
	c, err := wk.New(token, wk.WithBaseURL(*base), wk.WithRateLimit(*rpm, *burst))
	if err != nil {
		log.Fatal(err)
	}
	ctx := context.Background()
	start := time.Now()
	files := map[string]any{}
	unknown := map[string]int{}

	fmt.Printf("%-26s %6s  %8s  %s\n", "endpoint", "items", "requests", "time")
	step := func(name string, f func() (int, error)) {
		t0, r0 := time.Now(), c.Requests()
		n, err := f()
		if err != nil {
			log.Fatalf("%s: %v", name, err)
		}
		fmt.Printf("%-26s %6d  %8d  %v\n", name, n, c.Requests()-r0, time.Since(t0).Round(time.Millisecond))
	}

	var user wk.User
	step("user", func() (int, error) {
		r, err := c.GetRaw(ctx, "user")
		if err != nil {
			return 0, err
		}
		files[wkmock.FileUser] = r
		check("user", r.Data, &user, unknown)
		return 1, nil
	})

	var summary wk.Summary
	step("summary", func() (int, error) {
		r, err := c.GetRaw(ctx, "summary")
		if err != nil {
			return 0, err
		}
		check("summary", r.Data, &summary, unknown)
		return 1, nil
	})

	auxMeanings, imageless := 0, 0
	collect("subjects", wkmock.FileSubjects, c, ctx, step, files, unknown, func(r wk.Resource[json.RawMessage], s *wk.Subject) {
		if len(s.AuxiliaryMeanings) > 0 {
			auxMeanings++
		}
		if s.Characters == nil {
			imageless++
		}
	})
	collect[wk.Assignment]("assignments", wkmock.FileAssignments, c, ctx, step, files, unknown, nil)
	collect[wk.StudyMaterial]("study_materials", wkmock.FileStudyMaterials, c, ctx, step, files, unknown, nil)
	collect[wk.SpacedRepetitionSystem]("spaced_repetition_systems", wkmock.FileSRS, c, ctx, step, files, unknown, nil)
	collect[wk.ReviewStatistic]("review_statistics", wkmock.FileReviewStatistics, c, ctx, step, files, unknown, nil)

	step("assignments (incremental)", func() (int, error) {
		n := 0
		err := c.Assignments(ctx, url.Values{"updated_after": {wk.Time(start)}}, func(p *wk.Collection[wk.Assignment]) error {
			n += len(p.Data)
			return nil
		})
		return n, err
	})
	fmt.Printf("total: %d requests in %v\n\n", c.Requests(), time.Since(start).Round(time.Millisecond))

	fmt.Printf("user %s: level %d, subscription %s (max level %d)\n",
		user.Username, user.Level, user.Subscription.Type, user.Subscription.MaxLevelGranted)
	if len(summary.Lessons) > 0 && len(summary.Reviews) > 0 {
		fmt.Printf("lessons now %d, reviews now %d\n", len(summary.Lessons[0].SubjectIDs), len(summary.Reviews[0].SubjectIDs))
	}
	fmt.Printf("subjects with auxiliary_meanings: %d, without characters: %d\n", auxMeanings, imageless)

	if len(unknown) == 0 {
		fmt.Println("schema: every field in every payload is modelled by internal/wk")
	} else {
		fmt.Println("schema: fields NOT modelled by internal/wk:")
		keys := make([]string, 0, len(unknown))
		for k := range unknown {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Printf("  %-60s %d\n", k, unknown[k])
		}
	}

	if *record != "" {
		if err := os.MkdirAll(*record, 0o700); err != nil {
			log.Fatal(err)
		}
		for name, v := range files {
			raw, err := json.Marshal(v)
			if err != nil {
				log.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(*record, name), raw, 0o600); err != nil {
				log.Fatal(err)
			}
		}
		fmt.Printf("recorded raw fixtures to %s\n", *record)
	}
}

// collect walks a collection raw, keeps the raw resources for recording, and
// decodes each into T to check the schema.
func collect[T any](name, file string, c *wk.Client, ctx context.Context, step func(string, func() (int, error)),
	files map[string]any, unknown map[string]int, visit func(wk.Resource[json.RawMessage], *T)) {
	var all []wk.Resource[json.RawMessage]
	step(name, func() (int, error) {
		err := c.EachRaw(ctx, name, nil, func(p *wk.Collection[json.RawMessage]) error {
			for _, r := range p.Data {
				var v T
				check(name+"/"+r.Object, r.Data, &v, unknown)
				if visit != nil {
					visit(r, &v)
				}
			}
			all = append(all, p.Data...)
			return nil
		})
		return len(all), err
	})
	files[file] = all
}

// check decodes raw into v strictly, counting fields v doesn't model.
// Strict decoding stops at the first unknown field, so each payload is
// re-checked with that field removed until it decodes cleanly.
func check(prefix string, raw json.RawMessage, v any, unknown map[string]int) {
	if err := json.Unmarshal(raw, v); err != nil {
		unknown[prefix+": decode error: "+err.Error()]++
		return
	}
	var obj map[string]json.RawMessage
	if json.Unmarshal(raw, &obj) != nil {
		return
	}
	for range len(obj) {
		b, _ := json.Marshal(obj)
		dec := json.NewDecoder(bytes.NewReader(b))
		dec.DisallowUnknownFields()
		err := dec.Decode(v)
		if err == nil {
			return
		}
		var field string
		if _, scanErr := fmt.Sscanf(err.Error(), "json: unknown field %q", &field); scanErr != nil {
			unknown[prefix+": "+err.Error()]++
			return
		}
		if _, top := obj[field]; !top {
			unknown[prefix+".(nested)."+field]++ // inside an object or array; reported once
			return
		}
		unknown[prefix+"."+field]++
		delete(obj, field)
	}
}

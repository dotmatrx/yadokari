package wksync

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/dotmatrx/yadokari/internal/store"
	"github.com/dotmatrx/yadokari/internal/wk"
	"github.com/dotmatrx/yadokari/internal/wkmock"
)

func TestUploadAgainstMock(t *testing.T) {
	clock := now
	e := setup(t, wkmock.SampleData(now), &clock)
	ctx := context.Background()
	if _, err := e.syncer.Run(ctx, all()); err != nil {
		t.Fatal(err)
	}
	items, err := e.store.ReviewItems(ctx, clock, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 4 {
		t.Fatalf("%d review items, want 4", len(items))
	}
	if items[2].Subject.Object != wk.ObjectKanji || len(items[2].Synonyms) != 1 {
		t.Errorf("kanji 一 should carry the 'single' synonym: %+v", items[2])
	}

	// Queue two reviews: kanji 一 with two reading misses, radical 二 perfect.
	at := clock.Add(-time.Minute)
	if err := e.store.QueueReview(ctx, 101, wk.ReviewInput{AssignmentID: 1002, IncorrectReadingAnswers: 2, CreatedAt: &at}); err != nil {
		t.Fatal(err)
	}
	if err := e.store.QueueReview(ctx, 3, wk.ReviewInput{AssignmentID: 1001, CreatedAt: &at}); err != nil {
		t.Fatal(err)
	}
	if err := e.store.QueueReview(ctx, 3, wk.ReviewInput{AssignmentID: 1001}); !errors.Is(err, store.ErrAlreadyQueued) {
		t.Fatalf("double queue: %v", err)
	}
	// Queued items no longer count as due, even before upload.
	if q, _ := e.store.Queue(ctx, clock, 3); q.Reviews != 2 {
		t.Errorf("reviews while queued: %d, want 2", q.Reviews)
	}

	up := &Uploader{Client: e.client, Store: e.store}
	res, err := up.Flush(ctx)
	if err != nil || res.Sent != 2 {
		t.Fatalf("flush: %+v %v", res, err)
	}
	if p, f, _ := e.store.OutboxCounts(ctx); p != 0 || f != 0 {
		t.Errorf("outbox after flush: %d pending, %d failed", p, f)
	}
	if w := e.mock.Writes(); len(w) != 2 || w[0].Path != "/v2/reviews" {
		t.Errorf("mock saw %+v", w)
	}
	// The store took the server's updated assignment: stage 2 → 1, not due.
	items, _ = e.store.ReviewItems(ctx, clock, 3)
	if len(items) != 2 {
		t.Errorf("after upload %d items due, want 2", len(items))
	}
	if q, _ := e.store.Queue(ctx, clock, 3); q.Reviews != 2 {
		t.Errorf("reviews after upload: %d, want 2", q.Reviews)
	}
}

func TestUploadRejectedAndRequeue(t *testing.T) {
	clock := now
	e := setup(t, wkmock.SampleData(now), &clock)
	ctx := context.Background()
	e.syncer.Run(ctx, all())

	// Assignment 1003 (kanji 二) is not due until 15:00: the server refuses it.
	if err := e.store.QueueReview(ctx, 102, wk.ReviewInput{AssignmentID: 1003}); err != nil {
		t.Fatal(err)
	}
	up := &Uploader{Client: e.client, Store: e.store}
	res, err := up.Flush(ctx)
	if err != nil || res.Rejected != 1 {
		t.Fatalf("flush: %+v %v", res, err)
	}
	all, _ := e.store.Outbox(ctx, true)
	if len(all) != 1 || all[0].Failed == "" {
		t.Fatalf("rejected review should stay, marked: %+v", all)
	}
	// A second flush does not retry it.
	if res, _ := up.Flush(ctx); res.Rejected != 0 || len(e.mock.Writes()) != 1 {
		t.Errorf("rejected review retried: %+v", res)
	}
	// Reviewing the item again replaces the failed row.
	if err := e.store.QueueReview(ctx, 102, wk.ReviewInput{AssignmentID: 1003}); err != nil {
		t.Errorf("re-queue after rejection: %v", err)
	}
}

func TestUploadHeldForLiveAPI(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	live, _ := wk.New("tok") // default base: the live API, a development client
	st.QueueReview(ctx, 1, wk.ReviewInput{AssignmentID: 1})
	st.QueueReview(ctx, 2, wk.ReviewInput{AssignmentID: 2})

	res, err := (&Uploader{Client: live, Store: st}).Flush(ctx)
	if !errors.Is(err, ErrHeld) || res.Held != 2 || res.Sent != 0 || !res.Live {
		t.Fatalf("live writes off must hold everything: %+v %v", res, err)
	}
	if live.Requests() != 0 {
		t.Fatal("a request reached the network")
	}
	if p, _, _ := st.OutboxCounts(ctx); p != 2 {
		t.Errorf("held reviews must stay queued: %d", p)
	}
}

func TestConcurrentFlushesPostOnce(t *testing.T) {
	clock := now
	e := setup(t, wkmock.SampleData(now), &clock)
	ctx := context.Background()
	e.syncer.Run(ctx, all())
	at := clock.Add(-time.Minute)
	e.store.QueueReview(ctx, 3, wk.ReviewInput{AssignmentID: 1001, CreatedAt: &at})
	e.store.QueueReview(ctx, 101, wk.ReviewInput{AssignmentID: 1002, CreatedAt: &at})

	up := &Uploader{Client: e.client, Store: e.store}
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() { defer wg.Done(); up.Flush(ctx) }()
	}
	wg.Wait()
	if w := e.mock.Writes(); len(w) != 2 {
		t.Errorf("mock received %d reviews, want 2", len(w))
	}
	if p, f, _ := e.store.OutboxCounts(ctx); p != 0 || f != 0 {
		t.Errorf("outbox: %d pending, %d failed", p, f)
	}
}

func TestUploadLessonStarts(t *testing.T) {
	clock := now
	e := setup(t, wkmock.SampleData(now), &clock)
	ctx := context.Background()
	if _, err := e.syncer.Run(ctx, all()); err != nil {
		t.Fatal(err)
	}
	lessons, err := e.store.LessonItems(ctx, 3)
	if err != nil || len(lessons) != 2 {
		t.Fatalf("%d lessons, want 2: %v", len(lessons), err)
	}
	a := lessons[0].Assignment
	if err := e.store.QueueStart(ctx, a.ID, a.Data.SubjectID, clock); err != nil {
		t.Fatal(err)
	}
	if err := e.store.QueueStart(ctx, a.ID, a.Data.SubjectID, clock); !errors.Is(err, store.ErrAlreadyQueued) {
		t.Fatalf("double queue: %v", err)
	}
	// A queued start is no longer a lesson, even before upload.
	if q, _ := e.store.Queue(ctx, clock, 3); q.Lessons != 1 {
		t.Errorf("lessons while queued: %d, want 1", q.Lessons)
	}
	if l, _ := e.store.LessonItems(ctx, 3); len(l) != 1 {
		t.Errorf("lesson items while queued: %d, want 1", len(l))
	}
	box, _ := e.store.Outbox(ctx, false)
	if len(box) != 1 || box[0].Kind != store.KindStart || box[0].AssignmentID != a.ID || !box[0].StartedAt.Equal(clock) {
		t.Fatalf("outbox %+v", box)
	}

	res, err := (&Uploader{Client: e.client, Store: e.store}).Flush(ctx)
	if err != nil || res.Sent != 1 {
		t.Fatalf("flush: %+v %v", res, err)
	}
	w := e.mock.Writes()
	if len(w) != 1 || w[0].Method != "PUT" || w[0].Path != fmt.Sprintf("/v2/assignments/%d/start", a.ID) {
		t.Fatalf("mock saw %+v", w)
	}
	// The store took the started assignment: Apprentice 1, no longer a lesson.
	out, _ := e.store.ReviewOutcomes(ctx, []int{a.ID})
	if out[a.ID].SRSStage != 1 || out[a.ID].Queued {
		t.Errorf("outcome %+v", out[a.ID])
	}
	if q, _ := e.store.Queue(ctx, clock, 3); q.Lessons != 1 {
		t.Errorf("lessons after upload: %d, want 1", q.Lessons)
	}
}

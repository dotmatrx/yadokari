-- The outbox also carries lesson starts (PUT /assignments/:id/start).
-- kind: 'review' (payload: wk.ReviewInput) or 'start' (payload: {"started_at": …}).
ALTER TABLE outbox ADD COLUMN kind TEXT NOT NULL DEFAULT 'review';

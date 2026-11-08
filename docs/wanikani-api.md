# WaniKani API v2 — working notes

Summarized from the official reference, https://docs.api.wanikani.com/20170710/
(read 2026-11-07). The reference is authoritative; this file records what we code
against and where the docs are ambiguous.

## Transport

- Base URL `https://api.wanikani.com/v2/`, HTTPS only.
- Headers: `Authorization: Bearer <token>`, `Wanikani-Revision: 20170710`,
  `Content-Type: application/json; charset=utf-8` for writes.
- Rate limit: 60 requests/minute per token. Response headers `RateLimit-Limit`,
  `RateLimit-Remaining`, `RateLimit-Reset` (epoch seconds). Over the limit → 429 with
  body text `Rate Limit Exceeded`. No `Retry-After` documented.
- Conditional requests on every endpoint: send `If-None-Match` (ETag) or
  `If-Modified-Since` (Last-Modified); unchanged → 304 with empty body. `If-None-Match`
  wins if both are sent.
- Errors: `{"error": "<string>", "code": <int>}`. Codes seen: 401, 403, 404, 409, 422,
  429, 500, 503.
- Dates: ISO 8601 with microseconds, e.g. `2017-09-05T23:38:10.695133Z`.

## Envelopes and pagination

- Resource: `{id, object, url, data_updated_at, data}`. `/user` and `/summary` have no
  top-level `id`; summary's object is `report`.
- Collection: `{object: "collection", url, pages: {next_url, previous_url, per_page},
  total_count, data_updated_at (null if empty), data: [resource...]}`.
- Cursor pagination by id (`page_after_id`); filters carry into `next_url`. Follow
  `next_url`, never build it. Page size: 1000 for subjects, 500 elsewhere. No per-page
  request parameter.
- Array filters are comma-separated (`subject_ids=8,16,64`).

## Resources we use

- **user**: `username, level, profile_url, started_at, current_vacation_started_at,
  subscription{active, type (free|recurring|lifetime|unknown), max_level_granted,
  period_ends_at}, preferences{lessons_batch_size, lessons_autoplay_audio,
  reviews_autoplay_audio, reviews_display_srs_indicator, reviews_presentation_order
  (shuffled|lower_levels_first), extra_study_autoplay_audio, …}`. Treat `unknown`
  subscription as free.
- **summary**: `lessons[{available_at, subject_ids}]`, `next_reviews_at`,
  `reviews[25 hourly buckets]`.
- **subjects** (`radical`, `kanji`, `vocabulary`, `kana_vocabulary`): common
  `characters (null for some radicals), level, slug, lesson_position (per level),
  meanings[{meaning, primary, accepted_answer}], auxiliary_meanings[{meaning, type:
  whitelist|blacklist}], meaning_mnemonic, hidden_at, document_url, created_at,
  spaced_repetition_system_id`.
  - radical: `amalgamation_subject_ids, character_images[{url, content_type,
    metadata}]`. Each image radical lists seven PNG sizes plus one SVG, but only the
    SVG URL is served: every PNG URL answers 403 (checked 2026-11-07). The docs' "SVG
    only" is right in practice.
  - kanji: `component_subject_ids, amalgamation_subject_ids,
    visually_similar_subject_ids, readings[{reading, primary, accepted_answer, type:
    onyomi|kunyomi|nanori}], reading_mnemonic, meaning_hint, reading_hint`.
  - vocabulary: `component_subject_ids, readings[{reading, primary,
    accepted_answer}], reading_mnemonic, parts_of_speech, context_sentences[{en, ja}],
    pronunciation_audios[{url, content_type, metadata{…}}]`; hints may be present.
  - kana_vocabulary: no readings or components; meaning question only.
  - Mnemonic markup: `<radical> <kanji> <vocabulary> <meaning> <reading>`, plus `<ja>`
    seen in examples.
- **assignments**: `subject_id, subject_type, srs_stage, unlocked_at, started_at,
  available_at, passed_at, burned_at, resurrected_at, hidden, created_at` (examples also
  show `level`). Filters include `immediately_available_for_lessons`,
  `immediately_available_for_review`, `subject_ids`, `subject_types`, `levels`,
  `srs_stages`, `updated_after`, `ids`.
- **study_materials**: `subject_id, subject_type, meaning_synonyms, meaning_note,
  reading_note, hidden`.
- **spaced_repetition_systems**: stage positions and `stages[{position, interval,
  interval_unit}]`. Two systems: 1 normal, 2 accelerated.
- **review_statistics**: per-subject correct/incorrect/streak counts.
- `GET /reviews` is deprecated (always empty); `GET /reviews/<id>` always 404.

## Writes (irreversible)

- `POST /reviews` body `{"review": {assignment_id | subject_id,
  incorrect_meaning_answers, incorrect_reading_answers, created_at?}}`. Either id is
  sufficient per the prose (the table marks both required — verify). Use 0 reading
  misses for radicals and kana_vocabulary. `created_at` must be in the past and after
  the assignment's `available_at`. Response: a `review` resource with `id` 0, plus
  `resources_updated{assignment, review_statistic}`.
- `PUT /assignments/<id>/start` body `{"assignment": {"started_at"?}}`. Requires
  srs_stage 0, started_at null, unlocked_at set, level ≤ min(user level,
  max_level_granted). Result: srs_stage 1, started_at and available_at set.
- `POST /study_materials` `{"study_material": {subject_id, meaning_synonyms?,
  meaning_note?, reading_note?}}` (one per subject); `PUT /study_materials/<id>` same
  minus subject_id.

## Observed in real data (level-60 account, 2026-11-07)

- 9,452 subjects: 6,786 vocabulary, 2,102 kanji, 504 radicals, 60 kana_vocabulary.
  43 hidden. 19 radicals have `characters: null`.
- `auxiliary_meanings` on 4,358 subjects: 16,394 whitelist, 1,407 blacklist entries.
- Character images: 3,479 `image/png` entries (all 403 when fetched) and 498
  `image/svg+xml` (served). The SVGs style strokes through a CSS `<style>` class with
  `var(--color-text, #000)` and use clip paths; see `internal/glyph/svg.go`.
- Audio: `audio/webm` and `audio/mpeg` (no ogg), two per recording.
- Kanji reading types: onyomi, kunyomi, nanori. Kanji always carry `reading_hint`
  (often null); vocabulary never has hints.
- Assignments do **not** include `level` (the doc examples do). Get it from the subject.
- Full sync = 53 requests: user 1, summary 1, subjects 10, assignments 19,
  study_materials 1, SRS 1, review_statistics 19, plus 1 incremental check.

## Not documented — verify against the live API with a read-only token

- Token permission names required for each write.
- Success status codes for writes (assume any 2xx).
- Exact 422 bodies for invalid reviews / starts.
- Trailing slash on write URLs (headers say `/reviews/`, examples use `/reviews`).

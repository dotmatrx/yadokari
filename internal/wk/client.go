// Package wk is a typed client for the WaniKani API v2.
//
// Writes (POST /reviews, PUT /assignments/:id/start, study materials) are
// irreversible on the real service. The client refuses any non-GET request to
// the live API host unless it was built with WithLiveWrites(true); requests to
// any other base URL (the mock server) are always allowed.
package wk

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"
)

// DefaultBaseURL is the live WaniKani API.
const DefaultBaseURL = "https://api.wanikani.com/v2/"

const liveHost = "api.wanikani.com"

// ErrLiveWritesDisabled is returned for a write to the live API when the
// client was not explicitly allowed to make one.
var ErrLiveWritesDisabled = errors.New("wk: this build doesn't write to the live WaniKani API (a development build)")

// APIError is a non-2xx response from the API.
type APIError struct {
	Status  int    // HTTP status
	Code    int    // "code" from the error body, if any
	Message string // "error" from the error body, or the raw body
}

func (e *APIError) Error() string {
	return fmt.Sprintf("wk: HTTP %d: %s", e.Status, e.Message)
}

// IsStatus reports whether err is an APIError with the given HTTP status.
func IsStatus(err error, status int) bool {
	var ae *APIError
	return errors.As(err, &ae) && ae.Status == status
}

// Conditional carries validators from a previous response for a conditional GET.
type Conditional struct {
	ETag         string
	LastModified string
}

// RateLimit is the parsed RateLimit-* response headers.
type RateLimit struct {
	Limit     int
	Remaining int
	Reset     time.Time
	Known     bool
}

// Meta describes a response beyond its body.
type Meta struct {
	Status       int
	ETag         string
	LastModified string
	NotModified  bool // 304: the caller's cached copy is current; nothing was decoded
	RateLimit    RateLimit
}

// Client talks to the WaniKani API (or a compatible mock).
type Client struct {
	base       *url.URL
	token      string
	hc         *http.Client
	limiter    *rate.Limiter
	liveWrites bool
	maxRetries int

	now   func() time.Time
	sleep func(context.Context, time.Duration) error

	mu           sync.Mutex
	blockedUntil time.Time

	requests atomic.Int64

	writeLog   io.Writer
	writeLogMu sync.Mutex
	version    string
}

// Option configures a Client.
type Option func(*Client) error

// WithBaseURL points the client at a different server, e.g. the mock.
func WithBaseURL(raw string) Option {
	return func(c *Client) error {
		if !strings.HasSuffix(raw, "/") {
			raw += "/"
		}
		u, err := url.Parse(raw)
		if err != nil {
			return fmt.Errorf("wk: base URL: %w", err)
		}
		c.base = u
		return nil
	}
}

// WithHTTPClient replaces the HTTP client.
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) error { c.hc = hc; return nil }
}

// WithRateLimit sets the client-side request budget. The server allows 60/min
// per token, shared with the user's other tools; the default is 55/min.
func WithRateLimit(perMinute, burst int) Option {
	return func(c *Client) error {
		c.limiter = rate.NewLimiter(rate.Every(time.Minute/time.Duration(perMinute)), burst)
		return nil
	}
}

// WithLiveWrites allows writes to the live API. Only the app's own builds
// made to be used set it (cmd/yadokari: liveWrites), never development builds.
func WithLiveWrites(allow bool) Option {
	return func(c *Client) error { c.liveWrites = allow; return nil }
}

// WithWriteLog appends one JSON line per write attempt (POST/PUT/DELETE) to
// w: the request body, the HTTP status and the raw response body, or the
// transport error. The token is never logged. This is the record of what
// WaniKani actually accepted or refused.
func WithWriteLog(w io.Writer) Option {
	return func(c *Client) error { c.writeLog = w; return nil }
}

// WithVersion records the app version in each write-log entry.
func WithVersion(v string) Option {
	return func(c *Client) error { c.version = v; return nil }
}

// logWrite records one write attempt; failures to log are ignored.
func (c *Client) logWrite(method string, u *url.URL, payload []byte, status int, body []byte, err error) {
	if c.writeLog == nil || method == http.MethodGet {
		return
	}
	entry := struct {
		Time     time.Time       `json:"time"`
		Version  string          `json:"version,omitempty"`
		Method   string          `json:"method"`
		URL      string          `json:"url"`
		Request  json.RawMessage `json:"request,omitempty"`
		Status   int             `json:"status,omitempty"`
		Response string          `json:"response,omitempty"`
		Error    string          `json:"error,omitempty"`
	}{Time: c.now().UTC(), Version: c.version, Method: method, URL: u.String(), Status: status, Response: string(body)}
	if json.Valid(payload) {
		entry.Request = payload
	}
	if err != nil {
		entry.Error = err.Error()
	}
	line, mErr := json.Marshal(entry)
	if mErr != nil {
		return
	}
	c.writeLogMu.Lock()
	defer c.writeLogMu.Unlock()
	_, _ = c.writeLog.Write(append(line, '\n'))
}

// New builds a client for the given personal API token.
func New(token string, opts ...Option) (*Client, error) {
	if token == "" {
		return nil, errors.New("wk: empty API token")
	}
	base, _ := url.Parse(DefaultBaseURL)
	c := &Client{
		base:       base,
		token:      token,
		hc:         &http.Client{Timeout: 30 * time.Second},
		limiter:    rate.NewLimiter(rate.Every(time.Minute/55), 5),
		maxRetries: 3,
		now:        time.Now,
		sleep:      sleepCtx,
	}
	for _, o := range opts {
		if err := o(c); err != nil {
			return nil, err
		}
	}
	return c, nil
}

// Requests returns how many HTTP requests the client has sent (including retries).
func (c *Client) Requests() int64 { return c.requests.Load() }

// IsLive reports whether the client points at the live API.
func (c *Client) IsLive() bool { return c.base.Host == liveHost }

func sleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// resolve turns a path (relative to the base) or an absolute next_url into a URL.
// Absolute URLs must be on the base host so the token is never sent elsewhere.
func (c *Client) resolve(ref string, q url.Values) (*url.URL, error) {
	u, err := c.base.Parse(strings.TrimPrefix(ref, "/"))
	if err != nil {
		return nil, fmt.Errorf("wk: bad URL %q: %w", ref, err)
	}
	if u.Scheme != c.base.Scheme || u.Host != c.base.Host {
		return nil, fmt.Errorf("wk: refusing to send token to %s (base is %s)", u.Host, c.base.Host)
	}
	if len(q) > 0 {
		u.RawQuery = q.Encode()
	}
	return u, nil
}

// do performs one API call with rate limiting and 429 retries, decoding a 2xx
// body into out (if non-nil).
func (c *Client) do(ctx context.Context, method string, u *url.URL, body any, cond Conditional, out any) (Meta, error) {
	if method != http.MethodGet && u.Host == liveHost && !c.liveWrites {
		return Meta{}, ErrLiveWritesDisabled
	}

	var payload []byte
	if body != nil {
		var err error
		if payload, err = json.Marshal(body); err != nil {
			return Meta{}, fmt.Errorf("wk: encode body: %w", err)
		}
	}

	for attempt := 0; ; attempt++ {
		if err := c.waitTurn(ctx); err != nil {
			return Meta{}, err
		}

		req, err := http.NewRequestWithContext(ctx, method, u.String(), bytes.NewReader(payload))
		if err != nil {
			return Meta{}, err
		}
		req.Header.Set("Authorization", "Bearer "+c.token)
		req.Header.Set("Wanikani-Revision", Revision)
		req.Header.Set("Accept", "application/json")
		if payload != nil {
			req.Header.Set("Content-Type", "application/json; charset=utf-8")
		}
		if cond.ETag != "" {
			req.Header.Set("If-None-Match", cond.ETag)
		}
		if cond.LastModified != "" {
			req.Header.Set("If-Modified-Since", cond.LastModified)
		}

		c.requests.Add(1)
		resp, err := c.hc.Do(req)
		if err != nil {
			c.logWrite(method, u, payload, 0, nil, err)
			return Meta{}, fmt.Errorf("wk: %s %s: %w", method, u.Path, err)
		}
		raw, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
		resp.Body.Close()
		c.logWrite(method, u, payload, resp.StatusCode, raw, err)
		if err != nil {
			return Meta{}, fmt.Errorf("wk: read body: %w", err)
		}

		meta := Meta{
			Status:       resp.StatusCode,
			ETag:         resp.Header.Get("ETag"),
			LastModified: resp.Header.Get("Last-Modified"),
			RateLimit:    parseRateLimit(resp.Header),
		}
		c.noteRateLimit(meta.RateLimit)

		switch {
		case resp.StatusCode == http.StatusNotModified:
			meta.NotModified = true
			return meta, nil

		case resp.StatusCode == http.StatusTooManyRequests:
			// The request was rejected, not processed, so retrying is safe even for writes.
			if attempt >= c.maxRetries {
				return meta, apiError(resp.StatusCode, raw)
			}
			wait := c.retryDelay(meta.RateLimit, attempt)
			c.block(c.now().Add(wait))
			continue

		case resp.StatusCode >= 500 && method == http.MethodGet && attempt < c.maxRetries:
			// Only idempotent reads are retried on server errors; a failed write's
			// outcome is unknown and must be resolved by the caller (outbox).
			if err := c.sleep(ctx, backoff(attempt)); err != nil {
				return meta, err
			}
			continue

		case resp.StatusCode < 200 || resp.StatusCode > 299:
			return meta, apiError(resp.StatusCode, raw)
		}

		if out != nil && len(raw) > 0 {
			if err := json.Unmarshal(raw, out); err != nil {
				return meta, fmt.Errorf("wk: decode %s: %w", u.Path, err)
			}
		}
		return meta, nil
	}
}

func (c *Client) waitTurn(ctx context.Context) error {
	c.mu.Lock()
	until := c.blockedUntil
	c.mu.Unlock()
	if d := until.Sub(c.now()); d > 0 {
		if err := c.sleep(ctx, d); err != nil {
			return err
		}
	}
	return c.limiter.Wait(ctx)
}

func (c *Client) block(until time.Time) {
	c.mu.Lock()
	if until.After(c.blockedUntil) {
		c.blockedUntil = until
	}
	c.mu.Unlock()
}

// noteRateLimit pauses all requests until the reset when the budget is spent.
func (c *Client) noteRateLimit(rl RateLimit) {
	if rl.Known && rl.Remaining <= 0 && !rl.Reset.IsZero() {
		c.block(rl.Reset)
	}
}

func (c *Client) retryDelay(rl RateLimit, attempt int) time.Duration {
	if rl.Known && !rl.Reset.IsZero() {
		if d := rl.Reset.Sub(c.now()); d > 0 {
			return d + 250*time.Millisecond
		}
	}
	return backoff(attempt)
}

func backoff(attempt int) time.Duration {
	d := time.Second << attempt
	return min(d, time.Minute)
}

func parseRateLimit(h http.Header) RateLimit {
	var rl RateLimit
	lim, err1 := strconv.Atoi(h.Get("RateLimit-Limit"))
	rem, err2 := strconv.Atoi(h.Get("RateLimit-Remaining"))
	if err1 != nil || err2 != nil {
		return rl
	}
	rl.Limit, rl.Remaining, rl.Known = lim, rem, true
	if reset, err := strconv.ParseInt(h.Get("RateLimit-Reset"), 10, 64); err == nil {
		rl.Reset = time.Unix(reset, 0)
	}
	return rl
}

func apiError(status int, raw []byte) error {
	e := &APIError{Status: status}
	var body struct {
		Error string `json:"error"`
		Code  int    `json:"code"`
	}
	if json.Unmarshal(raw, &body) == nil && body.Error != "" {
		e.Message, e.Code = body.Error, body.Code
	} else {
		e.Message = strings.TrimSpace(string(raw))
		if e.Message == "" {
			e.Message = http.StatusText(status)
		}
	}
	return e
}

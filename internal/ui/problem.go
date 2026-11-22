package ui

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"os"

	"github.com/dotmatrx/yadokari/internal/audio"
	"github.com/dotmatrx/yadokari/internal/wk"
)

// userError is a message written for the user, shown as is.
type userError string

func (e userError) Error() string { return string(e) }

// problem returns a short, user-facing description of err (what failed is
// up to the caller to say) and logs the full error, which is for the
// developer. Messages meant for the user pass through unlogged.
func problem(what string, err error) string {
	var ue userError
	if errors.As(err, &ue) || errors.Is(err, audio.ErrNoPlayer) {
		return err.Error()
	}
	slog.Warn(what, "err", err)
	var ae *wk.APIError
	var ne net.Error
	switch {
	case errors.As(err, &ae):
		switch {
		case ae.Status == 401:
			return "WaniKani rejected the API token"
		case ae.Status == 403:
			return "the API token doesn't allow this"
		case ae.Status == 429:
			return "WaniKani is busy; try again shortly"
		case ae.Status >= 500:
			return "WaniKani is having problems"
		default:
			return "WaniKani refused it"
		}
	case errors.As(err, &ne), errors.Is(err, context.DeadlineExceeded), os.IsTimeout(err):
		return "can't reach WaniKani"
	}
	return "something went wrong (details in yadokari.log)"
}

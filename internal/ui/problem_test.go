package ui

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"testing"

	"github.com/jtharris/yadokari/internal/audio"
	"github.com/jtharris/yadokari/internal/wk"
)

func TestProblemSpeaksPlainly(t *testing.T) {
	dns := &url.Error{Op: "Post", URL: "https://api.wanikani.com/v2/reviews",
		Err: &net.OpError{Op: "dial", Net: "tcp", Err: &net.DNSError{Err: "server misbehaving", Name: "api.wanikani.com"}}}
	for _, c := range []struct {
		err  error
		want string
	}{
		{fmt.Errorf("uploading review for assignment 1: %w", dns), "can't reach WaniKani"},
		{context.DeadlineExceeded, "can't reach WaniKani"},
		{&wk.APIError{Status: 401}, "WaniKani rejected the API token"},
		{fmt.Errorf("x: %w", &wk.APIError{Status: 503}), "WaniKani is having problems"},
		{&wk.APIError{Status: 422}, "WaniKani refused it"},
		{audio.ErrNoPlayer, audio.ErrNoPlayer.Error()},
		{userError("no reviews are due"), "no reviews are due"},
		{errors.New("sqlite: disk I/O error"), "something went wrong (details in yadokari.log)"},
	} {
		if got := problem("test", c.err); got != c.want {
			t.Errorf("problem(%v) = %q, want %q", c.err, got, c.want)
		}
	}
}

package ui

import (
	"errors"
	"strings"

	"charm.land/huh/v2"
)

// TokenPageURL is where users create personal access tokens.
const TokenPageURL = "https://www.wanikani.com/settings/personal_access_tokens"

// ErrAborted means the user cancelled the first-run form.
var ErrAborted = huh.ErrUserAborted

// AskToken runs the first-run form and returns the token typed in.
func AskToken() (string, error) {
	var token string
	form := huh.NewForm(huh.NewGroup(
		huh.NewNote().
			Title("Welcome to yadokari").
			Description("yadokari needs a WaniKani personal access token.\n\n"+
				"Create one at "+TokenPageURL+"\n"+
				"For reviews and lessons, tick reviews:create and assignments:start;\n"+
				"a token without them only shows the dashboard.\n\n"+
				"yadokari is an unofficial client, not affiliated with WaniKani or Tofugu LLC."),
		huh.NewInput().
			Title("API token").
			EchoMode(huh.EchoModePassword).
			Value(&token).
			Validate(func(s string) error {
				s = strings.TrimSpace(s)
				if len(s) < 32 || strings.ContainsAny(s, " \t") {
					return errors.New("that doesn't look like a WaniKani token (36 characters, like 1234abcd-…)")
				}
				return nil
			}),
	))
	if err := form.Run(); err != nil {
		return "", err
	}
	return strings.TrimSpace(token), nil
}

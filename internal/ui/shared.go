package ui

// shared is state App owns and screens only read: whether kitty uploads
// can be sent yet, and what audio is doing (the header badge). Screens
// reach it through ReviewDeps / PageDeps.
type shared struct {
	graphicsReady bool // the alt screen is up

	audio    audioState
	audioGen int // numbers each play, so a stale "done" can't end a newer one
}

type audioState int

const (
	audioIdle    audioState = iota
	audioLoading            // downloading the recording
	audioPlaying            // the player is running: sound should be coming out
)

// audioBadge is shown at the front of the header while audio is busy.
func (s *shared) audioBadge() string {
	switch s.audio {
	case audioLoading:
		return mutedSt.Render("♪ loading…")
	case audioPlaying:
		return boldSt.Render("🔊 playing")
	}
	return ""
}

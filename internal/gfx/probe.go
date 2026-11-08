package gfx

import (
	"bytes"
	"errors"
	"os"
	"time"

	"golang.org/x/sys/unix"
)

// probeQuery asks for kitty graphics support (a 1×1 RGB query that the
// terminal must not store), then sends DA1. Every terminal answers DA1, so
// its reply arriving first, or alone, means "no kitty graphics".
const probeQuery = "\x1b_Gi=31,s=1,v=1,a=q,t=d,f=24;AAAA\x1b\\" + "\x1b[c"

// Probe asks the terminal on /dev/tty whether it supports kitty graphics.
// It must run before Bubble Tea takes over the terminal. This is more
// reliable than the environment: toolbox, for one, replaces TERM with
// xterm-256color when the container lacks the terminal's terminfo.
func Probe(timeout time.Duration) (bool, error) {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return false, err
	}
	defer tty.Close()
	fd := int(tty.Fd())

	old, err := unix.IoctlGetTermios(fd, unix.TCGETS)
	if err != nil {
		return false, err
	}
	raw := *old
	raw.Lflag &^= unix.ICANON | unix.ECHO
	raw.Cc[unix.VMIN], raw.Cc[unix.VTIME] = 0, 0
	if err := unix.IoctlSetTermios(fd, unix.TCSETS, &raw); err != nil {
		return false, err
	}
	defer unix.IoctlSetTermios(fd, unix.TCSETS, old)

	if _, err := tty.WriteString(probeQuery); err != nil {
		return false, err
	}
	var reply []byte
	buf := make([]byte, 256)
	deadline := time.Now().Add(timeout)
	for {
		left := time.Until(deadline)
		if left <= 0 {
			return false, errors.New("gfx: terminal did not answer the graphics probe")
		}
		fds := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
		n, err := unix.Poll(fds, int(left.Milliseconds())+1)
		if err == unix.EINTR {
			continue
		}
		if err != nil {
			return false, err
		}
		if n == 0 {
			continue
		}
		m, err := unix.Read(fd, buf)
		if err != nil && err != unix.EINTR && err != unix.EAGAIN {
			return false, err
		}
		reply = append(reply, buf[:max(m, 0)]...)
		if kitty, done := parseProbe(reply); done {
			return kitty, nil
		}
	}
}

// parseProbe reads the terminal's replies so far: done once the DA1 reply
// (ESC [ ? … c) has arrived; kitty if a graphics reply for image 31 said OK.
func parseProbe(b []byte) (kitty, done bool) {
	kitty = bytes.Contains(b, []byte("\x1b_Gi=31;OK"))
	if i := bytes.Index(b, []byte("\x1b[?")); i >= 0 {
		done = bytes.IndexByte(b[i:], 'c') >= 0
	}
	return kitty, done
}

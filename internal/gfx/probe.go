package gfx

import (
	"bytes"
	"errors"
	"image/color"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

// probeQuery asks for kitty graphics support (a 1×1 RGB query that the
// terminal must not store) and, by a feature query (kitty 0.50), for Unicode
// placeholders (e=T,U=1), then the cell size in pixels (CSI 16 t), then
// asks for palette colours 4–6 (OSC 4; the glyph fields of the "terminal"
// theme) and the text and background colours (OSC 10, 11), then the
// terminal's name and version (XTVERSION, CSI > q), then sends DA1. Every
// terminal answers DA1, so its reply arriving means every answer that is
// coming has come.
const probeQuery = "\x1b_Gi=31,s=1,v=1,a=q,t=d,f=24;AAAA\x1b\\" +
	"\x1b_Ga=q,i=32,e=T,U=1\x1b\\" + "\x1b[16t" +
	"\x1b]4;4;?\x1b\\\x1b]4;5;?\x1b\\\x1b]4;6;?\x1b\\" +
	"\x1b]10;?\x1b\\\x1b]11;?\x1b\\" + "\x1b[>0q" + "\x1b[c"

// Probed is what the terminal said.
type Probed struct {
	Kitty bool // kitty graphics supported
	// Placeholders is the answer to the feature query for Unicode
	// placeholders; Unknown from terminals without feature queries.
	Placeholders Support
	CellW, CellH int // cell size in pixels; 0 if not reported
	// Palette holds the colours the terminal reported for palette
	// entries 0–15 (only 4–6 are asked); alpha 0 means unknown.
	Palette [16]color.RGBA
	// Foreground and Background are the terminal's text and background
	// colours; alpha 0 means not reported.
	Foreground, Background color.RGBA
	// Terminal is the terminal's name and version from XTVERSION (e.g.
	// "WezTerm 20240203-110809-5046fc22"); "" if it didn't say.
	Terminal string
}

// Support is a terminal's answer to a kitty graphics feature query.
type Support uint8

const (
	Unknown     Support = iota // no feature queries: the terminal didn't say
	Supported                  // the reply named the feature
	Unsupported                // feature queries understood, the feature not
)

// Probe asks the terminal on /dev/tty about kitty graphics and its cell
// size. It must run before Bubble Tea takes over the terminal. This is more
// reliable than the environment and the tty ioctl: toolbox, for one,
// replaces TERM with xterm-256color when the container lacks the terminal's
// terminfo, and its pty reports no pixel size.
func Probe(timeout time.Duration) (Probed, error) {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return Probed{}, err
	}
	defer tty.Close()
	fd := int(tty.Fd())

	old, err := unix.IoctlGetTermios(fd, ioctlGetTermios)
	if err != nil {
		return Probed{}, err
	}
	raw := *old
	raw.Lflag &^= unix.ICANON | unix.ECHO
	raw.Cc[unix.VMIN], raw.Cc[unix.VTIME] = 0, 0
	if err := unix.IoctlSetTermios(fd, ioctlSetTermios, &raw); err != nil {
		return Probed{}, err
	}
	defer unix.IoctlSetTermios(fd, ioctlSetTermios, old)

	if _, err := tty.WriteString(probeQuery); err != nil {
		return Probed{}, err
	}
	var reply []byte
	buf := make([]byte, 256)
	deadline := time.Now().Add(timeout)
	for {
		left := time.Until(deadline)
		if left <= 0 {
			return Probed{}, errors.New("gfx: terminal did not answer the graphics probe")
		}
		fds := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
		n, err := unix.Poll(fds, int(left.Milliseconds())+1)
		if err == unix.EINTR {
			continue
		}
		if err != nil {
			return Probed{}, err
		}
		if n == 0 {
			continue
		}
		m, err := unix.Read(fd, buf)
		if err != nil && err != unix.EINTR && err != unix.EAGAIN {
			return Probed{}, err
		}
		reply = append(reply, buf[:max(m, 0)]...)
		if p, done := parseProbe(reply); done {
			return p, nil
		}
	}
}

var (
	cellReply    = regexp.MustCompile(`\x1b\[6;(\d+);(\d+)t`)
	paletteReply = regexp.MustCompile(`\x1b\]4;(\d+);rgb:([0-9a-fA-F]{1,4})/([0-9a-fA-F]{1,4})/([0-9a-fA-F]{1,4})`)
	featureReply = regexp.MustCompile(`\x1b_G([^;\x1b]*);`)
	versionReply = regexp.MustCompile(`\x1bP>\|([^\x1b\a]*)(?:\x1b\\|\a)`)
	textReply    = regexp.MustCompile(`\x1b\](1[01]);rgb:([0-9a-fA-F]{1,4})/([0-9a-fA-F]{1,4})/([0-9a-fA-F]{1,4})`)
)

// channel scales an OSC colour component of 1–4 hex digits to 8 bits.
func channel(hex []byte) uint8 {
	v, _ := strconv.ParseUint(string(hex), 16, 32)
	max := uint64(1)<<(4*len(hex)) - 1
	return uint8(v * 255 / max)
}

// parseProbe reads the terminal's replies so far: done once the DA1 reply
// (ESC [ ? … c) has arrived.
func parseProbe(b []byte) (p Probed, done bool) {
	p.Kitty = bytes.Contains(b, []byte("\x1b_Gi=31;OK"))
	if m := cellReply.FindSubmatch(b); m != nil {
		p.CellH, _ = strconv.Atoi(string(m[1]))
		p.CellW, _ = strconv.Atoi(string(m[2]))
	}
	for _, m := range paletteReply.FindAllSubmatch(b, -1) {
		if n, err := strconv.Atoi(string(m[1])); err == nil && n < len(p.Palette) {
			p.Palette[n] = color.RGBA{channel(m[2]), channel(m[3]), channel(m[4]), 0xff}
		}
	}
	for _, m := range textReply.FindAllSubmatch(b, -1) {
		c := color.RGBA{channel(m[2]), channel(m[3]), channel(m[4]), 0xff}
		if string(m[1]) == "10" {
			p.Foreground = c
		} else {
			p.Background = c
		}
	}
	for _, m := range featureReply.FindAllSubmatch(b, -1) {
		keys := strings.Split(string(m[1]), ",")
		if slices.Contains(keys, "i=32") && slices.Contains(keys, "e=T") {
			p.Placeholders = Unsupported
			if slices.Contains(keys, "U=1") {
				p.Placeholders = Supported
			}
		}
	}
	if m := versionReply.FindSubmatch(b); m != nil {
		p.Terminal = string(m[1])
	}
	if i := bytes.Index(b, []byte("\x1b[?")); i >= 0 {
		done = bytes.IndexByte(b[i:], 'c') >= 0
	}
	return p, done
}

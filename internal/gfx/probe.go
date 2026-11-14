package gfx

import (
	"bytes"
	"errors"
	"image/color"
	"os"
	"regexp"
	"strconv"
	"time"

	"golang.org/x/sys/unix"
)

// probeQuery asks for kitty graphics support (a 1×1 RGB query that the
// terminal must not store) and the cell size in pixels (CSI 16 t), then
// asks for palette colours 4–6 (OSC 4; the glyph fields of the "terminal"
// theme), then sends DA1. Every terminal answers DA1, so its reply arriving
// means every answer that is coming has come.
const probeQuery = "\x1b_Gi=31,s=1,v=1,a=q,t=d,f=24;AAAA\x1b\\" + "\x1b[16t" +
	"\x1b]4;4;?\x1b\\\x1b]4;5;?\x1b\\\x1b]4;6;?\x1b\\" + "\x1b[c"

// Probed is what the terminal said.
type Probed struct {
	Kitty        bool // kitty graphics supported
	CellW, CellH int  // cell size in pixels; 0 if not reported
	// Palette holds the colours the terminal reported for palette
	// entries 0–15 (only 4–6 are asked); alpha 0 means unknown.
	Palette [16]color.RGBA
}

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

	old, err := unix.IoctlGetTermios(fd, unix.TCGETS)
	if err != nil {
		return Probed{}, err
	}
	raw := *old
	raw.Lflag &^= unix.ICANON | unix.ECHO
	raw.Cc[unix.VMIN], raw.Cc[unix.VTIME] = 0, 0
	if err := unix.IoctlSetTermios(fd, unix.TCSETS, &raw); err != nil {
		return Probed{}, err
	}
	defer unix.IoctlSetTermios(fd, unix.TCSETS, old)

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
	if i := bytes.Index(b, []byte("\x1b[?")); i >= 0 {
		done = bytes.IndexByte(b[i:], 'c') >= 0
	}
	return p, done
}

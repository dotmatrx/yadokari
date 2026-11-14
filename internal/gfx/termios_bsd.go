//go:build darwin || freebsd || netbsd || openbsd

package gfx

import "golang.org/x/sys/unix"

// The ioctls that get and set terminal attributes.
const (
	ioctlGetTermios = unix.TIOCGETA
	ioctlSetTermios = unix.TIOCSETA
)

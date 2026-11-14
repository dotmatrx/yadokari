package gfx

import "golang.org/x/sys/unix"

// The ioctls that get and set terminal attributes.
const (
	ioctlGetTermios = unix.TCGETS
	ioctlSetTermios = unix.TCSETS
)

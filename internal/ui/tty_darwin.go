//go:build darwin

package ui

import (
	"os"
	"syscall"
	"unsafe"
)

type winsize struct {
	Row    uint16
	Col    uint16
	Xpixel uint16
	Ypixel uint16
}

const (
	tiocgwin = 0x40087468
	tiocgeta = 0x40487413
	tiocseta = 0x80487414
)

func terminalSize(f *os.File) (int, int, bool) {
	if !IsTerminal(f) {
		return 0, 0, false
	}
	var ws winsize
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), tiocgwin,
		uintptr(unsafe.Pointer(&ws)))
	if errno != 0 {
		return 0, 0, false
	}
	return int(ws.Col), int(ws.Row), true
}

func makeRaw(f *os.File) (*TermiosState, error) {
	fd := f.Fd()
	var old syscall.Termios
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, tiocgeta,
		uintptr(unsafe.Pointer(&old))); errno != 0 {
		return nil, errno
	}
	raw := old
	raw.Iflag &^= syscall.IGNBRK | syscall.BRKINT | syscall.PARMRK | syscall.ISTRIP |
		syscall.INLCR | syscall.IGNCR | syscall.ICRNL | syscall.IXON
	raw.Oflag &^= syscall.OPOST
	raw.Lflag &^= syscall.ECHO | syscall.ECHONL | syscall.ICANON | syscall.ISIG | syscall.IEXTEN
	raw.Cflag &^= syscall.CSIZE | syscall.PARENB
	raw.Cflag |= syscall.CS8
	raw.Cc[syscall.VMIN] = 1
	raw.Cc[syscall.VTIME] = 0
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, tiocseta,
		uintptr(unsafe.Pointer(&raw))); errno != 0 {
		return nil, errno
	}
	return &TermiosState{opaque: old}, nil
}

func restoreTermios(f *os.File, st *TermiosState) error {
	if st == nil {
		return nil
	}
	old, ok := st.opaque.(syscall.Termios)
	if !ok {
		return syscall.EINVAL
	}
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), tiocseta,
		uintptr(unsafe.Pointer(&old)))
	if errno != 0 {
		return errno
	}
	return nil
}

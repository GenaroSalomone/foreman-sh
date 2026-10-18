//go:build !windows

package invokers

import (
	"os"
	"syscall"
	"unsafe"
)

// ttyDevice is `[ -t fd ]`: a character device that answers the terminal ioctl.
func ttyDevice(f *os.File) bool {
	var t syscall.Termios
	_, _, e := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), ioctlReadTermios, uintptr(unsafe.Pointer(&t)))
	return e == 0
}

func kill0(pid int) bool { return syscall.Kill(pid, 0) == nil }

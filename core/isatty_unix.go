//go:build darwin || linux || freebsd || netbsd || openbsd

package main

import (
	"os"
	"syscall"
	"unsafe"
)

func isTerminal(f *os.File) bool {
	var t syscall.Termios
	_, _, e := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), ioctlReadTermios, uintptr(unsafe.Pointer(&t)))
	return e == 0
}

func enableVT(*os.File) {}

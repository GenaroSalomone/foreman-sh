//go:build windows

package main

import (
	"os"
	"syscall"
)

func isTerminal(f *os.File) bool {
	var m uint32
	return syscall.GetConsoleMode(syscall.Handle(f.Fd()), &m) == nil
}

// enableVT asks the console to interpret the ANSI escapes hw prints; a console
// that refuses simply shows them plain.
func enableVT(f *os.File) {
	var m uint32
	if syscall.GetConsoleMode(syscall.Handle(f.Fd()), &m) != nil {
		return
	}
	k := syscall.NewLazyDLL("kernel32.dll").NewProc("SetConsoleMode")
	k.Call(f.Fd(), uintptr(m|0x0004))
}

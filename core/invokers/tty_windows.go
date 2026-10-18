package invokers

import "os"

func ttyDevice(f *os.File) bool { return true }
func kill0(pid int) bool        { p, err := os.FindProcess(pid); return err == nil && p != nil }

//go:build !windows

package invokers

import (
	"os"
	"syscall"
)

// ownedByOther reports a file whose owner is not the running user. Where the platform
// exposes no owner (see owner_windows.go) nothing is reported.
func ownedByOther(st os.FileInfo) bool {
	if stt, ok := st.Sys().(*syscall.Stat_t); ok {
		return int(stt.Uid) != os.Getuid()
	}
	return false
}

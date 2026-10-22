//go:build !windows

package main

import (
	"errors"
	"os"
	"strings"
	"syscall"
)

// hsUmask077 is the bash's `umask 077`: what the child creates is 0600 too.
func hsUmask077() { syscall.Umask(0o077) }

func hsIsExec(p string) bool { return syscall.Access(p, 1) == nil }

// hsLookPath is `command -v name` against the PATH of env: the first executable regular file, "" when none.
func hsLookPath(name string, env []string) string {
	if strings.Contains(name, "/") {
		if hsIsExec(name) {
			return name
		}
		return ""
	}
	path := ""
	for _, kv := range env {
		if strings.HasPrefix(kv, "PATH=") {
			path = kv[5:]
		}
	}
	for _, d := range strings.Split(path, ":") {
		if d == "" {
			d = "."
		}
		p := d + "/" + name
		if st, err := os.Stat(p); err == nil && st.Mode().IsRegular() && hsIsExec(p) {
			return p
		}
	}
	return ""
}

// hsExecProcess is `exec "$@"`: this process becomes the command, found on the PATH of the child's environment.
// It returns only when that failed: 127 for a command not found, 126 for one that cannot run.
func hsExecProcess(argv, env []string) (int, string) {
	path := hsLookPath(argv[0], env)
	if path == "" {
		if strings.Contains(argv[0], "/") {
			if _, err := os.Stat(argv[0]); err != nil {
				return 127, "cannot execute: No such file or directory"
			}
			if st, _ := os.Stat(argv[0]); st.IsDir() {
				return 126, "cannot execute: Is a directory"
			}
			return 126, "cannot execute: Permission denied"
		}
		return 127, "not found"
	}
	err := syscall.Exec(path, argv, env)
	if errors.Is(err, syscall.ENOEXEC) { // no #! line: the bash runs such a file as a shell script
		err = syscall.Exec("/bin/sh", append([]string{"sh", path}, argv[1:]...), env)
	}
	return 126, "cannot execute: " + strings.TrimPrefix(err.Error(), "exec format error")
}

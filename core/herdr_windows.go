//go:build windows

package main

import (
	"io"
	"os"
	"strings"
	"time"
)

// dialHerdr opens the named pipe herdr serves on Windows: the pipe's name is the
// path herdr reports (bin/herdr-rpc _pipe_name), under \\.\pipe\.
func dialHerdr(path string, timeout time.Duration) (io.ReadWriteCloser, error) {
	p := strings.ReplaceAll(path, "/", `\`)
	if !strings.HasPrefix(strings.ToLower(p), `\\.\pipe\`) {
		p = `\\.\pipe\` + p
	}
	deadline := time.Now().Add(timeout)
	for {
		f, err := os.OpenFile(p, os.O_RDWR, 0)
		if err == nil {
			return f, nil
		}
		if time.Now().After(deadline) {
			return nil, err
		}
		time.Sleep(20 * time.Millisecond)
	}
}

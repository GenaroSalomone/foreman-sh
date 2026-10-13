//go:build !windows

package main

import (
	"io"
	"net"
	"time"
)

// dialHerdr opens herdr's unix socket.
func dialHerdr(path string, timeout time.Duration) (io.ReadWriteCloser, error) {
	return net.DialTimeout("unix", path, timeout)
}

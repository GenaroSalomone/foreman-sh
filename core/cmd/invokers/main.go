// invokers is ask-invoker and done-invoker as one small binary.
//
//	invokers --bin-dir <brain/bin> ask-invoker|done-invoker <args…>
//
// bin/ask-invoker and bin/done-invoker exec it (HW_IMPL=go); the bash they carry
// is the rollback.
package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"hwcore/invokers"
)

func main() {
	args := os.Args[1:]
	if len(args) < 4 || args[0] != "--bin-dir" {
		fmt.Fprintln(os.Stderr, "usage: invokers --bin-dir <brain/bin> ask-invoker|done-invoker <args…>")
		os.Exit(64)
	}
	binDir, prog, rest := args[1], args[2], args[3:]
	c := invokers.NewCtx(prog, binDir)
	ch := make(chan os.Signal, 2)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		s := <-ch
		code := 130
		if s == syscall.SIGTERM {
			code = 143
		}
		c.SignalExit(code)
	}()
	switch prog {
	case "ask-invoker":
		os.Exit(invokers.Ask(c, rest))
	case "done-invoker":
		os.Exit(invokers.Done(c, rest))
	}
	fmt.Fprintf(os.Stderr, "invokers: unknown program %q\n", prog)
	os.Exit(64)
}

package main

// The output vocabulary of bin/hw (bin/hw:655-672): `info` is "  · " on stdout,
// `warn` is "  ! " on stderr, `die` is "  ✗ " on stderr and ends the process.
// The failure glyph has exactly one site, here, as in bash.

import (
	"fmt"
	"io"
)

type term struct {
	out, err io.Writer
	// Colour: off under NO_COLOR (any non-empty value), TERM=dumb, and whenever
	// stdout or stderr is not a terminal.
	cOK, cWarn, cErr, cDim, cB, c0 string
}

func newTerm(out, err io.Writer, color bool) *term {
	t := &term{out: out, err: err}
	if color {
		t.cOK, t.cWarn, t.cErr, t.cDim, t.cB, t.c0 = "\033[32m", "\033[33m", "\033[31m", "\033[2m", "\033[1m", "\033[0m"
	}
	return t
}

func (t *term) ok(msg string)   { fmt.Fprintf(t.out, "  %s✓%s %s\n", t.cOK, t.c0, msg) }
func (t *term) info(msg string) { fmt.Fprintf(t.out, "  %s·%s %s\n", t.cDim, t.c0, msg) }
func (t *term) warn(msg string) { fmt.Fprintf(t.err, "  %s!%s %s\n", t.cWarn, t.c0, msg) } // # MUTATION-ANCHOR: 816-G26
func (t *term) step(msg string) { fmt.Fprintf(t.out, "\n%s%s%s\n", t.cB, msg, t.c0) }

// exitNow unwinds to main with the status the verb ends with.
type exitNow struct{ code int }

func (t *term) dieRC(rc int, msg string) {
	fmt.Fprintf(t.err, "  %s✗%s %s\n", t.cErr, t.c0, msg)
	panic(exitNow{rc})
}

func (t *term) die(msg string) { t.dieRC(1, msg) }

func (t *term) printf(format string, args ...any) { fmt.Fprintf(t.out, format, args...) }
func (t *term) errf(format string, args ...any)   { fmt.Fprintf(t.err, format, args...) }

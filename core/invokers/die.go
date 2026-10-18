// Package invokers is the Go port of bin/ask-invoker, bin/done-invoker and
// bin/invoker-common.sh: an executor's two channels to its brainer.
package invokers

import (
	"fmt"
	"sort"
)

// Die is a refusal: the program prints "<prog>: <Msg>" on stderr and exits Code
// (1 unless set). It is what `die` is in the bash.
type Die struct {
	Msg  string
	Code int
}

func (d *Die) Error() string { return d.Msg }

func die(format string, a ...any) *Die {
	return &Die{Msg: fmt.Sprintf(format, a...), Code: 1}
}

var sprintf = fmt.Sprintf

func sortStrings(s []string) { sort.Strings(s) }

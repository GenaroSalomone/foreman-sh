// Package engramhealth is the one probe of an engram serve's HTTP API. A serve under load answers /health in 2-3 s, so a
// short budget reads a slow serve as a dead one; every caller shares this budget, this retry and this refusal text.
package engramhealth

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// DefaultBudget is the wait for one try; HW_ENGRAM_HEALTH_TIMEOUT (seconds, may be fractional) overrides it.
const DefaultBudget = 10 * time.Second // engram-lento-M01

// Tries is how many times a transport failure is attempted. An HTTP status of 400 and over is an answer: it is not retried.
const Tries = 2 // engram-lento-M02

// Budget is the per-try wait for the value of HW_ENGRAM_HEALTH_TIMEOUT: anything that is not a positive number is the default.
func Budget(env string) time.Duration {
	s, err := strconv.ParseFloat(strings.TrimSpace(env), 64)
	if err != nil || s <= 0 {
		return DefaultBudget
	}
	return time.Duration(s * float64(time.Second))
}

// Result is what a probe established.
type Result struct {
	Body   string
	OK     bool
	Tries  int           // attempts made
	Budget time.Duration // wait of each attempt
	Status int           // HTTP status of the last answer; 0 when none came
}

// Get is `curl -sf -m <budget> url` with one retry after a transport failure: the body, OK false on a transport failure
// (both tries) or an HTTP status of 400 and over (the first try).
func Get(url string, budget time.Duration) Result {
	r := Result{Budget: budget}
	cl := &http.Client{Timeout: budget}
	for r.Tries < Tries {
		r.Tries++
		resp, err := cl.Get(url)
		if err != nil {
			continue
		}
		b, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			continue
		}
		r.Status = resp.StatusCode
		r.Body = string(b)
		r.OK = resp.StatusCode < 400
		return r // engram-lento-M03
	}
	return r
}

// Refusal is the line for a probe that did not answer: how long it waited and how often, so a slow serve is told apart from
// a dead one.
func Refusal(url string, r Result) string {
	n := "tries" // engram-lento-M04
	if r.Tries == 1 {
		n = "try"
	}
	s := fmt.Sprintf("no engram serve answered at %s within %s s (%d %s)", url, strconv.FormatFloat(r.Budget.Seconds(), 'f', -1, 64), r.Tries, n)
	if r.Status != 0 {
		s += ": it answered HTTP " + strconv.Itoa(r.Status)
	}
	return s
}

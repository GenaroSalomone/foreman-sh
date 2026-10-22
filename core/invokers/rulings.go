package invokers

import (
	"os"
	"sort"
	"strconv"
	"strings"
)

// rulingsPending is ruling_queue_pending: the undelivered rulings of a run,
// oldest first. The unnumbered `pending-ruling` an hw from before the queue
// wrote is the oldest of all; the numbered ones follow in numeric order.
func rulingsPending(d string) []string {
	var out []string
	if isRegular(d + "/pending-ruling") {
		out = append(out, d+"/pending-ruling")
	}
	ents, _ := os.ReadDir(d)
	type numbered struct {
		n    int64
		path string
	}
	var num []numbered
	for _, e := range ents {
		suffix, ok := strings.CutPrefix(e.Name(), "pending-ruling.")
		if !ok || suffix == "" || !isDigits(suffix) || !isRegular(d+"/"+e.Name()) {
			continue
		}
		v, _ := strconv.ParseInt(suffix, 10, 64)
		num = append(num, numbered{v, d + "/" + e.Name()})
	}
	// `sort -n -k1,1`: by number, a tie by the whole line
	sort.Slice(num, func(i, j int) bool {
		if num[i].n != num[j].n {
			return num[i].n < num[j].n
		}
		return num[i].path < num[j].path
	})
	for _, x := range num {
		out = append(out, x.path)
	}
	return out
}

// ClaimRulings is ruling_queue_claim_all: every pending ruling of the run in
// d, oldest first, each taken with a rename (done-invoker, a turn end and a
// sender's retraction race for the same names; exactly one rename wins each).
// A ruling is consumed BEFORE it is handed over: a crash loses one rather than
// repeating it. It returns the delivery text, and false when there was none.
func (c *Ctx) ClaimRulings(d, via string) (string, bool) {
	at := c.stamp()
	count := 0
	first, body := "", ""
	for _, f := range rulingsPending(d) {
		n := strings.TrimPrefix(strings.TrimPrefix(f, d+"/pending-ruling"), ".")
		if n == "" {
			n = "legacy"
		}
		claim := d + "/.pending-ruling.claimed." + n + "." + itoa(c.Pid)
		if os.Rename(f, claim) != nil {
			continue
		}
		b, _ := os.ReadFile(claim)
		text := strings.TrimRight(string(b), "\n")
		if text != "" {
			count++
			if n != "legacy" {
				os.WriteFile(d+"/ruling-delivered."+n, []byte("at="+at+"\nvia="+via+"\n"), 0o666)
			}
			c.transcriptAppend(d, "RULING-DELIVERED", "ruling:"+n+" via="+via, text)
			if count == 1 {
				first = text
			}
			body += "── ruling " + itoa(count) + " ──\n" + text + "\n\n"
		}
		os.Remove(claim)
	}
	if count == 0 {
		return "", false
	}
	c.quiet(c.BinDir+"/cockpit-state", "--kick", "--rundir", d)
	os.WriteFile(d+"/ruling-delivered", []byte("at="+at+"\nvia="+via+"\ncount="+itoa(count)+"\n"), 0o666)
	if count == 1 {
		return first, true
	}
	return itoa(count) + " RULINGS, in the order your brainer sent them. Where a later one disagrees with an earlier one, the later one stands.\n\n" + strings.TrimSuffix(body, "\n\n"), true
}

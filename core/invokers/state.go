package invokers

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
)

// runDir is invoker_run_dir.
func (c *Ctx) runDir() string {
	return c.or("HW_WORKDIR", c.tmpdir()) + "/.hw/" + c.or("HW_RUN", "norun")
}

// taskSeq is invoker_task_seq: the digits of <run>/task, 1 when there are none or 0.
func (c *Ctx) taskSeq() string {
	var n string
	if b, err := os.ReadFile(c.runDir() + "/task"); err == nil {
		var d []byte
		for _, ch := range b {
			if ch >= '0' && ch <= '9' {
				d = append(d, ch)
			}
		}
		n = string(d)
	}
	if n == "" || n == "0" {
		return "1"
	}
	return n
}

// atoi10 is the number a digit string stands for, read in base 10.
func atoi10(s string) int {
	n := 0
	for i := 0; i < len(s) && n < 1<<40; i++ {
		n = n*10 + int(s[i]-'0')
	}
	return n
}

// stateDir is invoker_state_dir: the run dir for task 1, t<N> after it.
func (c *Ctx) stateDir() string {
	seq := c.taskSeq()
	if atoi10(seq) > 1 {
		return c.runDir() + "/t" + seq
	}
	return c.runDir()
}

// envelopePrefix is invoker_envelope_prefix.
func (c *Ctx) envelopePrefix() string {
	seq := c.taskSeq()
	if atoi10(seq) > 1 {
		return c.or("HW_RUN", "norun") + ":t" + seq
	}
	return c.or("HW_RUN", "norun")
}

// receiptValue is `jq -r 'select(.key==$k) | .value' receipt.jsonl | tail -1`
// with its failures read as empty.
func receiptValue(path, key string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var outs []string
	for {
		var v any
		if err := dec.Decode(&v); err != nil {
			if err != io.EOF {
				// jq prints what it had, then fails; the caller reads that as empty on error only
			}
			break
		}
		if v == nil {
			continue
		}
		m, ok := v.(map[string]any)
		if !ok {
			// `.key` on a non-object stops jq with an error
			break
		}
		if k, isStr := m["key"].(string); isStr && k == key {
			outs = append(outs, jqRawString(m["value"]))
		}
	}
	if len(outs) == 0 {
		return ""
	}
	full := strings.Join(outs, "\n")
	lines := strings.Split(full, "\n")
	return strings.TrimRight(lines[len(lines)-1], "\n")
}

// transcriptAppend is transcript_append: best effort, never fails.
func (c *Ctx) transcriptAppend(dir, kind, envelope, text string) {
	if dir == "" {
		return
	}
	base := dir[strings.LastIndexByte(dir, '/')+1:]
	if len(base) > 1 && base[0] == 't' && base[1] >= '0' && base[1] <= '9' && isDigits(base[1:]) {
		dir = dir[:strings.LastIndexByte(dir, '/')]
	}
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		return
	}
	lines := strings.Split(text+"\n", "\n")
	lines = lines[:len(lines)-1]
	for i, l := range lines {
		lines[i] = "    | " + l
	}
	if envelope == "" {
		envelope = "-"
	}
	f, err := os.OpenFile(dir+"/transcript.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o666)
	if err != nil {
		return
	}
	defer f.Close()
	io.WriteString(f, c.stamp()+"  "+kind+"  envelope="+envelope+"\n"+strings.Join(lines, "\n")+"\n")
}

// ── the outbox ─────────────────────────────────────────────────────────────

func (c *Ctx) outboxDir() string { return c.stateDir() + "/outbox" }

func outboxKey(envelope string) string {
	b := []byte(envelope)
	for i, ch := range b {
		if !(ch >= 'A' && ch <= 'Z' || ch >= 'a' && ch <= 'z' || ch >= '0' && ch <= '9' || ch == '.' || ch == '_' || ch == '-') {
			b[i] = '_'
		}
	}
	return string(b)
}

func (c *Ctx) outboxWriteMeta(envelope, state, status string) error {
	d := c.outboxDir()
	key := outboxKey(envelope)
	if status == "" {
		if b, err := os.ReadFile(d + "/" + key + ".meta"); err == nil {
			for _, l := range strings.Split(string(b), "\n") {
				if strings.HasPrefix(l, "status=") {
					status = strings.TrimPrefix(l, "status=")
					break
				}
			}
		}
	}
	run := c.or("HW_RUN", "norun")
	body := "envelope=" + envelope + "\nproject=" + c.get("HW_PROJECT") + "\ntask=" + c.get("HW_TASK") +
		"\nrun=" + run + "\nstatus=" + status + "\nstate=" + state + "\nat=" + c.stamp() + "\npid=" + itoa(c.Pid) + "\n"
	return os.WriteFile(d+"/"+key+".meta", []byte(body), 0o666)
}

// outboxPut writes the report down before it is sent.
func (c *Ctx) outboxPut(envelope, message, status string) error {
	d := c.outboxDir()
	key := outboxKey(envelope)
	if err := os.MkdirAll(d, 0o777); err != nil {
		return err
	}
	state := "pending"
	if b, err := os.ReadFile(d + "/" + key + ".meta"); err == nil {
		for _, l := range strings.Split(string(b), "\n") {
			if strings.HasPrefix(l, "state=uncertain") {
				state = "uncertain"
			}
		}
	}
	if err := os.WriteFile(d+"/"+key+".msg.tmp", []byte(message), 0o666); err != nil {
		return err
	}
	if err := os.Rename(d+"/"+key+".msg.tmp", d+"/"+key+".msg"); err != nil {
		return err
	}
	return c.outboxWriteMeta(envelope, state, status)
}

// outboxSettle: admitted, nothing left to redeliver.
func (c *Ctx) outboxSettle(envelope string) {
	d := c.outboxDir()
	key := outboxKey(envelope)
	os.Remove(d + "/" + key + ".msg")
	os.Remove(d + "/" + key + ".meta")
}

// deliveryUncertain: channel-send exit 5, said on disk.
func (c *Ctx) deliveryUncertain(envelope, route string) {
	d := c.outboxDir()
	key := outboxKey(envelope)
	os.WriteFile(c.stateDir()+"/delivery-uncertain",
		[]byte("envelope="+envelope+"\nroute="+route+"\nat="+c.stamp()+"\nchannel_send_exit=5\n"), 0o666)
	if _, err := os.Stat(d + "/" + key + ".meta"); err == nil {
		c.outboxWriteMeta(envelope, "uncertain", "")
	}
}

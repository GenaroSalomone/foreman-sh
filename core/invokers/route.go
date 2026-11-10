package invokers

import (
	"os"
	"path/filepath"
	"strings"

	"hwcore/internal/mux"
)

// vendorHasNativeTransport is invoker_vendor_has_native_transport.
func vendorHasNativeTransport(v string) bool {
	switch v {
	case "", "claude", "codex", "pi":
		return false
	}
	return true
}

// route is invoker_route: the vendor's native route, or herdr.
func (c *Ctx) route() string {
	v := c.get("HW_EXECUTOR_VENDOR")
	if v != "" && v == c.get("HW_INVOKER_VENDOR") && vendorHasNativeTransport(v) &&
		c.get("HW_INVOKER_SESSION") != "" && c.get("HW_INVOKER_ENDPOINT") != "" {
		return v
	}
	if mux.IsHCAddress(c.get, c.get("HW_INVOKER_PANE")) {
		return "hc"
	}
	return "herdr"
}

func (c *Ctx) channelSend() string { return c.BinDir + "/channel-send" }

// deliver is invoker_deliver: channel-send, its streams ours. reportState is the
// state dir done-invoker names for the held-for-reply fact.
func (c *Ctx) deliver(message, reportState string) int {
	route := c.route()
	o := runOpts{stdin: c.Stdin, stdout: c.Stdout, stderr: c.Stderr}
	if route == "herdr" || route == "hc" {
		if reportState != "" {
			return c.run(o, c.channelSend(), "--report", "--report-state", reportState, route, c.get("HW_INVOKER_PANE"), "-", message)
		}
		return c.run(o, c.channelSend(), "--report", route, c.get("HW_INVOKER_PANE"), "-", message)
	}
	return c.run(o, c.channelSend(), route, c.get("HW_INVOKER_SESSION"), c.get("HW_INVOKER_ENDPOINT"), message)
}

type sender struct{ vendor, session, endpoint string }

// resolveSender is invoker_resolve_sender: the pane's own native address, live.
func (c *Ctx) resolveSender(named string) sender {
	pane := named
	if pane == "" && mux.Backend(c.get) != mux.HC { // an app terminal's HERDR_PANE_ID is its launcher's pane, never its own
		pane = c.get("HERDR_PANE_ID")
	}
	s := sender{}
	if named == "" {
		s.vendor = c.get("HW_EXECUTOR_VENDOR")
	}
	if pane == "" {
		return s
	}
	info, _ := c.out("herdr", "agent", "get", pane)
	doc, _ := decodeDoc(info)
	if v, ok := jqPath(doc, "result", "agent", "agent_session", "value"); ok {
		s.session = jqRawString(v)
	}
	if s.vendor == "" {
		if v, ok := jqPath(doc, "result", "agent", "agent"); ok {
			s.vendor = jqRawString(v)
		}
	}
	switch s.vendor {
	case "opencode":
		process, _ := c.out("herdr", "pane", "process-info", "--pane", pane)
		if port := opencodePort(process); port != "" {
			s.endpoint = "http://127.0.0.1:" + port
		}
	case "claude", "codex", "pi":
		s.endpoint = "-"
	}
	return s
}

// opencodePort is the jq of invoker_resolve_sender: the --port of the first
// foreground process whose argv has "opencode".
func opencodePort(process string) string {
	doc, ok := decodeDoc(process)
	if !ok {
		return ""
	}
	fg, ok := jqPath(doc, "result", "process_info", "foreground_processes")
	list, isList := fg.([]any)
	if !ok || !isList {
		return ""
	}
	for _, p := range list {
		pm, _ := p.(map[string]any)
		argv, _ := pm["argv"].([]any)
		has := false
		for _, a := range argv {
			if a == "opencode" {
				has = true
			}
		}
		if !has {
			continue
		}
		for i, a := range argv {
			if a == "--port" {
				if i+1 < len(argv) && argv[i+1] != nil {
					return jqRawString(argv[i+1])
				}
				return ""
			}
		}
		return ""
	}
	return ""
}

// bashQ is bash's printf %q.
func bashQ(s string) string {
	if s == "" {
		return "''"
	}
	var sb strings.Builder
	for i := 0; i < len(s); i++ {
		ch := s[i]
		switch {
		case ch == '\n' || ch == '\t' || ch < 0x20 || ch == 0x7f || (ch >= 0x80 && ch <= 0x9f):
			return bashAnsiQ(s)
		case strings.IndexByte("\\'\"` !$&()*;<>?[]^{|}", ch) >= 0, (ch == '#' || ch == '~') && i == 0:
			sb.WriteByte('\\')
		}
		sb.WriteByte(ch)
	}
	return sb.String()
}

func bashAnsiQ(s string) string {
	var sb strings.Builder
	sb.WriteString("$'")
	for i := 0; i < len(s); i++ {
		switch ch := s[i]; {
		case ch == '\n':
			sb.WriteString(`\n`)
		case ch == '\t':
			sb.WriteString(`\t`)
		case ch == '\\' || ch == '\'':
			sb.WriteByte('\\')
			sb.WriteByte(ch)
		case ch < 0x20 || ch == 0x7f || (ch >= 0x80 && ch <= 0x9f):
			sb.WriteString("\\" + octal3(int(ch)))
		default:
			sb.WriteByte(ch)
		}
	}
	sb.WriteString("'")
	return sb.String()
}

func octal3(n int) string {
	return string([]byte{byte('0' + n>>6&7), byte('0' + n>>3&7), byte('0' + n&7)})
}

// replyCommand is invoker_reply_command: the command line the brainer runs to
// answer, and (with a hold file) the pending-reply fact it will honour.
func (c *Ctx) replyCommand(logicalID, intent, holdFile string) (string, error) {
	rulingFlag := ""
	switch intent {
	case "answer":
	case "ruling":
		rulingFlag = "--ruling "
	default:
		return "", die("internal: unknown reply intent %s", intent)
	}
	s := c.resolveSender("")
	var route, target, endpoint string
	if vendorHasNativeTransport(s.vendor) && s.session != "" && s.endpoint != "" {
		route, target, endpoint = s.vendor, s.session, s.endpoint
	} else {
		route, target, endpoint = "herdr", c.get("HERDR_PANE_ID"), "-"
		if mux.Backend(c.get) == mux.HC {
			route, target = "hc", c.selfAddress()
		}
	}
	if holdFile != "" {
		if !strings.HasPrefix(holdFile, "/") {
			return "", die("internal: pending reply file must be absolute")
		}
		body := "version=1\nstate=undelivered\nintent=" + intent + "\n" +
			"route=" + route + "\ntarget=" + target + "\nlogical_id=" + logicalID + "\n" +
			"pane=" + c.selfAddress() + "\nrun=" + c.or("HW_RUN", "norun") + "\n"
		if err := os.WriteFile(holdFile, []byte(body), 0o600); err != nil {
			return "", die("could not record pending reply fact at %s", holdFile)
		}
	}
	cs := bashQ(c.channelSend())
	if route == "opencode" {
		if logicalID == "" {
			return "", die("internal: OpenCode reply command needs a stable envelope id")
		}
		if holdFile != "" {
			return cs + " " + rulingFlag + "--require processed --id " + bashQ(logicalID) + " --reply-hold " + bashQ(holdFile) + " " + bashQ(route) + " " + bashQ(target) + " " + bashQ(endpoint) + ` "<your answer>"`, nil
		}
		return cs + " " + rulingFlag + "--require processed --id " + bashQ(logicalID) + " " + bashQ(route) + " " + bashQ(target) + " " + bashQ(endpoint) + ` "<your answer>"`, nil
	}
	if intent == "ruling" {
		if holdFile != "" {
			return cs + " --ruling --reply-hold " + bashQ(holdFile) + " " + bashQ(route) + " " + bashQ(target) + " " + bashQ(endpoint) + ` "<your ruling>"`, nil
		}
		return cs + " --ruling " + bashQ(route) + " " + bashQ(target) + " " + bashQ(endpoint) + ` "<your ruling>"`, nil
	}
	if holdFile != "" {
		return cs + " --report --reply-hold " + bashQ(holdFile) + " " + bashQ(route) + " " + bashQ(target) + " " + bashQ(endpoint) + ` "<your answer>"`, nil
	}
	return cs + " --report " + bashQ(route) + " " + bashQ(target) + " " + bashQ(endpoint) + ` "<your answer>"`, nil
}

// hcLiveTerminals is `hc ls --json` (live terminals only): address -> cwd. false when hc cannot answer, which is not
// evidence that a terminal is gone.
func (c *Ctx) hcLiveTerminals() (map[string]string, bool) {
	out, rc := c.out("hc", "ls", "--json", "--label", "label=brain")
	if rc != 0 {
		return nil, false
	}
	doc, ok := decodeDoc(out)
	v, _ := jqPath(doc, "terminals")
	list, isList := v.([]any)
	if !ok || !isList {
		return nil, false
	}
	live := map[string]string{}
	for _, e := range list {
		m, _ := e.(map[string]any)
		addr, _ := m["address"].(string)
		cwd, _ := m["cwd"].(string)
		live[addr] = cwd
	}
	return live, true
}

// followBrainer re-points an hc invoker whose terminal is gone at the lane's CURRENT brainer: `brain <lane>` reopened
// it as a new terminal (t6 -> t9), and the address baked into the executor's environment still names the old one.
// Gone is what `hc get` says of the recorded pane (not live, or unknown): a live invoker is never re-pointed, whatever
// its labels (the resolver accepts an explicit HW_INVOKER_PANE without label=brain). The replacement is the live
// label=brain terminal whose cwd is brain/<project>; none (it is closed) leaves the address alone, so the report is
// queued for the reopen instead. It returns whether the old address is gone and no brainer replaced it.
func (c *Ctx) followBrainer() (gone bool) {
	pane := c.get("HW_INVOKER_PANE")
	if !mux.IsHCAddress(c.get, pane) || c.get("HW_PROJECT") == "" { // no project, no brainer directory to match
		return false
	}
	if c.hcTerminalLive(pane) {
		return false
	}
	live, ok := c.hcLiveTerminals()
	if !ok {
		return false
	}
	want, _ := filepath.EvalSymlinks(filepath.Join(filepath.Dir(c.BinDir), c.get("HW_PROJECT")))
	for addr, cwd := range live {
		if got, _ := filepath.EvalSymlinks(cwd); got != "" && got == want {
			c.Env["HW_INVOKER_PANE"] = addr
			c.repointRunEnv(pane, addr)
			return false
		}
	}
	return true
}

// repointRunEnv rewrites the run's env file row for the closed brainer: channel-send's --report-state proof compares the
// file's HW_INVOKER_PANE with the resolved receiver and refused the re-pointed report ("t2, expected t7"). Best effort:
// without the file the proof refuses on its own, as before.
func (c *Ctx) repointRunEnv(old, addr string) {
	path := c.runDir() + "/env"
	b, err := os.ReadFile(path)
	if err != nil {
		return
	}
	row := "HW_INVOKER_PANE='" + old + "'"
	if !strings.Contains(string(b), row+"\n") {
		return
	}
	tmp := path + ".repoint"
	if os.WriteFile(tmp, []byte(strings.Replace(string(b), row+"\n", "HW_INVOKER_PANE='"+addr+"'\n", 1)), 0o600) != nil {
		return
	}
	if os.Rename(tmp, path) != nil {
		os.Remove(tmp)
	}
}

// hcTerminalLive is `hc get <id>` saying the terminal is live. A failed `hc get` reads as "not live", which is only
// the first half of "gone": followBrainer asks `hc ls` next and keeps the address when hc cannot answer that either.
func (c *Ctx) hcTerminalLive(id string) bool {
	out, rc := c.out("hc", "get", id, "--json")
	if rc != 0 {
		return false
	}
	doc, _ := decodeDoc(out)
	v, _ := jqPath(doc, "state")
	return v == "live"
}

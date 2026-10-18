package invokers

import (
	"os"
	"strings"
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
	return "herdr"
}

func (c *Ctx) channelSend() string { return c.BinDir + "/channel-send" }

// deliver is invoker_deliver: channel-send, its streams ours. reportState is the
// state dir done-invoker names for the held-for-reply fact.
func (c *Ctx) deliver(message, reportState string) int {
	route := c.route()
	o := runOpts{stdin: c.Stdin, stdout: c.Stdout, stderr: c.Stderr}
	if route == "herdr" {
		if reportState != "" {
			return c.run(o, c.channelSend(), "--report", "--report-state", reportState, "herdr", c.get("HW_INVOKER_PANE"), "-", message)
		}
		return c.run(o, c.channelSend(), "--report", "herdr", c.get("HW_INVOKER_PANE"), "-", message)
	}
	return c.run(o, c.channelSend(), route, c.get("HW_INVOKER_SESSION"), c.get("HW_INVOKER_ENDPOINT"), message)
}

type sender struct{ vendor, session, endpoint string }

// resolveSender is invoker_resolve_sender: the pane's own native address, live.
func (c *Ctx) resolveSender(named string) sender {
	pane := named
	if pane == "" {
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
	}
	if holdFile != "" {
		if !strings.HasPrefix(holdFile, "/") {
			return "", die("internal: pending reply file must be absolute")
		}
		body := "version=1\nstate=undelivered\nintent=" + intent + "\n" +
			"route=" + route + "\ntarget=" + target + "\nlogical_id=" + logicalID + "\n" +
			"pane=" + c.get("HERDR_PANE_ID") + "\nrun=" + c.or("HW_RUN", "norun") + "\n"
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

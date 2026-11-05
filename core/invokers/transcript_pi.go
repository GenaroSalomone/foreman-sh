package invokers

// Proof from a pi executor's own session file. Two facts measured from real sessions under
// ~/.pi/agent/sessions/:
//
//   - the receipt's `session_id` for a pi pane is the session FILE's path, so that path is what is read;
//   - a skill runs as a READ of its SKILL.md: pi has no Skill tool and no `/skill:` message.
//
// A `subagent` call (or the older `Agent`) names the agent it launches. Nothing counts until the call's own
// `toolResult` arrives without `isError`: an unknown agent fails exactly that way, and the call alone would read as
// a run that never happened.

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// piSessionsRoot is where pi keeps its sessions: <agent dir>/sessions. The agent dir is $PI_CODING_AGENT_DIR when
// set, else <HOME>/.pi/agent, and it expands a leading `~` and resolves a relative value against the cwd — the same
// resolution hw uses to launch pi.
func (c *Ctx) piSessionsRoot() string {
	home := c.get("HOME")
	d := c.get("PI_CODING_AGENT_DIR")
	switch {
	case d == "":
		if home == "" {
			return ""
		}
		d = filepath.Join(home, ".pi", "agent")
	case d == "~" && home != "":
		d = home
	case strings.HasPrefix(d, "~/") && home != "":
		d = filepath.Join(home, d[2:])
	}
	root := filepath.Join(d, "sessions")
	if !filepath.IsAbs(root) && c.Cwd != "" {
		root = filepath.Join(c.Cwd, root)
	}
	return root
}

// piTranscriptFiles is the pi session file the receipt names, and its own subagents' sessions. The receipt records
// a path: it is accepted only as an absolute `.jsonl` regular file under pi's sessions root. Read-only.
func (c *Ctx) piTranscriptFiles(sid string) []string {
	if !filepath.IsAbs(sid) || !strings.HasSuffix(sid, ".jsonl") || !isRegular(sid) {
		return nil
	}
	root := c.piSessionsRoot()
	if root == "" {
		return nil
	}
	if rel, err := filepath.Rel(root, sid); err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil
	}
	if !piSessionIsOfRun(sid, c.get("HW_WORKDIR")) {
		return nil
	}
	files := []string{sid}
	dir := strings.TrimSuffix(sid, ".jsonl")
	var kids []string
	children, _ := os.ReadDir(dir)
	for _, child := range children {
		if !child.IsDir() {
			continue
		}
		runs, err := os.ReadDir(filepath.Join(dir, child.Name()))
		if err != nil {
			continue
		}
		for _, run := range runs {
			if !run.IsDir() || !strings.HasPrefix(run.Name(), "run-") {
				continue
			}
			p := filepath.Join(dir, child.Name(), run.Name(), "session.jsonl")
			if isRegular(p) {
				kids = append(kids, p)
			}
		}
	}
	sort.Strings(kids)
	return append(files, kids...)
}

// piSessionIsOfRun: the session's header (`{"type":"session","cwd":…}`, its first line) names the directory pi ran in; it
// must be this run's workdir, or a receipt naming another run's session would prove this one's reviews. With no workdir
// to compare against (outside an executor) there is nothing to check.
func piSessionIsOfRun(sid, workdir string) bool {
	if workdir == "" {
		return true
	}
	fh, err := os.Open(sid)
	if err != nil {
		return false
	}
	defer fh.Close()
	var head struct {
		Type string `json:"type"`
		Cwd  string `json:"cwd"`
	}
	line, _ := bufio.NewReaderSize(fh, 1<<16).ReadBytes('\n')
	if json.Unmarshal(line, &head) != nil || head.Type != "session" || head.Cwd == "" {
		return false
	}
	return sameDir(head.Cwd, workdir)
}

func sameDir(a, b string) bool {
	canon := func(p string) string {
		if r, err := filepath.EvalSymlinks(p); err == nil {
			return r
		}
		return filepath.Clean(p)
	}
	return canon(a) == canon(b)
}

// piSkillName is the skill a read path runs, only under the layouts measured: `<plugins>/<plugin>/skills/<name>/SKILL.md`
// proves `<plugin>:<name>` and `<name>`, and `.claude/skills/<name>/SKILL.md` (a project or user skill) proves the bare
// name. Any other file named SKILL.md runs no skill, and a mention in text is prose.
func piSkillName(path string) (full, bare string) {
	var parts []string
	for _, p := range strings.Split(path, "/") {
		if p != "" {
			parts = append(parts, p)
		}
	}
	if len(parts) < 4 || parts[len(parts)-1] != "SKILL.md" || parts[len(parts)-3] != "skills" {
		return "", ""
	}
	bare = parts[len(parts)-2]
	if parts[len(parts)-4] == ".claude" {
		return "", bare
	}
	if len(parts) >= 5 && parts[len(parts)-5] == "plugins" {
		return parts[len(parts)-4] + ":" + bare, bare
	}
	return "", ""
}

// piChild is one child of a pi-subagents call; it ran when it carries both an agent and a task.
type piChild struct {
	Agent string `json:"agent"`
	Task  string `json:"task"`
}

// piExtractor reads a pi jsonl line. A toolCall proves nothing until the toolResult with its own id arrives: an
// `isError` result, or none at all, means nothing ran. Only an ASSISTANT call counts — a toolCall quoted in a user
// message or in a result is text about a call, not one.
type piExtractor struct {
	pending map[string][]string
}

func (e *piExtractor) flush(*TranscriptProof) { e.pending = map[string][]string{} }

func (e *piExtractor) line(raw []byte, p *TranscriptProof) {
	var line struct {
		Type    string `json:"type"`
		Message struct {
			Role       string `json:"role"`
			ToolCallID string `json:"toolCallId"`
			IsError    bool   `json:"isError"`
			Content    []struct {
				Type      string          `json:"type"`
				ID        string          `json:"id"`
				Name      string          `json:"name"`
				Arguments json.RawMessage `json:"arguments"`
			} `json:"content"`
		} `json:"message"`
	}
	if json.Unmarshal(raw, &line) != nil || line.Type != "message" {
		return
	}
	switch line.Message.Role {
	case "toolResult":
		names, ok := e.pending[line.Message.ToolCallID]
		if !ok {
			return
		}
		delete(e.pending, line.Message.ToolCallID)
		if line.Message.IsError {
			return
		}
		for _, n := range names {
			p.Names[n] = true
		}
		// this call's own names, together: a toolCall is one call, so its `workflow:"parallel"`
		// children are one group (retro-fixes-rc3 item 7).
		p.Groups = append(p.Groups, names)
	case "assistant":
		for _, b := range line.Message.Content {
			if b.Type != "toolCall" || b.ID == "" {
				continue
			}
			if names := piCallNames(b.Name, b.Arguments); len(names) > 0 {
				e.pending[b.ID] = names
			}
		}
	}
}

// piCallNames is what one pi toolCall would prove: the skill a read of a measured SKILL.md layout runs, or the agents
// a `subagent` call (single or workflow) or an older `Agent`/`Task` names. Empty when the call proves nothing.
func piCallNames(name string, args json.RawMessage) []string {
	switch name {
	case "read":
		var a struct {
			Path string `json:"path"`
		}
		if json.Unmarshal(args, &a) != nil {
			return nil
		}
		full, bare := piSkillName(a.Path)
		var names []string
		if bare != "" {
			names = append(names, bare)
		}
		if full != "" {
			names = append(names, full)
		}
		return names
	case "subagent":
		// pi-subagents' tool also manages agents (`action`: get, list, status, update…), which runs nothing, and runs
		// asynchronously unless the call says `async:false`: an async call's own result is only "launched", so it does not
		// show the child ran. Only a call that executes and waits proves an agent.
		var a struct {
			Action   string          `json:"action"`
			Agent    string          `json:"agent"`
			Task     string          `json:"task"`
			Async    *bool           `json:"async"`
			Workflow json.RawMessage `json:"workflow"`
			Tasks    []piChild       `json:"tasks"`
			Chain    []struct {
				piChild
				Parallel []piChild `json:"parallel"`
			} `json:"chain"`
			Args struct {
				Tasks []struct {
					Agent string `json:"agent"`
					Task  string `json:"task"`
				} `json:"tasks"`
			} `json:"args"`
		}
		if json.Unmarshal(args, &a) != nil || a.Action != "" || a.Async == nil || *a.Async {
			return nil
		}
		var names []string
		add := func(cs ...piChild) {
			for _, c := range cs {
				if c.Agent != "" && c.Task != "" {
					names = append(names, c.Agent)
				}
			}
		}
		add(piChild{a.Agent, a.Task})
		add(a.Tasks...)
		for _, st := range a.Chain {
			add(st.piChild)
			add(st.Parallel...)
		}
		// workflow:"review" is pi-subagents' named review resource, which runs the `reviewer` agent. workflow:"parallel"
		// is the shipped parallel resource: its `args.tasks` are the children it launches, together, so a foreground call
		// naming both judges IS both judges. Any other workflow (true, a script path, a name we do not know) runs code
		// foreman cannot read an agent out of, so its `args` prove nothing (a stated limit: review-skip).
		var wf string
		if json.Unmarshal(a.Workflow, &wf) == nil && wf == "review" {
			names = append(names, "reviewer")
		}
		if wf == "parallel" {
			for _, t := range a.Args.Tasks {
				if t.Agent != "" && t.Task != "" {
					names = append(names, t.Agent)
				}
			}
		}
		return names
	case "Agent", "Task":
		var a struct {
			SubagentType string `json:"subagent_type"`
		}
		if json.Unmarshal(args, &a) != nil {
			return nil
		}
		if a.SubagentType != "" {
			return []string{a.SubagentType}
		}
	case "agent":
		// pstack's own runner (plugins/pstack/pi/agent-tools.ts), how a pi executor launches pstack's review agent:
		// a FOREGROUND call waits for the child and answers with its result, while run_in_background only answers
		// "running" and proves the launch, not the run. The agent is Claude's `subagent_type`.
		var a struct {
			SubagentType    string `json:"subagent_type"`
			RunInBackground *bool  `json:"run_in_background"`
		}
		if json.Unmarshal(args, &a) != nil || a.SubagentType == "" || (a.RunInBackground != nil && *a.RunInBackground) {
			return nil
		}
		return []string{a.SubagentType}
	}
	return nil
}

// ProofFromPiFiles scans pi session jsonl for toolCall blocks whose own toolResult answered them without an error,
// from `since` on.
func ProofFromPiFiles(files []string, since time.Time) *TranscriptProof {
	p, _ := scanTranscripts(files, since, func() transcriptExtractor {
		return &piExtractor{pending: map[string][]string{}}
	})
	return p
}

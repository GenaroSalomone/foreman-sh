package invokers

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// TranscriptProof is what an executor's own Claude transcript (and its subagents') says it invoked: the names of its
// Skill tool_uses and the subagent_type of its Agent/Task tool_uses, from the moment the task's receipt line was written.
type TranscriptProof struct {
	Names map[string]bool
}

// Has is true when name ran: a namespaced `plugin:skill` must be equal to one of the names; a bare one must be the
// skill part of a namespaced name (a receipt written before this gate named bare skills). Never a substring:
// `pstack:thermo-nuclear-code-quality-review-subagent` is not `thermo-nuclear-code-quality-review`.
func (p *TranscriptProof) Has(name string) bool {
	if p == nil {
		return false
	}
	if strings.Contains(name, ":") {
		return p.Names[name] // # MUTATION-ANCHOR: 1150-G12
	}
	for n := range p.Names {
		if i := strings.LastIndex(n, ":"); i >= 0 && n[i+1:] == name {
			return true
		}
	}
	return p.Names[name]
}

// transcriptFiles is the executor's jsonl and its subagents', found by session id under every Claude config dir
// the account may have used (CLAUDE_CONFIG_DIR first, then ~/.claude*): a session id is unique, so the encoded
// cwd the path carries is not needed. Read-only.
func (c *Ctx) transcriptFiles(sid string) []string {
	if sid == "" || strings.ContainsAny(sid, "/*?[\\") {
		return nil
	}
	var roots []string
	if d := c.get("CLAUDE_CONFIG_DIR"); d != "" {
		roots = append(roots, d)
	}
	if h := c.get("HOME"); h != "" {
		m, _ := filepath.Glob(filepath.Join(h, ".claude*"))
		sort.Strings(m)
		roots = append(roots, m...)
	}
	for _, r := range roots {
		main, _ := filepath.Glob(filepath.Join(r, "projects", "*", sid+".jsonl"))
		if len(main) == 0 {
			continue
		}
		files := []string{main[0]}
		subs, _ := filepath.Glob(filepath.Join(strings.TrimSuffix(main[0], ".jsonl"), "subagents", "*.jsonl")) // # MUTATION-ANCHOR: 1150-G13
		sort.Strings(subs)
		return append(files, subs...)
	}
	return nil
}

// ReadTranscriptProof reads the transcripts of the session the receipt names. It returns nil and why when the
// transcript cannot be reached (another vendor, no session, no file): the caller says so, it never passes silently.
func (c *Ctx) ReadTranscriptProof(receipt string, since time.Time) (*TranscriptProof, string) {
	vendor, sid := receiptValue(receipt, "session_vendor"), receiptValue(receipt, "session_id")
	if vendor != "claude" {
		if vendor == "" {
			vendor = "no session_vendor in the receipt"
		}
		return nil, "the executor is not a Claude session (" + vendor + ")"
	}
	if sid == "" || strings.HasPrefix(sid, "none") {
		return nil, "the receipt has no session id"
	}
	files := c.transcriptFiles(sid)
	if len(files) == 0 {
		return nil, "no transcript file for session " + sid
	}
	return ProofFromFiles(files, since), ""
}

// ProofFromFiles scans jsonl transcripts for assistant tool_use blocks named Skill, Agent or Task.
func ProofFromFiles(files []string, since time.Time) *TranscriptProof {
	p := &TranscriptProof{Names: map[string]bool{}}
	for _, f := range files {
		fh, err := os.Open(f)
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(fh)
		sc.Buffer(make([]byte, 1<<20), 1<<28)
		for sc.Scan() {
			var line struct {
				Type      string `json:"type"`
				Timestamp string `json:"timestamp"`
				Message   struct {
					Content json.RawMessage `json:"content"`
				} `json:"message"`
			}
			if json.Unmarshal(sc.Bytes(), &line) != nil || line.Type != "assistant" {
				continue
			}
			if !since.IsZero() {
				if ts, err := time.Parse(time.RFC3339Nano, line.Timestamp); err == nil && ts.Before(since) { // # MUTATION-ANCHOR: 1150-G15
					continue
				}
			}
			var blocks []struct {
				Type  string `json:"type"`
				Name  string `json:"name"`
				Input struct {
					Skill        string `json:"skill"`
					SubagentType string `json:"subagent_type"`
				} `json:"input"`
			}
			if json.Unmarshal(line.Message.Content, &blocks) != nil {
				continue
			}
			for _, b := range blocks {
				if b.Type != "tool_use" {
					continue
				}
				switch b.Name {
				case "Skill":
					if b.Input.Skill != "" {
						p.Names[b.Input.Skill] = true
					}
				case "Agent", "Task":
					if b.Input.SubagentType != "" {
						p.Names[b.Input.SubagentType] = true
					}
				}
			}
		}
		fh.Close()
	}
	return p
}

// receiptAt is the `at` of the last receipt line with this key: when the task's closing was written. Zero when absent.
func receiptAt(path, key string) time.Time {
	b, err := os.ReadFile(path)
	if err != nil {
		return time.Time{}
	}
	var at string
	for _, l := range strings.Split(string(b), "\n") {
		var r struct {
			Key string `json:"key"`
			At  string `json:"at"`
		}
		if json.Unmarshal([]byte(l), &r) == nil && r.Key == key {
			at = r.At
		}
	}
	t, _ := time.Parse(time.RFC3339Nano, at)
	return t
}

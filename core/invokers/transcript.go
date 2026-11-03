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

// TranscriptProof is what an executor's own transcript (and its subagents') says it invoked: the names of its
// Skill tool_uses and the subagent_type of its Agent/Task tool_uses (claude), or the skills it read SKILL.md for and
// the agents it launched with the subagent/Agent tool (pi), from the moment the task's receipt line was written.
type TranscriptProof struct {
	Names  map[string]bool
	Vendor string // the vendor whose session was read; the refusal names that vendor's mechanism
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

// transcriptExtractor reads one line of one vendor's transcript and adds to p the names that line proves. A vendor
// whose call is answered on a later line keeps that state and drops what was never answered in flush.
type transcriptExtractor interface {
	line(raw []byte, p *TranscriptProof)
	flush(p *TranscriptProof)
}

// vendorTranscript is one vendor's two halves: where its session files live, and how one of its lines is read. What
// counts as a proof belongs to the reader, not here.
type vendorTranscript struct {
	files     func(*Ctx, string) []string
	extractor func() transcriptExtractor
}

var transcriptVendors = map[string]vendorTranscript{
	"claude": {files: (*Ctx).transcriptFiles, extractor: func() transcriptExtractor { return claudeExtractor{} }},
	"pi": {files: (*Ctx).piTranscriptFiles, extractor: func() transcriptExtractor {
		return &piExtractor{pending: map[string][]string{}}
	}},
}

// ReadTranscriptProof reads the transcripts of the session the receipt names. It returns nil and why when the
// transcript cannot be reached (a vendor with no reader, no session, no file, an unreadable one): the caller says
// so, it never passes silently.
func (c *Ctx) ReadTranscriptProof(receipt string, since time.Time) (*TranscriptProof, string) {
	vendor, sid := receiptValue(receipt, "session_vendor"), receiptValue(receipt, "session_id")
	v, ok := transcriptVendors[vendor]
	if !ok {
		if vendor == "" {
			return nil, "no transcript reader for this executor: the receipt has no session_vendor"
		}
		return nil, "no transcript reader for vendor " + vendor
	}
	if sid == "" || strings.HasPrefix(sid, "none") {
		return nil, "the receipt has no session id"
	}
	files := v.files(c, sid)
	if len(files) == 0 {
		return nil, "no transcript file for session " + sid
	}
	p, err := scanTranscripts(files, since, v.extractor)
	if err != nil {
		return nil, "the transcript of session " + sid + " could not be read: " + err.Error()
	}
	p.Vendor = vendor
	return p, ""
}

// scanTranscripts is the one loop both vendors share: open, buffer, the `since` cut, the reader's own line, and the
// scanner's own error, which is returned rather than read as an empty proof.
func scanTranscripts(files []string, since time.Time, newExtractor func() transcriptExtractor) (*TranscriptProof, error) {
	p := &TranscriptProof{Names: map[string]bool{}}
	for _, f := range files {
		fh, err := os.Open(f)
		if err != nil {
			return p, err
		}
		ex := newExtractor()
		sc := bufio.NewScanner(fh)
		sc.Buffer(make([]byte, 1<<20), 1<<28)
		for sc.Scan() {
			raw := sc.Bytes()
			if !since.IsZero() {
				var env struct {
					Timestamp string `json:"timestamp"`
				}
				if json.Unmarshal(raw, &env) == nil {
					if ts, err := time.Parse(time.RFC3339Nano, env.Timestamp); err == nil && ts.Before(since) { // # MUTATION-ANCHOR: 1150-G15
						continue
					}
				}
			}
			ex.line(raw, p)
		}
		err = sc.Err()
		fh.Close()
		if err != nil {
			return p, err
		}
		ex.flush(p)
	}
	return p, nil
}

// claudeExtractor reads a claude jsonl line: a `Skill` tool_use proves the skill it names, and an `Agent`/`Task`
// tool_use proves its `subagent_type`. A call is recorded on one line, so every assistant tool_use counts.
type claudeExtractor struct{}

func (claudeExtractor) flush(*TranscriptProof) {}

func (claudeExtractor) line(raw []byte, p *TranscriptProof) {
	var line struct {
		Type    string `json:"type"`
		Message struct {
			Content json.RawMessage `json:"content"`
		} `json:"message"`
	}
	if json.Unmarshal(raw, &line) != nil || line.Type != "assistant" {
		return
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
		return
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

// ProofFromFiles scans claude jsonl transcripts for assistant tool_use blocks named Skill, Agent or Task.
func ProofFromFiles(files []string, since time.Time) *TranscriptProof {
	p, _ := scanTranscripts(files, since, func() transcriptExtractor { return claudeExtractor{} })
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

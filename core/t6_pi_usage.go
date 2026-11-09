package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// THE opencode-go USAGE READ (pi-quota-stop-reports, 2026-11-09). The plan's cap is shared dollars for the whole key and
// a spent one ends a pi run at its first model call; the manifest says so BEFORE the dispatch. It WARNS and never blocks,
// and it fails open: a key that is not there, a refusal, a timeout or a body that is not what is expected print nothing
// that could stop a dispatch.
//
// MEASURED 2026-11-09 (one read, the real endpoint): GET https://opencode.ai/zen/go/v1/usage answers
// {"usage":{"rolling":{status,percent,resetsAt},"weekly":{…},"monthly":{…}}} — three windows, not the single
// `rolling.percent` the earlier note named; `monthly` is the cap that ends the month, and its status was "rate-limited"
// at percent 100 while rolling and weekly were "ok". Python's default User-Agent got a 403; curl's got 200, so hw sends its own.
const (
	piUsageURL      = "https://opencode.ai/zen/go/v1/usage"
	piUsageWarnAt   = 70
	piUsageTimeout  = 3 * time.Second
	piUsageMaxBytes = 64 << 10
)

type piUsageWindow struct {
	Status   string  `json:"status"`
	Percent  float64 `json:"percent"`
	ResetsAt string  `json:"resetsAt"`
}

// piUsageLine is the manifest text for a usage body: every window, and a WARNING when any is at piUsageWarnAt or more or
// is not "ok". ok=false: the body is not the shape measured, so nothing is said.
func piUsageLine(body []byte) (line string, ok bool) {
	var j struct {
		Usage map[string]piUsageWindow `json:"usage"`
	}
	if json.Unmarshal(body, &j) != nil || len(j.Usage) == 0 {
		return "", false
	}
	var parts, hot []string
	for _, name := range []string{"rolling", "weekly", "monthly"} {
		w, has := j.Usage[name]
		if !has {
			continue
		}
		p := fmt.Sprintf("%s %.0f%%", name, w.Percent)
		if w.Status != "" && w.Status != "ok" {
			p += " " + w.Status
		}
		if w.ResetsAt != "" {
			p += " (resets " + w.ResetsAt + ")"
		}
		parts = append(parts, p)
		if w.Percent >= piUsageWarnAt || (w.Status != "" && w.Status != "ok") {
			hot = append(hot, name)
		}
	}
	if len(parts) == 0 {
		return "", false
	}
	line = strings.Join(parts, ", ")
	if len(hot) > 0 {
		line += fmt.Sprintf(" — WARNING: %s at %d%% or more (or not ok); a spent plan ends the run at its first model call (provider_quota) and the brainer is told. Not blocked: the dispatch goes on.", strings.Join(hot, "+"), piUsageWarnAt)
	}
	return line, true
}

// piUsageManifest is the `pi usage` line, or "" when the dispatch does not run on opencode-go or no key can be read.
// The key comes from pi's own auth.json and never reaches argv or any output.
func (d *dsp) piUsageManifest() string {
	if d.model != "" && !strings.HasPrefix(d.model, "opencode-go/") {
		return ""
	}
	raw, err := os.ReadFile(filepath.Join(d.piAgentDir(), "auth.json"))
	if err != nil {
		return ""
	}
	var auth map[string]struct {
		Key string `json:"key"`
	}
	if json.Unmarshal(raw, &auth) != nil || auth["opencode-go"].Key == "" {
		return ""
	}
	url := orDefaultStr(d.env("HW_PI_USAGE_URL"), piUsageURL)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("Authorization", "Bearer "+auth["opencode-go"].Key)
	req.Header.Set("User-Agent", "hw-usage/1")
	resp, err := (&http.Client{Timeout: piUsageTimeout}).Do(req)
	if err != nil {
		return "unavailable (" + err.Error() + ") — not blocking"
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, piUsageMaxBytes))
	if resp.StatusCode != 200 {
		return fmt.Sprintf("unavailable (HTTP %d) — not blocking", resp.StatusCode)
	}
	if line, ok := piUsageLine(body); ok {
		return line
	}
	return "unavailable (unexpected body) — not blocking"
}

package invokers

import (
	"encoding/json"
	"os"
	"strings"

	"hwcore/internal/engramhealth"
)

// engramServeURL is engram_serve_url.
func (c *Ctx) engramServeURL() string {
	if v := c.get("HW_ENGRAM_URL"); v != "" {
		return v
	}
	return "http://127.0.0.1:" + c.or("ENGRAM_PORT", "7437")
}

// engramStoreDir is engram_store_dir: the store an `engram mcp` from this
// environment opens.
func (c *Ctx) engramStoreDir() string {
	d := c.get("ENGRAM_DATA_DIR")
	if strings.TrimSpace(d) != "" {
		return d
	}
	return c.get("HOME") + "/.engram"
}

// engramGet is `curl -sf` of the serve under the shared budget (HW_ENGRAM_HEALTH_TIMEOUT, default 10 s) and its one retry.
func (c *Ctx) engramGet(url string) engramhealth.Result {
	return engramhealth.Get(url, engramhealth.Budget(c.get("HW_ENGRAM_HEALTH_TIMEOUT")))
}

// engramVerdict is engram_serve_verdict: one line, and 0 this store, 2 another
// store's serve, 3 unverifiable.
func (c *Ctx) engramVerdict(url string) (string, int) {
	dir := c.engramStoreDir()
	want := ""
	if b, err := os.ReadFile(dir + "/.instance-id"); err == nil {
		want = strings.Join(strings.Fields(string(b)), "")
	}
	probe := c.engramGet(url + "/health")
	if !probe.OK {
		return engramhealth.Refusal(url, probe), 3
	}
	health := probe.Body
	got := ""
	var h map[string]any
	if json.Unmarshal([]byte(health), &h) == nil {
		if v, ok := h["instance_id"]; ok && v != nil && v != false {
			got = jqRawString(v)
		}
	}
	switch {
	case want == "":
		return "cannot prove the serve at " + url + " is this store: " + dir + "/.instance-id is missing", 3
	case got == "":
		return "cannot prove the serve at " + url + " is this store: its /health names no instance_id", 3
	case got != want:
		return "FOREIGN engram serve at " + url + ": instance " + got + ", but this store (" + dir + ") is instance " + want + " — a serve started under another HOME or ENGRAM_DATA_DIR holds the port", 2
	}
	return "engram serve at " + url + " is this store (" + dir + ", instance " + got + ")", 0
}

// engramObservationProject is engram_observation_project: the project of a live
// observation, or "".
func (c *Ctx) engramObservationProject(id, url string) string {
	body := c.engramGet(url + "/observations/" + id)
	if !body.OK {
		return ""
	}
	var o map[string]any
	if json.Unmarshal([]byte(body.Body), &o) != nil {
		return ""
	}
	if v, ok := o["project"]; ok && v != nil && v != false {
		return jqRawString(v)
	}
	return ""
}

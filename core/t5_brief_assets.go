package main

// Brief assets: a brief may name files kept under brain/<lane>/briefs/assets/ (a screenshot, a fixture). The brain
// guard blocks an executor from opening the brain path, so hw checks each named file exists BEFORE anything is built
// and, at launch, copies it into $HW_ARTIFACTS/brief-assets/ and tells the executor that path.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var briefAssetRef = regexp.MustCompile(`briefs/assets/([A-Za-z0-9][A-Za-z0-9._/-]*)`)

// briefAssetNames is every distinct file the brief text names under briefs/assets/, in order of appearance.
func briefAssetNames(text string) []string {
	var names []string
	seen := map[string]bool{}
	for _, m := range briefAssetRef.FindAllStringSubmatch(text, -1) {
		n := strings.TrimRight(m[1], "./-") // a sentence's full stop is not part of the name
		if n == "" || seen[n] {
			continue
		}
		seen[n] = true
		names = append(names, n)
	}
	return names
}

func (d *dsp) briefAssetsDir() string { return d.c.brain + "/" + d.proj + "/briefs/assets" }

// checkBriefAssets refuses a dispatch whose brief names an asset that does not exist; it records the names that do.
func (d *dsp) checkBriefAssets() {
	d.brAssets = nil
	if d.brief == "" {
		return
	}
	data, err := os.ReadFile(d.brief)
	if err != nil {
		return
	}
	root := d.briefAssetsDir()
	var missing []string
	for _, n := range briefAssetNames(string(data)) {
		st, err := os.Stat(filepath.Join(root, filepath.FromSlash(n)))
		if strings.Contains(n, "..") || err != nil || !st.Mode().IsRegular() { // # MUTATION-ANCHOR: 1200-R4-01
			missing = append(missing, n)
			continue
		}
		d.brAssets = append(d.brAssets, n)
	}
	if len(missing) > 0 {
		d.die("the brief names assets that are not files under " + root + ": " + strings.Join(missing, ", ") + ". NOTHING was built. Put each there (briefs/assets/<file>) or take the reference out of the brief.")
	}
}

// copyBriefAssets copies the named assets into $HW_ARTIFACTS/brief-assets/ and returns the note the executor reads.
func (l *lch) copyBriefAssets() string {
	if len(l.brAssets) == 0 {
		return ""
	}
	art := l.env("HW_ARTIFACTS")
	if art == "" {
		l.die("the brief names assets (" + strings.Join(l.brAssets, ", ") + ") and this run has no $HW_ARTIFACTS to copy them to.")
	}
	dest := art + "/brief-assets"
	for _, n := range l.brAssets {
		data, err := os.ReadFile(filepath.Join(l.briefAssetsDir(), filepath.FromSlash(n)))
		if err != nil {
			l.die("cannot read the brief asset " + n + ": " + err.Error())
		}
		to := filepath.Join(dest, filepath.FromSlash(n))
		l.fs.mkdirAll(filepath.Dir(to))
		if err := l.fs.write(to, data, 0o644); err != nil { // # MUTATION-ANCHOR: 1200-R4-02
			l.die("cannot copy the brief asset " + n + " to " + to + ": " + err.Error())
		}
	}
	l.info("brief assets copied to " + dest + ": " + strings.Join(l.brAssets, ", "))
	return "BRIEF ASSETS — hw copied the files this brief names under briefs/assets/ to " + dest + ": " + strings.Join(l.brAssets, ", ") + ".\nOpen them there; the brain path they came from is blocked for you."
}

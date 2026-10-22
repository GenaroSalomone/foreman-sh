package main

import (
	"os"
	"strings"
)

// claim is the python `claim` of the bash: take every well-formed <id>.req of the outbox into the state's work directory by rename
// — which moves a link rather than following it — keep only the plain files, drop what is not a safe id, and return the ids.
func (o *sbOutbox) claim(w string) []string {
	var ids []string
	_ = o.at(func(dir string, name func(string) string) error {
		ents, err := os.ReadDir(dir)
		if err != nil {
			return err
		}
		for _, e := range ents {
			n := e.Name()
			if !strings.HasSuffix(n, ".req") {
				continue
			}
			rid := strings.TrimSuffix(n, ".req")
			if !sbRidRe.MatchString(rid) { // MUTATION-ANCHOR: s8-rid
				sbUnlink(name(n))
				continue
			}
			dst := w + "/" + rid + ".claimed"
			if sbRename(name(n), dst) != nil {
				continue
			}
			fi, err := os.Lstat(dst)
			if err != nil {
				continue
			}
			if !fi.Mode().IsRegular() { // MUTATION-ANCHOR: s8-claim-plain
				if fi.IsDir() {
					os.Remove(dst)
				} else {
					sbUnlink(dst)
				}
				continue
			}
			ids = append(ids, rid)
		}
		return nil
	})
	return ids
}

// deliver is _deliver: replace the ENTRY <name> of the outbox with src — rename(2) replaces a planted link or refuses a planted
// directory, and never writes through either — or drop src.
func (o *sbOutbox) deliver(src, name string) {
	err := o.at(func(dir string, n func(string) string) error { return sbRename(src, n(name)) }) // MUTATION-ANCHOR: s8-deliver
	if err != nil {
		os.Remove(src)
	}
}

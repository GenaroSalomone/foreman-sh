package main

// brain-archive in Go (bin/brain-archive): an encrypted, verified copy of brain on a DIFFERENT device. It refuses a destination
// on brain's own device (the id `df` names, not a path that looks different), bundles every ref, tars the working tree without
// .git, encrypts both with a symmetric gpg passphrase read from the terminal or stdin (never argv), decrypts the bundle back
// from the destination to prove it, and writes MANIFEST.txt. The programs it runs are the bash's: git, df, du, tar, zstd, gpg, shasum.

import (
	"bufio"
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// baDevOf is `df "$1" 2>/dev/null | awk 'NR==2 {print $1}'`; pipefail makes a failed df the status of the substitution.
func baDevOf(path string) (string, int) {
	out, err := exec.Command("df", path).Output()
	if err != nil {
		return "", exitCodeOf(err)
	}
	lines := strings.Split(string(out), "\n")
	if len(lines) < 2 {
		return "", 0
	}
	if f := strings.Fields(lines[1]); len(f) > 0 {
		return f[0], 0
	}
	return "", 0
}

// baSize is `$(du -h "$f" | cut -f1)`.
func baSize(path string) string {
	out, _ := exec.Command("du", "-h", path).Output()
	s := trimNL(string(out))
	if i := strings.Index(s, "\t"); i >= 0 {
		s = s[:i]
	}
	return s
}

func baRun(w *wenv, args []string) int {
	const prog = "brain-archive"
	die := func(m string) int { w.errf("%s: %s\n", prog, m); return 1 }
	info := func(m string) { w.outf("  · %s\n", m) }
	ok := func(m string) { w.outf("  ✓ %s\n", m) }

	brain := w.getenv("BRAIN")
	if brain == "" {
		brain = filepath.Dir(w.selfDir())
	}
	if !bcIsDir(brain + "/.git") {
		return die("no git repository at " + brain)
	}

	dest := ""
	if len(args) > 0 {
		dest = args[0]
	}
	if dest == "" {
		return die("usage: " + prog + " <destination-directory>\n" +
			"       The destination must be on a different device than " + brain + ".\n" +
			"       Plug in an external disk, or mount one, and pass its path.\n" +
			"       There is no default: a default would end up on this disk.")
	}
	if !bcIsDir(dest) {
		return die("destination does not exist: " + dest)
	}
	if !accessOK(dest, 2) {
		return die("destination is not writable: " + dest)
	}

	// ── THE CHECK THAT MAKES THIS WORTH RUNNING ──
	brainDev, rc := baDevOf(brain)
	if rc != 0 {
		return rc
	}
	destDev, rc := baDevOf(dest)
	if rc != 0 {
		return rc
	}
	if brainDev == "" || destDev == "" {
		return die("could not read the device for " + brain + " or " + dest)
	}
	if brainDev == destDev { // MUTATION-ANCHOR: s6-ba-samedev
		return die(dest + " is on " + destDev + ", the same device as " + brain + ".\n\n" +
			"  That is what the brain's own backup bare repo already is. A second copy on the\n" +
			"  disk you are protecting against does not protect against it.\n\n" +
			"  Use an external disk, or a mounted volume that is not " + brainDev + ".")
	}
	ok("destination is on " + destDev + "; brain is on " + brainDev)

	if _, err := exec.LookPath("gpg"); err != nil {
		return die("gpg not installed — brew install gnupg")
	}
	if _, err := exec.LookPath("zstd"); err != nil {
		return die("zstd not installed — brew install zstd")
	}

	stamp := time.Now().UTC().Format("20060102T150405Z")
	out := dest + "/brain-" + stamp
	if err := os.MkdirAll(out, 0o777); err != nil {
		return die("cannot create " + out)
	}
	tmpRoot := w.getenv("TMPDIR")
	if tmpRoot == "" {
		tmpRoot = "/tmp"
	}
	tmp, err := os.MkdirTemp(tmpRoot, "brain-archive.")
	if err != nil {
		w.errf("mktemp: %v\n", err)
		return 1
	}
	defer os.RemoveAll(tmp)

	git := func(quiet bool, a ...string) (string, int) {
		cmd := exec.Command("git", append([]string{"-C", brain}, a...)...)
		var o bytes.Buffer
		cmd.Stdout = &o
		if !quiet {
			cmd.Stderr = w.stderr
		}
		if err := cmd.Run(); err != nil {
			return o.String(), exitCodeOf(err)
		}
		return o.String(), 0
	}

	head, rc := git(false, "rev-parse", "HEAD")
	if rc != 0 {
		return rc
	}
	headSHA := trimNL(head)
	info("HEAD " + headSHA)

	// ── history ── a bundle, not a tar of .git: a single, self-verifying file, not racy against the agents writing here
	if _, rc := git(true, "bundle", "create", tmp+"/brain.bundle", "--all"); rc != 0 {
		return die("git bundle failed")
	}
	if _, rc := git(true, "bundle", "verify", tmp+"/brain.bundle"); rc != 0 {
		return die("git bundle verify failed on the bundle just written — do not trust it")
	}
	ok("bundle written and verified (" + baSize(tmp+"/brain.bundle") + ")")

	// THE BUNDLE MUST CONTAIN THE HEAD WE READ.
	heads, rc := git(false, "bundle", "list-heads", tmp+"/brain.bundle")
	if rc != 0 {
		return rc
	}
	os.WriteFile(tmp+"/heads.txt", []byte(heads), 0o666)
	hasHead := false
	for _, l := range strings.Split(heads, "\n") {
		if strings.HasPrefix(l, headSHA) {
			hasHead = true
		}
	}
	if !hasHead {
		return die("the bundle does not contain HEAD (" + headSHA + ") — refusing to call this a backup")
	}
	ok("bundle contains HEAD and " + strconv.Itoa(strings.Count(heads, "\n")) + " ref(s)")

	// ── working tree, without .git ──
	if !baTarZstd(w, brain, tmp+"/worktree.tar.zst") {
		return die("tar/zstd of the working tree failed")
	}
	if err := (&exec.Cmd{Path: lookOr("zstd"), Args: []string{"zstd", "-t", "-q", tmp + "/worktree.tar.zst"}, Stdout: w.stdout, Stderr: w.stderr}).Run(); err != nil {
		return die("the compressed working tree does not decompress")
	}
	ok("working tree archived and decompression-tested (" + baSize(tmp+"/worktree.tar.zst") + ")")

	// ── encrypt ── NEVER AN ARGUMENT: a terminal prompts, otherwise the first line of stdin is the passphrase
	passFile := tmp + "/pass"
	os.WriteFile(passFile, nil, 0o600)
	var gpgArgs []string
	if w.stdinFile != nil && isTerminal(w.stdinFile) {
		info("gpg will ask for a passphrase once, and use it for both files")
	} else {
		info("reading the passphrase from stdin (no terminal)")
		line, _ := bufio.NewReader(w.stdin).ReadBytes('\n')
		os.WriteFile(passFile, line, 0o600)
		if len(line) == 0 {
			return die("stdin gave an empty passphrase")
		}
		gpgArgs = []string{"--batch", "--yes", "--pinentry-mode", "loopback", "--passphrase-file", passFile}
	}
	gpg := func(quiet bool, a ...string) error {
		cmd := exec.Command("gpg", a...)
		if w.stdinFile != nil {
			cmd.Stdin = w.stdinFile
		} else {
			cmd.Stdin = w.stdin
		}
		cmd.Stdout = w.stdout
		if !quiet {
			cmd.Stderr = w.stderr
		}
		return cmd.Run()
	}
	enc := func(src, dst string) []string {
		a := append([]string{"--symmetric", "--cipher-algo", "AES256"}, gpgArgs...)
		return append(a, "--output", dst, src)
	}
	if err := gpg(false, enc(tmp+"/brain.bundle", out+"/brain.bundle.gpg")...); err != nil {
		return die("gpg failed on the bundle")
	}
	if err := gpg(false, enc(tmp+"/worktree.tar.zst", out+"/worktree.tar.zst.gpg")...); err != nil {
		return die("gpg failed on the working tree archive")
	}
	ok("both files encrypted with AES256; the same passphrase restores both")

	// ── prove the written files are readable ── decrypt the bundle back out of the DESTINATION and verify it there
	dec := append([]string{"--quiet", "--decrypt"}, gpgArgs...)
	dec = append(dec, "--output", tmp+"/roundtrip.bundle", out+"/brain.bundle.gpg")
	if err := gpg(true, dec...); err != nil {
		return die("the written bundle does not decrypt — the archive is not usable")
	}
	if _, rc := git(true, "bundle", "verify", tmp+"/roundtrip.bundle"); rc != 0 {
		return die("the decrypted bundle does not verify — the archive is not usable")
	}
	ok("round-trip: the bundle on " + destDev + " decrypts and verifies")

	var m strings.Builder
	m.WriteString("brain archive\n")
	m.WriteString("created      " + stamp + "\n")
	m.WriteString("source       " + brain + " (" + brainDev + ")\n")
	m.WriteString("destination  " + out + " (" + destDev + ")\n")
	m.WriteString("HEAD         " + headSHA + "\n")
	m.WriteString("\nrefs\n")
	m.WriteString(baIndent(heads))
	m.WriteString("\nsha256\n")
	sum := exec.Command("shasum", "-a", "256", "brain.bundle.gpg", "worktree.tar.zst.gpg")
	sum.Dir = out
	sum.Stderr = w.stderr
	sumOut, err := sum.Output()
	if err != nil {
		return exitCodeOf(err)
	}
	m.WriteString(baIndent(string(sumOut)))
	m.WriteString("\nrestore\n")
	m.WriteString("  gpg -d brain.bundle.gpg > brain.bundle && git clone brain.bundle brain\n")
	m.WriteString("  gpg -d worktree.tar.zst.gpg | zstd -d | tar -xf -    # over the clone\n")
	os.WriteFile(out+"/MANIFEST.txt", []byte(m.String()), 0o666)

	w.outf("\n")
	ok(out)
	w.outf("%s", baIndent(m.String()))
	return 0
}

// baIndent is `sed 's/^/  /'`.
func baIndent(s string) string {
	if s == "" {
		return ""
	}
	nl := strings.HasSuffix(s, "\n")
	lines := strings.Split(strings.TrimSuffix(s, "\n"), "\n")
	for i, l := range lines {
		lines[i] = "  " + l
	}
	r := strings.Join(lines, "\n")
	if nl {
		r += "\n"
	}
	return r
}

// baTarZstd is `tar --exclude='./.git' -cf - -C "$brain" . 2>/dev/null | zstd -q -T0 -19 > "$dst"`, pipefail.
func baTarZstd(w *wenv, brain, dst string) bool {
	f, err := os.Create(dst)
	if err != nil {
		return false
	}
	defer f.Close()
	tar := exec.Command("tar", "--exclude=./.git", "-cf", "-", "-C", brain, ".")
	zs := exec.Command("zstd", "-q", "-T0", "-19")
	pr, pw := io.Pipe()
	tar.Stdout, zs.Stdin = pw, pr
	zs.Stdout, zs.Stderr = f, w.stderr
	if tar.Start() != nil {
		return false
	}
	if zs.Start() != nil {
		tar.Process.Kill()
		tar.Wait()
		return false
	}
	terr := tar.Wait()
	pw.Close()
	zerr := zs.Wait()
	return terr == nil && zerr == nil
}

func lookOr(name string) string {
	if p, err := exec.LookPath(name); err == nil {
		return p
	}
	return name
}

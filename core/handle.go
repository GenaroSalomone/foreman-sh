package main

// `hw handle <name>...` (cmd_handle, bin/hw:553): present or absent, for each
// name, and nothing else. It asks the Keychain for the item's attributes and
// never for its value.

import (
	"os"
	"os/exec"
	"os/user"
	"runtime"
	"strings"
)

func (a *app) keychainAvailable() bool {
	if a.getenv("HW_NO_KEYCHAIN") == "1" { // # MUTATION-ANCHOR: 816-G29
		return false
	}
	_, err := exec.LookPath("security")
	return err == nil
}

func unameS() string {
	if out, err := exec.Command("uname", "-s").Output(); err == nil {
		return strings.TrimRight(string(out), "\n")
	}
	switch runtime.GOOS {
	case "darwin":
		return "Darwin"
	case "linux":
		return "Linux"
	case "windows":
		return "Windows_NT"
	}
	return runtime.GOOS
}

func handleExists(h string) bool {
	cmd := exec.Command("security", "find-generic-password", "-s", h)
	cmd.Env = childEnv()
	cmd.Stdout, cmd.Stderr = nil, nil
	return cmd.Run() == nil
}

func whoami() string {
	if u := os.Getenv("USER"); u != "" {
		return u
	}
	if out, err := exec.Command("id", "-un").Output(); err == nil {
		return strings.TrimRight(string(out), "\n")
	}
	if u, err := user.Current(); err == nil {
		return u.Username
	}
	return ""
}

func (a *app) cmdHandle() int {
	if len(a.args) < 1 {
		a.die("usage: hw handle <keychain-handle> [<handle>...]")
	}
	if !a.keychainAvailable() {
		a.warn("no keychain backend on " + unameS() + ": hw cannot tell whether a handle exists")
		a.warn("the macOS Keychain is the only store hw checks. Here, keep a credential in a file or an env var")
		a.warn("only its consumer can read (chmod 600, outside every repo) and name that in the brief instead of a handle")
		return 2 // # MUTATION-ANCHOR: 816-G12
	}
	a.step("keychain handles")
	rc := 0
	for _, h := range a.args {
		if handleExists(h) {
			a.ok(h + " — present")
		} else {
			a.warn(h + " — ABSENT (nothing in the keychain has that service name)")
			rc = 1 // # MUTATION-ANCHOR: 816-G11
		}
	}
	if rc != 0 {
		a.printf("\n  %sstore one with:%s security add-generic-password -s <handle> -a %s -w '<value>'\n", a.cDim, a.c0, whoami())
		a.printf("  %sonly presence is checked here — hw never reads a value, and neither should a brainer.%s\n\n", a.cDim, a.c0)
	} else {
		a.printf("\n  %sonly presence was checked — no value was read, printed or logged.%s\n\n", a.cDim, a.c0)
	}
	return rc
}

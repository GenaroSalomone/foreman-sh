package invokers

import (
	"os"
	"os/exec"
)

// closerScript is INVOKER_CLOSER_SH: run `hw <args>` after the invoker has exited.
const closerScript = `
pid="$1"; state="$2"; hw="$3"; shift 3
export HW_DONE_INVOKER_PID="$pid"
n=0
while kill -0 "$pid" 2>/dev/null && [ "$n" -lt 300 ]; do sleep 0.1; n=$((n + 1)); done  # 899-M03
sleep "${HW_CLOSE_GRACE_SECONDS:-1}"
[ -d "$state" ] || exit 0
rm -f "$state/close-failed"
rc=0
"$hw" "$@" || rc=$?
if [ "$rc" != 0 ]; then  # 899-M05
  printf "rc=%s\nat=%s\nlog=%s\nthe report was delivered; hw %s exited %s and the pane may still be open\n" \
    "$rc" "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$state/close.log" "$*" "$rc" > "$state/close-failed"
  herdr notification show "executor close FAILED (exit $rc)" --body "hw $* — see $state/close.log" --sound request >/dev/null 2>&1 || true
fi
printf "%s\n" "$rc" > "$state/close-rc"
`

// closeAfterExit is invoker_close_after_exit: start `hw <args>` in a process of its
// own that outlives this one. False when it could not be started, so the caller
// closes in the foreground.
func (c *Ctx) closeAfterExit(state string, args ...string) bool {
	if st, err := os.Stat(state); err != nil || !st.IsDir() {
		return false
	}
	logf, err := os.OpenFile(state+"/close.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o666)
	if err != nil {
		return false
	}
	defer logf.Close()
	bash, ok := c.lookPath("bash")
	if !ok {
		return false
	}
	argv := append([]string{"-c", closerScript, "_", itoa(c.Pid), state, c.BinDir + "/hw"}, args...)
	cmd := exec.Command(bash, argv...)
	cmd.Dir = c.Cwd
	cmd.Env = c.environ()
	cmd.Stdin = nil
	cmd.Stdout, cmd.Stderr = logf, logf
	cmd.SysProcAttr = detachedAttr() // 899-M04
	if err := cmd.Start(); err != nil {
		return false
	}
	go cmd.Wait() // never waited for by the invoker's exit; reaps in tests
	return true
}

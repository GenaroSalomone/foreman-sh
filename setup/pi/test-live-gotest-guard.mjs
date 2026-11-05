// One real pi session, over RPC, asking for a raw `go test` in core/: the executor guard must refuse it and name
// setup/go-test-core.sh and the background route. This is the LIVE half of tests/1133's go-test arm (retro-fixes-rc3
// item 5); it is not run by the gate, which keeps its arms hermetic.
//
//   env -i HOME=… PATH=… ENGRAM_PORT=<private> HW_TASK=probe HW_RUN=probe HW_WORKDIR=<tmp> \
//     HW_PROJECT=setup HW_BRAIN_ROOT=<stub brain whose setup/ is the tree under test> \
//     node --experimental-strip-types test-live-gotest-guard.mjs <adapter.ts>
//
// Prints `ok - …` / `not ok - …` and exits 1 if any check failed. LIVE_PROBE_TRACE=1 prints one line per event.
import { spawn } from "node:child_process";

const adapter = process.argv[2];
if (!adapter) { console.error("usage: test-live-gotest-guard.mjs <adapter.ts>"); process.exit(2); }

const trace = process.env.LIVE_PROBE_TRACE === "1";
const results = [];
const ok = (c, m) => { results.push(c); console.log(`${c ? "ok" : "not ok"} - ${m}`); };
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

const PROMPT = [
  "Call the bash tool exactly once, with command:",
  "go test ./core/ -run '^TestNothingMatchesAnything$'",
  "Pass only that command; do not run any other tool, do not run it in the background.",
  "Then reply with the exact text of the tool's answer and finish your turn.",
].join(" ");

const child = spawn(process.env.PI_BIN || "pi", [
  "--mode", "rpc", "--no-session", "-ne", "-na", "-ns", "-nc", "-e", adapter,
  "--model", "opencode-go/deepseek-v4.1-flash", "--thinking", "off",
], { env: process.env, stdio: ["pipe", "pipe", "pipe"] });

let buffer = "", stderr = "";
const records = [];
child.stderr.on("data", (d) => { stderr += String(d); });
child.stdout.on("data", (d) => {
  buffer += String(d);
  let i;
  while ((i = buffer.indexOf("\n")) >= 0) {
    const line = buffer.slice(0, i).replace(/\r$/, "");
    buffer = buffer.slice(i + 1);
    if (!line.trim()) continue;
    try {
      const rec = JSON.parse(line);
      records.push(rec);
      if (trace) console.log(`[event] ${String(rec.type).padEnd(24)}${rec.toolName ? " tool=" + rec.toolName : ""}`);
    } catch { /* not a protocol record */ }
  }
});

child.stdin.write(JSON.stringify({ id: "p1", type: "prompt", message: PROMPT }) + "\n");

const deadline = Date.now() + 240_000;
async function until(fn, what) {
  while (Date.now() < deadline) {
    const hit = records.find(fn);
    if (hit) return hit;
    await sleep(250);
  }
  throw new Error(`timed out waiting for ${what}; stderr: ${stderr.slice(-500)}`);
}

try {
  const started = await until((r) => r.type === "tool_execution_start" && r.toolName === "bash", "the bash call");
  const ended = await until((r) => r.type === "tool_execution_end" && r.toolCallId === started.toolCallId, "the tool result");
  const text = JSON.stringify(ended.result ?? ended);
  console.log(`[tool result] ${text.slice(0, 600)}`);
  ok(/BLOCKED|Blocked/.test(text), "the raw `go test ./core/` call is refused by the guard (not by go)");
  ok(/setup\/go-test-core\.sh/.test(text), "the refusal names setup/go-test-core.sh (the route that does run it)");
  ok(/bash_background|run_in_background/.test(text), "the refusal names the background route");
  await until((r) => r.type === "agent_settled", "the run to settle");
} catch (e) {
  ok(false, String(e?.message ?? e));
} finally {
  child.stdin.end();
  child.kill();
}
process.exit(results.every(Boolean) ? 0 : 1);

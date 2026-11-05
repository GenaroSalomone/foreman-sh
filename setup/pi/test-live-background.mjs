// One real pi session, over RPC, proving the background shell's wake: the model starts a command with
// `bash_background`, ends its turn, and a NEW run starts when the command exits — carrying the exit code and
// the log path. RPC mode because `pi -p` ends when the run settles, and the wake would land with no process
// left to receive it (the same reason a pane, not a print run, is where an executor waits).
//
//   env -i HOME=… PATH=… ENGRAM_PORT=<private> HW_TASK=probe HW_RUN=probe HW_WORKDIR=<tmp> 
//     HW_PROJECT=setup HW_BRAIN_ROOT=<stub> node --experimental-strip-types test-live-background.mjs <adapter.ts>
//
// Prints `ok - …` / `not ok - …` lines and exits 1 if any check failed. LIVE_PROBE_TRACE=1 prints one line per
// session event, which is the transcript the commit quotes.
import { spawn } from "node:child_process";
import { existsSync, readFileSync } from "node:fs";

const adapter = process.argv[2];
if (!adapter) { console.error("usage: test-live-background.mjs <adapter.ts>"); process.exit(2); }

const trace = process.env.LIVE_PROBE_TRACE === "1";
const results = [];
const ok = (c, m) => { results.push(c); console.log(`${c ? "ok" : "not ok"} - ${m}`); };
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

const PROMPT = [
  "Call the bash_background tool exactly once, with command: sleep 25; echo LIVE-PROBE-DONE",
  "Pass only that command. Then reply with the single word STARTED and finish your turn immediately: do not wait",
  "for the command, do not call bash to check on it, and do not call any other tool. When its exit notice arrives",
  "later, reply with the exit code it names and nothing else.",
].join(" ");

const child = spawn(process.env.PI_BIN || "pi", [
  "--mode", "rpc", "--no-session", "-ne", "-na", "-ns", "-nc", "-e", adapter,
  "--model", "opencode-go/deepseek-v4.1-flash", "--thinking", "off",
], { env: process.env, stdio: ["pipe", "pipe", "pipe"] });

let buffer = "", stderr = "";
const records = []; // { at, rec }
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
      records.push({ at: Date.now(), rec });
      if (trace) {
        const what = rec.toolName ? ` tool=${rec.toolName}` : "";
        console.log(`[event] ${String(rec.type).padEnd(20)}${what}`);
      }
    } catch { /* not a protocol record */ }
  }
});

child.stdin.write(JSON.stringify({ id: "p1", type: "prompt", message: PROMPT }) + "\n");

const deadline = Date.now() + 200_000;
const has = (fn) => records.some(({ rec }) => fn(rec));
// The predicate sees the record WRAPPER ({ at, rec }), because "after the settle" is a fact about arrival time.
async function until(fn, what) {
  while (Date.now() < deadline) {
    const hit = records.find(fn);
    if (hit) return hit;
    await sleep(250);
  }
  throw new Error(`timed out waiting for ${what}`);
}

try {
  // 1. the tool ran, and it was pi's own tool, not a shell call
  const started = await until((x) => x.rec.type === "tool_execution_start" && x.rec.toolName === "bash_background", "bash_background to run");
  ok(!!started, "a real pi session calls bash_background (the tool exists in the model's own tool list)");
  const ended = await until((x) => x.rec.type === "tool_execution_end" && x.rec.toolCallId === started.rec.toolCallId, "the tool result");
  const resultText = JSON.stringify(ended.rec.result?.content ?? []);
  ok(ended.rec.isError !== true && /Started /.test(resultText) && /log:/.test(resultText),
     "the tool returns at once with the job and its log path (nothing waits inside the turn)");

  // 2. the turn settled with the command still running (that is the state the wake has to end)
  const settled = await until((x) => x.rec.type === "agent_settled", "the first run to settle");

  // 3. the wake: a new run, started by the exit notice, with the exit code and the log
  const wake = await until((x) => JSON.stringify(x.rec).includes("exited with code"), "the background exit notice");
  const wakeText = JSON.stringify(wake.rec);
  ok(wake.at > settled.at, "the exit notice arrived AFTER the turn ended (the session was woken, not polled)");
  ok(/exited with code 0/.test(wakeText), "the exit notice names the command's exit code");
  const logPath = /(\/[^"\\\s]+\.log)/.exec(wakeText)?.[1] ?? "";
  ok(!!logPath && existsSync(logPath), `the exit notice names a log that is on disk: ${logPath}`);
  ok(!!logPath && existsSync(logPath) && /LIVE-PROBE-DONE/.test(readFileSync(logPath, "utf8")),
     "the log holds what the command wrote");

  // pi emits agent_start BEFORE the custom message's own message event, so the run the wake starts is the first
  // agent_start after the settle — the notice lands inside it, which is what "the session was woken" means.
  const run2 = await until((x) => x.rec.type === "agent_start" && x.at >= settled.at, "the run the wake started");
  ok(!!run2, "the exit notice STARTS a new run in the session (an agent_start after the settle)");

  const answer = await until((x) => x.rec.type === "message_end" && x.rec.message?.role === "assistant" && x.at > run2.at, "the model's answer to the wake");
  ok(!!answer, "the woken run produces an answer (the model acted on the wake, not on a poll)");
} catch (e) {
  ok(false, String(e?.message ?? e));
  if (stderr) console.log(`[stderr tail]\n${stderr.split("\n").slice(-20).join("\n")}`);
} finally {
  try { child.stdin.end(); } catch { /* already closed */ }
  try { child.kill("SIGTERM"); } catch { /* already gone */ }
}

process.exit(results.every(Boolean) ? 0 : 1);

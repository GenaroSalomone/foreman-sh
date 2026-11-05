// One real pi session, over RPC, against a LOCAL scripted provider (setup/pi/stub-provider.mjs: no quota, no network):
// the "model" runs a foreground `bash` call that sets NO timeout and takes 20 s. With the adapter's default shortened to 3 s (HW_PI_BASH_DEFAULT_TIMEOUT_S=3), the call must end at about 3 s and
// the result must carry hw's note naming bash_background; the adapter before this change lets it run the 20 s
// (LIVE-DONE in the result, no note). setup/pi/test-live-background.mjs is the same harness for the background shell.
//
//   env -i HOME=… PATH=… ENGRAM_PORT=<private> HW_TASK=probe HW_RUN=probe HW_WORKDIR=<tmp> HW_PROJECT=setup
//     HW_BRAIN_ROOT=<brain> HW_PI_BASH_DEFAULT_TIMEOUT_S=3 node --experimental-strip-types test-live-bash-timeout.mjs <adapter.ts>
//
// Prints `ok - …` / `not ok - …` lines and exits 1 if any check failed.
import { spawn } from "node:child_process";
import { startStubProvider } from "./stub-provider.mjs";

const adapter = process.argv[2];
if (!adapter) { console.error("usage: test-live-bash-timeout.mjs <adapter.ts>"); process.exit(2); }

const results = [];
const ok = (c, m) => { results.push(c); console.log(`${c ? "ok" : "not ok"} - ${m}`); };
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

const stub = await startStubProvider({ command: "sleep 20; echo LIVE-DONE", final: "FINISHED" });
const PROMPT = "run the command";

const child = spawn(process.env.PI_BIN || "pi", [
  "--mode", "rpc", "--no-session", "-ne", "-na", "-ns", "-nc", "-e", adapter,
  "--model", "stub/stub", "--thinking", "off",
], { env: { ...process.env, PI_CODING_AGENT_DIR: stub.agentDir }, stdio: ["pipe", "pipe", "pipe"] });

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
    try { records.push({ at: Date.now(), rec: JSON.parse(line) }); } catch { /* not a protocol record */ }
  }
});
child.stdin.write(JSON.stringify({ id: "p1", type: "prompt", message: PROMPT }) + "\n");

const deadline = Date.now() + 150_000;
async function until(fn, what) {
  while (Date.now() < deadline) {
    const hit = records.find(fn);
    if (hit) return hit;
    await sleep(250);
  }
  throw new Error(`timed out waiting for ${what}`);
}

try {
  const started = await until((x) => x.rec.type === "tool_execution_start" && x.rec.toolName === "bash", "the bash call to start");
  ok(!!started, "a real pi session runs a foreground bash call");
  const ended = await until((x) => x.rec.type === "tool_execution_end" && x.rec.toolCallId === started.rec.toolCallId, "the bash result");
  const secs = (ended.at - started.at) / 1000;
  const text = JSON.stringify(ended.rec.result?.content ?? []);
  ok(!/LIVE-DONE/.test(text) && secs < 15, `the call with no timeout was ended by the bound, not left to run its 20 s (${secs.toFixed(1)} s)`);
  ok(/Command timed out after 3 seconds/.test(text), "the result says the command timed out after the bounded 3 s");
  ok(/bounded it at 3 s/.test(text) && /bash_background/.test(text), "the result carries hw's note naming bash_background");
} catch (e) {
  ok(false, String(e?.message ?? e));
  if (stderr) console.log(`[stderr tail]\n${stderr.split("\n").slice(-20).join("\n")}`);
} finally {
  try { child.stdin.end(); } catch { /* already closed */ }
  try { child.kill("SIGTERM"); } catch { /* already gone */ }
  await stub.close();
}

process.exit(results.every(Boolean) ? 0 : 1);

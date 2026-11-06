// One real pi session, over RPC, proving that an ASYNC subagent wakes its parent when it ends — with
// `@earendil-works/chord` ABSENT from the host pi package (pi 1.2+ does not ship it, and `pi update` removes a hand-installed one).
//
//   node --experimental-strip-types test-live-subagent-wake.mjs --pi <pi cli.js> --subagents <pi-subagents dir> [--adapter <hw-executor.ts>]
//     [--mid-turn] [--expect woke|none]
//
// The model is a local scripted provider (setup/pi/stub-provider.mjs): the parent calls `subagent` with async:true and ends its
// turn; the child answers CHILD-OK after a delay. Prints `ok - …` / `not ok - …` and exits 1 on a failed check. The final line
// `WAKES <n>` is the number of runs the child's completion started in the parent (the criterion is exactly one).
//   --mid-turn   the parent starts a long foreground command after launching the child, so the child ends DURING a turn.
//   --compact    the parent COMPACTS (RPC `compact`, summarised by the local provider, held for 9 s) while the child runs
//                (it ends at 5 s), so the child's completion arrives during the compaction.
import { spawn } from "node:child_process";
import { mkdirSync, writeFileSync, mkdtempSync, readFileSync } from "node:fs";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { startStubProvider } from "./stub-provider.mjs";

const arg = (name) => { const i = process.argv.indexOf(name); return i > 0 ? process.argv[i + 1] : undefined; };
const piCli = arg("--pi"), subDir = arg("--subagents"), adapter = arg("--adapter");
const midTurn = process.argv.includes("--mid-turn");
const compact = process.argv.includes("--compact");
const expect = arg("--expect") || "woke";
if (!piCli || !subDir) { console.error("usage: test-live-subagent-wake.mjs --pi <cli.js> --subagents <dir> [--adapter <ts>] [--mid-turn] [--expect woke|none]"); process.exit(2); }

const results = [];
const ok = (c, m) => { results.push(c); console.log(`${c ? "ok" : "not ok"} - ${m}`); };
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
const trace = process.env.LIVE_PROBE_TRACE === "1";

const work = mkdtempSync(join(tmpdir(), "subagent-wake-"));
const agents = join(work, "agents"); mkdirSync(agents, { recursive: true });
writeFileSync(join(agents, "probe-child.md"), "---\nname: probe-child\ndescription: answers with a fixed word after a delay\nmodel: stub/stub\ntools: read\n---\nAnswer exactly what the task says.\n");

const CHILD_DELAY_MS = compact ? 5000 : midTurn ? 4000 : 3000;
const stub = await startStubProvider({
  respond: (body) => {
    const tools = (body.tools ?? []).map((t) => t.function?.name);
    const msgs = Array.isArray(body.messages) ? body.messages : [];
    if (!tools.includes("subagent")) {
      // a compaction asks the provider for a SUMMARY of the conversation: hold it past the child's end
      if (compact && msgs.some((m) => /summar/i.test(JSON.stringify(m.content)) && m.role !== "tool")) return { text: "## Summary\nThe parent launched the background child probe-child and is waiting for it.", delayMs: 9000 };
      return { text: "CHILD-OK", delayMs: CHILD_DELAY_MS };                                      // the child
    }
    const toolResults = msgs.filter((m) => m.role === "tool");
    const launched = toolResults.some((m) => /probe-child|async|background/i.test(JSON.stringify(m.content)));
    if (!launched) return { tool: "subagent", args: { agent: "probe-child", task: "Reply with CHILD-OK", async: true } };
    if (midTurn && toolResults.length === 1) return { tool: "bash", args: { command: "sleep 9; echo LONG-DONE", timeout: 30 } };
    const last = msgs[msgs.length - 1];
    return { text: last?.role === "tool" ? "STARTED" : "WOKE" };
  },
});

const env = { ...process.env, PI_CODING_AGENT_DIR: stub.agentDir, PI_SUBAGENT_EXTRA_AGENT_DIRS: agents, ENGRAM_PORT: "9", PI_OFFLINE: "1" };
// the extension entry is what the package declares (`pi.extensions`): ./bundle/index.js in 0.85.0, ./index.js in 0.77.0
const subEntry = JSON.parse(readFileSync(join(subDir, "package.json"), "utf8")).pi?.extensions?.[0] ?? "./index.js";
const args = [piCli, "--mode", "rpc", "--no-session", "-ne", "-na", "-ns", "-nc", "-e", join(subDir, subEntry)];
if (adapter) args.push("-e", adapter);
args.push("--model", "stub/stub", "--thinking", "off");
const child = spawn("node", args, { env, cwd: work, stdio: ["pipe", "pipe", "pipe"] });

let buffer = "", stderr = "";
const records = [];
child.stderr.on("data", (d) => { stderr += String(d); });
child.stdout.on("data", (d) => {
  buffer += String(d);
  let i;
  while ((i = buffer.indexOf("\n")) >= 0) {
    const line = buffer.slice(0, i).replace(/\r$/, ""); buffer = buffer.slice(i + 1);
    if (!line.trim()) continue;
    try { const rec = JSON.parse(line); records.push({ at: Date.now(), rec }); if (trace) console.log(`[event] ${String(rec.type).padEnd(22)}${rec.toolName ? " tool=" + rec.toolName : ""}`); } catch { /* not a record */ }
  }
});
child.stdin.write(JSON.stringify({ id: "p1", type: "prompt", message: "Launch the background subagent and end your turn." }) + "\n");

const deadline = Date.now() + 90_000;
const until = async (fn, what) => { while (Date.now() < deadline) { const hit = records.find(fn); if (hit) return hit; await sleep(200); } throw new Error(`timed out waiting for ${what}`); };
let wakes = 0, notices = 0;
try {
  const launched = await until((x) => x.rec.type === "tool_execution_end" && x.rec.toolName === "subagent", "the subagent tool result");
  ok(launched.rec.isError !== true, `the async subagent launched (${JSON.stringify(launched.rec.result?.content ?? []).slice(0, 160)})`);
  await until((x) => x.rec.type === "agent_settled", "the first run to settle");
  if (compact) child.stdin.write(JSON.stringify({ id: "c1", type: "compact" }) + "\n");
  // the window in which the child's completion must reach the parent ONCE (a second copy is a double wake)
  await sleep(expect === "none" ? 20_000 : 25_000);
  wakes = records.filter((x) => x.rec.type === "agent_start").length - 1;
  const parentRequests = stub.requests.filter((b) => (b.tools ?? []).some((t) => t.function?.name === "subagent"));
  const lastParent = parentRequests[parentRequests.length - 1];
  // the completion as the model sees it: pi-subagents' own notice, a user-role message ("Background task completed: probe-child")
  notices = (lastParent?.messages ?? []).filter((m) => m.role === "user" && /Background task completed/.test(JSON.stringify(m.content))).length;
  if (process.env.LIVE_PROBE_DUMP === "1") console.log("[last parent request]", JSON.stringify((lastParent?.messages ?? []).map((m) => ({ role: m.role, c: typeof m.content === "string" ? m.content.slice(0, 220) : JSON.stringify(m.content).slice(0, 220) })), null, 1));
  const wokeText = records.some((x) => x.rec.type === "message_end" && x.rec.message?.role === "assistant" && /WOKE/.test(JSON.stringify(x.rec.message)));
  const childRan = stub.requests.some((b) => !(b.tools ?? []).some((t) => t.function?.name === "subagent"));
  if (expect === "none") ok(wakes === 0 && !wokeText && notices === 0, `no wake reached the parent (the failure being proved): ${wakes} run(s) after the first, ${notices} notice(s)`);
  else {
    ok(childRan, "the child ran (the provider saw a request with no `subagent` tool)");
    ok(notices === 1, `the child's completion reached the parent's conversation exactly ONCE: ${notices} notice(s)`);
    // idle parent: the completion STARTS one run. Mid-turn parent: it is steered into the run in flight, no extra run.
    if (compact) ok(records.some((x) => x.rec.type === "compaction_start") && records.some((x) => x.rec.type === "compaction_end"), `the parent compacted while the child ran (compaction_start and compaction_end seen)`);
    ok(midTurn ? wakes === 0 : wakes === 1, midTurn ? `a mid-turn completion joins the run in flight (no extra run): ${wakes}` : `an idle parent is woken by exactly ONE new run: ${wakes}`);
    ok(wokeText, "the parent answered the notice (WOKE)");
  }
} catch (e) {
  ok(false, String(e?.message ?? e));
  if (stderr) console.log(`[stderr tail]\n${stderr.split("\n").slice(-25).join("\n")}`);
} finally {
  try { child.stdin.end(); } catch { /* closed */ }
  try { child.kill("SIGTERM"); } catch { /* gone */ }
  await stub.close();
}
console.log(`WAKES ${wakes} NOTICES ${notices}`);
process.exit(results.every(Boolean) ? 0 : 1);

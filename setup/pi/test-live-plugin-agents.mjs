// One real pi session, over RPC, against a LOCAL scripted provider (setup/pi/stub-provider.mjs: no quota, no network):
// the "model" calls pi-subagents' `subagent` with agent `pstack:comment-sicko` — the name a Claude executor reads from
// --plugin-dir, and the one a review closing names. Measured 2026-11-09: three pi executors in a row asked for it and the
// tool answered `Unknown agent 'pstack:thermo-nuclear-code-quality-review-subagent'`, because nothing registered a plugin's
// agents with pi-subagents. `--no-plugin-agents` is that old state (what hw did before it named the directory); with
// HW_PI_PLUGIN_AGENT_DIRS the adapter registers each file as `plugin:name` and the child runs.
//
//   env -i HOME=… PATH=… ENGRAM_PORT=<private> HW_TASK=probe HW_RUN=probe HW_WORKDIR=<tmp> HW_PROJECT=setup
//     node --experimental-strip-types test-live-plugin-agents.mjs <adapter.ts> --subagents <pi-subagents dir>
//       --plugins <plugins root, holding thermo/ponytail/pstack> [--no-plugin-agents]
//
// Prints `ok - …` / `not ok - …` lines, the tool result verbatim, and exits 1 if any check failed. The plugin skills are
// asked from pi itself (`get_commands`), so the same session proves the agents AND what `-e <plugin>` loaded.
import { spawn } from "node:child_process";
import { startStubProvider } from "./stub-provider.mjs";

const arg = (name) => { const i = process.argv.indexOf(name); return i > 0 ? process.argv[i + 1] : undefined; };
const adapter = process.argv[2];
const subDir = arg("--subagents");
const pluginsRoot = arg("--plugins");
const noPluginAgents = process.argv.includes("--no-plugin-agents");
if (!adapter || !subDir || !pluginsRoot) {
  console.error("usage: test-live-plugin-agents.mjs <adapter.ts> --subagents <dir> --plugins <dir> [--no-plugin-agents]");
  process.exit(2);
}

const results = [];
const ok = (c, m) => { results.push(c); console.log(`${c ? "ok" : "not ok"} - ${m}`); };
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
const AGENT = "pstack:comment-sicko";

const stub = await startStubProvider({
  respond: (body) => {
    const tools = (body.tools ?? []).map((t) => t.function?.name);
    const msgs = Array.isArray(body.messages) ? body.messages : [];
    // a child (the agent's own session) has no subagent tool: it answers the one word the parent will be judged by
    if (!tools.includes("subagent")) return { text: "CHILD-OK" };
    // any subagent tool result in the conversation means the call already happened
    if (msgs.some((m) => m.role === "tool")) return { text: "PARENT-DONE" };
    return { tool: "subagent", args: { agent: AGENT, task: "Reply with CHILD-OK", async: false } };
  },
});

const env = {
  ...process.env,
  PI_CODING_AGENT_DIR: stub.agentDir,
  HW_PI_SUBAGENTS_DIR: subDir,
  ENGRAM_PORT: "9",
  PI_OFFLINE: "1",
};
if (noPluginAgents) delete env.HW_PI_PLUGIN_AGENT_DIRS;
else env.HW_PI_PLUGIN_AGENT_DIRS = `pstack=${pluginsRoot}/pstack/agents`;

const subEntry = JSON.parse(await import("node:fs").then((fs) => fs.readFileSync(`${subDir}/package.json`, "utf8"))).pi?.extensions?.[0] ?? "./index.js";
const argv = ["--mode", "rpc", "--no-session", "-ne", "-na", "-ns", "-nc", "-e", `${subDir}/${subEntry}`, "-e", adapter];
for (const p of ["thermo", "ponytail", "pstack"]) argv.push("-e", `${pluginsRoot}/${p}`);
argv.push("--model", "stub/stub", "--thinking", "off");

const child = spawn(process.env.PI_BIN || "pi", argv, { env, cwd: process.env.HW_WORKDIR, stdio: ["pipe", "pipe", "pipe"] });
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
    try { records.push(JSON.parse(line)); } catch { /* not a protocol record */ }
  }
});
const send = (rec) => child.stdin.write(JSON.stringify(rec) + "\n");
send({ id: "p1", type: "prompt", message: "Call the subagent tool once and print its result." });

const deadline = Date.now() + 180_000;
async function until(fn, what) {
  while (Date.now() < deadline) {
    const hit = records.find(fn);
    if (hit) return hit;
    await sleep(250);
  }
  throw new Error(`timed out waiting for ${what}`);
}

try {
  const started = await until((r) => r.type === "tool_execution_start" && r.toolName === "subagent", "the subagent call to start");
  ok(!!started, "a real pi session calls pi-subagents' subagent tool for a plugin agent");
  const ended = await until((r) => r.type === "tool_execution_end" && r.toolCallId === started.toolCallId, "the subagent result");
  const text = JSON.stringify(ended.result?.content ?? ended.result ?? "", null, 0);
  console.log(`[subagent result] ${text.slice(0, 400)}`);
  if (noPluginAgents) {
    ok(/Unknown agent/i.test(text), "no registration (a plugin's agents are unknown to pi-subagents): the tool answers `Unknown agent`");
  } else {
    ok(!/Unknown agent/i.test(text), "with the plugin's agents registered, the name resolves: no `Unknown agent`");
    ok(!ended.result?.isError && /CHILD-OK/.test(text), "the plugin agent RAN: the child's answer is in the tool result");
  }
  // what `-e <plugin>` loaded, asked from pi itself: the review pieces a closing names
  send({ id: "c1", type: "get_commands" });
  const cmd = await until((r) => r.type === "response" && r.command === "get_commands", "get_commands");
  const names = (cmd.data?.commands ?? []).filter((c) => c.source === "skill").map((c) => c.name);
  console.log(`[skills] ${names.join(" ")}`);
  for (const want of ["skill:thermo-nuclear-code-quality-review", "skill:ponytail-review", "skill:blast-radius", "skill:thermo-nuclear-review"]) {
    ok(names.includes(want), `the declared plugin's skill is loaded: ${want}`);
  }
} catch (e) {
  ok(false, String(e?.message ?? e));
  if (stderr) console.log(`[stderr tail]\n${stderr.split("\n").slice(-20).join("\n")}`);
} finally {
  try { child.stdin.end(); } catch { /* already closed */ }
  try { child.kill("SIGTERM"); } catch { /* already gone */ }
  await stub.close();
}

process.exit(results.every(Boolean) ? 0 : 1);

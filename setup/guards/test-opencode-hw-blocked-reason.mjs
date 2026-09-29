// Behavioural mapping tests for the OpenCode blocked-reason adapter. Producer
// labels below distinguish reproduced events from synthetic mapping coverage.
import {
  chmodSync,
  copyFileSync,
  existsSync,
  mkdirSync,
  mkdtempSync,
  readFileSync,
  rmSync,
  symlinkSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const source = process.env.ADAPTER_SRC ??
  join(dirname(fileURLToPath(import.meta.url)), "..", "opencode-hw-blocked-reason.js");
const base = mkdtempSync(join(tmpdir(), "hw-blocked-reason-"));
const originalHome = process.env.HOME;
let pass = 0;
let fail = 0;

function check(name, condition, detail = "") {
  if (condition) {
    pass += 1;
    console.log(`ok - ${name}`);
  } else {
    fail += 1;
    console.log(`not ok - ${name}${detail ? ` :: ${detail}` : ""}`);
  }
}

let scenarioNumber = 0;
function scenario(agentStatus) {
  const home = `${base}/scenario-${scenarioNumber++}`;
  const brain = join(home, "brain");
  const bin = join(brain, "bin");
  const log = join(home, "calls.jsonl");
  mkdirSync(bin, { recursive: true });
  writeFileSync(join(bin, "herdr-rpc"), `#!/usr/bin/env bash
printf '%s\\t%s\\n' "$2" "$3" >> ${JSON.stringify(log)}
case "$2" in
  agent.get) printf '{"agent":{"agent_status":"${agentStatus}"}}' ;;
  *) echo '{"type":"ok"}' ;;
esac
`);
  chmodSync(join(bin, "herdr-rpc"), 0o755);
  process.env.HOME = home;
  // A copied adapter (mutants) cannot find its brain by its own path; name it.
  process.env.HW_BRAIN_ROOT = brain;
  process.env.HERDR_ENV = "1";
  process.env.HERDR_SOCKET_PATH = "/tmp/test-herdr.sock";
  process.env.HERDR_PANE_ID = "wX:p1";
  const lines = () => existsSync(log)
    ? readFileSync(log, "utf8").trim().split("\n").filter(Boolean)
    : [];
  return {
    tokens: () => lines()
      .filter((line) => line.startsWith("pane.report_metadata"))
      .map((line) => JSON.parse(line.split("\t")[1]).tokens),
  };
}

const event = (plugin, type, properties = {}) => plugin.event({ event: { type, properties } });
const load = async () => {
  const mod = await import(`${pathToFileURL(source).href}?test=${Math.random()}`);
  return mod.HwBlockedReasonPlugin();
};

try {
  {
    // Installed as OpenCode has it: a link in the plugins directory to
    // <brain>/setup/<adapter>, the brain nowhere near the home, no env naming it.
    const s = scenario("blocked");
    const brain = process.env.HW_BRAIN_ROOT;
    delete process.env.HW_BRAIN_ROOT;
    mkdirSync(join(brain, "setup"), { recursive: true });
    mkdirSync(join(brain, "..", "plugins"), { recursive: true });
    copyFileSync(source, join(brain, "setup", "opencode-hw-blocked-reason.js"));
    const link = join(brain, "..", "plugins", "hw-blocked-reason.js");
    symlinkSync(join(brain, "setup", "opencode-hw-blocked-reason.js"), link);
    const mod = await import(`${pathToFileURL(link).href}?test=${Math.random()}`);
    const plugin = await mod.HwBlockedReasonPlugin();
    await event(plugin, "question.asked", { sessionID: "root" });
    check("installed by link, the adapter finds bin/ of the brain it links into",
      s.tokens().at(-1)?.blocked_reason === "question", JSON.stringify(s.tokens()));
  }
  {
    const s = scenario("blocked"); const plugin = await load();
    await event(plugin, "question.asked", { sessionID: "root" });
    const tokens = s.tokens().at(-1);
    check("reproduced producer: root question.asked -> question/root",
      tokens?.blocked_reason === "question" && tokens?.blocked_scope === "root", JSON.stringify(tokens));
  }
  {
    const s = scenario("blocked"); const plugin = await load();
    await event(plugin, "session.created", { sessionID: "child", info: { id: "child", parentID: "root" } });
    await event(plugin, "question.asked", { sessionID: "child" });
    const tokens = s.tokens().at(-1);
    check("reproduced producer: child question.asked -> question/child",
      tokens?.blocked_reason === "question" && tokens?.blocked_scope === "child", JSON.stringify(tokens));
  }
  {
    const s = scenario("idle"); const plugin = await load();
    await event(plugin, "session.created", { sessionID: "child", info: { id: "child", parentID: "root" } });
    await event(plugin, "session.error", { sessionID: "child" });
    check("mapping only: child session.error publishes nothing", s.tokens().length === 0, JSON.stringify(s.tokens()));
  }
  {
    const s = scenario("blocked"); const plugin = await load();
    await event(plugin, "session.error", { sessionID: "root" });
    check("synthetic non-producer coverage: root session.error -> error",
      s.tokens().at(-1)?.blocked_reason === "error", JSON.stringify(s.tokens().at(-1)));
  }
  {
    const s = scenario("blocked"); const plugin = await load();
    await event(plugin, "question.asked", { sessionID: "root" });
    await event(plugin, "question.rejected", { sessionID: "root" });
    check("resolved prompt while herdr remains blocked -> stuck",
      s.tokens().at(-1)?.blocked_reason === "stuck", JSON.stringify(s.tokens().at(-1)));
  }
  {
    const s = scenario("idle"); const plugin = await load();
    await event(plugin, "question.asked", { sessionID: "root" });
    await event(plugin, "question.rejected", { sessionID: "root" });
    const tokens = s.tokens().at(-1);
    check("resolved prompt while herdr is idle -> clear",
      tokens?.blocked_reason === null && tokens?.blocked_scope === null, JSON.stringify(tokens));
  }
  {
    const s = scenario("blocked"); const plugin = await load();
    await event(plugin, "session.idle", { sessionID: "root" });
    check("root idle while herdr remains blocked does not publish stuck", s.tokens().length === 0,
      JSON.stringify(s.tokens()));
  }
  {
    const s = scenario("idle"); const plugin = await load();
    await event(plugin, "session.idle", { sessionID: "root" });
    const tokens = s.tokens().at(-1);
    check("root idle while herdr is idle clears stale metadata",
      tokens?.blocked_reason === null && tokens?.blocked_scope === null, JSON.stringify(tokens));
  }
  {
    const s = scenario("idle"); const plugin = await load();
    await event(plugin, "session.created", { sessionID: "child", info: { id: "child", parentID: "root" } });
    await event(plugin, "question.asked", { sessionID: "child" });
    await event(plugin, "permission.asked", { sessionID: "root" });
    await event(plugin, "question.rejected", { sessionID: "child" });
    const tokens = s.tokens().at(-1);
    check("reproduced producer: root permission remains after child answer",
      tokens?.blocked_reason === "permission" && tokens?.blocked_scope === "root", JSON.stringify(tokens));
  }
  {
    const s = scenario("blocked"); const plugin = await load();
    await event(plugin, "session.created", { sessionID: "child", info: { id: "child", parentID: "root" } });
    await event(plugin, "question.asked", { sessionID: "child" });
    await event(plugin, "permission.asked", { sessionID: "root" });
    check("root prompt outranks an earlier child prompt", s.tokens().at(-1)?.blocked_scope === "root");
  }
  {
    scenario("blocked");
    delete process.env.HERDR_PANE_ID;
    const plugin = await load();
    check("adapter is inert outside a herdr pane", Object.keys(plugin).length === 0);
  }
  {
    const home = `${base}/unreachable`;
    mkdirSync(home, { recursive: true });
    process.env.HOME = home;
    // An empty brain: there is no bin/herdr-rpc to reach.
    process.env.HW_BRAIN_ROOT = home;
    process.env.HERDR_ENV = "1";
    process.env.HERDR_SOCKET_PATH = "/tmp/test-herdr.sock";
    process.env.HERDR_PANE_ID = "wX:p1";
    const plugin = await load();
    let survived = true;
    try {
      await event(plugin, "question.asked", { sessionID: "root" });
      await event(plugin, "question.rejected", { sessionID: "root" });
    } catch {
      survived = false;
    }
    check("herdr unreachable does not break the OpenCode turn", survived);
  }
  {
    const s = scenario("blocked"); const plugin = await load();
    await event(plugin, "permission.asked", { sessionID: "root" });
    await event(plugin, "session.created", { sessionID: "child", info: { id: "child", parentID: "root" } });
    await event(plugin, "question.asked", { sessionID: "child" });
    const tokens = s.tokens().at(-1);
    check("root prompt outranks a child prompt in either insertion order",
      tokens?.blocked_scope === "root" && tokens?.blocked_reason === "permission", JSON.stringify(tokens));
  }
} finally {
  if (originalHome === undefined) delete process.env.HOME;
  else process.env.HOME = originalHome;
  rmSync(base, { recursive: true, force: true });
}

console.log(`\n${pass} passed, ${fail} failed`);
process.exitCode = fail ? 1 : 0;

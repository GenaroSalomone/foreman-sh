// Behavioural guard for the managed OpenCode background-child lifecycle patch.
// It drives the real plugin module through a temporary Herdr-shaped socket, so
// assertions see the exact pane.report_agent states it emits.
import net from "node:net";
import { existsSync, mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

// The subject is the frozen patched copy of herdr's plugin (setup/fixtures), not the live
// install: `herdr integration install opencode` replaces the live file, and that machine event
// must not turn the suite red. setup/check-machine checks the live file with the same --check.
// MANAGED_SRC points the driver at a mutant or at another copy.
const source = process.env.MANAGED_SRC ??
  join(dirname(fileURLToPath(import.meta.url)), "../fixtures/herdr-agent-state.js");
if (!existsSync(source)) {
  console.log(`skip - herdr's opencode plugin is not installed (${source}), so its background-state patch cannot be driven`);
  process.exit(0);
}
const base = mkdtempSync(join(tmpdir(), "herdr-opencode-background-"));
const original = {
  HERDR_ENV: process.env.HERDR_ENV,
  HERDR_SOCKET_PATH: process.env.HERDR_SOCKET_PATH,
  HERDR_PANE_ID: process.env.HERDR_PANE_ID,
};
let pass = 0;
let fail = 0;

function check(name, condition, detail = "") {
  if (condition) { pass += 1; console.log(`ok - ${name}`); }
  else { fail += 1; console.log(`not ok - ${name}${detail ? ` :: ${detail}` : ""}`); }
}

let number = 0;
async function scenario() {
  // On Windows the plugin dials `\\.\pipe\<HERDR_SOCKET_PATH>` (its requestOnce),
  // so the variable is a pipe NAME there; a listen on a file path is refused
  // with EACCES (windows.yml 37090376222). Elsewhere it is the socket's path.
  const name = process.platform === "win32" ? `herdr-opencode-background-${process.pid}-${number}` : join(base, `socket-${number}`);
  const socket = process.platform === "win32" ? `\\\\.\\pipe\\${name}` : name;
  number += 1;
  const states = [];
  const server = net.createServer((client) => {
    let data = "";
    client.on("data", (chunk) => {
      data += chunk;
      const nl = data.indexOf("\n");
      if (nl < 0) return;
      const request = JSON.parse(data.slice(0, nl));
      if (request.method === "pane.report_agent") states.push(request.params.state);
      client.end('{"type":"ok"}\n');
    });
  });
  await new Promise((resolve) => server.listen(socket, resolve));
  process.env.HERDR_ENV = "1";
  process.env.HERDR_SOCKET_PATH = name;
  process.env.HERDR_PANE_ID = "wX:p1";
  const mod = await import(`${pathToFileURL(source).href}?case=${Math.random()}`);
  const plugin = await mod.HerdrAgentStatePlugin();
  const event = async (type, properties = {}) => plugin.event({ event: { type, properties } });
  return {
    event,
    states,
    close: async () => new Promise((resolve) => server.close(resolve)),
  };
}

try {
  {
    const s = await scenario();
    await s.event("session.created", { sessionID: "child", info: { id: "child", parentID: "root" } });
    await s.event("session.status", { sessionID: "root", status: { type: "idle" } });
    check("one live child overrides root idle with working", s.states.join(",") === "working,working", s.states);
    await s.event("session.idle", { sessionID: "child" });
    check("last child completion returns idle", s.states.at(-1) === "idle", s.states);
    await s.event("session.updated", { sessionID: "child", info: { id: "child", parentID: "root" } });
    check("post-completion child update does not resurrect liveness", s.states.at(-1) === "idle", s.states);
    await s.close();
  }
  {
    const s = await scenario();
    await s.event("message.updated", {
      sessionID: "root",
      info: { id: "message", parentID: "previous-message" },
    });
    await s.event("session.idle", { sessionID: "root" });
    check("root message lineage does not become a phantom child", s.states.at(-1) === "idle", s.states);
    await s.close();
  }
  {
    const s = await scenario();
    for (const id of ["c1", "c2", "c3", "c4"]) {
      await s.event("session.created", { sessionID: id, info: { id, parentID: "root" } });
    }
    for (const id of ["c1", "c2", "c3"]) await s.event("session.idle", { sessionID: id });
    check("four children remain working until the final completion", s.states.at(-1) === "working", s.states);
    await s.event("session.idle", { sessionID: "c4" });
    check("fourth child completion returns idle", s.states.at(-1) === "idle", s.states);
    await s.event("session.updated", { sessionID: "c4", info: { id: "c4", parentID: "root" } });
    check("final child summary update keeps the four-child parent idle", s.states.at(-1) === "idle", s.states);
    await s.close();
  }
  {
    const s = await scenario();
    await s.event("session.created", { sessionID: "child", info: { id: "child", parentID: "root" } });
    await s.event("question.asked", { sessionID: "child" });
    await s.event("session.idle", { sessionID: "root" });
    check("child question outranks liveness and root idle", s.states.at(-1) === "blocked", s.states);
    await s.event("question.rejected", { sessionID: "child" });
    check("resolved child question restores working while child remains live", s.states.at(-1) === "working", s.states);
    await s.close();
  }
  {
    const s = await scenario();
    await s.event("question.asked", { sessionID: "root" });
    await s.event("session.idle", { sessionID: "root" });
    check("root question remains blocked through root idle", s.states.at(-1) === "blocked", s.states);
    await s.event("question.rejected", { sessionID: "root" });
    check("resolved root question preserves normal working transition", s.states.at(-1) === "working", s.states);
    await s.event("session.idle", { sessionID: "root" });
    check("genuinely idle root remains idle", s.states.at(-1) === "idle", s.states);
    await s.close();
  }
} finally {
  for (const [key, value] of Object.entries(original)) {
    if (value === undefined) delete process.env[key]; else process.env[key] = value;
  }
  rmSync(base, { recursive: true, force: true });
}

console.log(`\n${pass} passed, ${fail} failed`);
process.exitCode = fail ? 1 : 0;

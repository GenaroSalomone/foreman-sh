// The inverse guard, opencode half: a PRODUCT executor does not write in brain.
//
// THE DECISION IS NOT HERE. It is in `deny_brain_writes.py` beside this file,
// and this plugin asks it, per tool call, with the same payload Claude Code
// hands the Python hook. `deny-repo-writes` keeps a JavaScript port of its
// Python because it predates this shape; a second port of the inverse rules
// would be one more pair to keep in step by hand, and the drift that forced
// eight copies into two is the reason not to start one.
//
// hw loads it for a product executor only, through OPENCODE_CONFIG pointing at
// `brain-guard.opencode.json` beside this file, whose `plugin` list is measured
// to be appended to the configured one rather than replacing it. No directory
// loads it, so a brainer, a setup executor and the operator never see it.
//
// FAILS CLOSED. opencode's only refusal is a throw from `tool.execute.before`,
// so a guard that cannot run — no python3, a crash, unreadable output — throws
// too, naming why: it has not established that the call is safe.
import { spawnSync } from "node:child_process";
import { dirname, isAbsolute, join } from "node:path";
import { fileURLToPath } from "node:url";

const GUARD = join(dirname(fileURLToPath(import.meta.url)), "deny_brain_writes.py");

// opencode's tool names → the Claude-shaped payload the Python half reads.
// Anything else (read, grep, glob, list, webfetch, task…) writes nothing.
const WRITES = new Set(["write", "edit", "multiedit", "patch", "apply_patch"]);

function payloadFor(tool, args, sessionDir) {
  const a = args ?? {};
  if (tool === "bash") {
    // The cwd axis: opencode's bash has no persistent shell, and `workdir`
    // is how an agent is told to change directory.
    const cwd = typeof a.workdir === "string" && a.workdir
      ? (isAbsolute(a.workdir) ? a.workdir : join(sessionDir, a.workdir))
      : sessionDir;
    return { tool_name: "bash", tool_input: { command: a.command ?? "" }, cwd };
  }
  if (!WRITES.has(tool)) return null;
  return { tool_name: tool, tool_input: a, cwd: sessionDir };
}

function verdict(payload, env = process.env) {
  const r = spawnSync("python3", [GUARD], {
    input: JSON.stringify(payload), encoding: "utf8", env, timeout: 60000,
  });
  if (r.error || r.status !== 0) {
    return `Blocked: the brain guard could not run (${r.error?.message ?? `exit ${r.status}: ${(r.stderr ?? "").trim().slice(0, 300)}`}). ` +
      "It reached no verdict, and a guard that reached no verdict has not established that this is safe.";
  }
  const out = (r.stdout ?? "").trim();
  if (!out) return null;
  try {
    const d = JSON.parse(out).hookSpecificOutput;
    return d?.permissionDecision === "deny" ? d.permissionDecisionReason : null;
  } catch {
    return `Blocked: the brain guard answered something unreadable (${out.slice(0, 200)}).`;
  }
}

export const DenyBrainWrites = async (pluginInput) => {
  const sessionDir = pluginInput?.directory ?? pluginInput?.worktree ?? process.cwd();
  return {
    "tool.execute.before": async (input, output) => {
      const payload = payloadFor(input?.tool, output?.args, sessionDir);
      if (!payload) return;
      const reason = verdict(payload);
      if (reason) throw new Error(reason);
    },
  };
};

// ONE EXPORT, because opencode calls every exported function of a plugin file
// as a plugin. The helpers ride on it for the tests.
DenyBrainWrites.payloadFor = payloadFor;
DenyBrainWrites.verdict = verdict;

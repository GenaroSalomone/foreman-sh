// OpenCode adapter: publishes WHY a pane is `blocked`.
//
// Kept separate because `herdr integration install opencode` overwrites its
// managed state plugin (~/.config/opencode/plugins/herdr-agent-state.js).
//
// THE PROBLEM. herdr's `agent_status: blocked` is one word for several
// different situations, and every consumer of it was left to guess which:
//
//   producer          scope  what the parent pane SHOWS          evidence
//   ----------------- ------ ------------------------------------ ------------------
//   question.asked    root   the modal, `esc dismiss` on screen  measured live
//   permission.asked  root   the prompt, on screen               measured live
//   session.error     root   nothing                              UNVERIFIED mapping
//   question.asked    child  NOTHING — the modal lives behind    measured live
//   permission.asked  child  `ctrl+x down view subagents`        UNVERIFIED mapping
//
// Root `permission.asked` was reproduced 2026-08-26 with agent-level
// `bash: ask` and a harmless `true` command. Root
// `session.error` has NOT been reproduced: an invalid model id silently fell
// back to a valid model. That row and child `permission.asked` remain the
// managed plugin's source mapping, not measured runtime claims.
//
// Measured 2026-08-26: a background subagent's question modal leaves the
// parent pane showing only `✓ Worker Task (background)` and `ctrl+x down view
// subagents`. `esc dismiss` is absent from the visible buffer, and `escape`
// sent to the parent does not reach the child's modal — but `ctrl+x`, `down`,
// `escape` does. A consumer reading the screen cannot tell that case from a
// `session.error`, and the two want opposite responses.
//
// So the distinction is published where it is actually known — at the event —
// instead of being re-derived by each consumer from 40 lines of terminal.
//
// TWO TOKENS, deliberately. herdr caps a pane at 16 metadata tokens and
// ask-invoker already spends up to 8 on a chunked question plus 6 hw tokens.
// `blocked_reason` and `blocked_scope` are the minimum that carry the fact;
// timing is already available as `state_change_seq`.
//
//   blocked_reason  question | permission | error | stuck
//   blocked_scope   root | child
//
// `stuck` is the value this adapter exists to be able to say. Measured
// 2026-08-26 on a live pane: a background subagent's question was rejected
// (`question.rejected`), the child session went idle, the ROOT session went
// idle, the modal left the screen — and herdr went on reporting `blocked`.
// Typed input did not reach the prompt box. Every prompt the pane had raised
// was resolved and there was nothing left for a human to answer, yet no
// consumer gating on `idle,done` could ever reach it.
//
// A separate low-density probe later established that root `session.idle` can
// also arrive after an ordinary completed background child while herdr still
// reports blocked. Root idle therefore clears a reachable pane but cannot, by
// itself, prove a stuck pane. That proof remains limited to a real prompt's
// UNBLOCKING event emptying the outstanding set.
//
// That is why the prompt-unblocking path does NOT simply clear. Mirroring the
// managed plugin's event table does not reproduce herdr's state — the two
// disagree, and herdr is the authority every consumer actually reads. So on
// an unblocking event this adapter ASKS herdr what the pane is, and reports
// what it is told. `stuck` is not a guess about a terminal; it is two
// authoritative facts put together: nothing is outstanding, and herdr still
// says blocked.

import { execFile } from "node:child_process";
import { readFile, readdir } from "node:fs/promises";
import { realpathSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { promisify } from "node:util";

const execFileP = promisify(execFile);
// Windows cannot execute a `#!` script (hw, herdr-rpc are bash): Git Bash's own
// bash runs it, found from the mount root bin/msys-compat.sh exports, not from
// PATH, where `bash` is WSL's launcher. A cold start there is ~10x slower.
const execFileAsync = process.platform === "win32"
  ? (file, args, opts = {}) => execFileP(
      process.env.HW_MSYS_ROOT ? `${process.env.HW_MSYS_ROOT}/usr/bin/bash.exe` : "bash",
      [file, ...args], { ...opts, timeout: (opts.timeout ?? 0) * 10 })
  : execFileP;

// The brain this plugin serves: HW_BRAIN_ROOT when set, else the tree the
// plugin file itself lives in. The installed plugin is a symlink to
// <brain>/setup/<this file>, so its real path names the brain it was installed
// from; a copied plugin has to say so through HW_BRAIN_ROOT.
function brainBin() {
  const root = process.env.HW_BRAIN_ROOT ||
    join(dirname(realpathSync(fileURLToPath(import.meta.url))), "..");
  return join(root, "bin");
}
const SOURCE = "hw:opencode-blocked";
const TTL_MS = 86_400_000;

// A pane blocks once but can hold several outstanding prompts at a time — a
// root permission and a background child's question, say. Clearing on the
// first reply would report the pane reachable while the other modal still
// owns the keyboard, so the tokens track a SET and clear only when it empties.
const outstanding = new Map(); // key -> { reason, scope }
const childSessions = new Set();

// Mirrors herdr-agent-state.js (integration v10). Kept as data so a future
// integration version can be diffed against it rather than re-read.
const BLOCKING = new Map([
  ["question.asked", "question"],
  ["permission.asked", "permission"],
  ["session.error", "error"],
]);
const UNBLOCKING = new Set([
  "question.replied",
  "question.rejected",
  "permission.replied",
]);

function isHerdrPane() {
  return (
    process.env.HERDR_ENV === "1" &&
    Boolean(process.env.HERDR_SOCKET_PATH) &&
    Boolean(process.env.HERDR_PANE_ID)
  );
}

async function publish(tokens) {
  try {
    const params = {
      pane_id: process.env.HERDR_PANE_ID,
      source: SOURCE,
      ttl_ms: TTL_MS,
      tokens,
    };
    const rpc = join(brainBin(), "herdr-rpc");
    await execFileAsync(rpc, ["call", "pane.report_metadata", JSON.stringify(params)], {
      timeout: 2_000,
    });
  } catch {
    // Reporting must never break an OpenCode turn.
  }
}

// `null` is herdr's delete sentinel — verified against a live pane, not
// inferred: report_metadata MERGES, so an omitted key would leave a stale
// `blocked_reason` behind after the pane was reachable again.
const CLEAR = { blocked_reason: null, blocked_scope: null };

// A delivered contract challenge is an intentional disk-backed hold, not a
// stuck OpenCode root. Read the same pending-reply fact the turn-end predicate
// trusts rather than inferring intent from terminal text or agent_status.
async function hasPendingRuling() {
  const workdir = process.env.HW_WORKDIR;
  const run = process.env.HW_RUN;
  const pane = process.env.HERDR_PANE_ID;
  if (!workdir || !run || !pane) return false;

  const runDir = join(workdir, ".hw", run);
  let sequence = 1;
  try {
    const raw = (await readFile(join(runDir, "task"), "utf8")).trim();
    if (/^[1-9][0-9]*$/.test(raw)) sequence = Number(raw);
  } catch {
    // Task one deliberately has no counter file.
  }
  const stateDir = sequence > 1 ? join(runDir, `t${sequence}`) : runDir;

  try {
    const names = await readdir(stateDir);
    for (const name of names) {
      if (!name.startsWith("pending-reply-")) continue;
      const rows = (await readFile(join(stateDir, name), "utf8")).split(/\r?\n/);
      const facts = new Map(rows.filter((row) => row.includes("=")).map((row) => {
        const at = row.indexOf("=");
        return [row.slice(0, at), row.slice(at + 1)];
      }));
      if (facts.get("version") === "1" && facts.get("state") === "delivered" &&
          facts.get("intent") === "ruling" && facts.get("pane") === pane &&
          facts.get("run") === run) return true;
    }
  } catch {
    return false;
  }
  return false;
}

// herdr is the authority on whether the pane is blocked, so ask it rather
// than inferring from the event stream. One extra RPC, and only on the path
// where a block is ending — never in the hot path of a normal turn.
async function agentStatus() {
  try {
    const rpc = join(brainBin(), "herdr-rpc");
    const { stdout } = await execFileAsync(
      rpc,
      ["call", "agent.get", JSON.stringify({ target: process.env.HERDR_PANE_ID })],
      { timeout: 2_000 },
    );
    return JSON.parse(stdout)?.agent?.agent_status;
  } catch {
    return undefined;
  }
}

// Nothing is outstanding any more. Herdr agreeing that the pane is reachable
// always clears the tokens. Only a real prompt-unblocking event may turn a
// remaining blocked state into `stuck`; root session.idle is not enough proof.
async function settle(scope, publishStuck = true) {
  if (await hasPendingRuling()) {
    await publish({ blocked_reason: "awaiting-ruling", blocked_scope: null });
    return;
  }
  const status = await agentStatus();
  if (status === "blocked") {
    if (publishStuck) await publish({ blocked_reason: "stuck", blocked_scope: scope });
    return;
  }
  if (status === undefined) return; // could not ask; leave the last word standing
  await publish(CLEAR);
}

function summarise() {
  // A root prompt outranks a child one in the report: it is the one a human
  // can answer without first pressing `ctrl+x down`, so it is the one a
  // consumer should act on first.
  let best;
  for (const entry of outstanding.values()) {
    if (!best || (best.scope === "child" && entry.scope === "root")) best = entry;
  }
  return best;
}

export const HwBlockedReasonPlugin = async () => {
  if (!isHerdrPane()) return {};

  return {
    event: async ({ event }) => {
      const type = event?.type;
      const properties = event?.properties ?? {};
      const sessionID =
        typeof properties.sessionID === "string" && properties.sessionID
          ? properties.sessionID
          : undefined;
      const info = properties.info;

      // Same child-detection as the managed plugin: a session that arrives
      // carrying a parentID is a subagent's, and its modal is not on screen.
      if (info?.id && info.parentID) childSessions.add(info.id);

      const reason = BLOCKING.get(type);
      if (reason) {
        const scope = sessionID && childSessions.has(sessionID) ? "child" : "root";

        // A CHILD `session.error` is swallowed by the managed plugin — its
        // CHILD_EVENT_STATES has no entry for it, so the pane never goes
        // blocked. Publishing a reason for a state that will not exist would
        // be worse than silence: a consumer would refuse a reachable pane.
        if (type === "session.error" && scope === "child") return;

        outstanding.set(sessionID ?? `${type}:root`, { reason, scope });
        const best = summarise();
        await publish({ blocked_reason: best.reason, blocked_scope: best.scope });
        return;
      }

      if (UNBLOCKING.has(type)) {
        outstanding.delete(sessionID ?? `${type}:root`);
        if (outstanding.size > 0) {
          const best = summarise();
          await publish({ blocked_reason: best.reason, blocked_scope: best.scope });
          return;
        }
        await settle(sessionID && childSessions.has(sessionID) ? "child" : "root");
        return;
      }

      // Root idle can clear stale tokens once herdr agrees the pane is
      // reachable. It cannot publish `stuck`: measured 2026-08-26, an ordinary
      // completed background child can leave herdr briefly reporting blocked
      // when root idle arrives, without any human prompt having existed.
      if (type === "session.idle" && sessionID && !childSessions.has(sessionID)) {
        if (outstanding.size === 0) await settle("root", false);
      }
    },
  };
};

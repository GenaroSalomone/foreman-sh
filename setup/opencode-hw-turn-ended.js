// OpenCode adapter for hw's "turn ended without reporting" contract.
// Kept separate because `herdr integration install opencode` overwrites its
// managed state plugin.

import { execFile } from "node:child_process";
import { realpathSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { promisify } from "node:util";

const execFileP = promisify(execFile);
// The brain this plugin serves: HW_BRAIN_ROOT when set, else the tree the
// plugin file itself lives in. The installed plugin is a symlink to
// <brain>/setup/<this file>, so its real path names the brain it was installed
// from; a copied plugin has to say so through HW_BRAIN_ROOT.
function brainBin() {
  const root = process.env.HW_BRAIN_ROOT ||
    join(dirname(realpathSync(fileURLToPath(import.meta.url))), "..");
  return join(root, "bin");
}

const childSessions = new Set();
let rootSessionID;
let inputSinceIdle = false;

function isHwExecutor() {
  return (
    process.env.HERDR_ENV === "1" &&
    process.env.HERDR_SOCKET_PATH &&
    process.env.HERDR_PANE_ID &&
    process.env.HW_TASK &&
    process.env.HW_RUN &&
    process.env.HW_WORKDIR
  );
}

// How long `hw executor-turn-end` may take before the adapter gives up on it.
// 2s is the answer on an idle machine; HW_TURN_END_TIMEOUT_MS lets a caller that
// measures on a loaded one (setup/tests/595: the bash stub is killed before it
// writes its line at load 60+) say so, without moving the default.
function turnEndTimeoutMs() {
  const n = Number(process.env.HW_TURN_END_TIMEOUT_MS);
  return Number.isFinite(n) && n > 0 ? n : 2_000;
}

async function publishTurnEnded(hadInput) {
  if (!isHwExecutor()) return;

  try {
    const hw = join(brainBin(), "hw");
    await execFileP(hw, ["executor-turn-end"], { // # MUTATION-ANCHOR: 43-M05
      timeout: turnEndTimeoutMs(),
      // Whether a message reached the root session since its last idle. One
      // prompt can end in more than one `session.idle` (measured 2026-10-02 on
      // a failed request: two), and hw counts turns after a report; a repeated
      // idle with nothing new said to the pane is not one.
      env: { ...process.env, HW_TURN_HAD_INPUT: hadInput ? "1" : "0" },
    });
  } catch {
    // OpenCode event handlers cannot veto session.idle. hw records a refused
    // handback as a vendor-neutral verdict that hw wait will not proceed past.
  }
}

export const HwTurnEndedPlugin = async () => ({
  "chat.message": async ({ sessionID }) => {
    if (sessionID && !childSessions.has(sessionID)) {
      rootSessionID = sessionID;
      inputSinceIdle = true;
    }
  },
  event: async ({ event }) => {
    const properties = event?.properties ?? {};
    const sessionID = properties.sessionID;
    const info = properties.info;

    if (info?.id && info.parentID) childSessions.add(info.id);
    if (event?.type !== "session.idle" || !sessionID) return;
    if (childSessions.has(sessionID) || sessionID !== rootSessionID) return;
    const hadInput = inputSinceIdle;
    inputSinceIdle = false;
    await publishTurnEnded(hadInput);
  },
});

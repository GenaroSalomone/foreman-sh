// OpenCode adapter for hw's "turn ended without reporting" contract.
// Kept separate because `herdr integration install opencode` overwrites its
// managed state plugin.

import { execFile } from "node:child_process";
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

async function publishTurnEnded(hadInput) {
  if (!isHwExecutor()) return;

  try {
    const hw = join(brainBin(), "hw");
    await execFileAsync(hw, ["executor-turn-end"], { // # MUTATION-ANCHOR: 43-M05
      timeout: 2_000,
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

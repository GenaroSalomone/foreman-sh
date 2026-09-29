// Lane shim: the `setup` brainer's read-only guard, opencode half.
//
// THE LOGIC IS NOT HERE. It is in `setup/guards/deny-repo-writes.js`, once, for
// all four lanes — with the Python half beside it. Read
// `setup/guards/deny_repo_writes.py`'s docstring for why eight copies became
// two.
//
// The lane name below is the only thing this file decides.
//
// THE IMPORT IS DYNAMIC AND GUARDED, and that is deliberate. A STATIC
// `import { makeDenyRepoWrites } from "..."` makes this file itself unloadable
// when the shared module is missing or broken — and what opencode does with a
// plugin that fails to load is not established here; the plausible answers
// (skip it, log and continue) both leave the lane unguarded with no refusal
// anywhere. Loading it inside the hook instead means this file always loads,
// and an unusable module becomes a THROW on every bash call — which is the one
// refusal mechanism opencode is documented to honour. Eight self-contained
// copies could not fail this way; two shared ones can.
const LANE = "setup";
const SHARED = "../../../setup/guards/deny-repo-writes.js";

export const DenyRepoWrites = async (pluginInput) => {
  try {
    const { makeDenyRepoWrites } = await import(SHARED);
    return await makeDenyRepoWrites(LANE)(pluginInput);
  } catch (error) {
    const why = String(error?.message ?? error);
    return {
      "tool.execute.before": async (input) => {
        if (input.tool !== "bash") return;
        throw new Error(
          `Blocked: the setup read-only guard could not load its shared module ` +
          `(${SHARED}): ${why}. Every bash command is refused until it loads, ` +
          "because a guard that is not running cannot tell a protected tree " +
          "from any other directory.");
      },
    };
  }
};

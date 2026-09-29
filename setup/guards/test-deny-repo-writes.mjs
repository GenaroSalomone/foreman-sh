// Cross-runtime AND cross-lane conformance for the read-only guard.
//
// Python is exercised through its PreToolUse stdin contract, from each lane's
// real registered hook path; JS through each lane's real opencode plugin and
// its `tool.execute.before` hook, with bash `args.workdir`. Both are the shims
// an agent actually loads, so a broken shim fails here rather than in a pane.
//
// WHAT THIS SUITE USED TO PROVE, AND WHY THAT WAS NOT ENOUGH. Until 2026-09-06
// it asserted, per lane, that the lane's Python and the lane's JavaScript
// agreed. It never compared one lane against another. So an 11.5 KB spread
// across the four Python copies — one lane's copy alone had heredoc
// stripping, quote masking and the teardown exemption, and that same copy
// alone had LOST the boundary-correct cwd test — sat under a green 180/180 for
// as long as each lane happened to agree with its own twin on 15 vectors. That
// is the same "confidence without protection" shape the guard itself is about,
// one level up.
//
// So there are three assertions per vector per lane now:
//   <lane>/<runtime>   the verdict is what the vector says
//   <lane> parity      python and javascript agree            (runtime parity)
//   cross-lane         every lane agrees with every other     (LOGIC parity)
//
// Cross-lane is the new one, and it is what makes "one implementation" a
// measured claim instead of a filesystem observation. A vector marked
// `lane_specific` is exempt and must instead declare `expect_with_worktree_roots`
// (or, for a difference no policy field carries, `expect_by_lane`) — that is
// how a deliberate per-lane difference (the spent-worktree teardown) stays
// visible as an assertion rather than becoming the next silent drift.
import { mkdirSync, mkdtempSync, readFileSync, realpathSync, rmSync, symlinkSync } from "node:fs";
import { spawnSync } from "node:child_process";
import { homedir, tmpdir } from "node:os";
import { dirname, basename, join } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const root = process.env.DENY_GUARD_ROOT ?? fileURLToPath(new URL("../..", import.meta.url));
const vectors = JSON.parse(readFileSync(new URL("./deny-repo-writes-vectors.json", import.meta.url)))
  .filter((v) => !v._comment);

// THE LANE ROOTS COME FROM THE GUARD, NOT FROM A COPY OF THEM HERE. This file
// used to carry its own table of the four lanes' repo/worktrees pairs, which is
// one more place for the thing being tested and the test to disagree.
const { LANES } = await import(
  pathToFileURL(`${root}/setup/guards/deny-repo-writes.js`).href);
const laneNames = Object.keys(LANES);

const outside = "/tmp/deny-repo-writes-out";
const base = mkdtempSync(join(tmpdir(), "deny-repo-writes-vec-"));

// Per-lane renderings that only a real filesystem can provide. `{{link}}` is a
// symlink INTO the lane's protected root, created outside it — the shape that
// every pre-unification copy walked straight past. `{{dotdot}}` reaches the
// same place through a `..` from a sibling that need not exist. `{{sibling}}` is
// the opposite: a name that merely BEGINS with a protected root.
function laneRender(lane) {
  const { repo, worktrees } = LANES[lane];
  const link = `${base}/${lane}-link`;
  // The guard hands roots back in its canonical spelling, which on Windows is
  // `/c/...`: a link must point at the spelling Windows itself opens.
  const winSpelling = (p) => process.platform === "win32" ? p.replace(/^\/([a-z])(?=\/|$)/, "$1:") : p;
  symlinkSync(winSpelling(repo), link);
  // `{{dangling}}` points at a path INSIDE the repo that does not exist, so the
  // link dangles in every context — the brain's, where the repo exists, and the
  // export's, where it does not. A write through it creates the target in the
  // repo; the guard must follow it the way os.path.realpath does.
  const dangling = `${base}/${lane}-dangling`;
  symlinkSync(winSpelling(`${repo}/.deny-guard-absent-target`), dangling);
  // `{{dangling_rel}}` is a dangling link whose RELATIVE target climbs with
  // `..`, sitting in a directory reached through ANOTHER symlink. Its `..` must
  // count from the directory's real location, as the kernel and os.path.realpath
  // count them: from there it lands inside the repo, while from the link's
  // lexical parent the same climb lands outside, under `${base}`. Built entirely
  // outside the repo; POSIX only (a relative climb to `/` has no Windows twin),
  // so on Windows it renders as `{{dangling}}`, which carries the same verdict.
  let danglingRel = dangling;
  if (process.platform !== "win32") {
    const real = `${base}/${lane}-real`;
    mkdirSync(real);
    const via = `${base}/${lane}-via/a/b/c`;
    mkdirSync(via, { recursive: true });
    symlinkSync(real, `${via}/dir`);
    const climb = "../".repeat(realpathSync(real).split("/").filter(Boolean).length);
    symlinkSync(`${climb}${repo.slice(1)}/.deny-guard-absent-target`, `${real}/dl`);
    danglingRel = `${via}/dir/dl`;
  }
  return {
    repo,
    worktrees,
    link,
    dangling,
    danglingRel,
    dotdot: `${dirname(repo)}/.deny-guard-absent/../${basename(repo)}/sub`,
    sibling: `${repo}-deny-guard-sibling`,
    // The repo spelled through `$HOME`, for a vector about the guard resolving
    // the variable rather than matching the text. A repo outside the home has
    // no such spelling and renders as itself.
    homeRepo: repo.startsWith(`${homedir()}/`) ? `$HOME${repo.slice(homedir().length)}` : repo,
  };
}

const category = (reason) =>
  reason.includes("shell redirect") ? "redirect"
    : reason.includes("in-place") ? "inplace"
      : reason.includes("git ") ? "git" : "mutator";

const triggerMatches = (reason, trigger) => !trigger || ({
  // The trigger text became "the command names a protected root (<path>)" on
  // 2026-09-07: a lane now protects the other lanes' product repos too, so
  // "the repo or worktree path" no longer says WHICH tree, and a cross-lane
  // denial that cannot name its tree reads as a misconfiguration.
  names: /command names (?:a protected root|the repo or worktree path)/,
  cwd: /working directory is inside it|shell's cwd is inside it/,
  cd: /command itself cd's into it/,
  resolved: /a path in the command RESOLVES inside it/,
}[trigger]).test(reason);

const render = (value, r) => value
  .replaceAll("{{repo}}", r.repo)
  .replaceAll("{{worktrees}}", r.worktrees)
  .replaceAll("{{link}}", r.link)
  .replaceAll("{{dangling_rel}}", r.danglingRel)
  .replaceAll("{{dangling}}", r.dangling)
  .replaceAll("{{dotdot}}", r.dotdot)
  .replaceAll("{{sibling}}", r.sibling)
  .replaceAll("{{home_repo}}", r.homeRepo)
  .replaceAll("{{outside}}", outside)
  .replaceAll("{{out}}", `${outside}/copy`);

function python(path, command, cwd) {
  const payload = JSON.stringify({ tool_name: "Bash", tool_input: { command }, cwd });
  const r = spawnSync("python3", [path], { input: payload, encoding: "utf8" });
  if (r.status !== 0) return { crashed: `${r.status}: ${r.stderr}` };
  const out = r.stdout.trim();
  if (!out) return { allowed: true, reason: "" };
  try { return { allowed: false, reason: JSON.parse(out).hookSpecificOutput.permissionDecisionReason }; }
  catch { return { crashed: `invalid hook output: ${out}` }; }
}

async function javascript(path, sessionDir, command, cwd) {
  const mod = await import(`${pathToFileURL(path).href}?case=${Math.random()}`);
  const hooks = await mod.DenyRepoWrites({ directory: sessionDir });
  try {
    await hooks["tool.execute.before"]({ tool: "bash", sessionID: "test", callID: "test" }, { args: { command, workdir: cwd } });
    return { allowed: true, reason: "" };
  } catch (error) { return { allowed: false, reason: String(error.message ?? error) }; }
}

let pass = 0, fail = 0;
const ok = (name) => { pass++; console.log(`ok - ${name}`); };
const notOk = (name, detail = "") => { fail++; console.log(`not ok - ${name}${detail ? ` :: ${detail}` : ""}`); };

// shape[vectorId][lane] = "allow" | "deny:<category>" | "crashed"
const shape = Object.create(null);
// expected[vectorId][lane] = the verdict the lane's own policy row asks for.
const expected = Object.create(null);
// voice[lane] = Set of lanes whose `writeHere` this lane's denials named.
const voice = Object.create(null);

try {
  for (const laneName of laneNames) {
    const r = laneRender(laneName);
    // THE ROOT LANE LIVES ONE DIRECTORY UP. `brain` is `~/brain`
    // itself, not `~/brain/brain` — measured once, a session
    // started at the root loaded no settings and no guard at all, so the root
    // became a lane like the other four and its registered files sit directly
    // under it.
    const laneDir = laneName === "brain" ? root : `${root}/${laneName}`;
    const pyPath = `${laneDir}/.claude/hooks/deny-repo-writes.py`;
    const jsPath = `${laneDir}/.opencode/plugin/deny-repo-writes.js`;
    for (const vector of vectors) {
      const command = render(vector.command, r);
      const cwd = render(vector.cwd, r);
      // `expect_with_worktree_roots` is the expectation for a lane whose policy
      // declares `worktree_roots` — the property that grants the spent-worktree
      // teardown — so the difference follows the table, not a lane's name.
      const expect = vector.expect_by_lane?.[laneName]
        ?? (vector.expect_with_worktree_roots && LANES[laneName].worktreeRoots?.length
          ? vector.expect_with_worktree_roots : vector.expect);
      (expected[vector.id] ??= Object.create(null))[laneName] = expect;
      const results = [
        ["python", python(pyPath, command, cwd)],
        ["javascript", await javascript(jsPath, laneDir, command, cwd)],
      ];
      for (const [runtime, result] of results) {
        const good = !result.crashed && result.allowed === (expect === "allow") &&
          (expect === "allow" || (category(result.reason) === vector.category && triggerMatches(result.reason, vector.trigger)));
        if (good) ok(`${laneName}/${runtime} ${vector.id}`);
        else notOk(`${laneName}/${runtime} ${vector.id}`,
          result.crashed ?? `${result.allowed ? "allowed" : `${category(result.reason)} ${result.reason}`}`);
        // `content_only: true` marks a vector whose ONLY signal is a protected
        // path sitting inside a quoted argument — the guard cannot confirm
        // that is the command's actual destination, and the message must say
        // so rather than assert a destination it never resolved (2026-09-09:
        // "the command names a protected root" used to read as a confirmed
        // target even when the match came from inside `-m "..."` prose).
        if (vector.content_only && expect !== "allow") {
          if (!result.crashed && !result.allowed && /could not confirm/.test(result.reason)) {
            ok(`${laneName}/${runtime} ${vector.id} names its own uncertainty`);
          } else {
            notOk(`${laneName}/${runtime} ${vector.id} names its own uncertainty`,
              result.crashed ?? result.reason);
          }
        }
        // `message_contains: [...]` asserts every listed substring appears in
        // the denial's reason — for wording the vector's ALLOW/DENY shape and
        // category/trigger cannot express on their own (e.g. that a redirect
        // denial names the RESOLVED destination, not just that it denied).
        if (vector.message_contains && expect !== "allow") {
          const missing = result.crashed ? vector.message_contains
            : vector.message_contains.filter((s) => !result.reason.includes(s));
          if (!result.crashed && missing.length === 0) {
            ok(`${laneName}/${runtime} ${vector.id} message contains expected text`);
          } else {
            notOk(`${laneName}/${runtime} ${vector.id} message contains expected text`,
              result.crashed ?? `missing ${JSON.stringify(missing)} in: ${result.reason}`);
          }
        }
      }
      // A DENIAL MUST BE IN ITS OWN LANE'S VOICE.
      //
      // Until 2026-09-07 a shim naming the wrong lane guarded visibly wrong
      // trees, and the deny/allow shape caught it. Now every lane protects the
      // product repos, so lane B's roots are a SUPERSET of lane A's — a lane-A
      // shim mislabelled as lane B denies everything lane A would and the shape
      // sees nothing. Measured that day: the `shim-lane-name` mutant survived.
      //
      // What still differs is what the denial TELLS the reader. `writeHere` is
      // where it says to write instead, and a brainer sent to another lane's
      // store is a real failure, so that is the discriminator now.
      // Collected per lane and judged once below, because not every deny
      // branch carries the redirect sentence (the copy-in diagnosis ends on the
      // destination operand instead). Asking it of every vector would fail a
      // correct file; asking it of the SET is exactly the property that matters.
      for (const [, result] of results) {
        if (result.crashed || result.allowed) continue;
        for (const [other, cfg] of Object.entries(LANES)) {
          if (result.reason.includes(cfg.writeHere)) (voice[laneName] ??= new Set()).add(other);
        }
      }

      const [py, js] = results.map(([, result]) => result);
      const same = !py.crashed && !js.crashed && py.allowed === js.allowed &&
        (py.allowed || category(py.reason) === category(js.reason));
      if (same) ok(`${laneName} parity ${vector.id}`);
      else notOk(`${laneName} parity ${vector.id}`,
        `py=${py.crashed ?? (py.allowed ? "allow" : category(py.reason))} js=${js.crashed ?? (js.allowed ? "allow" : category(js.reason))}`);

      (shape[vector.id] ??= Object.create(null))[laneName] =
        py.crashed ? "crashed" : py.allowed ? "allow" : `deny:${category(py.reason)}`;
    }
  }

  // ── EVERY LANE DENIES IN ITS OWN VOICE ─────────────────────────────────
  //
  // Until 2026-09-07 a shim naming the wrong lane guarded visibly wrong trees
  // and the deny/allow SHAPE caught it. Now every lane protects the product
  // repos, so lane B's roots are a SUPERSET of lane A's: a lane-A shim
  // mislabelled as lane B denies everything lane A would, and the shape sees
  // nothing. Measured that day — the `shim-lane-name` mutant survived.
  //
  // What still differs is what the denial TELLS the reader: `writeHere`, the
  // store it sends them to instead. A brainer sent to another lane's store is
  // a real failure, so that is the discriminator the shape no longer provides.
  for (const laneName of laneNames) {
    const named = [...(voice[laneName] ?? [])];
    if (!named.includes(laneName)) {
      notOk(`${laneName} lane voice`,
        `no denial from ${laneName} named its own writeHere (${LANES[laneName].writeHere}); it named ${JSON.stringify(named)}. A shim that names the wrong lane now produces the same deny/allow shape, so this sentence is what is left to catch it`);
    } else if (named.length > 1) {
      notOk(`${laneName} lane voice`,
        `${laneName}'s denials named more than one lane's writeHere: ${JSON.stringify(named)}`);
    } else {
      ok(`${laneName} denies in its own voice (${LANES[laneName].writeHere})`);
    }
  }

  // ── CROSS-LANE LOGIC PARITY ────────────────────────────────────────────
  // Each lane was rendered with its OWN roots, so an identical decision shape
  // across all four is the claim "there is one implementation" — measured,
  // rather than inferred from the fact that the files import the same module.
  for (const vector of vectors) {
    const byLane = shape[vector.id];
    const shapes = [...new Set(Object.values(byLane))];
    // A LANE-SPECIFIC VECTOR DIFFERS WHERE THE TABLE MAKES IT DIFFER, and only
    // there. Until 2026-09-29 this demanded a difference unconditionally, which
    // is a claim about OUR lane table, not about the guard: the exported
    // `guards.json` grants `worktree_roots` to no lane, so every lane correctly
    // denies the teardown and the check failed a correct guard. Each lane's
    // own verdict is still asserted above against its policy row; this asks
    // that the lanes split exactly as far as those rows split.
    if (vector.lane_specific) {
      const wanted = new Set(Object.values(expected[vector.id])).size;
      if (wanted > 1 && shapes.length > 1) ok(`cross-lane ${vector.id} DIFFERS by lane, as declared (${JSON.stringify(byLane)})`);
      else if (wanted === 1 && shapes.length === 1) ok(`cross-lane ${vector.id} (${shapes[0]} in all ${laneNames.length} lanes: no lane's policy grants the difference)`);
      else notOk(`cross-lane ${vector.id}`,
        wanted > 1
          ? `declared lane_specific and the lane table grants the difference, but every lane agreed (${shapes[0]}) — the exemption is gone`
          : `no lane's policy grants a difference, yet the lanes disagree — ${JSON.stringify(byLane)}`);
      continue;
    }
    if (shapes.length === 1) ok(`cross-lane ${vector.id} (${shapes[0]} in all ${laneNames.length} lanes)`);
    else notOk(`cross-lane ${vector.id}`,
      `lanes disagree — ${JSON.stringify(byLane)}. Undeclared drift is what the eight-copy split cost; declare it with lane_specific or fix it`);
  }
} finally {
  rmSync(base, { recursive: true, force: true });
}

console.log(`\n${pass} passed, ${fail} failed`);
process.exitCode = fail ? 1 : 0;

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
// THREE DRIVERS, NOT TWO (2026-09-29). Codex's guard is a third entry point,
// `deny-repo-writes-codex.py`, that resolves its OWN cwd and lane from the
// payload and the environment before it asks the shared core anything. Until
// today none of the vectors reached it, so the audit's measured 80 x 2 drivers
// left the only guard that scopes itself untested by the suite that claims to
// test "the guards". It runs every vector below, with one difference that is
// the design and not a gap: a brain-ROOT session is guarded only when the
// payload's cwd is inside brain (a launch-time variable naming the bare root is
// deliberately not trusted, see `main` there), so for the `brain` lane a vector
// whose cwd is outside brain EXPECTS `allow` under Codex, and says so in its
// name. Nothing is skipped.
//
// A SECOND ARM RUNS ON A SANDBOX POLICY (`"sandbox"` vectors). A vector that
// needs a protected root to exist, or to be absent, cannot lean on the machine
// it runs on: the brain's has every root and the export's has none, and that is
// how a hole in the JavaScript guard was found by the export and not here. The
// sandbox arm writes its own `guards.json` under a temporary HOME, copies the
// guards beside it, and builds exactly the tree each vector needs.
//
// Cross-lane is the new one, and it is what makes "one implementation" a
// measured claim instead of a filesystem observation. A vector marked
// `lane_specific` is exempt and must instead declare `expect_with_worktree_roots`
// (or, for a difference no policy field carries, `expect_by_lane`) — that is
// how a deliberate per-lane difference (the spent-worktree teardown) stays
// visible as an assertion rather than becoming the next silent drift.
import { copyFileSync, existsSync, mkdirSync, mkdtempSync, readFileSync, realpathSync, rmSync, symlinkSync, writeFileSync } from "node:fs";
import { spawnSync } from "node:child_process";
import { homedir, tmpdir } from "node:os";
import { dirname, basename, join } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const root = process.env.DENY_GUARD_ROOT ?? fileURLToPath(new URL("../..", import.meta.url));
const allVectors = JSON.parse(readFileSync(new URL("./deny-repo-writes-vectors.json", import.meta.url)))
  .filter((v) => !v._comment);
const vectors = allVectors.filter((v) => !v.sandbox);
const sandboxVectors = allVectors.filter((v) => v.sandbox);

// THE LANE ROOTS COME FROM THE GUARD, NOT FROM A COPY OF THEM HERE. This file
// used to carry its own table of the four lanes' repo/worktrees pairs, which is
// one more place for the thing being tested and the test to disagree.
const { LANES, BRAIN_ROOT } = await import(
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
    // The repo reached WITHOUT spelling it (2026-09-29, audit F5/F8): its bare
    // name for a `../<name>` climb from `{{sibling}}`, its parent for a `*`, the
    // name with its last letter turned into `?`, and a brace that expands to it.
    repoName: basename(repo),
    repoParent: dirname(repo),
    repoGlob: `${dirname(repo)}/${basename(repo).slice(0, -1)}?`,
    repoBrace: `${dirname(repo)}/{deny-guard-other,${basename(repo)}}`,
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
  .replaceAll("{{repo_name}}", r.repoName)
  .replaceAll("{{repo_parent}}", r.repoParent)
  .replaceAll("{{repo_glob}}", r.repoGlob)
  .replaceAll("{{repo_brace}}", r.repoBrace)
  .replaceAll("{{outside}}", outside)
  .replaceAll("{{out}}", `${outside}/copy`);

function python(path, command, cwd, env) {
  const payload = JSON.stringify({ tool_name: "Bash", tool_input: { command }, cwd });
  const r = spawnSync("python3", [path], { input: payload, encoding: "utf8", env: env ?? process.env });
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

// CODEX'S PROTOCOL IS ITS OWN: exit 0 allows, exit 2 refuses with the reason on
// stderr (Claude's JSON on stdout would print a blob and allow). The lane comes
// from the payload's cwd or from `CODEX_PROJECT_DIR`, so the environment is
// built here rather than inherited: `PWD` and `CLAUDE_PROJECT_DIR` are what an
// operator's own shell leaves lying around, and either one naming a lane would
// guard a case the vector meant to leave unguarded.
function codexEnv(laneDir, neutralDir, home) {
  const env = { ...process.env, CODEX_PROJECT_DIR: laneDir, PWD: neutralDir };
  delete env.CLAUDE_PROJECT_DIR;
  if (home) env.HOME = home;
  return env;
}

function codex(path, command, cwd, env, neutralDir, toolName = "Bash") {
  const payload = JSON.stringify({ tool_name: toolName, tool_input: { command }, cwd });
  const r = spawnSync("python3", [path], { input: payload, encoding: "utf8", env, cwd: neutralDir });
  if (r.status === 0) return { allowed: true, reason: "" };
  if (r.status === 2) return { allowed: false, reason: r.stderr.trim() };
  return { crashed: `${r.status}: ${r.stderr}` };
}

let pass = 0, fail = 0;
const ok = (name) => { pass++; console.log(`ok - ${name}`); };
const notOk = (name, detail = "") => { fail++; console.log(`not ok - ${name}${detail ? ` :: ${detail}` : ""}`); };

// Per-driver tally, so the conformance count says how many assertions EACH of
// the three drivers ran, and a driver that quietly ran none shows as a zero.
const driverTally = { python: { pass: 0, fail: 0 }, javascript: { pass: 0, fail: 0 }, codex: { pass: 0, fail: 0 } };
const tally = (runtime, good) => { driverTally[runtime][good ? "pass" : "fail"]++; };

// One verdict against one vector: the deny/allow shape, its category and
// trigger, and the two wording assertions a vector may carry. Shared by the
// lane arm and the sandbox arm so a check added to one cannot miss the other.
function judge(label, runtime, vector, expect, result) {
  const good = !result.crashed && result.allowed === (expect === "allow") &&
    (expect === "allow" || (category(result.reason) === vector.category && triggerMatches(result.reason, vector.trigger)));
  tally(runtime, good);
  if (good) ok(label);
  else notOk(label, result.crashed ?? `${result.allowed ? "allowed" : `${category(result.reason)} ${result.reason}`}`);
  // `content_only: true` marks a vector whose ONLY signal is a protected
  // path sitting inside a quoted argument — the guard cannot confirm
  // that is the command's actual destination, and the message must say
  // so rather than assert a destination it never resolved (2026-09-09:
  // "the command names a protected root" used to read as a confirmed
  // target even when the match came from inside `-m "..."` prose).
  if (vector.content_only && expect !== "allow") {
    const g = !result.crashed && !result.allowed && /could not confirm/.test(result.reason);
    tally(runtime, g);
    if (g) ok(`${label} names its own uncertainty`);
    else notOk(`${label} names its own uncertainty`, result.crashed ?? result.reason);
  }
  // `message_contains: [...]` asserts every listed substring appears in
  // the denial's reason — for wording the vector's ALLOW/DENY shape and
  // category/trigger cannot express on their own (e.g. that a redirect
  // denial names the RESOLVED destination, not just that it denied).
  if (vector.message_contains && expect !== "allow") {
    const missing = result.crashed ? vector.message_contains
      : vector.message_contains.filter((s) => !result.reason.includes(s));
    const g = !result.crashed && missing.length === 0;
    tally(runtime, g);
    if (g) ok(`${label} message contains expected text`);
    else notOk(`${label} message contains expected text`,
      result.crashed ?? `missing ${JSON.stringify(missing)} in: ${result.reason}`);
  }
}

const verdictOf = (r) => r.crashed ? "crashed" : r.allowed ? "allow" : category(r.reason);

const codexPath = `${root}/setup/guards/deny-repo-writes-codex.py`;
// Codex guards a brain-ROOT session only from the payload's cwd (see the header).
const underBrain = (dir) => dir === BRAIN_ROOT || dir.startsWith(`${BRAIN_ROOT}/`);

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
    // Where a Codex session of this lane stands: the lane's own directory under
    // BRAIN, which is where `hw` starts it. The worktree this suite runs from
    // is not under BRAIN, so the lane cannot be read off `root`.
    const codexLaneDir = laneName === "brain" ? BRAIN_ROOT : `${BRAIN_ROOT}/${laneName}`;
    const cEnv = codexEnv(codexLaneDir, base);
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
      const codexUnguarded = laneName === "brain" && !underBrain(cwd);
      const results = [
        ["python", python(pyPath, command, cwd), expect, ""],
        ["javascript", await javascript(jsPath, laneDir, command, cwd), expect, ""],
        ["codex", codex(codexPath, command, cwd, cEnv, base),
          codexUnguarded ? "allow" : expect,
          codexUnguarded ? " (by design: a brain-root session is guarded only from a cwd inside brain)" : ""],
      ];
      for (const [runtime, result, wanted, note] of results) {
        judge(`${laneName}/${runtime} ${vector.id}${note}`, runtime, vector, wanted, result);
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

      const [py, js, cx] = results.map(([, result]) => result);
      const same = !py.crashed && !js.crashed && py.allowed === js.allowed &&
        (py.allowed || category(py.reason) === category(js.reason));
      if (same) ok(`${laneName} parity ${vector.id}`);
      else notOk(`${laneName} parity ${vector.id}`, `py=${verdictOf(py)} js=${verdictOf(js)}`);
      // Codex agrees with Python wherever Codex is guarding at all.
      if (!codexUnguarded) {
        const cxSame = !py.crashed && !cx.crashed && py.allowed === cx.allowed &&
          (py.allowed || category(py.reason) === category(cx.reason));
        if (cxSame) ok(`${laneName} codex parity ${vector.id}`);
        else notOk(`${laneName} codex parity ${vector.id}`, `py=${verdictOf(py)} codex=${verdictOf(cx)}`);
      }

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

  // ── WHAT ONLY CODEX DECIDES ────────────────────────────────────────────
  // Where a session is a brainer is Codex's own question (the other two vendors
  // answer it with per-directory registration), so the lane-resolution edges get
  // their own assertions instead of borrowing a vector's cwd.
  {
    // The root lane's own repo: every lane protects it, and a lane's extra trees
    // (a Cowork store) are not the root's to guard.
    const repo = LANES.brain.repo;
    const cmd = `rm -rf ${repo}/deny-guard-x`;
    const rootEnv = codexEnv(BRAIN_ROOT, base);
    const codexCases = [
      ["a session standing in the brain root is guarded", codex(codexPath, cmd, BRAIN_ROOT, rootEnv, base), "deny"],
      ["a session in a brain subdirectory that is no lane gets the root lane", codex(codexPath, cmd, `${BRAIN_ROOT}/bin`, rootEnv, base), "deny"],
      ["an executor's worktree outside brain is never guarded", codex(codexPath, cmd, outside, rootEnv, base), "allow"],
      ["a tool that is not Bash is none of this guard's business", codex(codexPath, cmd, BRAIN_ROOT, rootEnv, base, "apply_patch"), "allow"],
    ];
    for (const [name, result, want] of codexCases) {
      const good = !result.crashed && result.allowed === (want === "allow");
      tally("codex", good);
      if (good) ok(`codex ${name}`); else notOk(`codex ${name}`, result.crashed ?? result.reason);
    }
  }

  // ── A PAYLOAD OF THE WRONG SHAPE REFUSES; IT DOES NOT CRASH ─────────────
  // Measured 2026-09-29 (audit F12, probes M03-M05, M08): a payload that is not
  // an object, or a Bash payload whose `tool_input` is not one, raised
  // AttributeError OUTSIDE `main`'s try in the Claude hook and in Codex's, and
  // exit 1 is a NON-blocking error in both vendors — the command ran. The
  // runtime builds the payload, not the model, so the live risk is a vendor
  // changing its schema; a guard that cannot read one has not seen a safe
  // command. An empty or truncated stdin stays an allow (it names no tool),
  // and a tool that is not a shell stays none of this guard's business.
  {
    const laneDir = `${root}/setup`;
    const cEnv = codexEnv(`${BRAIN_ROOT}/setup`, base);
    const raw = (path, input, env, cwd) => spawnSync("python3", [path], { input, encoding: "utf8", env: env ?? process.env, cwd });
    const claudeVerdict = (r) => r.status !== 0 ? `crashed ${r.status}`
      : !r.stdout.trim() ? "allow"
        : /"permissionDecision": "deny"/.test(r.stdout) ? "deny" : `invalid ${r.stdout.trim()}`;
    const codexVerdict = (r) => r.status === 0 ? "allow" : r.status === 2 ? "deny" : `crashed ${r.status}`;
    const shapes = [
      ["a JSON array", "[]", "deny"],
      ["a JSON string", '"Bash"', "deny"],
      ["a Bash tool_input that is a string", JSON.stringify({ tool_name: "Bash", tool_input: "rm -rf x", cwd: outside }), "deny"],
      ["a Bash tool_input that is null", JSON.stringify({ tool_name: "Bash", tool_input: null, cwd: outside }), "deny"],
      ["a Bash tool_input that is a list", JSON.stringify({ tool_name: "Bash", tool_input: ["rm", "-rf", "x"], cwd: outside }), "deny"],
      ["an empty stdin (names no tool)", "", "allow"],
      ["a non-shell tool with a string tool_input", JSON.stringify({ tool_name: "Read", tool_input: "x", cwd: outside }), "allow"],
    ];
    for (const [name, input, want] of shapes) {
      const py = claudeVerdict(raw(`${laneDir}/.claude/hooks/deny-repo-writes.py`, input));
      tally("python", py === want);
      if (py === want) ok(`malformed payload, claude hook: ${name} -> ${want}`);
      else notOk(`malformed payload, claude hook: ${name} -> ${want}`, py);
      const cx = codexVerdict(raw(codexPath, input, cEnv, base));
      tally("codex", cx === want);
      if (cx === want) ok(`malformed payload, codex: ${name} -> ${want}`);
      else notOk(`malformed payload, codex: ${name} -> ${want}`, cx);
    }
  }

  // ── THE SANDBOX ARM ────────────────────────────────────────────────────
  // Every tree a `"sandbox"` vector names is built here, under a HOME of its
  // own, from a policy written here: no verdict below depends on which roots
  // the machine running it happens to have. `sandbox: "exist"` builds the roots,
  // `"absent"` leaves them out, `"both"` runs the vector against each.
  //
  // POSIX only. A relative climb to `/` and a case-insensitive spelling of a
  // drive have no Windows twin here; Windows folds every path to one lowercase
  // spelling in the guard itself, and the vector says nothing new there.
  if (process.platform === "win32") {
    ok("sandbox arm: not run on win32 (the guard folds drive paths to one spelling there; the sandbox vectors are POSIX filesystem shapes)");
  } else {
    const lane = laneNames.find((n) => n !== "brain");
    if (!lane) throw new Error("guards.json names no lane besides brain — the sandbox arm has no non-root lane to drive Codex through");
    const realPolicy = JSON.parse(readFileSync(`${root}/guards.json`, "utf8"));
    const flip = (name) => name.replace(/[a-z]/gi, (c) => (c === c.toLowerCase() ? c.toUpperCase() : c.toLowerCase()));

    function buildSandbox(rootsExist) {
      const sb = realpathSync(mkdtempSync(join(base, "sb-")));
      const home = `${sb}/home`;
      const wsRoot = `${home}/Workspace`;
      const brainDir = `${wsRoot}/brain`;
      const repoName = basename(LANES.brain.repo);
      const repo = `${wsRoot}/${repoName}`;
      const worktrees = `${wsRoot}/worktrees`;
      const out = `${sb}/out`;
      mkdirSync(home);
      mkdirSync(out);
      if (rootsExist) { mkdirSync(repo, { recursive: true }); mkdirSync(worktrees); mkdirSync(brainDir); }
      const laneRow = (writeHere) => ({ repo, worktrees, write_here: writeHere, where: "the sandbox repo" });
      writeFileSync(`${sb}/guards.json`, JSON.stringify({
        brain_root: brainDir,
        product_repos: [repo],
        defaults: realPolicy.defaults,
        lanes: { brain: laneRow(`${brainDir}/<project>/`), [lane]: laneRow(`${brainDir}/${lane}/`) },
      }, null, 2));
      const place = (rel) => {
        mkdirSync(dirname(`${sb}/${rel}`), { recursive: true });
        copyFileSync(`${root}/${rel}`, `${sb}/${rel}`);
      };
      for (const f of ["deny_repo_writes.py", "deny-repo-writes.js", "deny-repo-writes-codex.py"]) place(`setup/guards/${f}`);
      place(`${lane}/.claude/hooks/deny-repo-writes.py`);
      place(`${lane}/.opencode/plugin/deny-repo-writes.js`);
      // What a link to each protected shape looks like, all built OUTSIDE the
      // repo. `dl_*` dangle when the roots are absent and point at a directory
      // (or a not-yet-created child of one) when they exist; the verdict is the
      // same, which is the claim.
      const link = (name, target) => { symlinkSync(target, `${out}/${name}`); return `${out}/${name}`; };
      const mid = link("dl-chain-mid", `${repo}/x`);
      const climb = "../".repeat(out.split("/").filter(Boolean).length);
      // A link whose target spells a protected root in the WRONG CASE.
      const caseRepo = `${wsRoot}/${flip(repoName)}`;
      const r = {
        repo, worktrees, home,
        dl_root: link("dl-root", repo),
        dl_deep: link("dl-deep", `${repo}/a/b/c`),
        dl_chain: link("dl-chain", mid),
        dl_rel: link("dl-rel", `${climb}${repo.slice(1)}/x`),
        dl_outside: link("dl-outside", `${out}/absent-target`),
        case_link: link("case-link", caseRepo),
        case_repo: caseRepo,
        case_ancestor: `${home}/${flip("Workspace")}/${repoName}`,
        case_sibling: `${wsRoot}/${flip(repoName)}-deny-guard-sibling`,
        sibling: `${repo}-deny-guard-sibling`,
      };
      // Whether THIS filesystem treats two spellings as one. APFS and NTFS do,
      // ext4 does not, and a case-variant path means the protected tree only on
      // the first kind; on the second it is another, nonexistent path.
      mkdirSync(`${sb}/case-probe`);
      const caseInsensitive = existsSync(`${sb}/CASE-PROBE`);
      return { sb, brainDir, r, caseInsensitive };
    }

    const renderSb = (value, t) => value
      .replaceAll("{{repo}}", t.r.repo).replaceAll("{{worktrees}}", t.r.worktrees)
      .replaceAll("{{dl_root}}", t.r.dl_root).replaceAll("{{dl_deep}}", t.r.dl_deep)
      .replaceAll("{{dl_chain}}", t.r.dl_chain).replaceAll("{{dl_rel}}", t.r.dl_rel)
      .replaceAll("{{dl_outside}}", t.r.dl_outside).replaceAll("{{case_link}}", t.r.case_link)
      .replaceAll("{{case_repo}}", t.r.case_repo).replaceAll("{{case_ancestor}}", t.r.case_ancestor)
      .replaceAll("{{case_sibling}}", t.r.case_sibling).replaceAll("{{sibling}}", t.r.sibling)
      .replaceAll("{{outside}}", outside).replaceAll("{{out}}", `${outside}/copy`);

    const realHome = process.env.HOME;
    try {
      for (const rootsExist of [false, true]) {
        const t = buildSandbox(rootsExist);
        const mode = rootsExist ? "roots exist" : "roots absent";
        const fsNote = t.caseInsensitive ? "case-insensitive fs" : "case-sensitive fs";
        const pyEnv = { ...process.env, HOME: t.r.home };
        const cEnv = codexEnv(`${t.brainDir}/${lane}`, base, t.r.home);
        process.env.HOME = t.r.home;
        for (const vector of sandboxVectors) {
          if (vector.sandbox !== "both" && vector.sandbox !== (rootsExist ? "exist" : "absent")) continue;
          const command = renderSb(vector.command, t);
          const cwd = renderSb(vector.cwd, t);
          const expect = !t.caseInsensitive && vector.expect_case_sensitive_fs ? vector.expect_case_sensitive_fs : vector.expect;
          const tag = `sandbox[${mode}, ${fsNote}] ${vector.id}`;
          const results = [
            ["python", python(`${t.sb}/${lane}/.claude/hooks/deny-repo-writes.py`, command, cwd, pyEnv)],
            ["javascript", await javascript(`${t.sb}/${lane}/.opencode/plugin/deny-repo-writes.js`, `${t.sb}/${lane}`, command, cwd)],
            ["codex", codex(`${t.sb}/setup/guards/deny-repo-writes-codex.py`, command, cwd, cEnv, base)],
          ];
          for (const [runtime, result] of results) judge(`${tag} (${runtime})`, runtime, vector, expect, result);
          const [py, js, cx] = results.map(([, result]) => result);
          const agree = !py.crashed && !js.crashed && !cx.crashed && py.allowed === js.allowed && py.allowed === cx.allowed &&
            (py.allowed || (category(py.reason) === category(js.reason) && category(py.reason) === category(cx.reason)));
          if (agree) ok(`${tag} parity: all three drivers agree`);
          else notOk(`${tag} parity`, `py=${verdictOf(py)} js=${verdictOf(js)} codex=${verdictOf(cx)}`);
        }
      }
    } finally {
      process.env.HOME = realHome;
    }
  }

  for (const [runtime, t] of Object.entries(driverTally)) {
    console.log(`# driver ${runtime}: ${t.pass + t.fail} assertions, ${t.pass} passed, ${t.fail} failed`);
    if (t.pass + t.fail === 0) notOk(`driver ${runtime} ran no assertions`);
  }
} finally {
  rmSync(base, { recursive: true, force: true });
}

console.log(`\n${pass} passed, ${fail} failed`);
process.exitCode = fail ? 1 : 0;

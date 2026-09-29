// Real-filesystem effects for all four OpenCode guard LANES. The shared module
// is copied and has only ONE lane's protected roots repointed into a disposable
// sandbox; no command below can name a real protected tree.
//
// Before 2026-09-06 this repointed `const REPO`/`const WORKTREES` in each lane's
// own plugin file, because there were four of them. There is one now, with a
// lane table, so the rewrite targets that lane's entry — and still fails loudly
// if it cannot find it, since a repoint that silently no-ops would run every
// assertion below against the REAL trees.
import { execFileSync } from "node:child_process";
import {
  existsSync,
  mkdirSync,
  mkdtempSync,
  readFileSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

// fileURLToPath, not `.pathname`: on Windows that is `/D:/...`, which no fs call opens.
const root = process.env.DENY_GUARD_ROOT ?? fileURLToPath(new URL("../..", import.meta.url));
// Every lane the policy table names except the root, read from guards.json
// rather than listed here: a lane added to the table is exercised with no edit.
const lanes = Object.keys(JSON.parse(readFileSync(`${root}/guards.json`, "utf8")).lanes ?? {})
  .filter((lane) => lane !== "brain");
if (lanes.length === 0) throw new Error("guards.json names no lane — nothing below would run");
// The shell that executes each command: Git Bash's own on Windows (a native
// node has no /bin, and PATH's first bash there can be WSL's launcher).
const BASH = process.platform === "win32" && process.env.HW_MSYS_ROOT
  ? `${process.env.HW_MSYS_ROOT}/usr/bin/bash.exe` : "/bin/bash";
// Forward slashes: the path is interpolated into bash commands, which read a
// backslash as an escape (Windows' tmpdir() returns a backslash drive path).
const base = mkdtempSync(join(tmpdir(), "deny-repo-writes-fs-")).replaceAll("\\", "/");
const payload = Buffer.from("7b226e223a22c3b1202d206f6b227d0aff000a", "hex");
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

// Repoint one lane's `repo` and `worktrees` in a COPY of guards.json, beside a
// copy of the shared module laid out the way the module finds its policy
// (`<root>/setup/guards/deny-repo-writes.js` reads `<root>/guards.json`).
// Until 2026-09-23 the table was a literal inside the module and this rewrote
// its source; the values moved to guards.json, so the rewrite moved with them.
// It still fails loudly when the lane is missing, since a repoint that silently
// no-ops would run every assertion below against the REAL trees.
function repointGuard(lane, repo, worktrees) {
  const policy = JSON.parse(readFileSync(`${root}/guards.json`, "utf8"));
  if (!policy.lanes?.[lane]?.repo || !policy.lanes[lane].worktrees) {
    throw new Error(
      `${lane}: could not find its repo/worktrees entry in guards.json — the ` +
      `repoint would silently no-op and every assertion below would run ` +
      `against the REAL protected trees`);
  }
  policy.lanes[lane].repo = repo;
  policy.lanes[lane].worktrees = worktrees;
  const tree = `${base}/${lane}-guard`;
  mkdirSync(`${tree}/setup/guards`, { recursive: true });
  writeFileSync(`${tree}/guards.json`, JSON.stringify(policy, null, 2));
  const path = `${tree}/setup/guards/deny-repo-writes.js`;
  writeFileSync(path, readFileSync(`${root}/setup/guards/deny-repo-writes.js`, "utf8"));
  return path;
}

async function loadGuard(path, lane, sessionDir) {
  const mod = await import(`${pathToFileURL(path).href}?v=${Math.random()}`);
  const hooks = await mod.makeDenyRepoWrites(lane)({ directory: sessionDir, worktree: sessionDir });
  const before = hooks["tool.execute.before"];
  return async (command, workdir) => {
    const args = { command };
    if (workdir !== undefined && workdir !== null) args.workdir = workdir;
    try {
      await before({ tool: "bash", sessionID: "test", callID: "test" }, { args });
      return { allowed: true, reason: "" };
    } catch (error) {
      return { allowed: false, reason: String(error.message ?? error) };
    }
  };
}

async function attempt(guard, command, workdir) {
  const verdict = await guard(command, workdir);
  let ran = false;
  if (verdict.allowed) {
    execFileSync(BASH, ["-c", command], { cwd: workdir, stdio: "pipe" });
    ran = true;
  }
  return { ...verdict, ran };
}

try {
  for (const lane of lanes) {
    const sandbox = `${base}/${lane}-sandbox`;
    const repo = `${sandbox}/protected`;
    const worktrees = `${sandbox}/worktrees`;
    const outside = `${sandbox}/outside`;
    const source = `${repo}/artifacts/raw.bin`;
    const guard = await loadGuard(repointGuard(lane, repo, worktrees), lane, sandbox);

    const reset = () => {
      rmSync(sandbox, { recursive: true, force: true });
      mkdirSync(`${repo}/artifacts`, { recursive: true });
      mkdirSync(worktrees, { recursive: true });
      mkdirSync(outside, { recursive: true });
      writeFileSync(source, payload);
      writeFileSync(`${sandbox}/payload.bin`, payload);
    };

    reset();
    let result = await attempt(guard, `cp ${source} ${outside}/copied.bin`, sandbox);
    check(`${lane}: cp OUT is allowed and executes`, result.allowed && result.ran, result.reason);
    check(
      `${lane}: cp OUT copies exact bytes`,
      existsSync(`${outside}/copied.bin`) &&
        Buffer.compare(readFileSync(source), readFileSync(`${outside}/copied.bin`)) === 0,
    );

    reset();
    result = await attempt(guard, `cp ${sandbox}/payload.bin ${repo}/injected.bin`, sandbox);
    check(`${lane}: cp IN is denied`, !result.allowed && !result.ran, result.reason);
    check(`${lane}: denied cp IN creates nothing`, !existsSync(`${repo}/injected.bin`));

    reset();
    result = await attempt(guard, `mv ${repo}/artifacts ${outside}/artifacts`, sandbox);
    check(`${lane}: mv OUT is denied`, !result.allowed && !result.ran, result.reason);
    check(
      `${lane}: denied mv OUT preserves source and creates no destination`,
      existsSync(source) && !existsSync(`${outside}/artifacts`),
    );
  }
} finally {
  rmSync(base, { recursive: true, force: true });
}

console.log(`\n${pass} passed, ${fail} failed`);
process.exitCode = fail ? 1 : 0;

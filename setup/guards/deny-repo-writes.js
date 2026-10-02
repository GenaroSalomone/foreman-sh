// The read-only guard, once, for every lane — the opencode half.
//
// The Python half is `deny_repo_writes.py` beside this file, and it carries the
// full account of why eight copies became two: read its module docstring first.
// The short version is that four Python copies had drifted 11.5 KB apart, in
// BOTH directions, while the eight-copy conformance suite stayed green — it
// compared each lane's Python against its own JavaScript and never compared one
// lane against another.
//
// What is shared here is detection. What stays per lane is the policy in
// `guards.json` at brain's root, loaded below: protected roots, prose, and
// EXEMPTIONS. An exemption is not a check, so a lane with no `worktree_roots`
// cannot reach the spent-worktree teardown branch.
//
// Why a guard at all: `permission.edit` matches tool *paths*, and a shell
// redirect is not a path — `echo x > docs/file.md` is one bash call whose
// argument merely contains a filename. That hole is not theoretical; it is how
// the first enforcement test of this setup wrote a probe file with every deny
// rule in place. And matching the string is not enough either: `..`, a symlink,
// or a sibling directory whose name merely begins with a protected root all
// defeat a substring test. Every boundary question below RESOLVES, then
// compares.
//
// KEEP THIS FILE AND THE PYTHON IN STEP. `setup/guards/` holds the conformance
// suite that proves you did, and it now compares lanes as well as runtimes.

import { lstatSync, readFileSync, readlinkSync, realpathSync, statSync } from "node:fs";
import { userInfo } from "node:os";
import { basename, dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

// NATIVE WINDOWS: ONE SPELLING OF A PATH, as in the Python half (read the
// comment above `_canon` there). On Windows every root, working directory and
// path token is folded to `/c/data/x` — drive as a leading segment, forward
// slashes, lower case — before it is compared, and realpath runs on the native
// spelling. Folding only makes spellings equal, so it adds matches and never
// removes one. Elsewhere each helper is the identity. The policy's `~` is
// `userInfo().homedir`, which on Windows is the profile directory of the
// account's token, not USERPROFILE. `DENY_REPO_WRITES_AS_NATIVE_WINDOWS=1`
// folds on any platform, so the folding is testable where Windows is not.
const NATIVE_WINDOWS = process.platform === "win32";
const WINPATHS = NATIVE_WINDOWS || process.env.DENY_REPO_WRITES_AS_NATIVE_WINDOWS === "1";
const DRIVE = /^([A-Za-z]):(?:[\\/]|$)/;
const TEXT_DRIVE = /(?<![\w])([A-Za-z]):\//g;

export function canon(p) {
  if (!WINPATHS || typeof p !== "string" || !p) return p;
  let q = p.replace(/\\/g, "/");
  const m = DRIVE.exec(q);
  if (m) q = "/" + m[1] + q.slice(2);
  if (q.startsWith("/")) q = normPath(q);
  return q.toLowerCase();
}

function canonText(text) {
  if (!WINPATHS || typeof text !== "string") return text;
  return text.replace(/\\/g, "/").replace(TEXT_DRIVE, (_, d) => `/${d}/`).toLowerCase();
}

function isAbs(p) {
  if (typeof p !== "string") return false;
  if (WINPATHS) return p.startsWith("/") || p.startsWith("\\") || DRIVE.test(p);
  return p.startsWith("/");
}

function joinPath(base, rel) {
  if (WINPATHS) return canon(canon(base).replace(/\/+$/, "") + "/" + rel.replace(/\\/g, "/"));
  return base.replace(/\/+$/, "") + "/" + rel;
}

// Canonical spelling -> one Windows can open, or null.
function nativeOf(c) {
  const m = /^\/([a-z])(\/.*)?$/.exec(c);
  if (m) return `${m[1]}:${m[2] ?? "/"}`;
  if (c === "/tmp" || c.startsWith("/tmp/")) {
    const t = process.env.TEMP ?? process.env.TMP;
    return t ? t.replace(/\\/g, "/").replace(/\/+$/, "") + c.slice(4) : null;
  }
  const r = (process.env.HW_MSYS_ROOT ?? "").replace(/\\/g, "/").replace(/\/+$/, "");
  return r && c.startsWith("/") ? r + c : null;
}

// ── THE LANE TABLE ─────────────────────────────────────────────────────────
//
// THE VALUES LIVE IN `guards.json`, beside `projects.json` at brain's root, and
// the Python half reads the same file. Read the comment above `load_policy` in
// `deny_repo_writes.py` for what each key means and why. The short version:
// `productRepos` is the floor every lane protects, `worktreeRoots` is the only
// key that grants an exemption, and `~` is the home in the password database,
// not `$HOME`.
//
// A MISSING OR MALFORMED FILE FAILS CLOSED. It is loaded at import, so a bad
// policy makes this module fail to import, and every lane shim already turns
// that into a throw on every bash call. The checks match the Python ones
// exactly, and an unknown key is refused rather than ignored.
//
// NOT protected, on purpose: brain itself, where every brainer writes.
// `writeHere` names it for that reason.
export const POLICY_PATH = join(
  dirname(dirname(dirname(realpathSync(fileURLToPath(import.meta.url))))),
  "guards.json");

const TOP_KEYS = ["comment", "brain_root", "product_repos", "defaults", "lanes",
  "specialists_from"];
const LANE_REQUIRED = ["repo", "worktrees", "write_here", "where"];
const LANE_OPTIONAL = ["worktree_roots", "git_tail", "redirect_tail"];
const LANE_NAME = /^[a-z][a-z-]*$/;

function policyError(message) {
  return new Error(`deny-repo-writes: guards.json: ${message}`);
}

function policyPath(value, where, home) {
  if (typeof value !== "string" || !value) {
    throw policyError(`${where} must be a non-empty path string, got ${JSON.stringify(value)}`);
  }
  if (value === "~") value = home;
  else if (value.startsWith("~/")) value = home + value.slice(1);
  value = canon(value);
  if (!isAbs(value)) {
    throw policyError(`${where} must be absolute or start with ~/, got ${JSON.stringify(value)}`);
  }
  return value;
}

function policyText(value, where) {
  if (typeof value !== "string" || !value) {
    throw policyError(`${where} must be a non-empty string, got ${JSON.stringify(value)}`);
  }
  return value;
}

const isObject = (v) => v !== null && typeof v === "object" && !Array.isArray(v);

export function loadPolicy(path = POLICY_PATH) {
  let doc;
  try {
    doc = JSON.parse(readFileSync(path, "utf8"));
  } catch (error) {
    throw policyError(`cannot read ${path}: ${String(error?.message ?? error)}`);
  }
  if (!isObject(doc)) throw policyError(`${path} is not a JSON object`);
  const unknown = Object.keys(doc).filter((k) => !TOP_KEYS.includes(k)).sort();
  const missing = ["brain_root", "defaults", "lanes", "product_repos"]
    .filter((k) => !(k in doc));
  if (unknown.length || missing.length) {
    throw policyError(`unknown keys ${JSON.stringify(unknown)}, missing keys ${JSON.stringify(missing)}`);
  }
  const home = canon(userInfo().homedir);
  const brainRoot = policyPath(doc.brain_root, "brain_root", home);
  if (!Array.isArray(doc.product_repos) || !doc.product_repos.length) {
    throw policyError("product_repos must be a non-empty list");
  }
  const productRepos = Object.freeze(
    doc.product_repos.map((p, i) => policyPath(p, `product_repos[${i}]`, home)));
  const defaults = doc.defaults;
  if (!isObject(defaults)
      || Object.keys(defaults).sort().join(",") !== "git_tail,redirect_tail") {
    throw policyError("defaults must hold exactly git_tail and redirect_tail");
  }
  for (const key of Object.keys(defaults)) policyText(defaults[key], `defaults.${key}`);
  if (!isObject(doc.lanes) || !("brain" in doc.lanes)) {
    throw policyError("lanes must be an object that includes the brain root lane");
  }
  const lanes = {};
  for (const [name, raw] of Object.entries(doc.lanes)) {
    if (!LANE_NAME.test(name)) throw policyError(`lane name ${JSON.stringify(name)} is not a lowercase word`);
    if (!isObject(raw)) throw policyError(`lanes.${name} is not an object`);
    const laneUnknown = Object.keys(raw)
      .filter((k) => !LANE_REQUIRED.includes(k) && !LANE_OPTIONAL.includes(k)).sort();
    const laneMissing = LANE_REQUIRED.filter((k) => !(k in raw));
    if (laneUnknown.length || laneMissing.length) {
      throw policyError(`lanes.${name}: unknown keys ${JSON.stringify(laneUnknown)}, missing keys ${JSON.stringify(laneMissing)}`);
    }
    // AN OPTIONAL KEY IS ABSENT OR WELL-FORMED, NEVER null. `??` would read a
    // present `null` as absent and fall back to the default; the Python half's
    // `raw.get(key, default)` returns the `None` and refuses it. So only a
    // missing key takes the default here too, and a `null` fails closed in both.
    const opt = (key, fallback) => (key in raw ? raw[key] : fallback);
    const exempt = opt("worktree_roots", []);
    if (!Array.isArray(exempt)) throw policyError(`lanes.${name}.worktree_roots must be a list`);
    lanes[name] = {
      repo: policyPath(raw.repo, `lanes.${name}.repo`, home),
      worktrees: policyPath(raw.worktrees, `lanes.${name}.worktrees`, home),
      worktreeRoots: exempt.map((p, i) => policyPath(p, `lanes.${name}.worktree_roots[${i}]`, home)),
      alsoProtect: productRepos,
      writeHere: policyText(raw.write_here, `lanes.${name}.write_here`),
      where: policyText(raw.where, `lanes.${name}.where`),
      gitTail: policyText(opt("git_tail", defaults.git_tail), `lanes.${name}.git_tail`),
      redirectTail: policyText(opt("redirect_tail", defaults.redirect_tail), `lanes.${name}.redirect_tail`),
    };
  }
  return { brainRoot, productRepos, lanes };
}

const POLICY = loadPolicy();
export const BRAIN_ROOT = POLICY.brainRoot;
export const PRODUCT_REPOS = POLICY.productRepos;
export const LANES = POLICY.lanes;

// A guard that silently no-ops on an unknown lane is worse than no guard.
export function config(lane) {
  const raw = LANES[lane];
  if (!raw) {
    throw new Error(
      `deny-repo-writes: unknown lane ${JSON.stringify(lane)} — known lanes are ` +
      `${Object.keys(LANES).sort().join(", ")}. A shim and the lane table have ` +
      `diverged; the guard is NOT protecting anything until this is fixed.`);
  }
  // `repo`/`worktrees` stay the lane's OWN pair — the teardown exemption and the
  // prose key on them. `roots` is every tree the lane must not write, deduped
  // because a lane's own repo is normally in PRODUCT_REPOS too.
  const roots = [raw.repo, raw.worktrees];
  for (const extra of raw.alsoProtect ?? []) {
    if (!roots.includes(extra)) roots.push(extra);
  }
  // Resolved once, not once per token: `decide` now asks the boundary question
  // of every absolute path in a command.
  return { ...raw, lane, roots, resolvedRoots: resolveRoots(roots), rootIds: rootIds(roots) };
}

// ── DETECTION ──────────────────────────────────────────────────────────────
// Shared by every lane. Nothing here is lane-configured, and nothing here was
// dropped from any copy.

// Mutating shell verbs, word-anchored so `remove_stale_rows` or a path segment
// called `cp-report` cannot trip them.
const MUTATORS =
  /(?<![\w-])(rm|mv|cp|rsync|tee|touch|mkdir|rmdir|truncate|dd|ln|chmod|chown|install|patch|sponge|ed|ex|python|python3|node|ruby|perl|php|deno|bun|osascript|xargs)(?![\w-])/;
// `find` and `fd` are readers until an action writes or runs something — see
// the Python twin (2026-09-29, audit F9).
const FIND_WRITES = new RegExp(
  String.raw`(?<![\w-])find(?![\w-])[^|;&\n]*?\s-(?:delete|exec|execdir|ok|okdir|` +
  String.raw`fprint|fprint0|fprintf|fls)(?![\w-])` +
  String.raw`|(?<![\w-])(?:fd|fdfind)(?![\w-])[^|;&\n]*?\s(?:-[A-Za-z]*[xX]|--exec|` +
  String.raw`--exec-batch)(?![\w-])`, "g");
// sed/perl/awk only mutate with an in-place flag.
const INPLACE = /(?<![\w-])(sed|perl|gawk|awk)\b[^|;&]*\s-i\b/;
// Any redirect that creates or appends to a file (not 2>&1, not a heredoc).
const REDIRECT = /(?<![0-9&])>{1,2}(?!&)/;

// WORD-ANCHORING IS NOT COMMAND-POSITION. Measured 2026-09-10 with `cwd`
// inside the `lane-a` lane: `ls app/scripts/bin/node` and
// `cat app/docs/rm.md` were both DENIED, because `/` and `.`
// sit outside `[\w-]` — the same class `MUTATORS`'s lookaround excludes — so
// a path SEGMENT merely named after a mutator (`bin/node`, `rm.md`) satisfies
// the same boundary a real invocation does. Widening the lookaround to
// include `/`/`.` is the naive fix and it is unsafe: it would let
// `/usr/bin/python3 -c "open(...,'w')"` back through, which is exactly the
// hole `MUTATORS` grew interpreter names to close. The real distinction is
// POSITION, not spelling: a mutator counts only when it is the word actually
// being invoked as a command — the head of a simple command, optionally
// reached through variable assignments (`VAR=1 python3`), the `env`/
// `command`/`exec`/`xargs` wrappers, and/or a leading path
// (`/usr/bin/python3`) — and not when it is an operand of some OTHER command
// (`ls`, `cat`, `test -x`, `wc`). Kept in exact parity with the Python.
const CMD_BOUNDARY = /[;&|(`"'\n]|\$\(/g;
const CMD_PREFIX_ALLOWED =
  /^\s*(?:\w+=\S*\s+)*(?:(?:env|command|exec|xargs(?:\s+-\S+)*)\s+)*(?:[\w./-]*\/)?$/;

function isCommandPosition(text, pos) {
  const prefix = text.slice(0, pos);
  let lastBoundaryEnd = 0;
  CMD_BOUNDARY.lastIndex = 0;
  let m;
  while ((m = CMD_BOUNDARY.exec(prefix))) {
    lastBoundaryEnd = m.index + m[0].length;
    if (m.index === CMD_BOUNDARY.lastIndex) CMD_BOUNDARY.lastIndex += 1;
  }
  const segment = prefix.slice(lastBoundaryEnd);
  return CMD_PREFIX_ALLOWED.test(segment);
}
// git subcommands that change a tree or its refs.
const GIT_MUTATORS =
  /(?<![\w-])git\b[^|;&]*?(?<![\w-])(commit|add|rm|mv|checkout|switch|restore|reset|revert|merge|rebase|cherry-pick|apply|am|stash|push|clean|gc|prune|worktree\s+(add|remove|prune)|branch\s+-[dDmM]|tag|config|update-ref|symbolic-ref)(?![\w-])/;

// ── BOUNDARY: RESOLVE, THEN COMPARE ────────────────────────────────────────
//
// JS has no os.path.normpath. Written out rather than reached for through
// node:path so the predicate stays readable beside the Python it mirrors.
export function normPath(p) {
  const isAbs = p.startsWith("/");
  const out = [];
  for (const part of p.split("/")) {
    if (part === "" || part === ".") continue;
    if (part === "..") {
      if (out.length && out[out.length - 1] !== "..") out.pop();
      else if (!isAbs) out.push("..");
      continue;
    }
    out.push(part);
  }
  const joined = out.join("/");
  if (isAbs) return "/" + joined;
  return joined === "" ? "." : joined;
}

// node's realpathSync THROWS on a missing path where Python's os.path.realpath
// resolves as far as it can and returns the rest untouched. `WORKTREES` for
// one lane does not exist on disk today, so this difference is live,
// not hypothetical: walk up to the deepest existing ancestor and re-attach the
// tail, which is what Python does and what keeps the two halves in step.
//
// A DANGLING LINK IS FOLLOWED, NOT WALKED PAST. realpathSync also throws on a
// symlink whose target does not exist, and the walk used to step over it and
// re-attach the link's OWN name — so `<link> -> <repo>/absent` resolved to
// `<link>`, outside every root, and `echo x > <link>` / `mkdir <link>`, which
// create the target INSIDE the repo, were allowed. os.path.realpath follows it
// (measured 2026-09-29: py=deny js=allow on the live brain lane). So when the
// deepest existing head is a link, resolve its target with the tail re-attached,
// bounded the way the kernel bounds it (ELOOP); a loop answers as written.
const MAX_LINK_HOPS = 40;

function followDangling(head, tail, hops, again) {
  if (hops >= MAX_LINK_HOPS) return null;
  let target;
  try {
    if (!lstatSync(head).isSymbolicLink()) return null;
    target = readlinkSync(head);
  } catch {
    return null;
  }
  return again(target, tail, hops + 1);
}

function realPath(p, hops = 0) {
  if (NATIVE_WINDOWS) return realPathWindows(p, hops);
  if (WINPATHS && !p.startsWith("/")) return canon(p);
  if (!p.startsWith("/")) return normPath(p);
  const norm = normPath(p);
  const parts = norm.split("/").filter(Boolean);
  for (let keep = parts.length; keep >= 0; keep -= 1) {
    const head = "/" + parts.slice(0, keep).join("/");
    const tail = parts.slice(keep);
    try {
      const resolved = realpathSync(head);
      return canon(tail.length ? normPath(resolved + "/" + tail.join("/")) : resolved);
    } catch {
      const followed = followDangling(head, tail, hops, (target, rest, n) => {
        // A relative target counts its `..` from where the link REALLY lives,
        // as the kernel does — not from its lexical parent, which may itself
        // have been reached through a symlink.
        const abs = target.startsWith("/") ? target : realPath(dirname(head), n) + "/" + target;
        return realPath(rest.length ? abs + "/" + rest.join("/") : abs, n);
      });
      if (followed !== null) return followed;
      // keep walking up
    }
  }
  return canon(norm);
}

// A home as bash spells it: forward slashes, drive as a leading segment, case
// kept. Elsewhere, as given.
function shellHome(h) {
  if (!WINPATHS || typeof h !== "string") return h;
  const f = h.replaceAll("\\", "/");
  const m = /^([A-Za-z]):(\/|$)/.exec(f);
  return m ? "/" + m[1].toLowerCase() + f.slice(2) : f;
}

// The same walk on the native spelling: realpathSync.native of the deepest
// existing ancestor, the rest re-attached, the answer folded back to canonical.
// .native, because only the OS call expands an 8.3 short name (RUNNER~1) to the
// long one: two spellings of one directory would otherwise stay two.
function realPathWindows(p, hops = 0) {
  const c = canon(p);
  const n = typeof c === "string" ? nativeOf(c) : null;
  if (n === null) return c;
  const drive = n.slice(0, 3);
  const parts = n.slice(3).split("/").filter(Boolean);
  for (let keep = parts.length; keep >= 0; keep -= 1) {
    const head = drive + parts.slice(0, keep).join("/");
    const tail = parts.slice(keep);
    try {
      const resolved = realpathSync.native(head);
      return canon(tail.length ? resolved + "/" + tail.join("/") : resolved);
    } catch {
      // The same dangling-link follow as `realPath`, on the native spelling.
      const followed = followDangling(head, tail, hops, (target, rest, k) => {
        const t = target.replace(/\\/g, "/");
        const abs = DRIVE.test(t) ? t : t.startsWith("/") ? drive.slice(0, 2) + t
          : nativeOf(realPathWindows(canon(dirname(head)), k)) + "/" + t;
        return realPathWindows(canon(rest.length ? abs + "/" + rest.join("/") : abs), k);
      });
      if (followed !== null) return followed;
      // keep walking up
    }
  }
  return c;
}

// The forms a path may be recognised under: normalised, and resolved. Extra
// candidates and extra roots can only make `insideAny` MORE true, so this
// direction never opens a hole — which is why both sides are resolved even
// though no protected root is a symlink today (checked 2026-09-06).
function resolveRoots(roots) {
  const out = [];
  for (const root of roots) {
    if (!out.includes(root)) out.push(root);
    const real = realPath(root);
    if (!out.includes(real)) out.push(real);
  }
  return out;
}

// The forms a path may be recognised under: normalised, and resolved. The
// resolved candidate is ABSOLUTE-ONLY: this plugin's cwd has nothing to do with
// the shell's, so resolving a relative path would answer a boundary question
// about a path that was never named. Every caller already passes an absolute
// path; this makes the requirement structural rather than remembered.
function candidates(p) {
  const out = [WINPATHS ? canon(p) : normPath(p)];
  if (!isAbs(p)) return out;
  const real = realPath(p);
  if (!out.includes(real)) out.push(real);
  return out;
}

// dev:ino of every protected root that exists. Windows folds case in `canon`
// instead, so this stays empty there.
const idOf = (p) => {
  const st = statSync(p, { bigint: true, throwIfNoEntry: false });
  return st ? `${st.dev}:${st.ino}` : null;
};
function rootIds(roots) {
  const ids = new Set();
  if (WINPATHS) return ids;
  for (const root of roots) {
    let id = null;
    try { id = idOf(root); } catch { /* not statable: no identity */ }
    if (id !== null) ids.add(id);
  }
  return ids;
}

// True when the deepest EXISTING ancestor of `cand`, or any directory above it,
// IS a protected root: the same inode, whatever the spelling. See `_same_tree`
// in `deny_repo_writes.py`: APFS keeps two spellings of a name that differ
// only in case as one directory and no text comparison sees it
// (measured 2026-09-29, allowed by every driver). Only adds matches; a root
// that does not exist has no identity to match.
function sameTree(cfg, cand) {
  if (cfg.rootIds.size === 0 || !isAbs(cand)) return false;
  let p = normPath(cand);
  for (;;) {
    let id = null;
    try { id = idOf(p); } catch { /* not statable: keep climbing */ }
    if (id !== null && cfg.rootIds.has(id)) return true;
    const parent = dirname(p);
    if (parent === p) return false;
    p = parent;
  }
}

function insideAny(cfg, p) {
  if (!p) return false;
  const cands = candidates(p);
  for (const cand of cands) {
    for (const root of cfg.resolvedRoots) {
      if (cand === root || cand.startsWith(root + "/")) return true;
    }
  }
  return cands.some((cand) => sameTree(cfg, cand));
}

// Absolute-path-looking runs in a command. Deliberately crude: it is used only
// to ASK the boundary question of a token, and `insideAny` answers it. A token
// that is not really a path resolves to something outside the trees and changes
// nothing.
const ABS_TOKEN = /\/[^\s;|&()<>"']+/g;
const WIN_TOKEN = /(?<![\w])[A-Za-z]:[\\/][^\s;|&()<>"']*/g;

// The first absolute path in `probe` that lands inside, or null.
//
// THE GATE ITSELF WAS A SUBSTRING TEST. Measured 2026-09-06 against all eight
// pre-unification copies, with a symlink `ro-link -> app`:
// `rm -rf <ro-link>/src` and `echo probe > <ro-link>/HOLE.txt` were ALLOWED by
// every one of them. Every inner predicate had been taught to resolve, and the
// guard still let those through — because `probe.includes(repo)` decided
// whether any of them ran. Resolving the destination is pointless if the gate
// never opens.
//
// This only widens what REACHES the deny chain; the chain still decides, so a
// READ through a symlink (`rg -n foo <ro-link>/src`) matches no mutator and is
// still allowed.
export function resolvesProtected(cfg, probe) {
  const seen = new Set();
  for (const m of probe.matchAll(ABS_TOKEN)) {
    const token = m[0];
    if (seen.has(token)) continue;
    seen.add(token);
    if (insideAny(cfg, token)) return token;
  }
  // And in Windows' spelling, which has no leading `/` for ABS_TOKEN to start at.
  if (WINPATHS) {
    for (const m of probe.matchAll(WIN_TOKEN)) {
      const token = m[0];
      if (seen.has(token)) continue;
      seen.add(token);
      if (insideAny(cfg, token)) return token;
    }
  }
  // And as shell words — see the Python twin (2026-09-24): ABS_TOKEN stops at a
  // space or a quote, so a spaced root reached through a symlink never opened
  // the gate. Only adds tokens to ask about.
  for (const m of probe.matchAll(SHELL_WORD)) {
    const token = unquoteWord(m[0]);
    if (seen.has(token) || !isAbs(token)) continue;
    seen.add(token);
    if (insideAny(cfg, token)) return token;
  }
  return null;
}

// The cwd axis. Boundary-correct and `..`-normalising, unlike a bare
// startsWith: a sibling directory whose name merely BEGINS with a protected
// root (`app-notes`) is not inside it, and denying there is a
// false positive that teaches an agent to route around the guard.
export function insideProtected(cfg, dir) {
  if (!dir || !isAbs(dir)) return false;
  return insideAny(cfg, dir);
}

// True for a path strictly beneath one of the lane's worktree roots. Strictly:
// a root itself is not a worktree, and `REPO + "/.worktrees"` must never let
// `REPO` through. The trailing slash is what enforces both.
function underAWorktreeRoot(cfg, p) {
  p = canon(p);
  for (const root of cfg.worktreeRoots) {
    for (const r of [root, realPath(root)]) {
      if (p.startsWith(r + "/")) return true;
    }
  }
  return false;
}

// ── NARROW EXCEPTION: tearing down a spent worktree ────────────────────────
//
// `hw done` closes the space but leaves the worktree and its branch on disk. It
// left the brainer asking the operator to run two git commands after every
// task — the chore this whole setup exists to remove.
//
// Only two forms pass, and only because each REFUSES by itself when the thing
// it deletes still holds work: `git worktree remove <path>` on a dirty
// worktree, `git branch -d` on an unmerged branch. `--force`/`-f`/`-D` and
// `worktree prune` stay denied. An optional `-C <dir>` is permitted because
// `git worktree remove` must run from inside the registering repository.
//
// GATED ON `worktreeRoots` BEING NON-EMPTY — three of four lanes never had it.
const ALLOWED_TEARDOWN =
  /^\s*git\s+(?:-C\s+(?<c>[^\s;|&]+)\s+)?(?:worktree\s+remove\s+(?<wt>[^\s;|&]+)\s*|branch\s+-d\s+(?<br>[^;|&]+?)\s*)$/;
const FORCE = /(?<![\w-])(--force|-f|-D)(?![\w-])/;

function expandUser(p, home) {
  if (p === "~") return home;
  if (p.startsWith("~/")) return home + p.slice(1);
  return p;
}

function isSpentWorktreeTeardown(cfg, probe, home) {
  if (!cfg.worktreeRoots.length) return false;
  if (FORCE.test(probe)) return false;
  const m = ALLOWED_TEARDOWN.exec(probe);
  if (!m) return false;
  const g = m.groups ?? {};
  // A RELATIVE PATH REFUSES THE EXEMPTION — see the Python's note: resolving it
  // would use this process's cwd, not the shell's.
  if (g.c !== undefined && g.c !== null) {
    const raw = expandUser(g.c.replace(/^["']|["']$/g, ""), home);
    if (!isAbs(raw)) return false;
    const c = realPath(raw);
    if (c !== realPath(cfg.repo) && c !== cfg.repo && !underAWorktreeRoot(cfg, c)) {
      return false;
    }
  }
  if (g.wt === undefined || g.wt === null) return true; // `git branch -d`
  const rawWt = expandUser(g.wt.replace(/^["']|["']$/g, ""), home);
  if (!isAbs(rawWt)) return false;
  return underAWorktreeRoot(cfg, realPath(rawWt));
}

// ── WHAT IS DATA AND WHAT IS CODE ──────────────────────────────────────────
//
// `rg -n "cp " <REPO>/x` is a search pattern, not an invocation of `cp`, and a
// heredoc's prose is not command text. Denying those is a FALSE POSITIVE, and a
// false positive teaches the agent to route around the guard. Only one
// lane's Python copy carried this; all four lanes and both runtimes
// carry it now.
//
// The two escape hatches are what make masking safe: a quote that IS a script
// (`bash -c "rm x"`) and a heredoc that FEEDS a shell (`bash <<EOF`) stay fully
// visible to every regex.
const HEREDOC_START = /<<(-)?\s*(['"]?)(\w+)\2/;
const SHELL_INTERPRETERS = "(?:bash|sh|zsh|dash|ksh|ash)";
const SHELL_WRAPPERS = "(?:(?:env|command|exec)\\s+)*";
const SHELL_DASH_C = new RegExp(
  "(?:^|[|;&(])\\s*" + SHELL_WRAPPERS + "(?:[\\w./-]*/)?" +
  SHELL_INTERPRETERS + "\\b(?:\\s+-[\\w-]+)*\\s+-c\\s*$");
// `eval` runs its words as one script, every quoted word included — see the
// Python half (Judgment Day of guard-gate-relative-paths, 2026-09-30).
const SHELL_WORD_PART =
  "(?:[^\\s|;&()<>'\"\\\\]|\\\\[\\s\\S]|\"(?:[^\"\\\\]|\\\\[\\s\\S])*\"|'[^']*')";
const EVAL_ARGS = new RegExp(
  "(?:^|[|;&(`\\n])\\s*" + SHELL_WRAPPERS +
  "(?:(?:\\{|!|if|then|else|elif|do|while|until|time|builtin)\\s+)*" +
  "(?:[\\w./-]*/)?eval(?:\\s+" + SHELL_WORD_PART + "+)*\\s+" +
  SHELL_WORD_PART + "*$");
// bash deletes an UNESCAPED backslash-newline before it splits words — see the
// Python half. Not an escaped one, not inside single quotes, and not at the end
// of a `#` comment: bash ends the comment there and runs the next line. Where
// that quote-tracking scan differs from the quote-blind join, BOTH are scanned
// after a hard boundary, so a misread quote (`$'\''`) can only add a denial.
// Each copy is masked on its own, or a comment's quote pairs across the two
// See the Python half.
const WORD_START = " \t\n;&|()<>";
const LINE_CONTINUATION = /(?<!\\)((?:\\\\)*)\\\n/g;
const COPY_BOUNDARY = "\n;\n";
export function joinContinuations(text) {
  const aware = joinOutsideComments(text);
  const blind = text.replace(LINE_CONTINUATION, "$1");
  return aware === blind ? [aware] : [aware, blind];
}
function maskCopy(text) {
  // Unbalanced quotes: the masking is not trustworthy, so do not mask.
  const quoted = maskQuotes(text);
  return quoted.balanced ? quoted.masked : text;
}
function joinOutsideComments(text) {
  const out = [];
  const n = text.length;
  let i = 0;
  let quote = null;
  while (i < n) {
    const c = text[i];
    if (quote === "'") {
      if (c === "'") quote = null;
    } else if (c === "\\" && i + 1 < n) {
      if (text[i + 1] !== "\n") out.push(c + text[i + 1]);
      i += 2;
      continue;
    } else if (c === '"') {
      quote = quote === '"' ? null : '"';
    } else if (quote === null && c === "'") {
      quote = "'";
    } else if (quote === null && c === "#" && (!out.length || WORD_START.includes(out[out.length - 1].slice(-1)))) {
      let j = text.indexOf("\n", i);
      if (j < 0) j = n;
      out.push(text.slice(i, j));
      i = j;
      continue;
    }
    out.push(c);
    i += 1;
  }
  return out.join("");
}
// CONSUMERS WHOSE HEREDOC BODY CAN DETERMINE A DESTINATION — see the Python's
// note for the measurement: `xargs -I{} rm {} <<EOF` runs `rm` once per LINE
// of its stdin, `patch`'s destination is the `+++ b/<path>` header inside the
// diff it is fed, `ed`/`ex` read editing commands (including `w <path>`) from
// stdin, and a general interpreter's script can read its own stdin and act on
// it in ways this guard cannot parse away.
const HEREDOC_BODY_IS_LIVE =
  "(?:bash|sh|zsh|dash|ksh|ash|xargs|patch|ed|ex|" +
  "python|python3|node|ruby|perl|php|deno|bun|osascript)";
const SHELL_HEREDOC_TARGET = new RegExp(
  "(?:^|[|;&(])\\s*" + SHELL_WRAPPERS + "(?:[\\w./-]*/)?" +
  HEREDOC_BODY_IS_LIVE + "\\b(?=\\s|<<|$)");

function escapeRe(s) {
  return s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

// Drop heredoc body lines that are DATA — keep the ones that are CODE. A
// heredoc fed to a shell interpreter IS command text: the shell executes every
// line, so it stays in the scan. Line-based, not a real shell parser: good
// enough for `<<DELIM` … `DELIM`, `<<-DELIM` (leading tabs) and `<<'EOF'`.
export function stripHeredocBodies(text) {
  const lines = text.split("\n");
  const out = [];
  let i = 0;
  while (i < lines.length) {
    const line = lines[i];
    out.push(line);
    const m = HEREDOC_START.exec(line);
    if (!m) {
      i += 1;
      continue;
    }
    const dash = Boolean(m[1]);
    const delim = m[3];
    const closeRe = new RegExp("^" + (dash ? "\\t*" : "") + escapeRe(delim) + "\\s*$");
    const feedsShell = SHELL_HEREDOC_TARGET.test(line);
    i += 1;
    while (i < lines.length && !closeRe.test(lines[i])) {
      if (feedsShell) out.push(lines[i]); // code the shell executes
      i += 1;
    }
    if (i < lines.length) {
      out.push(lines[i]);
      i += 1;
    }
  }
  return out.join("\n");
}

// Blank the interior of quoted strings that are DATA, not CODE. The quote marks
// themselves survive, so a `>` right before a quoted redirect target is still
// visible. A quote immediately after a shell `-c` is left unmasked: it IS the
// script.
//
// A TOP-LEVEL BACKSLASH ESCAPES THE NEXT CHARACTER — see the Python's note for
// the measurement. `echo \' && rm -rf <REPO>/x \'` contains no real quote as far
// as bash is concerned, and without this the scanner blanked the `rm` out of the
// text every deny regex reads. This runtime never masked at all before, so the
// escape has to arrive with the masking, not after it.
// Returns {masked, balanced}. An UNTERMINATED quote blanks everything from the
// opening quote to the end of the text — a general-purpose way to hide a command
// from every deny regex. bash refuses to run such a line, but "the shell would
// have rejected it anyway" is a claim about another program's parser, and this
// guard does not lean on one: the caller falls back to the unmasked text.
export function maskQuotes(text) {
  const out = [];
  let balanced = true;
  const hasEval = text.includes("eval"); // the per-quote eval scan costs nothing without one
  let i = 0;
  const n = text.length;
  while (i < n) {
    const c = text[i];
    if (c === "\\" && i + 1 < n) {
      // Escaped: emit both characters and let NEITHER open a quote.
      out.push(c);
      out.push(text[i + 1]);
      i += 2;
      continue;
    }
    if (c === "'" || c === '"') {
      const quote = c;
      const quoteStart = i;
      out.push(c);
      i += 1;
      const start = i;
      while (i < n && text[i] !== quote) {
        if (quote === '"' && text[i] === "\\" && i + 1 < n) {
          i += 2;
          continue;
        }
        i += 1;
      }
      const interior = text.slice(start, i);
      const before = text.slice(0, quoteStart);
      if (SHELL_DASH_C.test(before) || (hasEval && EVAL_ARGS.test(before))) out.push(interior);
      else out.push(" ".repeat(interior.length));
      if (i < n) {
        out.push(text[i]);
        i += 1;
      } else {
        balanced = false; // ran off the end looking for the closer
      }
    } else {
      out.push(c);
      i += 1;
    }
  }
  return { masked: out.join(""), balanced };
}

// ── NARROW EXCEPTION: copying OUT of a protected tree ──────────────────────
//
// `hw done` tells the operator "Move what you need out first" before a worktree
// is reaped, and this guard denied exactly that move, stranding the artifacts in
// the one place it was just said they would not survive.
//
// The cause: the guard asked whether a mutating command NAMES a protected path,
// never WHERE in the argument list that path sits. In `cp SRC DST` only DST is
// written; a protected path in SRC is a READ, and reading is allowed.
//
// `mv` is deliberately absent from the tables below. Moving out of a protected
// tree REMOVES the source, so BOTH operands are write targets and it stays
// denied. The same reasoning excludes `rsync --remove-source-files`.
//
// Fail-closed by construction: SOURCE operands are the only tokens this never
// inspects, and a source is whatever is LEFT once argv[0], every option, every
// option value and the final destination have each been individually recognised
// and cleared. Anything unaccounted for returns a refusal.

// argv[0], with any leading directory stripped.
const COPY_LIKE = /^(?:[^\s/]*\/)*(cp|rsync|install)$/;

// A metacharacter means this is not one simple command, and parsing shell to
// decide that a write is safe is how a guard stops being one. A surviving `$`
// counts too: after expansion it means a variable this plugin cannot resolve.
const NOT_SIMPLE = /[<>|;&()`\n$]/;

// Options that decide WHICH operand is the destination, or that delete the
// source. Refused outright rather than parsed.
const COPY_ORDER_FLAGS = {
  cp: new Set(["-t", "--target-directory"]),
  install: new Set(["-t", "--target-directory", "-d", "--directory"]),
  rsync: new Set(["--remove-source-files"]),
};

// Per command: [self-contained short letters, value-taking short letters,
// self-contained long options, value-taking long options]. An ALLOWLIST: an
// option not in it refuses the exemption rather than being guessed at, because
// guessing is how an option VALUE gets mistaken for a source operand — and an
// option value can be a write target (`rsync --backup-dir DIR`) while a source
// operand never is.
const COPY_FLAGS = {
  cp: [
    "abcdfHiLlnPpRrsTuvXxZ",
    "S",
    new Set(["archive", "attributes-only", "backup", "copy-contents", "debug",
      "dereference", "force", "interactive", "link", "no-clobber",
      "no-dereference", "no-preserve", "no-target-directory",
      "one-file-system", "parents", "preserve", "recursive", "reflink",
      "remove-destination", "sparse", "strip-trailing-slashes",
      "symbolic-link", "update", "verbose"]),
    new Set(["suffix"]),
  ],
  install: [
    "bcCDpsTvZ",
    "mogS",
    new Set(["backup", "compare", "no-target-directory", "preserve-timestamps",
      "preserve-context", "strip", "verbose"]),
    new Set(["mode", "owner", "group", "suffix", "strip-program", "context"]),
  ],
  rsync: [
    "aAcdDgGhHiklLmnoOpPqrRsStuUvWxXzZ0",
    "efMBT",
    new Set(["archive", "checksum", "compress", "delete", "delete-after",
      "delete-before", "delete-during", "delete-excluded", "dirs", "dry-run",
      "existing", "group", "hard-links", "human-readable", "ignore-existing",
      "ignore-times", "itemize-changes", "links", "no-perms", "numeric-ids",
      "omit-dir-times", "one-file-system", "owner", "partial", "perms",
      "progress", "prune-empty-dirs", "quiet", "recursive", "relative",
      "size-only", "sparse", "stats", "times", "update", "verbose",
      "whole-file"]),
    new Set(["exclude", "include", "exclude-from", "include-from", "files-from",
      "filter", "rsh", "rsync-path", "chmod", "chown", "timeout", "bwlimit",
      "max-size", "min-size", "block-size", "temp-dir", "backup-dir", "suffix",
      "compare-dest", "copy-dest", "link-dest", "log-file", "partial-dir",
      "info", "debug", "out-format"]),
  ],
};

// JS has no shlex either. POSIX-mode word splitting, and it THROWS on an
// unbalanced quote — which the caller turns into a refusal, matching the
// Python's ValueError branch. NOT_SIMPLE has already rejected $, backtick and
// every other metacharacter before this runs, so the remaining surface is
// quoting alone.
export function shlexSplit(s) {
  const out = [];
  let cur = "";
  let started = false;
  let i = 0;
  while (i < s.length) {
    const c = s[i];
    if (c === "\\") {
      if (i + 1 >= s.length) throw new Error("No escaped character");
      cur += s[i + 1];
      started = true;
      i += 2;
      continue;
    }
    if (c === "'") {
      const j = s.indexOf("'", i + 1);
      if (j < 0) throw new Error("No closing quotation");
      cur += s.slice(i + 1, j);
      started = true;
      i = j + 1;
      continue;
    }
    if (c === '"') {
      i += 1;
      let closed = false;
      while (i < s.length) {
        if (s[i] === "\\" && i + 1 < s.length && "\"\\$`".includes(s[i + 1])) {
          cur += s[i + 1];
          i += 2;
          continue;
        }
        if (s[i] === '"') {
          closed = true;
          i += 1;
          break;
        }
        cur += s[i];
        i += 1;
      }
      if (!closed) throw new Error("No closing quotation");
      started = true;
      continue;
    }
    if (/\s/.test(c)) {
      if (started) {
        out.push(cur);
        cur = "";
        started = false;
      }
      i += 1;
      continue;
    }
    cur += c;
    started = true;
    i += 1;
  }
  if (started) out.push(cur);
  return out;
}

// True only when `token` DEMONSTRABLY resolves outside every protected root.
// Anything it cannot resolve — a relative path with no absolute cwd to anchor
// it, an rsync `host:path` remote spec — is false, i.e. treated as protected.
// This is the one direction that must never be optimistic: it is the test
// applied to the DESTINATION operand, to every option value and to every
// redirect target, and a wrong `true` here is a real write let through.
//
// The leading substring test is deliberately blunt and deliberately kept: a
// token that merely CONTAINS a protected root refuses the exemption without
// further argument. Over-refusing a write target is the safe direction.
export function landsOutsideProtected(cfg, token, cwd) {
  if (cfg.roots.some((root) => token.includes(root) || canonText(token).includes(root))) return false;
  if (!isAbs(token)) {
    // `host:path` / `rsync://` — a remote spec this plugin cannot resolve.
    if (token.split("/")[0].includes(":")) return false;
    if (!cwd || !isAbs(cwd)) return false;
    token = joinPath(cwd, token);
  }
  return !insideAny(cfg, token);
}

// Split argv[1:] into operands, or return null if anything is unrecognised.
// null means "this plugin does not understand this command line", which is
// always a refusal — never an allow.
export function copyOperands(cfg, argv, name, cwd) {
  const [selfShort, valueShort, selfLong, valueLong] = COPY_FLAGS[name];
  const orderFlags = COPY_ORDER_FLAGS[name];
  const operands = [];
  let i = 1;
  const n = argv.length;
  while (i < n) {
    const tok = argv[i];
    if (tok === "--") {
      operands.push(...argv.slice(i + 1));
      return operands;
    }
    if (!tok.startsWith("-") || tok === "-") {
      operands.push(tok);
      i += 1;
      continue;
    }
    if (tok.startsWith("--")) {
      const eq = tok.indexOf("=");
      const head = eq === -1 ? tok : tok.slice(0, eq);
      const attached = eq === -1 ? null : tok.slice(eq + 1);
      if (orderFlags.has(head)) return null;
      const longName = head.slice(2);
      if (attached !== null) {
        // Attached value: never confusable with an operand, but it can still
        // BE a write target, so it is checked like one.
        if (!selfLong.has(longName) && !valueLong.has(longName)) return null;
        if (!landsOutsideProtected(cfg, attached, cwd)) return null;
        i += 1;
        continue;
      }
      if (selfLong.has(longName)) {
        i += 1;
        continue;
      }
      if (valueLong.has(longName)) {
        if (i + 1 >= n) return null;
        if (!landsOutsideProtected(cfg, argv[i + 1], cwd)) return null;
        i += 2;
        continue;
      }
      return null;
    }
    // Short cluster, e.g. `-Rv`, `-m644`, `-m 644`.
    if (orderFlags.has(tok)) return null;
    let j = 1;
    let consumedValue = false;
    while (j < tok.length) {
      const letter = tok[j];
      if (orderFlags.has("-" + letter)) return null;
      if (valueShort.includes(letter)) {
        const rest = tok.slice(j + 1);
        if (rest) {
          if (!landsOutsideProtected(cfg, rest, cwd)) return null;
          i += 1;
        } else {
          if (i + 1 >= n) return null;
          if (!landsOutsideProtected(cfg, argv[i + 1], cwd)) return null;
          i += 2;
        }
        consumedValue = true;
        break;
      }
      if (!selfShort.includes(letter)) return null;
      j += 1;
    }
    if (!consumedValue) i += 1;
  }
  return operands;
}

// [allowed, note] — allowed only for a copy whose destination is outside.
// `note` explains a refusal, and is empty when the command is not copy-shaped
// at all. It goes into the deny message so a denied `cp` says WHY the
// source-vs-destination exception did not apply.
export function isCopyOutOfProtected(cfg, probe, cwd) {
  if (NOT_SIMPLE.test(probe)) {
    const words = probe.split(/\s+/).filter(Boolean);
    const head = words.length ? words[0] : "";
    if (COPY_LIKE.test(head)) {
      return [false,
        " This looks like a copy, but it is not one simple command (it contains" +
        " a redirect, a pipe, a chain, a substitution or an unexpanded" +
        " variable), so the source-vs-destination exception does not apply —" +
        " run the copy on its own."];
    }
    return [false, ""];
  }
  let argv;
  try {
    argv = shlexSplit(probe);
  } catch {
    return [false, ""];
  }
  if (argv.length === 0) return [false, ""];
  const m = COPY_LIKE.exec(argv[0]);
  if (!m) return [false, ""];
  // Only an argv[0] that NAMES a path can name a protected one; a bare `cp` is
  // resolved through PATH, and resolving it against the cwd instead would
  // misreport "the binary is in the repo" for every copy run from inside one.
  if (argv[0].includes("/") && !landsOutsideProtected(cfg, argv[0], cwd)) {
    return [false, ` The \`${argv[0]}\` being run is itself inside a protected tree.`];
  }
  const name = m[1];
  const operands = copyOperands(cfg, argv, name, cwd);
  if (operands === null) {
    return [false,
      ` This is a \`${name}\`, but it uses an option this guard does not parse` +
      ` (or one that moves the destination, like \`-t\`), so it cannot tell` +
      ` source from destination and refuses rather than guess. A plain` +
      ` \`${name} -R <src> <dst>\` is exempt when only the source is protected.`];
  }
  if (operands.length < 2) {
    return [false,
      ` This is a \`${name}\` with fewer than two operands, so there is no` +
      ` destination to check.`];
  }
  const dest = operands[operands.length - 1];
  if (!landsOutsideProtected(cfg, dest, cwd)) {
    return [false,
      ` The DESTINATION operand (${dest}) is inside a protected tree. Copying` +
      ` OUT is allowed; copying IN is not.`];
  }
  return [true, ""];
}

// {raw, resolved} for the first redirect target that lands inside a
// protected root, or null if every target resolves outside. Reading FROM a
// protected tree INTO brain used to be denied — a false positive that only
// teaches an agent to route around the guard.
//
// Every copy used to answer this with its own `t.startsWith(REPO)` — a raw
// substring test on a WRITE TARGET, exactly what `landsOutsideProtected` exists
// to refuse. They are one question now, so a symlinked or `..`-laden target is
// resolved rather than pattern-matched, and a relative target resolves against
// the cwd instead of being blanket-denied. An unresolvable target still denies.
//
// RETURNING THE RESOLVED PATH, not just a boolean — see the Python's note:
// with only a boolean, the deny message fell back to "the command names a
// protected root" even when the match came from resolving `$HOME` or a
// symlink, describing a literal-string test this branch does not run.
// A REDIRECT TARGET IS A SHELL WORD — see the Python twin (2026-09-24). The
// targets are a UNION: the base pattern's, exactly as before, plus the word
// after every `>` outside quotes. The first draft replaced the base pattern and
// was fail-open (Judgment Day, judge B): a `>` inside a quoted string swallowed
// the real redirect. A command can only gain targets, never lose one.
const WORD = String.raw`(?:"[^"]*"|'[^']*'|\\.|[^\s;|&()<>\\])+`;
const SHELL_WORD = new RegExp(WORD, "g");
const BARE_REDIRECT_TARGET = /(?<![0-9&])>{1,2}(?!&)\s*([^\s;|&()<>]+)/g;
function unquoteWord(word) {
  return word.replace(/"([^"]*)"|'([^']*)'|\\(.)|["']/g,
    (_m, dq, sq, esc) => dq ?? sq ?? esc ?? "");
}
function redirectTargets(probe) {
  const targets = [...probe.matchAll(BARE_REDIRECT_TARGET)].map((m) => m[1].replace(/^["']|["']$/g, ""));
  const word = new RegExp(WORD, "y");
  let i = 0, quote = null;
  const n = probe.length;
  while (i < n) {
    const c = probe[i];
    if (quote) {
      if (c === quote) quote = null;
      else if (c === "\\" && quote === '"') i += 1;
      i += 1;
      continue;
    }
    if (c === '"' || c === "'") quote = c;
    else if (c === "\\") i += 1;
    else if (c === ">" && !(i && (/[0-9]/.test(probe[i - 1]) || probe[i - 1] === "&"))) {
      let j = i + 1;
      if (j < n && probe[j] === ">") j += 1;
      if (j < n && probe[j] === "&") { i = j + 1; continue; }
      while (j < n && (probe[j] === " " || probe[j] === "\t")) j += 1;
      word.lastIndex = j;
      const m = word.exec(probe);
      if (m) targets.push(unquoteWord(m[0]));
      i = j;
      continue;
    }
    i += 1;
  }
  return targets;
}

export function redirectLandsInProtected(cfg, probe, cwd, segs = []) {
  for (const t of redirectTargets(probe)) {
    if (!landsOutsideProtected(cfg, t, cwd)) {
      let resolved = isAbs(t) ? t
        : (WINPATHS ? joinPath(cwd || ".", t) : normPath((cwd || ".").replace(/\/+$/, "") + "/" + t));
      if (isAbs(resolved)) resolved = realPath(resolved);
      return { raw: t, resolved };
    }
  }
  // And again from where each command really runs, with its variables
  // expanded — see the Python twin. Only adds targets.
  for (const seg of segs) {
    for (const [value, raw] of seg.redirects) {
      const landed = wordLands(cfg, value, seg.cwd, true);
      if (landed) return { raw, resolved: realPath(landed) };
    }
  }
  return null;
}

// ── WHERE A WORD REALLY LANDS: RELATIVE, AFTER A cd, THROUGH A VARIABLE ─────
//
// The Python twin carries the measurement (2026-09-29, audit F5-F8): the gate
// asked only absolute tokens, a redirect resolved against the payload's cwd
// and never the one a `cd` had just left, `$D` was never expanded, and globs
// and braces were compared as text. This is the same reading, step for step:
// each simple command's words and write-redirect targets, its own
// assignments and the environment expanded, braces expanded, and the
// directory it runs in after every `cd`/`pushd` before it. Everything here
// only ADDS: it opens the gate on more commands and gives the redirect check
// more targets; the chain still decides.
const MARK = "\u0000";
const GLOB_CHARS = /[*?[]/;
const WRITE_OPS = new Set([">", ">>", ">|", "&>", "&>>", ">&", "<>"]);
const REDIRECT_OP = /(\d*)(&>>|&>|>>|>\||>&|<>|<<<|<<-|<<|<&|>|<)/y;
const ASSIGNMENT = /^[A-Za-z_][A-Za-z0-9_]*=/;
const DECLARERS = new Set(["export", "local", "declare", "typeset", "readonly"]);
// Words that precede a command without being it (Judgment Day, 2026-09-29).
// Two lists, because they answer two questions. SHELL_WORDS are the shell's own
// reserved words and builtins: a `cd` reached through them runs IN the shell and
// moves its cwd. `env`, `nohup` and `exec` are external commands: `env cd /tmp`
// runs cd in a child and the shell stays where it was (measured 2026-09-29,
// Judgment Day re-judgment: round 1 skipped them for the cd and
// `env cd /tmp; rm -rf ../<repo>/src` went from DENY to ALLOW). They stay
// skippable only where that is harmless: finding a shell head for `-c` nesting,
// since `env bash -c '...'` is real code.
const SHELL_WORDS = new Set([
  "{", "}", "!", "if", "then", "else", "elif", "do", "while", "until",
  "time", "builtin", "command"]);
const SKIP_WORDS = new Set([...SHELL_WORDS, "env", "exec", "nohup"]);
const SHELLS = new Set(["sh", "bash", "zsh", "dash", "ksh", "ash"]);
const headIndex = (words, skip = SHELL_WORDS) => {
  let k = 0;
  while (k < words.length && (ASSIGNMENT.test(words[k][0]) || skip.has(words[k][0]))) k += 1;
  return k;
};
const MAX_NESTING = 16;
const normOf = (p) => (WINPATHS ? canon(p) : normPath(p));

// `a{b,c}d` -> [abd, acd], nested and repeated. Bounds the WORK, not just the
// output (Judgment Day, 2026-09-29): recursing into every alternative and
// truncating afterwards cost 2**k calls for k groups, a stalled hook that a
// timeout does not turn into a refusal. `limit` goes down and the loop stops
// once it is met. Same as the Python.
function brace(word, limit = 64) {
  const m = /\{([^{}]*,[^{}]*)\}/.exec(word);
  if (!m) return [word];
  let out = [];
  for (const alt of m[1].split(",")) {
    if (out.length >= limit) break;
    out = out.concat(brace(word.slice(0, m.index) + alt + word.slice(m.index + m[0].length), limit - out.length));
  }
  return out.slice(0, limit);
}

function closeParen(t, i) {
  let depth = 0, j = i;
  const n = t.length;
  while (j < n) {
    const ch = t[j];
    if (ch === "\\") { j += 2; continue; }
    if (ch === "'") {
      const k = t.indexOf("'", j + 1);
      j = k < 0 ? n : k + 1;
      continue;
    }
    if (ch === '"') {
      j += 1;
      while (j < n && t[j] !== '"') j += t[j] === "\\" ? 2 : 1;
      j += 1;
      continue;
    }
    if (ch === "(") depth += 1;
    else if (ch === ")") {
      depth -= 1;
      if (depth === 0) return j;
    }
    j += 1;
  }
  return n;
}

class Segments {
  constructor(text, at, home, local, segs, depth, origin) {
    Object.assign(this, { t: text, at, home, local, segs, depth, origin: origin === undefined ? at : origin });
    this.words = []; this.redirects = [];
    this.buf = []; this.start = 0; this.have = false; this.pending = null;
  }

  variable(name) {
    if (Object.prototype.hasOwnProperty.call(this.local, name)) return this.local[name];
    return process.env[name] ?? MARK;
  }

  nested(text) {
    if (this.depth < MAX_NESTING) {
      new Segments(text, this.at, this.home, this.local, this.segs, this.depth + 1, this.origin).run();
    }
  }

  begin(i) {
    if (!this.have) { this.start = i; this.have = true; }
  }

  dollar(i) {
    const t = this.t;
    const rest = t.slice(i);
    if (rest.startsWith("$(")) {
      const close = closeParen(t, i + 1);
      if (!rest.startsWith("$((")) this.nested(t.slice(i + 2, close));
      return [MARK, close + 1];
    }
    let m = /^\$\{([A-Za-z_][A-Za-z0-9_]*)(?:(:?[-=])([^}]*))?\}/.exec(rest);
    if (m) {
      let v = this.variable(m[1]);
      if (v === MARK && m[2]) v = m[3];
      return [v, i + m[0].length];
    }
    m = /^(?:\$\{[^}]*\}|\$[0-9@*#?$!-])/.exec(rest);
    if (m) return [MARK, i + m[0].length];
    m = /^\$([A-Za-z_][A-Za-z0-9_]*)/.exec(rest);
    if (m) return [this.variable(m[1]), i + m[0].length];
    return ["$", i + 1];
  }

  flushWord(i) {
    if (!this.have) return;
    const value = this.buf.join(""), raw = this.t.slice(this.start, i);
    if (this.pending !== null) {
      if (WRITE_OPS.has(this.pending)) this.redirects.push([value, raw]);
      this.pending = null;
    } else {
      const hk = headIndex(this.words);
      if (ASSIGNMENT.test(value) && (hk === this.words.length || DECLARERS.has(this.words[hk][0]))) {
        const eq = value.indexOf("=");
        this.local[value.slice(0, eq)] = value.slice(eq + 1);
      }
      this.words.push([value, raw]);
    }
    this.buf = []; this.have = false;
  }

  flushSeg(i) {
    this.flushWord(i);
    this.pending = null;
    const { words, redirects } = this;
    this.words = []; this.redirects = [];
    if (!(words.length || redirects.length)) return;
    this.segs.push({ words, redirects, cwd: this.at, start: this.origin });
    const k = headIndex(words, SKIP_WORDS);
    const name = k < words.length ? basename(words[k][0]) : "";
    // `sh -c '<script>'` and `eval <args>` are one word whose commands run
    // from here: read them too.
    if (SHELLS.has(name)) {
      for (let j = k + 1; j < words.length - 1; j += 1) {
        const v = words[j][0];
        if (v.startsWith("-") && !v.startsWith("--") && v.slice(1).includes("c")) {
          this.nested(words[j + 1][0]);
          break;
        }
      }
    } else if (name === "eval") {
      this.nested(words.slice(k + 1).map(([w]) => w).join(" "));
    }
    // a cd moves the shell only through the shell's own words: `env cd`,
    // `nohup cd` and `exec cd` run it in a child (see SHELL_WORDS)
    const ck = headIndex(words);
    const cname = ck < words.length ? basename(words[ck][0]) : "";
    if (cname === "cd" || cname === "pushd") {
      const args = words.slice(ck + 1).map(([w]) => w).filter((w) => !w.startsWith("-") || w === "-");
      const target = args.length ? args[0] : this.home;
      if (target === "-" || target.includes(MARK) || !target) this.at = null;
      else if (isAbs(target)) this.at = normOf(target);
      else this.at = this.at ? normOf(joinPath(this.at, target)) : null;
    } else if (cname === "popd") {
      this.at = null;
    }
  }

  run() {
    const t = this.t, n = t.length;
    let i = 0;
    while (i < n) {
      const c = t[i];
      if (c === "\\") {
        if (i + 1 < n && t[i + 1] !== "\n") { this.begin(i); this.buf.push(t[i + 1]); }
        i += 2;
        continue;
      }
      if (c === "'") {
        this.begin(i);
        let j = t.indexOf("'", i + 1);
        if (j < 0) j = n;
        this.buf.push(t.slice(i + 1, j));
        i = j + 1;
        continue;
      }
      if (c === '"') {
        this.begin(i);
        i += 1;
        while (i < n && t[i] !== '"') {
          if (t[i] === "\\" && i + 1 < n && '$`"\\\n'.includes(t[i + 1])) {
            if (t[i + 1] !== "\n") this.buf.push(t[i + 1]);
            i += 2;
          } else if (t[i] === "$") {
            const [v, next] = this.dollar(i);
            this.buf.push(v); i = next;
          } else if (t[i] === "`") {
            let j = t.indexOf("`", i + 1);
            if (j < 0) j = n;
            this.nested(t.slice(i + 1, j));
            this.buf.push(MARK); i = j + 1;
          } else {
            this.buf.push(t[i]); i += 1;
          }
        }
        i += 1;
        continue;
      }
      if (c === "$") {
        this.begin(i);
        const [v, next] = this.dollar(i);
        this.buf.push(v); i = next;
        continue;
      }
      if (c === "`") {
        this.begin(i);
        let j = t.indexOf("`", i + 1);
        if (j < 0) j = n;
        this.nested(t.slice(i + 1, j));
        this.buf.push(MARK); i = j + 1;
        continue;
      }
      if (c === "#" && !this.have) {
        const j = t.indexOf("\n", i);
        i = j < 0 ? n : j;
        continue;
      }
      if (c === "~" && !this.have && /^~(?:$|[/\s;&|)<>])/.test(t.slice(i))) {
        this.begin(i); this.buf.push(this.home); i += 1;
        continue;
      }
      let m = null;
      if ("<>".includes(c) || (c === "&" && t[i + 1] === ">") || ("0123456789".includes(c) && !this.have)) {
        REDIRECT_OP.lastIndex = i;
        m = REDIRECT_OP.exec(t);
      }
      if (m) {
        this.flushWord(i);
        const op = m[2];
        i = m.index + m[0].length;
        if (op === ">&" || op === "<&") {
          const dup = /^[ \t]*(?:\d+|-)(?=$|[\s;&|()<>])/.exec(t.slice(i));
          if (dup) { i += dup[0].length; continue; }
        }
        while (i < n && (t[i] === " " || t[i] === "\t")) i += 1;
        this.pending = op;
        continue;
      }
      if (c === " " || c === "\t") { this.flushWord(i); i += 1; continue; }
      if ("\n;|&".includes(c)) { this.flushSeg(i); i += 1; continue; }
      if (c === "(") {
        this.flushSeg(i);
        const close = closeParen(t, i);
        this.nested(t.slice(i + 1, close));
        i = close + 1;
        continue;
      }
      if (c === ")") { this.flushSeg(i); i += 1; continue; }
      this.begin(i);
      this.buf.push(c);
      i += 1;
    }
    this.flushSeg(n);
  }
}

export function shellSegments(text, cwd, home) {
  const segs = [];
  const at = cwd && isAbs(cwd) ? normOf(cwd) : null;
  new Segments(text, at, home, Object.create(null), segs, 0).run();
  return segs;
}

function globPartMatches(pattern, name) {
  const out = [];
  let i = 0;
  const n = pattern.length;
  while (i < n) {
    const c = pattern[i];
    if (c === "*") out.push("[^/]*");
    else if (c === "?") out.push("[^/]");
    else if (c === "[" && pattern.indexOf("]", i + 2) > 0) {
      const j = pattern.indexOf("]", i + 2);
      let body = pattern.slice(i + 1, j);
      if (body.startsWith("!")) body = "^" + body.slice(1);
      out.push("[" + body.replace(/\\/g, "\\\\") + "]");
      i = j;
    } else out.push(escapeRe(c));
    i += 1;
  }
  try {
    return new RegExp("^" + out.join("") + "$").test(name);
  } catch {
    return true; // a class this cannot read may match: refuse, not guess
  }
}

function globLands(cfg, pattern) {
  const pc = pattern.split("/").filter(Boolean);
  for (const root of cfg.resolvedRoots) {
    const rc = root.split("/").filter(Boolean);
    if (pc.length >= rc.length && rc.every((r, k) => globPartMatches(pc[k], r))) return true;
  }
  return false;
}

// Every word is asked bare (Judgment Day, 2026-09-29): `cd .. && rm -rf <name>`
// names the root with no `/`. It only opens the gate; the chain still decides.
function wordLands(cfg, value, cwd, bare = false) {
  for (let v of brace(value)) {
    if (v.includes(MARK)) v = v.split(MARK)[0];
    if (!v) continue;
    if (!(bare || isAbs(v) || v.includes("/") || v === "." || v === "..")) continue;
    let p;
    if (isAbs(v)) p = v;
    else if (cwd) p = joinPath(cwd, v);
    else continue;
    if (insideAny(cfg, p) || (GLOB_CHARS.test(v) && globLands(cfg, normOf(p)))) return normOf(p);
  }
  return null;
}

function segmentsResolveProtected(cfg, segs) {
  for (const seg of segs) {
    // Defence in depth: any cd this models wrongly would HIDE a word, so where
    // the tracked cwd differs from the payload's own, ask that too. Only adds
    // denials.
    const dirs = [seg.cwd];
    if (seg.start && seg.start !== seg.cwd) dirs.push(seg.start);
    for (const d of dirs) {
      for (const [value, raw] of seg.words) {
        const eq = value.indexOf("=");
        for (const v of [value, eq < 0 ? "" : value.slice(eq + 1)]) {
          const landed = v && wordLands(cfg, v, d, true);
          if (landed) return { raw, landed };
        }
      }
      for (const [value, raw] of seg.redirects) {
        const landed = wordLands(cfg, value, d, true);
        if (landed) return { raw, landed };
      }
    }
  }
  return null;
}

// ── THE DECISION ───────────────────────────────────────────────────────────
//
// A pure function of (lane, command, cwd): null to allow, or {rule, reason} to
// refuse. Split out from the hook so a harness can drive it directly; the hook
// below is the thin part that turns a refusal into opencode's only refusal
// mechanism, a throw.
// ── `env` OPTIONS ARE NOT THE PROGRAM — see `_unwrap_env` in the Python half.
// An `env` in command position is rewritten into the shape the rules already
// judge: options dropped, a -S/--split-string value spliced in as command text,
// -C/--chdir DIR as `cd DIR &&` in front, NAME=VALUE words moved before `env`.
const ENV_LONG = {
  "unset": "required", "chdir": "required", "split-string": "required",
  "argv0": "required", "ignore-environment": null, "null": null,
  "debug": null, "help": null, "version": null, "list-signal-handling": null,
  "block-signal": "optional", "default-signal": "optional",
  "ignore-signal": "optional",
};
const ENV_SHORT_VALUE = "uCSPa";
const ENV_WORD_END = new Set([" ", "\t", "\n", ";", "&", "|", "(", ")", "<", ">", "`"]);
const ENV_AT = /(?<![\w./-])(?:[\w./-]*\/)?env(?=[ \t]+\S)/g;

function envLong(name) {
  if (Object.hasOwn(ENV_LONG, name)) return name;
  const hits = Object.keys(ENV_LONG).filter((n) => n.startsWith(name));
  return hits.length === 1 ? hits[0] : null;
}

function envWord(text, i) {
  let j = i;
  const n = text.length, value = [];
  while (j < n && !ENV_WORD_END.has(text[j])) {
    const ch = text[j];
    if (ch === "\\" && j + 1 < n) { value.push(text[j + 1]); j += 2; }
    else if (ch === "'" && j > i && text[j - 1] === "$") {
      // $'…': a backslash escapes the next character, a quote included.
      let k = j + 1; const part = [];
      while (k < n && text[k] !== "'") {
        if (text[k] === "\\" && k + 1 < n) { part.push(text[k + 1]); k += 2; }
        else { part.push(text[k]); k += 1; }
      }
      value.push(part.join("")); j = k + 1;
    } else if (ch === "'") {
      let k = text.indexOf("'", j + 1);
      if (k < 0) k = n;
      value.push(text.slice(j + 1, k)); j = k + 1;
    } else if (ch === '"') {
      let k = j + 1; const part = [];
      while (k < n && text[k] !== '"') {
        if (text[k] === "\\" && k + 1 < n && '"\\$`'.includes(text[k + 1])) { part.push(text[k + 1]); k += 2; }
        else { part.push(text[k]); k += 1; }
      }
      value.push(part.join("")); j = k + 1;
    } else { value.push(ch); j += 1; }
  }
  const end = Math.min(j, n);
  return [text.slice(i, end), value.join(""), end];
}

function shQuote(s) {
  if (s === "") return "''";
  return /^[\w@%+=:,./-]+$/.test(s) ? s : "'" + s.replaceAll("'", "'\"'\"'") + "'";
}

const ENV_BOUNDARY = /[;&|(`\n]|\$\(/g;

function quoteSpans(text) {
  const spans = [];
  const n = text.length;
  let i = 0;
  while (i < n) {
    const ch = text[i];
    if (ch === "\\") { i += 2; continue; }
    if (ch === "'" || ch === '"') {
      const ansi = ch === "'" && i > 0 && text[i - 1] === "$";
      let j = i + 1;
      while (j < n && text[j] !== ch) j += text[j] === "\\" && (ch === '"' || ansi) ? 2 : 1;
      if (j >= n) return null;
      spans.push([i, j]);
      i = j + 1;
      continue;
    }
    i += 1;
  }
  return spans;
}

// The unwrap runs on a budget, and running out of it denies — see
// `_ENV_BUDGET` in the Python half, whose charges these mirror one for one.
const ENV_BUDGET = 1_000_000;

class EnvBudgetSpent extends Error {}

function spend(budget, cost) {
  budget.left -= cost;
  if (budget.left < 0) throw new EnvBudgetSpent();
}

function dashCBefore(text, a) {
  let j = a;
  while (j > 0 && /\s/.test(text[j - 1])) j -= 1;
  return j >= 2 && text.slice(j - 2, j) === "-c";
}

const EVAL_WORD = /(?<![\w.-])eval\s/;

function unwrapEnvOnce(text, budget) {
  spend(budget, text.length);
  let spans = quoteSpans(text);
  if (spans === null) return text;
  // `eval` as a word of its own followed by blank — see the Python half.
  const firstEval = text.search(EVAL_WORD);
  for (const [a, b] of [...spans].reverse()) {
    const dashC = dashCBefore(text, a);
    if (!dashC && !(firstEval >= 0 && firstEval < a)) continue;
    spend(budget, 2 * a);
    if ((dashC && SHELL_DASH_C.test(text.slice(0, a))) || EVAL_ARGS.test(text.slice(0, a))) {
      text = text.slice(0, a + 1) + unwrapEnv(text.slice(a + 1, b), budget) + text.slice(b);
    }
  }
  spend(budget, 3 * text.length);
  spans = quoteSpans(text) ?? [];
  // Sliced by UTF-16 index, the unit quoteSpans and every regex index use: a
  // code-point array (`[...text]`) shifted by one per astral character. Built
  // in one walk: re-slicing the whole string per span was O(n^2) of its own.
  const parts = [];
  let from = 0;
  for (const [a, b] of spans) { parts.push(text.slice(from, a), "x".repeat(b + 1 - a)); from = b + 1; }
  parts.push(text.slice(from));
  const flat = parts.join("");
  const ends = [...flat.matchAll(ENV_BOUNDARY)].map((bm) => bm.index + bm[0].length);
  let nb = 0, cut = 0;
  const out = [];
  let last = 0;
  for (const m of flat.matchAll(ENV_AT)) {
    if (m.index < last) continue;
    while (nb < ends.length && ends[nb] <= m.index) { cut = ends[nb]; nb += 1; }
    spend(budget, m.index - cut + 1);
    if (!CMD_PREFIX_ALLOWED.test(flat.slice(cut, m.index))) continue;
    let i = m.index + m[0].length;
    const n = text.length, chdir = [], assigns = [];
    let split = null, touched = false;
    for (;;) {
      while (i < n && (text[i] === " " || text[i] === "\t")) i += 1;
      if (i >= n || ENV_WORD_END.has(text[i])) break;
      const [raw, word, after] = envWord(text, i);
      let take = null;
      if (word === "--") { i = after; touched = true; break; }
      if (word.startsWith("--")) {
        const eqAt = word.indexOf("=");
        const name = eqAt < 0 ? word.slice(2) : word.slice(2, eqAt);
        const value = eqAt < 0 ? "" : word.slice(eqAt + 1);
        const full = envLong(name);
        const kind = full ? ENV_LONG[full] : null;
        if (kind === "required" && eqAt < 0) take = full;
        else if (kind === "required") {
          if (full === "chdir") chdir.push(value);
          else if (full === "split-string") split = value;
        }
      } else if (word.startsWith("-") && word.length > 1) {
        for (let k = 1; k < word.length; k += 1) {
          const letter = word[k];
          if (!ENV_SHORT_VALUE.includes(letter)) continue;
          const attached = word.slice(k + 1);
          const name = letter === "C" ? "chdir" : letter === "S" ? "split-string" : letter;
          if (!attached) take = name;
          else if (name === "chdir") chdir.push(attached);
          else if (name === "split-string") split = attached;
          break;
        }
      } else if (word === "-") {
        // the same as -i
      } else if (ASSIGNMENT.test(word)) {
        assigns.push(raw);
      } else break;
      touched = true; i = after;
      if (take !== null) {
        while (i < n && (text[i] === " " || text[i] === "\t")) i += 1;
        if (i < n && !ENV_WORD_END.has(text[i])) {
          const [, value, end] = envWord(text, i);
          i = end;
          if (take === "chdir") chdir.push(value);
          else if (take === "split-string") split = value;
        }
      }
      if (split !== null) break;
    }
    if (!touched) continue;
    const head = chdir.map((d) => `cd ${shQuote(d)} && `).join("") + assigns.map((a) => a + " ").join("");
    out.push(text.slice(last, m.index) + head + text.slice(m.index, m.index + m[0].length) + " "
      + (split !== null ? split + " " : ""));
    last = i;
  }
  out.push(text.slice(last));
  return out.join("");
}

// Only an `env` OUTSIDE every quoted string is rewritten (inside a `-c` script,
// only within it), repeated until nothing changes — see `_unwrap_env`.
// Throws EnvBudgetSpent past ENV_BUDGET, which decideReading denies.
export function unwrapEnv(text, budget = { left: ENV_BUDGET }) {
  for (let pass = 0; pass <= text.length; pass += 1) {
    const next = unwrapEnvOnce(text, budget);
    if (next === text) break;
    text = next;
  }
  return text;
}

// Two readings, and a denial from either stands — see `decide` in the Python.
export function decide(lane, command, cwd) {
  const verdict = decideReading(lane, command, cwd, false);
  if (verdict !== null) return verdict;
  return decideReading(lane, command, cwd, true);
}

function decideReading(lane, command, cwd, unwrap) {
  if (typeof command !== "string" || !command) return null;
  const cfg = config(lane);

  // Match against an EXPANDED copy. A literal absolute-path test let
  // `~/...` and `$HOME/...` through, and the tilde form is how an
  // agent naturally writes the path. The shell expands them after this plugin
  // has already decided, so expand them here first.
  // On Windows HOME may arrive as a backslash drive path (msys converts it for a native
  // program): spelled the way bash expands it, so a path built on it still
  // matches the patterns below before anything is folded.
  const HOME = shellHome(process.env.HOME ?? userInfo().homedir);
  const probe = command
    .split("${HOME}").join(HOME)
    .split("$HOME").join(HOME)
    .replace(/(^|[^\w~])~\//g, `$1${HOME}/`);

  // `noHeredoc` drops heredoc body lines (data, not command text) but keeps
  // quotes intact, so a real redirect target that happens to be quoted is still
  // resolvable. `masked` additionally blanks quoted-string interiors. Path
  // detection stays on the unmasked text — it needs the real path, quoted or not.
  // Unwrapped AFTER the heredoc strip — see the Python half.
  let noHeredoc = stripHeredocBodies(probe);
  if (unwrap) {
    let unwrapped;
    try {
      unwrapped = unwrapEnv(noHeredoc);
    } catch (e) {
      if (!(e instanceof EnvBudgetSpent)) throw e;
      return { rule: "env", reason:
        "Blocked: this command chains or nests `env` too deeply "
        + "for the guard to read it within its time budget. A hook "
        + "that runs out of time does not block, so a command the "
        + "guard cannot finish reading is refused rather than "
        + "allowed. Drop the `env` wrappers, or split the command." };
    }
    if (unwrapped === noHeredoc) return null;
    noHeredoc = unwrapped;
  }
  // Joined AFTER the heredoc strip, as in the Python half.
  const copies = joinContinuations(noHeredoc);
  const masked = copies.map(maskCopy).join(COPY_BOUNDARY);

  // EVERY protected root, not just the lane's own pair — see the Python half.
  const { roots } = cfg;
  let { where } = cfg;

  // Dangerous only if it can write AND can land in a protected tree — because
  // it names one, stands in one, or cds into one.
  //
  // SCANNED ON `noHeredoc`, NOT `probe` — see the Python's note for the
  // measurement: a `git commit` whose message was fed by `cat <<'EOF' ... EOF`
  // (data, not code — the heredoc feeds `cat`, so it never reaches a shell)
  // cited a protected root as evidence, and scanning the raw command text made
  // that citation indistinguishable from a real destination. No verb's write
  // destination is ever spelled inside the payload it is asked to write, only
  // in its own operands or the working directory, so a heredoc body is always
  // DATA for this question — exactly like it already is for the verb regexes
  // below, which read `masked`.
  // And in the shell's own spelling — see the Python twin (2026-09-24, a root
  // with spaces written with backslash escapes). Unescaping only ADDS matches.
  const unescaped = noHeredoc.replace(/\\(.)/g, "$1");
  const noHeredocC = canonText(noHeredoc);
  const unescapedC = canonText(unescaped);
  const namedRoot = roots.find((root) => noHeredoc.includes(root) || unescaped.includes(root)
    || noHeredocC.includes(root) || unescapedC.includes(root)) ?? null;
  const namesProtected = namedRoot !== null;
  // A cross-lane denial must name the tree it is protecting: `where` is the
  // lane's own prose and is the wrong sentence for another lane's tree.
  if (namedRoot !== null && namedRoot !== cfg.repo && namedRoot !== cfg.worktrees) {
    where = `another lane's protected tree (${namedRoot})`;
  }
  const standsIn = insideProtected(cfg, cwd);
  const cdsRe = new RegExp(`cd\\s+["']?(${roots.map(escapeRe).join("|")})`);
  let cdsInto = cdsRe.test(noHeredoc) || cdsRe.test(noHeredocC);
  // And any `cd`/`pushd` that lands there, however it is spelled — see the
  // Python twin (2026-09-29, audit F6).
  const segs = shellSegments(noHeredoc, cwd, HOME);
  const start = cwd && isAbs(cwd) ? normOf(cwd) : null;
  if (!cdsInto) {
    cdsInto = segs.some((seg) => seg.cwd && seg.cwd !== start && insideProtected(cfg, seg.cwd));
  }
  // And the same question asked of paths that do not SPELL a protected root — a
  // symlink into one, or a `..` traversal back into one. Skipped when a literal
  // already opened the gate, so the common case costs nothing.
  const resolvedToken = namesProtected ? null : resolvesProtected(cfg, noHeredoc);
  // And of every word as the shell will see it — see `shellSegments`.
  const segHit = (namesProtected || resolvedToken) ? null : segmentsResolveProtected(cfg, segs);
  const gateToken = resolvedToken ?? segHit?.raw ?? null;
  const gatePath = resolvedToken ? realPath(resolvedToken) : segHit?.landed ?? null;
  if (!(namesProtected || gateToken || standsIn || cdsInto)) return null;

  // Name WHICH condition tripped. A deny that only says "this command mutates
  // the repo" sends the reader to inspect the command text — and when the
  // trigger was the working directory, there is nothing there to find: under
  // opencode the bash tool carries it as its own `workdir` argument, so it
  // never appears in the command text at all.
  const triggers = [];
  if (standsIn) {
    triggers.push(
      `the working directory is inside it (${cwd}) — this is NOT in the command` +
      ` text above; opencode's bash tool carries the directory as a separate` +
      ` \`workdir\` argument. Re-run with a workdir outside the tree`);
  }
  if (namesProtected) triggers.push(`the command names a protected root (${namedRoot})`);
  if (gateToken) {
    triggers.push(
      `a path in the command RESOLVES inside it (${gateToken} -> ` +
      `${gatePath}) even though the root does not appear ` +
      "literally — a symlink, a `..` traversal, a path relative to where its " +
      "command runs, a variable, a glob or a brace");
  }
  if (cdsInto) triggers.push("the command itself cd's into it");
  const trigger = triggers.join("; and ");

  // Cleaning up after `hw done` is maintenance, not a repo write. Checked
  // before the git deny, and inert on a lane with no `worktreeRoots`.
  if (isSpentWorktreeTeardown(cfg, probe, HOME)) return null;

  // Copying OUT of a protected tree is a READ of it — in `cp SRC DST` only DST
  // is written. Honoured only when the sole reason we got this far is that the
  // command NAMES a protected path: when the working directory is inside one,
  // or the command cd's into one, a RELATIVE destination can land inside it
  // without ever naming it.
  let [copyOutOk, copyNote] = isCopyOutOfProtected(cfg, probe, cwd);
  if (copyOutOk) {
    if (!(standsIn || cdsInto)) return null;
    copyNote =
      " This copy's destination is outside the protected trees, but the working" +
      " directory is inside one (or the command cd's into one), so a relative" +
      " operand could still land inside one without naming it. Re-run it from" +
      " outside.";
  }

  // A CONTENT-ONLY MATCH IS NOT A CONFIRMED DESTINATION — see the Python's
  // note. `namedRoot`/`resolvedToken` answer "does a protected path appear in
  // this command's CODE", never "is it this command's write target", and a
  // path can appear in code as a quoted argument that is pure prose (a
  // citation, a commit message) rather than an operand. When the match
  // survives quote-masking it sat in plain, unquoted text — the shape every
  // real operand has — and the trigger stays confident; when masking blanked
  // it away, the only reason we are here is a quoted string. (A citation
  // inside a heredoc body never reaches this point at all: it was already
  // excluded from `namedRoot`/`resolvedToken` above.)
  const contentOnly = !standsIn && !cdsInto && (
    (namedRoot !== null && !masked.includes(namedRoot) && !canonText(masked).includes(namedRoot)) ||
    (gateToken !== null && !masked.includes(gateToken))
  );

  if (GIT_MUTATORS.test(masked)) {
    if (contentOnly) {
      return { rule: "git", reason:
        `Blocked: this git command's text contains the path of ${where} ` +
        `(${trigger}), but a git subcommand's actual destination is its cwd ` +
        `(or an explicit -C/--git-dir/--work-tree), never text elsewhere in ` +
        `the command — and this match sits inside a quoted argument, not one ` +
        `of those. I could not confirm whether this is a real destination or ` +
        `a citation, and refuse rather than guess. If you are only citing the ` +
        `path as evidence, move it into a heredoc body instead of a quoted ` +
        "argument (e.g. `git commit -F -` fed by `cat <<'EOF' ... EOF`): a " +
        `heredoc that does not feed a shell is already read as data, not as ` +
        `this trigger.` };
    }
    return { rule: "git", reason:
      `Blocked: this git subcommand mutates ${where} (${trigger}). ${cfg.gitTail}` };
  }
  if (INPLACE.test(masked)) {
    if (contentOnly) {
      return { rule: "inplace", reason:
        `Blocked: an in-place edit's command text contains the path of ` +
        `${where} (${trigger}), sitting inside a quoted argument rather than ` +
        `in the file operand itself. I could not confirm whether this is the ` +
        `edited file or a citation, and refuse rather than guess.` };
    }
    return { rule: "inplace", reason:
      `Blocked: in-place edit targeting ${where} (${trigger}). The brainer is ` +
      `read-only there.` };
  }
  // A redirect inside `sh -c '...'` is masked away with its quotes; the shell
  // reading found it (Judgment Day, 2026-09-29), so it opens this too.
  if (REDIRECT.test(masked) || segs.some((seg) => seg.redirects.length)) {
    const redirectHit = redirectLandsInProtected(cfg, noHeredoc, cwd, segs);
    if (redirectHit) {
      return { rule: "redirect", reason:
        `Blocked: shell redirect while targeting ${where} (${trigger}). This ` +
        `redirect's destination resolves to ${redirectHit.resolved} ` +
        `(written as \`${redirectHit.raw}\` in the command). This is the ` +
        `exact hole that path-based permission rules do not cover. Write to ` +
        `${cfg.writeHere} instead. ${cfg.redirectTail}` };
    }
  }
  let mutatorInvoked = false;
  {
    const re = new RegExp(MUTATORS.source, "g");
    let m;
    while ((m = re.exec(masked))) {
      if (isCommandPosition(masked, m.index)) { mutatorInvoked = true; break; }
      if (m.index === re.lastIndex) re.lastIndex += 1;
    }
    for (const f of masked.matchAll(FIND_WRITES)) {
      if (mutatorInvoked) break;
      if (isCommandPosition(masked, f.index)) mutatorInvoked = true;
    }
  }
  if (mutatorInvoked) {
    if (contentOnly) {
      return { rule: "mutator", reason:
        `Blocked: this command's text contains the path of ${where} ` +
        `(${trigger}), sitting inside a quoted argument rather than in a ` +
        `plain operand. That is as often a citation (evidence pasted into a ` +
        `decisions.md entry) as a real destination, and I could not confirm ` +
        `which. Refusing is the safe default. If you are only citing the ` +
        `path, move it into a heredoc body instead of a quoted argument: a ` +
        `heredoc that does not feed a shell is already read as data, not as ` +
        `this trigger. If you are instead trying to PRESERVE data by ` +
        `copying it OUT of a protected tree, run \`cp -R\`, \`rsync -a\` or ` +
        `\`install\` directly rather than through an interpreter — those ` +
        `parse source from destination and are already exempt when only ` +
        `the source is protected.` };
    }
    return { rule: "mutator", reason:
      `Blocked: file-mutating command targeting ${where} (${trigger}). The brainer ` +
      `is read-only there.${copyNote}` };
  }
  return null;
}

// Exported for the test harness only. Nothing in the tree imports these.
export const __internals = {
  LANES, config, normPath, realPath, shlexSplit, landsOutsideProtected,
  copyOperands, isCopyOutOfProtected, insideProtected, stripHeredocBodies,
  maskQuotes, redirectLandsInProtected, isSpentWorktreeTeardown, shellSegments,
};

// ── OpenCode's own file tools ──────────────────────────────────────────────
//
// `permission.edit` was meant to be this half, and on 2026-10-01 it was
// measured not to be (opencode 1.18.34, `opencode debug agent` plus live
// `opencode run` in a sandbox): OpenCode matches an edit rule against the path
// RELATIVE to the worktree, so a file outside it is asked about as
// `../x/c.txt` and an absolute deny like `<product repo>/**` never matches. Every
// protected root sits outside every lane's worktree, so every lane's edit deny
// was void. The file tools are therefore judged here, by absolute path, with
// the same boundary question the bash half asks (`insideProtected`). There is
// no Python twin: Claude's Edit/Write go through `permissions.deny`, and
// Codex's patch tool has its own reader in `deny-repo-writes-codex.py`.
const FILE_TOOLS = new Set(["write", "edit", "multiedit", "patch", "apply_patch"]);
const PATCH_HEADER = /^\*\*\* (?:Add File|Update File|Delete File|Move to): (.+)$/gm;

export function fileToolPaths(tool, args) {
  if (tool === "apply_patch" || tool === "patch") {
    const text = typeof args.patchText === "string" ? args.patchText
      : typeof args.patch === "string" ? args.patch : null;
    if (text === null) return null;
    return [...text.matchAll(PATCH_HEADER)].map((m) => m[1].trim());
  }
  return typeof args.filePath === "string" && args.filePath ? [args.filePath] : null;
}

function judgeFileTool(lane, laneError, tool, output, sessionDir) {
  if (laneError) {
    throw new Error(`Blocked: ${laneError.message} Every file write is refused until a ` +
      "lane this guard knows is named.");
  }
  const args = output?.args;
  const paths = args !== null && typeof args === "object" && !Array.isArray(args)
    ? fileToolPaths(tool, args) : null;
  if (paths === null) {
    throw new Error(`Blocked: the ${lane} read-only guard could not read which file the ` +
      `${tool} call writes, so it cannot tell a protected tree from any other. Refused rather than guessed.`);
  }
  const cfg = config(lane);
  for (const p of paths) {
    const abs = normPath(isAbs(p) ? p : joinPath(sessionDir, p));
    if (insideProtected(cfg, abs)) {
      throw new Error(`Blocked: ${tool} would write ${abs}, inside a product repo or a live ` +
        `worktree. The ${lane} lane is read-only there.`);
    }
  }
}

// The opencode plugin factory. Each lane's `.opencode/plugin/deny-repo-writes.js`
// is a two-line shim that names its lane and re-exports the result.
export function makeDenyRepoWrites(lane) {
  // AN UNKNOWN LANE DENIES; IT DOES NOT THROW AT LOAD. Throwing here was the
  // first draft, and it is fail-OPEN in the way that matters: what opencode
  // does with a plugin that throws while loading is not established, and the
  // plausible answers (skip the plugin, log and continue) both leave the lane
  // unguarded with no refusal anywhere. Refusing inside the hook uses the ONE
  // mechanism opencode is documented to honour — a throw from
  // `tool.execute.before` — so the failure is loud on every bash call instead.
  let laneError = null;
  try {
    config(lane);
  } catch (e) {
    laneError = e;
  }
  return async (pluginInput) => {
    // opencode's `tool.execute.before` input carries {tool, sessionID, callID}
    // and NO cwd — verified against @opencode-ai/plugin's shipped index.d.ts.
    // The Python hook receives the shell's cwd in its JSON payload; the JS API
    // has no equivalent, so the cwd comes from the bash tool's OWN `workdir`
    // argument ("The working directory to run the command in. Defaults to the
    // current directory. Use this instead of 'cd' commands."), falling back to
    // the session directory the plugin was handed at construction. This is the
    // one place the opencode API forces a different shape from the Python, and
    // it matters: `workdir` is how an opencode agent is TOLD to change
    // directory, so a guard that ignores it cannot see the cwd axis at all.
    const sessionDir = pluginInput?.directory ?? pluginInput?.worktree ?? process.cwd();
    return {
      "tool.execute.before": async (input, output) => {
        if (FILE_TOOLS.has(input.tool)) return judgeFileTool(lane, laneError, input.tool, output, sessionDir);
        if (input.tool !== "bash") return;
        if (laneError) {
          throw new Error(
            `Blocked: ${laneError.message} Every bash command is refused until a ` +
            "lane this guard knows is named, because a guard that cannot resolve " +
            "its lane cannot tell a protected tree from any other directory.");
        }
        // ARGUMENTS THAT ARE NOT AN OBJECT REFUSE. `output.args ?? {}` read a
        // string, a list or a missing `args` as an empty object, so `command`
        // came out as "" and the guard allowed a call it had not read. The
        // runtime builds `args`, not the model, so the live risk is a schema
        // change; a call this guard cannot read has not been seen to be safe.
        const args = output?.args;
        if (args === null || typeof args !== "object" || Array.isArray(args)) {
          throw new Error(
            `Blocked: the ${lane} read-only guard was handed bash arguments that are ` +
            `not an object (${args === null ? "null" : Array.isArray(args) ? "a list" : typeof args}), ` +
            "so it cannot read the command. Refused rather than guessed.");
        }
        const workdir =
          typeof args.workdir === "string" && args.workdir ? args.workdir : null;
        // A CRASH HERE ALREADY REFUSES, and that asymmetry with the Python is
        // worth naming rather than leaving to be rediscovered: opencode's
        // refusal IS a throw, so an exception out of `decide` fails closed for
        // free, while Claude Code reads a non-2 exit with no deny JSON as a
        // non-blocking error and lets the command run. The Python half had to
        // be taught this explicitly (see `main` there, 2026-09-07). It is
        // wrapped anyway so the refusal SAYS a crash happened — "a guard that
        // reached no verdict" and "a guard that refused you" call for different
        // moves by whoever reads it.
        let verdict;
        try {
          verdict = decide(lane, args.command ?? "", workdir ?? sessionDir);
        } catch (error) {
          throw new Error(
            `Blocked: the ${lane} read-only guard CRASHED while deciding ` +
            `(${String(error?.message ?? error)}). It reached no verdict, and a ` +
            "guard that reached no verdict has not established that this command " +
            "is safe. Every bash command is refused until this is fixed.");
        }
        // opencode's tool.execute.before returns void, so refusal is a throw.
        if (verdict) throw new Error(verdict.reason);
      },
    };
  };
}

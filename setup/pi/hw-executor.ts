// hw's executor adapter for pi — loaded by `hw` with `-e <brain>/setup/pi/hw-executor.ts`.
//
// One file, the pi half of everything the other vendors already have
// (docs/pi-vendor.md has the whole map):
//
//   1. THE WRITE GUARD. `tool_call` can block (`{ block: true, reason }`), so the
//      lane's read-only policy is judged here for EVERY tool that can write: `bash`,
//      `edit`, `write` with the SAME detection the opencode half uses
//      (`setup/guards/deny-repo-writes.js`, `decide` for a command, `insideProtected`
//      for a file path), and any other tool — an extension's, an MCP server's,
//      pi-subagents' — by what it is handed (`classifyTool`, `scanInput`). A handler
//      that throws also blocks — pi's documented fail-safe — and a guard that cannot
//      load its module blocks every bash/edit/write, as the opencode shim does.
//      A PRODUCT executor also carries the INVERSE guard (`HW_PI_BRAIN_GUARD=1`, set
//      by hw): it does not write in brain, work from it or commit a path into it —
//      the same Python decision (`deny_brain_writes.py`) the claude hook and the
//      opencode plugin ask. The two global hooks every agent is under (Keychain
//      secret reads, blind `pkill`) are asked here too: `-ne` drops the user-level
//      extension that would otherwise be their only pi door.
//      THE GUARD TRAVELS WITH THE SUBAGENTS: pi-subagents' children are sessions of
//      this process, and an extension is not inherited by them unless the host
//      requires it, so the root session registers this file as a REQUIRED CHILD
//      EXTENSION for every runner (pi-subagents' `registerRequiredChildExtensions`).
//   2. THE TURN END. `agent_before_settle` is pi's actionable boundary: it can
//      append a message and ask for one more model request, which is what
//      Claude's Stop hook `{"decision":"block","reason":…}` does. So it runs
//      `hw executor-turn-end --hook-json` (the vendor-neutral predicate: turn
//      tokens, the handback refusal, the queued-ruling delivery) and, when hw
//      answers `block`, hands the reason back as the next prompt. The payload names
//      the SESSION and the context use; hw compares the session with the one it
//      registered, like the Stop hook, so a child's turn end is not the executor's.
//   3. WHAT RAN, WRITTEN DOWN (the receipt's pi readers). The root session publishes
//      `<run>/pi-runtime.json` (the model and thinking that REALLY run, the context
//      use, the session) and `<run>/pi-inputs.jsonl` (the sha256 of every message
//      that reached the session — the `processed` proof of the brief's delivery).
//   4. THE ROSTER'S WAY OUT. A quota error of the provider ends a turn with
//      `outcome: error`; with `HW_PI_FALLBACK_MODEL` set (hw: the kind's fallback)
//      the model is switched and the run continues, and the switch is a receipt row.
//   5. ENGRAM THROUGH THE LABEL PROXY. `pi.registerMcpServer` adds `hw_engram`
//      (bin/engram-label-proxy around `engram mcp`), and the raw `engram` server
//      a user-level ~/.pi/agent/mcp.json may carry is refused in `tool_call`:
//      pi gives an mcp.json server precedence over a registered one of the same
//      name, so the proxy cannot take its name — it takes its place instead.
//
//   6. THE LANE'S SPECIALISTS. pi-subagents reads no `.claude/agents`; hw names the
//      ones the worktree carries (`HW_PI_CLAUDE_AGENT_DIRS`, PATH-style) and this
//      registers each as a RUNTIME agent through pi-subagents' process-local event,
//      translated in memory at session start (`claudeAgentDefinition`): nothing is
//      copied or rewritten on disk. The file's `model:` (a Claude alias) is resolved
//      through the lane's tier profile (`HW_PI_SUBAGENT_MODELS`) to a pi model and
//      thinking level, and the receipt's pi_specialists row names what each runs.
//
//   7. THE BACKGROUND SHELL. pi's `bash` has no run_in_background, so `bash_background`
//      starts a command, returns at once, and wakes the session with its exit code and
//      log path when it ends — Claude's background-Bash notification, on pi. The
//      sleep-poll rule (setup/guards/deny-executor-sleep-poll.py) is asked for pi's
//      `bash` too, and its refusal names that tool instead of a flag pi does not have.
//
//   9. CAN A BACKGROUND SUBAGENT START? (pi-adopt-gentle, 2026-11-05). pi-subagents wakes the parent itself (its parent-wake, which
//      holds during compaction and reserves one wake per idle parent) and needs `@earendil-works/chord` ONLY when the host pi
//      declares it (pi 0.85 to 1.1): since pi-subagents 0.78.0 a pi 1.2+ without chord starts and wakes fine (measured: setup/pi/
//      test-live-subagent-wake.mjs; 0.77.0 fails to START the child, so there is nothing to wake). hw owns no second wake: it
//      records at session start whether a child can start (`pi_background_subagents`, backgroundSubagentsStatus), which is the
//      check nothing made when `pi update` removed a hand-installed chord.
//   8. THE EXPLORATION BUDGET. Per request, the root session counts pi's discovery
//      results (read, grep, find, ls) and, past 3 lookups or 40 KB of text, steers
//      the run ONCE toward the explorer with pi's native sendMessage(deliverAs:
//      "steer"). hw arms it only where an explorer exists (HW_PI_EXPLORE_AGENT).
//
// Inert outside an hw executor (HW_TASK/HW_RUN/HW_WORKDIR) — no turn end, no receipt,
// no runtime record — except the guards, which are policy for any session that loads
// this file, the account's global rules, and the background-shell tool, a plain tool
// whose log then lives under the system temp dir.

import { execFile, spawn, spawnSync } from "node:child_process";
import { createHash } from "node:crypto";
import { appendFileSync, createWriteStream, existsSync, mkdirSync, readFileSync, readdirSync, realpathSync, renameSync, statSync, writeFileSync } from "node:fs";
import { homedir, tmpdir, userInfo } from "node:os";
import { dirname, isAbsolute, join, resolve } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const HERE = dirname(realpathSync(fileURLToPath(import.meta.url)));
const SELF = join(HERE, "hw-executor.ts");
const BRAIN = process.env.HW_BRAIN_ROOT || resolve(HERE, "..", "..");
const GUARD_MODULE = join(BRAIN, "setup", "guards", "deny-repo-writes.js");
const BRAIN_GUARD_MODULE = join(BRAIN, "setup", "guards", "deny-brain-writes.js");
const LANE = process.env.HW_PROJECT || "brain";

function isHwExecutor(): boolean {
  return !!(process.env.HW_TASK && process.env.HW_RUN && process.env.HW_WORKDIR);
}
function brainGuardOn(): boolean {
  return isHwExecutor() && process.env.HW_PI_BRAIN_GUARD === "1";
}
function runDir(): string | null {
  return isHwExecutor() ? join(process.env.HW_WORKDIR as string, ".hw", process.env.HW_RUN as string) : null;
}

// ── 1. the guard ────────────────────────────────────────────────────────────
let guard: any = null;
let guardError: string | null = null;
async function loadGuard() {
  if (guard || guardError) return;
  try {
    guard = await import(GUARD_MODULE);
    guard.config(LANE); // an unknown lane refuses below, it does not pass
  } catch (e: any) {
    guardError = String(e?.message ?? e);
    guard = null;
  }
}

const WRITE_TOOLS = new Set(["edit", "write"]);

// THE LANE POLICY IS THE BRAINER'S, AND A PRODUCT EXECUTOR WORKS INSIDE IT.
// guards.json protects each product lane's checkout AND its worktrees root, which
// is right for a brainer and wrong for the executor whose own worktree sits under
// that root (Judgment Day, both judges: every edit and every mutating bash of a
// product-lane pi executor was refused). Claude and opencode executors carry no lane
// repo-write policy at all; so here, when the executor's own workdir is inside
// its lane's protected trees, that workdir is exempt for edit/write (the main
// checkout and sibling worktrees stay refused) and bash is not judged by THIS policy —
// a shell standing in a protected cwd is refused wholesale by `decide`, with no way
// to tell its own worktree apart. What stops such an executor writing in brain is the
// inverse guard below; docs/pi-vendor.md lists the bash gap towards sibling worktrees.
function canonical(p: string): string {
  try { return realpathSync(p); } catch { return guard.normPath(p); }
}
function within(child: string, parent: string): boolean {
  const c = canonical(child).toLowerCase(), q = canonical(parent).toLowerCase().replace(/\/+$/, "");
  return c === q || c.startsWith(q + "/");
}
// THE SHELL OF A PRODUCT EXECUTOR (second pi parity task, 2026-10-12). `decide` refuses wholesale a command that
// STANDS in a protected tree, and the executor's own worktree is one, so bash used to skip the lane policy
// altogether: `echo x > <main checkout>/src/a.ts` and a write into a sibling worktree reached the shell
// (docs/pi-vendor.md, "Not at parity" 2). Now the executor's own workdir path is replaced by an inert token
// in the text `decide` reads, and the cwd it is given is not protected: what is left to refuse is every
// OTHER protected root a command names. STRING-LEVEL, and it says so: a relative path that climbs out of
// the worktree (`cd ../other && …`, `> ../../src/a.ts`) names no root and is not seen.
const NEUTRAL_CWD = "/";
const OWN_TOKEN = "/__hw_own_workdir__";
function escapeRe(t: string): string { return t.replace(/[.*+?^${}()|[\]\\]/g, "\\$&"); }
export function withoutOwnWorkdir(command: string, own: string): string {
  const home = homedir();
  let text = command.split("${HOME}").join(home).split("$HOME").join(home).replace(/(^|[^\w~])~\//g, `$1${home}/`);
  // `decide` expands every other `$NAME` from THIS process's environment, and an executor's environment names its
  // own worktree ($HW_WORKDIR, $HW_ARTIFACTS, $PWD…): left alone, `cat > $HW_ARTIFACTS/checks.md` reappears inside
  // `decide` as a write into a protected tree (Judgment Day round 1). A variable whose value is the own worktree or
  // inside it is expanded HERE, so the own-path token below covers it; any other variable is left for `decide`.
  for (const [name, value] of Object.entries(process.env)) {
    if (!value || !/^[A-Za-z_][A-Za-z0-9_]*$/.test(name) || !isAbsolute(value) || !within(value, own)) continue;
    text = text.replace(new RegExp("\\$\\{" + name + "\\}|\\$" + name + "(?![A-Za-z0-9_])", "g"), () => value);  // # MUTATION-ANCHOR: 991-M08
  }
  for (const form of new Set([own, canonical(own)])) {
    const bare = form.replace(/\/+$/, "");
    if (!bare) continue;
    // Followed by a boundary, so `.../own-task` does not swallow `.../own-task-2`.
    // Not when what follows the own path in the same word has a `..` SEGMENT (`<own>/..`, `<own>/src/../..`, `<own>//..`,
    // `<own>/./..`): that climbs OUT of the worktree (Judgment Day rounds 1 and 2), and the token would hide the climb
    // from `decide`'s `..` resolution. Left spelled out, `decide` resolves it. A name that merely starts with two dots
    // (`<own>/..hidden`) is not a segment.
    const edge = "(?=$|[/\\s\"'`;|&)<>:,=])";
    const climbs = (rest: string): boolean => rest.split("/").includes("..");  // # MUTATION-ANCHOR: 991-M09
    text = text.replace(new RegExp(escapeRe(bare) + "((?:/[^\\s\"'`;|&)<>:,=]*)?)" + edge, "gi"),
      (m: string, rest: string) => climbs(rest) ? m : OWN_TOKEN + rest);  // # MUTATION-ANCHOR: 991-M07
  }
  return text;
}
function ownWorkdirProtected(): string | null {
  const wd = process.env.HW_WORKDIR;
  if (!isHwExecutor() || !wd) return null;
  return guard.insideProtected(guard.config(LANE), canonical(wd)) ? wd : null;
}

// WHAT A TOOL IS, BY NAME, because pi's tools are an open set: the built-ins, whatever an
// extension registers (pi-subagents' `subagent`), every MCP tool (`mcp__<server>__<tool>`),
// and the nested calls codemode makes. A tool that is known to read is not judged; a tool whose
// name says it writes, and any tool whose name says nothing, is judged by its arguments.
// Reading is never refused: a lane reads product repos all day.
const READ_ONLY_TOOLS = new Set([
  "read", "grep", "find", "ls", "tool_search", "list_mcp_resources", "list_mcp_resource_templates",
  "read_mcp_resource", "web_search", "fetch_content", "get_search_content", "code_search",
]);
const READ_NAME = /^(?:read|get|list|search|find|query|fetch|view|show|describe|stat|head|tail|glob|grep|ls|status|check|inspect|diff|log|resolve|lookup|count|exists|whoami|ping|current)(?:[_-]|$)/i;
const WRITE_NAME = /(?:write|edit|create|delete|remove|move|rename|copy|patch|apply|save|put|mkdir|touch|append|replace|update|upload|exec|run|bash|shell|command|spawn|commit|push|checkout|reset|install|download|export)/i;
export function classifyTool(toolName: string): "read" | "bash" | "write" | "other" {
  if (toolName === "bash") return "bash";
  if (WRITE_TOOLS.has(toolName)) return "write";
  if (READ_ONLY_TOOLS.has(toolName)) return "read";
  const bare = toolName.replace(/^mcp__[^_]+(?:_[^_]+)*?__/, "");
  if (/^mcp__hw_engram__/.test(toolName)) return "read"; // memory, through the label proxy: no repo path
  if (READ_NAME.test(bare) && !WRITE_NAME.test(bare)) return "read";
  return "other";
}

// The argument names under which a tool is told WHAT TO WRITE, and under which it is
// told WHAT TO RUN. `cwd` is neither: handing an explorer a directory to read is routine,
// and what it then runs is judged in its own `bash`.
const PATH_KEYS = new Set([
  "path", "paths", "file", "files", "file_path", "filepath", "filename", "dest", "destination",
  "target", "output", "outputpath", "output_path", "outfile", "out", "to", "saveto", "save_to", "writeto", "write_to",
]);
const COMMAND_KEYS = new Set(["command", "cmd", "shell", "script", "commands"]);
export function scanInput(input: unknown, depth = 0): { paths: string[]; commands: string[]; unreadable: boolean } {
  const out = { paths: [] as string[], commands: [] as string[], unreadable: false };
  if (depth > 6) { out.unreadable = true; return out; }
  if (Array.isArray(input)) {
    for (const v of input) {
      const r = scanInput(v, depth + 1);
      out.paths.push(...r.paths); out.commands.push(...r.commands); out.unreadable ||= r.unreadable;
    }
    return out;
  }
  if (input === null || typeof input !== "object") return out;
  for (const [k, v] of Object.entries(input as Record<string, unknown>)) {
    const key = k.toLowerCase();
    if (typeof v === "string") {
      if (PATH_KEYS.has(key)) out.paths.push(v);
      else if (COMMAND_KEYS.has(key)) out.commands.push(v);
    } else if (Array.isArray(v) && v.every((x) => typeof x === "string") && (PATH_KEYS.has(key) || COMMAND_KEYS.has(key))) {
      (PATH_KEYS.has(key) ? out.paths : out.commands).push(...(v as string[]));
    } else if (v !== null && typeof v === "object") {
      const r = scanInput(v, depth + 1);
      out.paths.push(...r.paths); out.commands.push(...r.commands); out.unreadable ||= r.unreadable;
    }
  }
  return out;
}

// `~` as the guard reads it: the password-database home, not $HOME.
function absolutePath(p: string, cwd: string): string {
  const expanded = p.startsWith("~/") ? join(userInfo().homedir, p.slice(2)) : p;
  return guard.normPath(isAbsolute(expanded) ? expanded : join(cwd, expanded));
}

// ── 1b. THE SECRET READ GUARD ───────────────────────────────────────────────
// Claude's `permissions.deny` (~/.claude/settings.json) refuses its Read/Edit tools these paths, and the
// shared rule (CLAUDE.shared.md, Secrets) adds `*.db`, `*.credentials` and `secrets/`: they are never
// printed, copied or pasted. pi had NO such guard (measured 2026-10-12: pi's `read` and `bash` reached
// ~/.ssh/<key> and a deploy tool's .env). MIRRORED from the real deny list, entry by entry:
//   Read/Edit(.env, .env.*)               → `.env`, `.env.*`            (any directory)
//   Read/Edit(.ssh/*)                     → ~/.ssh/* (private keys, known_hosts, config)
//   Read/Edit(.credentials/*)             → a `.credentials` directory, and `*.credentials` (shared rule)
//   Read/Edit(Library/Keychains/*)        → ~/Library/Keychains/*
//   Read/Edit(.aws/credentials)           → ~/.aws/credentials
//   Read/Edit(.config/gh/hosts.yml)       → ~/.config/gh/hosts.yml
//   Read/Edit(**/*.pem, **/*.key)         → `*.pem`, `*.key`
//   Read/Edit(**/secrets/*)               → anything under a `secrets/` directory
// ADDED beyond the Claude list: `*.db` (shared rule) and the agents' own login files (pi's, codex's, opencode's `auth.json`).
// It judges `read`, `grep`, `edit`, `write`, every other tool's path arguments and every `bash` (and any command
// argument). A bash is judged by the paths its text NAMES, not by what it runs: this is a string-level guard, not a
// sandbox. A path assembled at run time (`cat $(printf .e)nv`) is not seen; `--sandbox` denies the OS-level read of
// ~/.ssh (bin/hw-sandbox) and nothing else (docs/pi-vendor.md, "The secret read guard").
const HOME_DIR = (() => { try { return userInfo().homedir; } catch { return process.env.HOME || ""; } })();
function homeDir(): string { return HOME_DIR; }
const SECRET_SUFFIXES: Array<[string, string]> = [[".db", "*.db"], [".credentials", "*.credentials"], [".pem", "*.pem"], [".key", "*.key"]];
// A path (absolute, lexically normalised) → the rule it breaks, or null. `bareHidden`: a bare `.foo.key` in a
// command is far likelier a jq/JSON expression than a file, so the suffix rules skip it (`.env*` and `.credentials` do not).
export function secretRule(path: string, bareHidden = false): string | null {
  const p = path.toLowerCase();
  const parts = p.split("/").filter(Boolean);
  const base = parts[parts.length - 1] ?? "";
  const dirs = parts.slice(0, -1);
  if (base === ".env" || base.startsWith(".env.")) return "a .env file";  // # MUTATION-ANCHOR: 989-M01
  if (base === ".credentials" || dirs.includes(".credentials")) return "a .credentials path";
  if (dirs.includes("secrets")) return "a secrets/ directory";  // # MUTATION-ANCHOR: 989-M02
  // by COMPONENT, not by the home prefix: `cd ~ && cat .ssh/k`, `~other/.ssh/k` and `"$HOME"/.ssh/k` all name an `.ssh` directory
  if (dirs.includes(".ssh")) return "the ~/.ssh directory";  // # MUTATION-ANCHOR: 989-M03
  if (p.includes("/library/keychains/")) return "~/Library/Keychains";
  if (p.endsWith("/.aws/credentials")) return "~/.aws/credentials";
  if (p.endsWith("/.config/gh/hosts.yml")) return "~/.config/gh/hosts.yml";
  if (p.endsWith("/.pi/agent/auth.json") || p.endsWith("/.codex/auth.json") || p.endsWith("/.local/share/opencode/auth.json")) return "an agent login file (auth.json)";
  if (!bareHidden) {
    for (const [suffix, name] of SECRET_SUFFIXES) if (base.length > suffix.length && base.endsWith(suffix)) return name;  // # MUTATION-ANCHOR: 989-M04
  }
  return null;
}
function secretRuleOfFile(path: string, cwd: string, bareHidden = false): string | null {
  const expanded = path.replace(/^~(?=\/|$)/, homeDir()).replace(/^\$\{?HOME\}?(?=\/|$)/, homeDir());
  const abs = resolve(isAbsolute(expanded) ? expanded : join(cwd, expanded));
  const lexical = secretRule(abs, bareHidden);
  if (lexical) return lexical;
  try { const real = realpathSync(abs); return real !== abs ? secretRule(real, bareHidden) : null; } catch { return null; }  // a symlink to a secret is the secret
}
// A glob token is judged by what it EXPANDS to in the real tree (`cat *.db`, `cat .e*`, `cat ~/.s*/k`), the way the shell
// would; nothing matching is nothing read. Segment by segment, `*` `?` and `[...]` (an unterminated `[` is a literal).
function globToRegExp(seg: string): RegExp {
  let re = "";
  for (let i = 0; i < seg.length; i++) {
    const c = seg[i];
    if (c === "*") re += "[^/]*";
    else if (c === "?") re += "[^/]";
    else if (c === "[") {
      const end = seg.indexOf("]", i + 2);
      if (end === -1) re += "\\[";
      else { re += "[" + seg.slice(i + 1, end).replace(/^!/, "^").replace(/\\/g, "\\\\") + "]"; i = end; }
    } else re += c.replace(/[.+^${}()|\\\]\/]/g, "\\$&");
  }
  try { return new RegExp("^" + re + "$", "i"); } catch { return /^$/; }
}
function expandGlob(abs: string, cap = 400): string[] {
  const parts = abs.split("/").filter((x) => x !== "");
  let level: string[] = ["/"];
  for (const seg of parts) {
    const next: string[] = [];
    for (const dir of level) {
      if (!/[*?\[]/.test(seg)) { next.push(join(dir, seg)); continue; }
      let names: string[] = [];
      try { names = readdirSync(dir); } catch { continue; }
      const re = globToRegExp(seg);
      for (const n of names) { if (re.test(n) && (!n.startsWith(".") || seg.startsWith("."))) next.push(join(dir, n)); if (next.length >= cap) break; }
    }
    level = next;
    if (!level.length) break;
  }
  return level;
}
function secretRuleOfGlob(token: string, cwd: string, asDir = false): string | null {
  if (!/[*?\[]/.test(token)) return null;
  const expanded = token.replace(/^~(?=\/|$)/, homeDir()).replace(/^\$\{?HOME\}?(?=\/|$)/, homeDir());
  const abs = resolve(isAbsolute(expanded) ? expanded : join(cwd, expanded));
  for (const m of expandGlob(abs)) { const r = secretRule(m) ?? (asDir ? secretRule(m + "/x", true) : null); if (r) return r; }  // # MUTATION-ANCHOR: 989-M14
  return null;
}
// the key is USED (`-i`), not read out. NOT rsync: its `-i` is --itemize-changes, so `rsync -i ~/.ssh/k /tmp/k` is a copy of the key
const SSH_USERS = new Set(["ssh", "scp", "sftp"]);  // # MUTATION-ANCHOR: 989-M10
// WHICH SHORT OPTIONS TAKE AN ARGUMENT IS PER PROGRAM, read from each tool's own help (2026-10-12; ripgrep 15.2.0,
// ag 2.2.0, ack 3.10.0, macOS BSD grep 2.6.0): a letter that is a bare flag in one tool and takes a value in another
// (`-D` is grep's ACTION and ag's --debug; `-f` is a pattern FILE in grep/rg, "follow" in ag and "list files" in ack)
// makes the parser swallow the real pattern and slot the real file. `num`: takes a number/text value; `numIfDigit`:
// optional value, taken only if the next word is all digits; `pat`: takes the PATTERN; `file`: takes a PATH.
const SEARCH_OPTS: Record<string, { num: string; numIfDigit: string; pat: string; file: string; longPat: string[]; longFile: string[] }> = {
  grep: { num: "ABCDdm", numIfDigit: "", pat: "e", file: "f", longPat: ["--regexp"], longFile: ["--file"] },
  rg:   { num: "ABCMEjmdtTgr", numIfDigit: "", pat: "e", file: "f", longPat: ["--regexp"], longFile: ["--file"] },
  ag:   { num: "mWGg", numIfDigit: "ABC", pat: "", file: "", longPat: [], longFile: [] },  // # MUTATION-ANCHOR: 989-M15
  ack:  { num: "mtT", numIfDigit: "ABCp", pat: "", file: "", longPat: [], longFile: [] },  // # MUTATION-ANCHOR: 989-M18  (-p is `p|proximate:1`, optional: ack 3.10.0 bin/ack:4334)
};
SEARCH_OPTS.egrep = SEARCH_OPTS.grep; SEARCH_OPTS.fgrep = SEARCH_OPTS.grep;
const DIR_LISTERS = new Set(["ls", "eza", "stat", "test", "[", "[[", "file"]);  // `~/.ssh` itself, listed: names are not secrets
const WRAPPERS = /^(?:(?:env|command|sudo|exec|time|nohup|builtin|nice)\s+(?:-\S+\s+)*)+/;
// Words as the shell forms them: quotes are removed WITHOUT splitting the word (`"$HOME"/.ssh/k`, `'.e''nv'` are one word
// each), and a word that is EMPTY after quote removal (`''`, `""`, `''""`, and the value of `--regexp=`) stays a word, as
// \u0000, so an empty search pattern keeps its slot instead of letting the file operand take it.
// One backslash escape of a $'...' string, starting AFTER the backslash at `at`: → the text it stands for and the index of its last char.
function ansiEscape(s: string, at: number): { text: string; end: number } {
  const c = s[at];
  const hex = (re: RegExp) => { const m = re.exec(s.slice(at + 1)); return m ? { text: String.fromCodePoint(parseInt(m[1], 16)), end: at + m[0].length } : null; };
  if (c === "x") return hex(/^([0-9a-fA-F]{1,2})/) ?? { text: "x", end: at };
  if (c === "u") return hex(/^([0-9a-fA-F]{1,4})/) ?? { text: "u", end: at };
  if (c === "U") return hex(/^([0-9a-fA-F]{1,8})/) ?? { text: "U", end: at };
  if (/[0-7]/.test(c)) { const m = /^[0-7]{1,3}/.exec(s.slice(at))!; return { text: String.fromCharCode(parseInt(m[0], 8) & 255), end: at + m[0].length - 1 }; }
  const named: Record<string, string> = { n: "\n", t: "\t", r: "\r", a: "\x07", b: "\b", e: "\x1b", E: "\x1b", f: "\f", v: "\v" };
  return { text: named[c] ?? c, end: at };
}
function shellWords(segment: string): string[] {
  const words: string[] = [];
  let cur = "", started = false, quote: string | null = null;
  const flush = () => {
    if (!started) return;
    started = false;
    const word = cur; cur = "";
    if (word === "") { words.push("\u0000"); return; }
    const parts = word.split(/[<>=]+/).filter(Boolean);
    words.push(...parts);
    if (/=$/.test(word)) words.push("\u0000");  // `--regexp=` : an empty value
  };
  for (let i = 0; i < segment.length; i++) {
    const c = segment[i];
    if (quote === "$'") {  // ANSI-C quoting: `\'` does not close it, and `\x2e` / `\056` / `\u002e` spell a character
      if (c === "'") quote = null;
      else if (c === "\\" && i + 1 < segment.length) { const d = ansiEscape(segment, i + 1); cur += d.text; i = d.end; }
      else cur += c;
    } else if (quote) {
      if (c === quote) quote = null;
      else if (c === "\\" && quote === '"' && i + 1 < segment.length) cur += segment[++i];
      else cur += c;
    } else if (c === "$" && segment[i + 1] === "'") { quote = "$'"; started = true; i++; }  // # MUTATION-ANCHOR: 989-M20
    else if (c === "'" || c === '"') { quote = c; started = true; }
    else if (c === "\\" && i + 1 < segment.length) { cur += segment[++i]; started = true; }
    else if (/\s/.test(c)) flush();
    else { cur += c; started = true; }
  }
  flush();
  return words;
}
// a search PATTERN word: the strict rules; plus the full rules when it names an existing file, and the directory rule when it
// names an existing directory (a mis-parsed option leaves the real operand here)
function slotRule(t: string, cwd: string): string | null {
  const kind = existsAs(t, cwd);
  return secretRuleOfFile(t, cwd, true)
    ?? (kind === "file" ? secretRuleOfFile(t, cwd) : null)  // # MUTATION-ANCHOR: 989-M17
    ?? (kind === "dir" ? secretRuleOfFile(t.replace(/\/*$/, "") + "/x", cwd, true) : null);
}
function existsAs(path: string, cwd: string): "file" | "dir" | null {
  try {
    const expanded = path.replace(/^~(?=\/|$)/, homeDir()).replace(/^\$\{?HOME\}?(?=\/|$)/, homeDir());
    const st = statSync(resolve(isAbsolute(expanded) ? expanded : join(cwd, expanded)));
    return st.isDirectory() ? "dir" : st.isFile() ? "file" : null;
  } catch { return null; }
}
// Command segments: the shell's own separators (`; & | ( ) $( \`` and newline), but NOT inside quotes — `grep -E 'a|b' secrets/x`
// is one command, and `grep ';' .env` too. `$(` and a backtick still run inside double quotes, so they split there.
function splitSegments(command: string): string[] {
  const out: string[] = [];
  let cur = "", quote: string | null = null;
  for (let i = 0; i < command.length; i++) {
    const c = command[i];
    if (quote === "'") { cur += c; if (c === "'") quote = null; continue; }
    if (quote === "$'") { cur += c; if (c === "\\" && i + 1 < command.length) cur += command[++i]; else if (c === "'") quote = null; continue; }
    if (c === "\\" && i + 1 < command.length) { cur += c + command[++i]; continue; }
    if (quote === '"') {
      if (c === '"') { quote = null; cur += c; }
      else if (c === "`" || (c === "$" && command[i + 1] === "(")) { out.push(cur); cur = ""; if (c === "$") i++; }
      else cur += c;
      continue;
    }
    if (c === "$" && command[i + 1] === "'") { quote = "$'"; cur += "$'"; i++; continue; }  // # MUTATION-ANCHOR: 989-M19
    if (c === "'" || c === '"') { quote = c; cur += c; continue; }
    if (c === "`" || (c === "$" && command[i + 1] === "(")) { out.push(cur); cur = ""; if (c === "$") i++; continue; }
    if (c === "\n" || c === ";" || c === "&" || c === "|" || c === "(" || c === ")") { out.push(cur); cur = ""; continue; }
    cur += c;
  }
  out.push(cur);
  return out;
}
export function secretInCommand(command: string, cwd: string, depth = 0): string | null {
  for (const rawSegment of splitSegments(command)) {
    const segment = rawSegment.trim();
    if (!segment) continue;
    const head = segment.replace(/^(?:[A-Za-z_][A-Za-z0-9_]*=\S*\s+)+/, "").replace(WRAPPERS, "");
    const program = (head.split(/\s+/)[0] || "").replace(/^.*\//, "").replace(/^["']|["']$/g, "");
    const tokens = shellWords(segment);  // # MUTATION-ANCHOR: 989-M11
    // the PATTERN of a search (`rg "\.db" src`) is TEXT: it is judged by the strict rules only (a pattern that IS `.env` or
    // `~/.ssh/k` is still refused), never by the suffix rules or as a glob — UNLESS it names a file or directory that EXISTS:
    // a mis-parsed option (the real pattern taken as an option's value) then leaves the real file in the pattern slot, and
    // what exists is judged in full. An option's own argument is parsed per program (SEARCH_OPTS), not guessed.
    const slots = new Set<number>(), consumed = new Set<number>();
    const opts = SEARCH_OPTS[program];
    if (opts) {
      const at = tokens.findIndex((x) => x.replace(/^.*\//, "") === program);
      let withE = false;
      scan: for (let j = at + 1; j < tokens.length; j++) {
        const t = tokens[j];
        if (opts.longPat.includes(t)) { withE = true; slots.add(j + 1); j++; continue; }
        if (opts.longFile.includes(t)) { withE = true; continue; }
        if (t.startsWith("--")) continue;
        if (t.startsWith("-") && t.length > 1) {
          for (let k = 1; k < t.length; k++) {
            const c = t[k], rest = t.slice(k + 1);
            if (opts.pat.includes(c)) { withE = true; if (!rest) { slots.add(j + 1); j++; } continue scan; }
            if (opts.file.includes(c)) {
              withE = true;
              if (rest) { const r = secretRuleOfFile(rest, cwd); if (r) return `${r} (\`${rest}\`)`; }
              continue scan;
            }
            if (opts.num.includes(c)) { if (!rest) { consumed.add(j + 1); j++; } continue scan; }
            if (opts.numIfDigit.includes(c)) { if (!rest && /^\d+$/.test(tokens[j + 1] ?? "")) { consumed.add(j + 1); j++; } continue scan; }
          }
          continue;
        }
        if (!withE) slots.add(j);
        break;
      }
    }
    for (let i = 0; i < tokens.length; i++) {
      const t = tokens[i];
      if (t.includes("://") || consumed.has(i) || t === "\u0000") continue;
      // a quoted STRING that is itself a command or a command line (`bash -c 'cat .env'`, `-o ProxyCommand='cat ~/.ssh/k'`)
      if (!slots.has(i) && depth < 4 && /[\s;&|()$`]/.test(t)) { const inner = secretInCommand(t, cwd, depth + 1); if (inner) return inner; }
      if (slots.has(i)) { const r = slotRule(t, cwd); if (r) return `${r} (\`${t.length > 60 ? t.slice(0, 57) + "…" : t}\`)`; continue; }  // # MUTATION-ANCHOR: 989-M13
      const usedAsIdentity = SSH_USERS.has(program) && (tokens[i - 1] === "-i" || tokens[i - 1]?.toLowerCase() === "identityfile");
      const bareHidden = !t.includes("/") && t.startsWith(".");
      let rule: string | null = null;
      if (!usedAsIdentity) {
        rule = DIR_LISTERS.has(program) && /[*?\[]/.test(t) ? null : (secretRuleOfFile(t, cwd, bareHidden) ?? secretRuleOfGlob(t, cwd, true)  /* a glob naming a protected DIRECTORY is entered by cd, cp -r, tar, a recursive search */);  // `ls *.db` names files, it does not read them
        // `~/.ssh` or `secrets` named as a directory: harmless to list, not to enter or copy from.
        if (!rule && !DIR_LISTERS.has(program) && (/^(?:cd|pushd)$/.test(program) || t.includes("/") || t.startsWith("~"))) rule = secretRuleOfFile(t.replace(/\/*$/, "") + "/x", cwd, true);
      }
      if (rule) return `${rule} (\`${t.length > 60 ? t.slice(0, 57) + "…" : t}\`)`;
    }
  }
  return null;
}
// What a tool call is handed to read, by tool: bash's text, every path-shaped argument, every command-shaped argument.
export function secretVerdict(toolName: string, input: unknown, cwd: string): string | null {
  try {
    return secretVerdictOf(toolName, input, cwd);
  } catch (e: any) {  // a guard that crashed reached no verdict: refuse, as every other guard here does
    return `Blocked: the secret read guard CRASHED while judging ${toolName} (${String(e?.message ?? e)}). It reached no verdict, so the call is refused.`;
  }
}
function secretVerdictOf(toolName: string, input: unknown, cwd: string): string | null {
  if (input === null || typeof input !== "object" || Array.isArray(input)) return null;
  const refusal = (what: string) =>
    `Blocked: ${toolName} would touch ${what}. Secrets are never printed, copied, pasted or committed ` +
    "(shared rule, Secrets). Do not work around this guard: if the task needs a value, report it blocked (done-invoker --blocked) naming the Keychain handle " +
    "or the file's name, never its content.";
  const o = input as Record<string, unknown>;
  const commands: string[] = [], paths: string[] = [];
  if (toolName === "bash") { if (typeof o.command === "string") commands.push(o.command); }
  else {
    const s = scanInput(input);
    paths.push(...s.paths); commands.push(...s.commands);
    if (typeof o.path === "string" && !paths.includes(o.path)) paths.push(o.path);
  }
  for (const p of paths) {
    // a RECURSIVE search of a protected directory returns what is inside it: the directory itself is refused for grep
    const rule = secretRuleOfFile(p, cwd) ?? (toolName === "grep" ? secretRuleOfFile(p.replace(/\/*$/, "") + "/x", cwd, true) : null);  // # MUTATION-ANCHOR: 989-M12
    if (rule) return refusal(`\`${p}\`, a secret path (${rule})`);
  }
  for (const c of commands) {
    const rule = secretInCommand(c, cwd);
    if (rule) return refusal(`a secret path named in the command: ${rule}`);
  }
  return null;
}
// A recursive search must not hand back the lines of a protected file: pi's grep walks a directory (--hidden)
// and a `.env` that no .gitignore covers answers like any file; so does a bash `grep -r KEY .` that names no secret path.
// Only the SEARCH SHAPE is filtered (a line that begins with the protected file's path): `cat .env` is refused by name.
export function redactSecretLines(text: string, cwd: string, searchPath: string | undefined): { text: string; dropped: number } {
  const sp = searchPath?.replace(/^~(?=\/|$)/, homeDir()).replace(/^\$\{?HOME\}?(?=\/|$)/, homeDir());
  const base = sp ? resolve(isAbsolute(sp) ? sp : join(cwd, sp)) : cwd;
  let dropped = 0;
  const kept = text.split("\n").filter((line) => {
    // `path:N: text` (pi's grep), `path-N- text` (its context lines), `path:text` (a recursive `grep -r` in bash)
    const m = /^(.+?)[:-]\d+[:-]/.exec(line) ?? /^([^\s:]+):/.exec(line);
    if (!m) return true;
    if (secretRuleOfFile(m[1], base)) { dropped++; return false; }
    return true;
  });
  return { text: kept.join("\n"), dropped };
}

// THE INVERSE GUARD (brain), asked per call with the Claude-shaped payload its Python half reads.
// A product executor only; fails closed like the opencode shim.
let brainGuard: any = null;
let brainGuardError: string | null = null;
async function loadBrainGuard() {
  if (brainGuard || brainGuardError) return;
  try {
    const mod = await import(BRAIN_GUARD_MODULE);
    brainGuard = mod.DenyBrainWrites;
    if (typeof brainGuard?.verdict !== "function") throw new Error("DenyBrainWrites.verdict is missing");
  } catch (e: any) {
    brainGuardError = String(e?.message ?? e);
    brainGuard = null;
  }
}
async function brainVerdict(payload: Record<string, unknown>): Promise<string | null> {
  await loadBrainGuard();
  if (!brainGuard) {
    return `Blocked: the brain guard could not load ${BRAIN_GUARD_MODULE} (${brainGuardError}). ` +
      "A product executor is refused every write until it does: a guard that is not running has not established that the call is safe.";
  }
  try {
    return brainGuard.verdict(payload) ?? null;
  } catch (e: any) {
    return `Blocked: the brain guard CRASHED while deciding (${String(e?.message ?? e)}). It reached no verdict, so the call is refused.`;
  }
}

// THE SLEEP-POLL RULE IS ASKED HERE TOO (pi parity, 2026-11-04). It is the one hw rule that lives in a personal
// guard rather than an exported one (setup/export/manifest), so the copy THIS TREE carries is the one asked:
// a session's rule is versioned with the session. It also judges a raw foreground `go test` over core/ now, which
// is why a command with neither `sleep` nor `tail` can still be sent to it (retro-fixes-rc3). (The personal ~/.claude/hooks copy is Claude's and is not a
// fallback on purpose — measured while building this: it is an older revision, so a pi executor would be
// judged by whatever age that install happened to have.) With no copy in the tree the rule does not run here,
// and the receipt says so once rather than letting a pi executor believe it was judged.
// The payload's `background_tool` is what makes the refusal name pi's own tool: the Python says
// run_in_background because that is Claude's flag, and pi has no such flag — it has `bash_background`.
const SLEEP_POLL_GUARD = join(BRAIN, "setup", "guards", "deny-executor-sleep-poll.py");
let sleepPollMissing = false;
// BACKGROUND WORK IN FLIGHT, as the sleep rule asks for it (a standalone `sleep` while work runs spends a turn on nothing: the
// completion arrives as a message). The jobs and monitors this process started, plus the async subagents pi-subagents announced
// and has not yet completed (its own `subagent:async-started` / `-complete` events, keyed by run id).
const ASYNC_SUBAGENTS = new Set<string>();
export function activeWork(): number {
  return bgRunning() + ASYNC_SUBAGENTS.size;
}
function sleepPollGuard(): string | null {
  if (existsSync(SLEEP_POLL_GUARD)) return SLEEP_POLL_GUARD;
  if (!sleepPollMissing) {
    sleepPollMissing = true;
    receiptRow("pi_sleep_poll", `NOT LOADED — no ${SLEEP_POLL_GUARD} in this tree, so the sleep-poll rule does not run for pi here`, "hw-executor.ts judge");
  }
  return null;
}
function sleepPollVerdict(command: string, bg: boolean): string | null {
  // The guard's own early exit, asked before the interpreter starts: a command with none of these words is never
  // judged by it (measured by Judgment Day round 1: a python start on every bash call is a cost for nothing).
  if (!command.includes("sleep") && !command.includes("tail") && !command.includes("pgrep") && !/go\s+test/.test(command)) return null;
  const file = sleepPollGuard();
  if (!file) return null;
  const r = spawnSync(process.platform === "win32" ? "python3" : "/usr/bin/python3", [file], {
    input: JSON.stringify({ tool_name: "Bash", tool_input: { command, run_in_background: bg }, background_tool: BG_TOOL, cwd: process.cwd(), active_work: activeWork() }),
    encoding: "utf8", timeout: 20000,
  });
  if (r.error) return `Blocked: the sleep-poll guard ${file} could not run (${r.error.message}), so this command was not judged.`;
  if (r.status === 2) return (r.stderr || "").trim() || `Blocked by the sleep-poll guard ${file}`;
  return null;
}

// THE TWO GLOBAL HOOKS EVERY AGENT IS UNDER (~/.claude/hooks, one Python rule set each, the same
// one Claude, Codex and OpenCode ask): a Keychain secret read, a blind pkill/killall.
// Exit 2 is their refusal; a python that could not start reached no verdict, so it refuses too.
const GLOBAL_BASH_HOOKS: Array<{ file: string; applies: RegExp }> = [
  { file: "block-keychain-secret-read.py", applies: /\bsecurity\b/ },
  { file: "deny-blind-process-kill.py", applies: /\b(?:pkill|killall|pgrep)\b/ },
];
function globalHookVerdict(command: string): string | null {
  for (const h of GLOBAL_BASH_HOOKS) {
    if (!h.applies.test(command)) continue;
    const script = join(homedir(), ".claude", "hooks", h.file);
    const r = spawnSync(process.platform === "win32" ? "python3" : "/usr/bin/python3", [script], {
      input: JSON.stringify({ tool_name: "Bash", tool_input: { command } }), encoding: "utf8", timeout: 20000,
    });
    if (r.error) return `Blocked: the global guard ${h.file} could not run (${r.error.message}), so this command was not judged.`;
    if (r.status === 2) return (r.stderr || "").trim() || `Blocked by ${h.file}`;
  }
  return null;
}

// A SETUP EXECUTOR DOES NOT WRITE IN THE BRAINER'S CHECKOUT OF BRAIN. Claude loads setup/guards/deny_setup_brain_writes.py
// from the tracked .claude/settings.json; pi has no settings hooks, so this asks the same file with the same payload.
// The file decides from the environment (HW_TASK + HW_PROJECT=setup), so for any other lane it is a no-op, and it fails
// closed: a python that could not start, or a guard that is missing, reached no verdict and refuses.
const SETUP_BRAIN_GUARD = join(BRAIN, "setup", "guards", "deny_setup_brain_writes.py");
function setupBrainGuardOn(): boolean {
  return isHwExecutor() && process.env.HW_PROJECT === "setup";
}
function setupBrainVerdict(payload: Record<string, unknown>): string | null {
  if (!existsSync(SETUP_BRAIN_GUARD)) {
    return `Blocked: the setup brain-write guard ${SETUP_BRAIN_GUARD} is not in this tree, so this call was not judged.`;
  }
  const r = spawnSync(process.platform === "win32" ? "python3" : "/usr/bin/python3", [SETUP_BRAIN_GUARD], {
    input: JSON.stringify(payload), encoding: "utf8", timeout: 20000,
  });
  if (r.error) return `Blocked: the setup brain-write guard could not run (${r.error.message}), so this call was not judged.`;
  if (r.status === 2) return (r.stderr || "").trim() || `Blocked by ${SETUP_BRAIN_GUARD}`;
  // FAIL CLOSED: a guard that crashed (exit 1 with a traceback) or was killed (status null) reached no verdict, and only
  // exit 0 is the guard saying "not mine / allowed" (Judgment Day, judge A).
  if (r.status !== 0) return `Blocked: the setup brain-write guard ended with ${r.status === null ? "a signal" : "exit " + r.status} and reached no verdict, so this call was not judged.`;
  try {
    const out = JSON.parse(r.stdout || "{}");
    if (out?.hookSpecificOutput?.permissionDecision === "deny") {
      return String(out.hookSpecificOutput.permissionDecisionReason || `Blocked by ${SETUP_BRAIN_GUARD}`);
    }
  } catch { /* no JSON on stdout: the guard said nothing */ }
  return null;
}

export async function judge(toolName: string, input: any, cwd: string): Promise<string | null> {
  const raw = /^mcp__engram__/.test(toolName);
  if (raw && isHwExecutor()) {
    return `Blocked: ${toolName} is the raw engram server, which skips this executor's label proxy. ` +
      `Use the same tool under mcp__hw_engram__ (engram-label-proxy, label ${process.env.ENGRAM_PROJECT || "?"}).`;
  }
  const secret = secretVerdict(toolName, input, cwd);  // # MUTATION-ANCHOR: 989-M05
  if (secret) return secret;
  const kind = classifyTool(toolName);
  if (kind === "read") return null;
  await loadGuard();
  if (!guard) {
    return `Blocked: the ${LANE} read-only guard could not load ${GUARD_MODULE} (${guardError}). ` +
      `Every ${toolName} call is refused until it loads, because a guard that is not running ` +
      "cannot tell a protected tree from any other directory.";
  }
  if (input === null || typeof input !== "object" || Array.isArray(input)) {
    return `Blocked: the ${LANE} read-only guard could not read the ${toolName} arguments. Refused rather than guessed.`;
  }
  const own = ownWorkdirProtected();
  const commandIn = (command: string, background = false): string | null => {
    const g = globalHookVerdict(command);
    if (g) return g;
    const sp = sleepPollVerdict(command, background);
    if (sp) return sp;
    // A product executor's shell is judged with its OWN worktree taken out of the picture: the lane's
    // checkout and its sibling worktrees stay protected, the worktree it was given does not.
    const judged = own ? withoutOwnWorkdir(command, own) : command;  // # MUTATION-ANCHOR: 991-M06
    try {
      const verdict = guard.decide(LANE, judged, own ? NEUTRAL_CWD : cwd);
      return verdict ? verdict.reason : null;
    } catch (e: any) {
      return `Blocked: the ${LANE} read-only guard CRASHED while deciding (${String(e?.message ?? e)}). ` +
        "It reached no verdict, so the command is refused.";
    }
  };
  const pathIn = (p: string): string | null => {
    const abs = absolutePath(p, cwd);
    if (own && within(abs, own)) return null;
    if (guard.insideProtected(guard.config(LANE), abs)) {
      return `Blocked: ${toolName} would write ${abs}, inside a product repo or a live worktree. ` +
        `The ${LANE} lane is read-only there.`;
    }
    return null;
  };
  // What the tool writes or runs, however its arguments are named.
  // `bash_background` runs its command in the background BY DEFINITION: the sleep-poll rule judges it with
  // Claude's run_in_background=true semantics (a long sleep or a short poll is fine there, an unbounded poll
  // loop or a follow is not).
  const backgroundTool = toolName === BG_TOOL || toolName === MONITOR_TOOL;
  let paths: string[] = [], commands: string[] = [];
  if (kind === "bash") {
    commands = [typeof input.command === "string" ? input.command : ""];
  } else if (kind === "write") {
    const p = typeof input.path === "string" && input.path ? input.path
      : typeof input.file_path === "string" && input.file_path ? input.file_path : null;
    if (!p) {
      return `Blocked: the ${LANE} read-only guard could not read which file ${toolName} writes. Refused rather than guessed.`;
    }
    paths = [p];
  } else {
    const s = scanInput(input);
    if (s.unreadable) {
      return `Blocked: the ${LANE} read-only guard could not read the ${toolName} arguments (nested too deep). Refused rather than guessed.`;
    }
    paths = s.paths; commands = s.commands;
  }
  for (const c of commands) { const r = commandIn(c, backgroundTool); if (r) return r; }
  for (const p of paths) { const r = pathIn(p); if (r) return r; }
  if (setupBrainGuardOn()) {  // # MUTATION-ANCHOR: 1241-M01
    for (const c of commands) {
      const r = setupBrainVerdict({ tool_name: "Bash", tool_input: { command: c }, cwd });
      if (r) return r;
    }
    for (const p of paths) {
      const r = setupBrainVerdict({ tool_name: "Write", tool_input: { file_path: absolutePath(p, cwd) }, cwd });
      if (r) return r;
    }
  }
  if (brainGuardOn()) {  // # MUTATION-ANCHOR: 988-M01
    for (const c of commands) {
      const r = await brainVerdict({ tool_name: "Bash", tool_input: { command: c }, cwd });
      if (r) return r;
    }
    for (const p of paths) {
      const r = await brainVerdict({ tool_name: "Write", tool_input: { file_path: absolutePath(p, cwd) }, cwd });
      if (r) return r;
    }
  }
  return null;
}

// ── 2. the turn end ─────────────────────────────────────────────────────────
function turnEndTimeoutMs(): number {
  const n = Number(process.env.HW_TURN_END_TIMEOUT_MS);
  return Number.isFinite(n) && n > 0 ? n : 20_000;
}

export function runTurnEnd(hadInput: boolean, extra: Record<string, unknown> = {}): Promise<string | null> {
  return new Promise((done) => {
    const hw = join(BRAIN, "bin", "hw");
    if (!existsSync(hw)) return done(null);
    execFile(hw, ["executor-turn-end", "--hook-json"], {
      timeout: turnEndTimeoutMs(),
      env: {
        ...process.env,
        HW_EXECUTOR_VENDOR: "pi",
        HW_TURN_HAD_INPUT: hadInput ? "1" : "0",
        HW_TURN_END_PAYLOAD: JSON.stringify({ hook_event_name: "Stop", vendor: "pi", ...extra }),
      },
    }, (_err, stdout) => {
      // exit 8 is hw's "block": the JSON is on stdout either way.
      const line = String(stdout || "").trim().split("\n").filter(Boolean).pop() || "";
      try {
        const j = JSON.parse(line);
        done(j?.decision === "block" && typeof j.reason === "string" ? j.reason : null);
      } catch {
        done(null);
      }
    });
  });
}

// ── 3. what ran, written down ───────────────────────────────────────────────
function sha256(text: string): string {
  return createHash("sha256").update(text, "utf8").digest("hex");
}
// The bytes a terminal hands a TUI are not always the bytes that were sent. MEASURED live on pi 1.1.0
// (herdr agent prompt into a TUI pane): unicode, blank lines and trailing spaces arrive intact, and the
// editor expands every TAB to four spaces; CR/LF and trailing newlines are the other usual drift. hw
// compares the exact hash first and this one second, and says which one matched.
export function normalizeInput(text: string): string {
  return text.replace(/\r\n?/g, "\n").replace(/\t/g, "    ").replace(/\n+$/g, "");
}
function writeAtomic(file: string, text: string) {
  const tmp = `${file}.${process.pid}.tmp`;
  writeFileSync(tmp, text);
  renameSync(tmp, file);
}
// The receipt's own format (`hw receipt` reads it): one JSON object per line, a fact with its source.
function receiptRow(key: string, value: string, source: string) {
  const dir = runDir();
  if (!dir) return;
  try {
    mkdirSync(dir, { recursive: true });
    appendFileSync(join(dir, "receipt.jsonl"), JSON.stringify({ key, value, source, at: new Date().toISOString() }) + "\n");
  } catch { /* the receipt is evidence, not a dependency of the work */ }
}

// THE PROVIDER'S "YOUR PLAN IS SPENT": the strings pi itself treats as terminal for opencode-go
// (its NON_RETRYABLE_PROVIDER_LIMIT_ERROR_PATTERN: GoUsageLimitError, FreeUsageLimitError,
// "Monthly usage limit reached", insufficient_quota, "quota exceeded", "available balance"),
// plus the generic wording of a 429 usage limit. A transient rate limit (a retry fixes it) is
// NOT in this list: pi retries those by itself and only a terminal error reaches the turn end.
export const QUOTA_ERROR = /GoUsageLimitError|FreeUsageLimitError|Monthly usage limit reached|usage limit (?:has been )?reached|insufficient_quota|quota exceeded|available balance|subscription_sharing_usage_limit_exceeded|SubscriptionUsageLimit/i;
// A credential or subscription problem is the KEY's, shared by every model of the plan: no other model fixes it.
export const AUTH_ERROR = /\b(?:401|403)\b|api key|unauthori[sz]ed|authenticat|forbidden|subscription (?:is )?required/i;
// THE FALLBACK IS FOR A MODEL ERROR, NOT FOR A SPENT QUOTA. opencode-go's cap is shared dollars for the whole
// key, so another Go model draws from the same cupo: a switch on a quota error only spends it faster. What a
// fallback does help with is the model itself failing (an upstream error, a malformed stream) while the plan
// is fine. Returns the model to switch to, or null.
export function modelErrorFallback(errorText: string, current: string, fallback: string | undefined): string | null {
  if (!fallback || !current || !errorText || current === fallback) return null;
  if (QUOTA_ERROR.test(errorText)) return null;  // # MUTATION-ANCHOR: 988-M03
  if (AUTH_ERROR.test(errorText)) return null;
  // The plan belongs to the provider: a fallback of another provider is not "the same plan's next
  // model" and is not taken on a guess.
  return current.split("/")[0] === fallback.split("/")[0] ? fallback : null;  // # MUTATION-ANCHOR: 988-M04
}
// A spent quota ends the run with nobody told: the receipt row
// is evidence, not a report, and the `outcome !== "completed"` return below skips executor-turn-end, so nothing reached
// HW_INVOKER_PANE and the brainer found two dead panes by reading them. Claude's Stop hook reports; pi's adapter does it
// here, once, through the same `done-invoker --blocked` an executor would run. The summary carries what the brainer needs
// to judge the stop without opening the pane: the quota, the branch HEAD and the checks.md verdict counts.
let quotaReported = false;
function execText(file: string, args: string[], cwd: string | undefined): Promise<string> {
  return new Promise((done) => execFile(file, args, { cwd, timeout: 120_000, env: process.env }, (_e, out) => done(String(out || "").trim())));
}
export function checksCounts(text: string): string {
  const n: Record<string, number> = { pass: 0, fail: 0, unresolved: 0 };
  for (const line of text.split("\n")) for (const cell of line.split("|")) if (cell.trim() in n) { n[cell.trim()]++; break; }
  return `checks.md ${n.pass} pass / ${n.fail} fail / ${n.unresolved} unresolved`;
}
export async function quotaBlockedSummary(err: string): Promise<string> {
  const wd = process.env.HW_WORKDIR;
  const head = (await execText("git", ["rev-parse", "--short", "HEAD"], wd)) || "unknown";
  const file = join(process.env.HW_ARTIFACTS || join(wd || ".", ".artifacts"), "checks.md");
  const checks = existsSync(file) ? checksCounts(readFileSync(file, "utf8")) : "no checks.md";
  return `Provider quota SPENT, run stopped by pi (${err.slice(0, 160)}). Branch HEAD ${head}; ${checks}. Resume after the quota resets or re-dispatch on another provider.`.slice(0, 900);
}
async function reportQuotaBlocked(err: string): Promise<void> {
  if (quotaReported) return;
  quotaReported = true;
  const tool = join(BRAIN, "bin", "done-invoker");
  if (!existsSync(tool)) { receiptRow("provider_quota_report", `NOT SENT — no ${tool}`, "hw-executor.ts agent_before_settle"); return; }
  const out = await execText(tool, ["--blocked", await quotaBlockedSummary(err)], process.env.HW_WORKDIR);
  receiptRow("provider_quota_report", `done-invoker --blocked ran: ${out.slice(0, 200) || "no output"}`, "hw-executor.ts agent_before_settle");
}

const THINKING_LEVELS = new Set(["off", "minimal", "low", "medium", "high", "xhigh", "max"]);

// ── 5. engram through the proxy ─────────────────────────────────────────────
function engramProxyServer(): Record<string, unknown> | null {
  const label = process.env.ENGRAM_PROJECT;
  const proxy = join(BRAIN, "bin", "engram-label-proxy");
  if (!label || !/^[A-Za-z0-9._-]+$/.test(label) || !existsSync(proxy)) return null;
  const engram = process.env.ENGRAM_BIN || "engram";
  return {
    command: proxy,
    args: ["--label", label, "--", engram, "mcp", "--tools=agent"],
    exposure: "direct",
    description: `engram persistent memory through hw's label proxy (label ${label})`,
  };
}

// ── 6. the lane's specialists ───────────────────────────────────────────────
// Claude's tool names → pi's. What pi has no counterpart for is dropped, and the drop is a receipt row: Task/Agent
// (a child's children), WebFetch/WebSearch, TodoWrite and every mcp__* (a foreground child loads no ambient extension).
const CLAUDE_TOOLS: Record<string, string> = {
  read: "read", grep: "grep", glob: "find", bash: "bash", edit: "edit", multiedit: "edit", write: "write", ls: "ls",
};
// The framework agents. hw's own specialist list (`piFrameworkPrefixes` in core/t6_pi.go) leaves out these AND `jd-`; this
// registration does not leave out `jd-`: jd-judge-a/b and jd-fix-agent are the Judgment Day judges, which a Claude
// executor finds in the account's agents dir. core/t6_pi_test.go holds the two sets apart by exactly `jd-`.
const NOT_SPECIALIST = /^(?:sdd-|speckit-|review-)/;

// WebFetch is pi-web-access's `fetch_content` (the package the machine's pi has installed). A foreground child loads no
// ambient extension, so the definition names the package in `extensions` (measured: a child with WebFetch fetched
// example.com; the same file through the old adapter answered NO WEB TOOL). WebSearch stays dropped: a child given
// `web_search` through the same `extensions` answered NO WEB TOOL (cause not found). Still dropped, each measured:
// Task/Agent (a child of a child: `allowNestedSubagents` and `tools: subagent` both left the child with no subagent tool)
// and mcp__* (pi-subagents hands a child an extension-registered MCP server, hw_engram, only when it runs in the
// background (which pi-subagents < 0.78.0 could not start without @earendil-works/chord in the host pi: "Background children require the
// host npm package"; see backgroundSubagentsStatus).
const CLAUDE_WEB_TOOLS: Record<string, string> = { webfetch: "fetch_content" };

export function mapClaudeTools(list: string[]): { tools: string[]; web: boolean; dropped: string[] } {
  const tools: string[] = [], dropped: string[] = [];
  let web = false;
  for (const t of list) {
    const key = t.toLowerCase();
    const to = CLAUDE_TOOLS[key] ?? CLAUDE_WEB_TOOLS[key];
    if (!to) dropped.push(t);
    else if (!tools.includes(to)) { tools.push(to); if (key in CLAUDE_WEB_TOOLS) web = true; }
  }
  return { tools, web, dropped };
}

// A Claude agent file → pi-subagents' runtime definition. `parse` is pi-subagents' own frontmatter reader, so the
// file is read the way a pi agent file is. `model:` IS carried, through the lane's tier profile (setup/decisions.md
// 2026-11-04): a Claude alias is not a pi model, so `piTierFor` resolves it (the lane's entry for the agent, then the
// entry for the alias, then a pi id the file names itself, then the lane's default) and the definition is registered
// with that pi model and thinking level. No `tools:` line means every tool, as in Claude.
export type PiTier = { model?: string; thinking?: string };

export function piSubagentTiers(env: Record<string, string | undefined> = process.env): Map<string, PiTier> {
  const out = new Map<string, PiTier>();
  for (const pair of String(env.HW_PI_SUBAGENT_MODELS || "").split(",").map((p) => p.trim()).filter(Boolean)) {
    const eq = pair.indexOf("=");
    if (eq <= 0) continue;
    const [model, thinking] = pair.slice(eq + 1).split("|");
    out.set(pair.slice(0, eq), {
      model: model && model !== "inherit" ? model : undefined,
      thinking: thinking && thinking !== "inherit" ? thinking : undefined,
    });
  }
  return out;
}

// piTierFor resolves one agent's model against the profile. `why` is the fact the receipt's pi_specialists row names:
// where the model came from, or why there is none (the child then runs on the executor's, Claude's own `inherit`).
const PI_TIERS = ["haiku", "sonnet", "opus"];
// the same shape the lane table validates (core/t6_pi.go rePiID): a file's `model:` that is not one is not carried
const PI_MODEL_ID = /^[a-z][a-z0-9-]*\/[A-Za-z0-9][A-Za-z0-9._-]*$/;
export function piTierFor(agent: string, fileModel: string | undefined, tiers: Map<string, PiTier> = piSubagentTiers()): { tier: PiTier; why: string } {
  if (![...PI_TIERS, "default"].includes(agent)) {
    const named = tiers.get(agent);
    if (named) return { tier: named, why: "the lane's entry for the agent" };
  }
  const named = String(fileModel ?? "").trim();
  const alias = named.toLowerCase();
  if (alias) {
    const hit = tiers.get(alias);
    if (hit && PI_TIERS.includes(alias)) return { tier: hit, why: `model: ${alias}` };
    // a pi id is carried AS THE FILE WROTE IT: the lane table admits uppercase in the id half, and lowering it
    // would register a model pi does not have while the same value in projects.json is honoured
    if (PI_MODEL_ID.test(named)) return { tier: { model: named }, why: `model: ${named}` };
    return { tier: {}, why: `model: ${named} has no pi mapping` };
  }
  const fallback = tiers.get("default")?.model;
  if (fallback && PI_TIERS.includes(fallback)) {
    return { tier: tiers.get(fallback) ?? {}, why: `no model:, the lane's subagent_model ${fallback}` };
  }
  return { tier: {}, why: "no model:, the lane inherits" };
}

export function claudeAgentDefinition(content: string, parse: any, webAccessDir?: string, tiers?: Map<string, PiTier>) {
  const { frontmatter: fm, body } = parse.parseFrontmatter(content);
  if (!fm?.name || !fm?.description) return null;
  const declared = parse.parseFrontmatterList(fm.tools);
  const mapped = declared ? mapClaudeTools(declared) : { tools: undefined, web: false, dropped: [] as string[] };
  const web = mapped.web && !!webAccessDir; // no pi-web-access on this machine: the web tools have nothing to come from
  const tools = web ? mapped.tools : mapped.tools?.filter((t) => t !== "fetch_content");
  const resolved = piTierFor(String(fm.name), fm.model, tiers);
  const model = resolved.tier.model;
  return {
    name: String(fm.name),
    definition: {
      description: String(fm.description), systemPrompt: body, defaultAsync: false,
      ...(model ? { model, ...(resolved.tier.thinking ? { thinking: resolved.tier.thinking } : {}) } : {}),
      ...(tools ? { tools } : {}), ...(web ? { extensions: [webAccessDir] } : {}),
    },
    resolved,
    dropped: web || !mapped.web ? mapped.dropped : [...mapped.dropped, "WebFetch (pi-web-access is not installed)"],
  };
}

// Registers every specialist of HW_PI_CLAUDE_AGENT_DIRS and returns what it did, for the receipt. A name a `.pi/agents`
// copy already provides (PI_SUBAGENT_EXTRA_AGENT_DIRS) is left to that copy.
async function registerClaudeSpecialists(pi: any, sub: string): Promise<{ registered: string[]; skipped: string[]; dropped: string[]; runs: string[]; disposers: Array<() => void> }> {
  const out = { registered: [] as string[], skipped: [] as string[], dropped: [] as string[], runs: [] as string[], disposers: [] as Array<() => void> };
  const tiers = piSubagentTiers();
  const dirs = (process.env.HW_PI_CLAUDE_AGENT_DIRS || "").split(":").filter(Boolean);
  // THE DECLARED PLUGINS' OWN AGENTS, `<plugin>=<dir>` (HW_PI_PLUGIN_AGENT_DIRS): a Claude executor reads them from
  // --plugin-dir as `plugin:name` (pstack:comment-sicko, which the review closing names), so that is the name registered.
  const pluginDirs = (process.env.HW_PI_PLUGIN_AGENT_DIRS || "").split(":").filter(Boolean)
    .map((spec) => { const eq = spec.indexOf("="); return eq > 0 ? { plugin: spec.slice(0, eq), dir: spec.slice(eq + 1) } : null; })
    .filter((x): x is { plugin: string; dir: string } => x !== null);
  if ((!dirs.length && !pluginDirs.length) || typeof pi.events?.emit !== "function") return out;
  const parse = await import(pathToFileURL(join(sub, "src", "agents", "frontmatter.js")).href);
  const webDir = join(dirname(sub), "pi-web-access");
  const webAccessDir = existsSync(join(webDir, "package.json")) ? webDir : undefined;
  const own = new Set<string>();
  for (const d of (process.env.PI_SUBAGENT_EXTRA_AGENT_DIRS || "").split(":").filter(Boolean)) {
    try { for (const f of readdirSync(d)) if (f.endsWith(".md")) own.add(f.slice(0, -3)); } catch { /* absent */ }
  }
  const seen = new Set<string>(); // the first directory wins, as the lane's agent shadows the account's in Claude
  const filesIn = (d: string): string[] => { try { return readdirSync(d).filter((f) => f.endsWith(".md")).sort(); } catch { return []; } };
  // One agent file -> one runtime registration. `plugin` is the only difference between a lane's directory and a
  // declared plugin's: the name a Claude executor reads from --plugin-dir is `<plugin>:<file name>` (pstack's own pi
  // extension resolves the same `pstack:<file name>`, setup/vendor/plugins/pstack/pi/config.ts), and hw's framework
  // prefixes and `.pi/agents` copies are the LANE's rules, not a plugin declaration's.
  const registerDir = (dir: string, plugin?: string): void => {
    for (const f of filesIn(dir)) {
      const base = f.slice(0, -3);
      if (!plugin && NOT_SPECIALIST.test(base)) continue;
      try {
        const def = claudeAgentDefinition(readFileSync(join(dir, f), "utf8"), parse, webAccessDir, tiers);
        if (!def) { out.skipped.push(`${plugin ? `${plugin}:${base}` : base} (no name/description)`); continue; }
        // a lane's agent goes by its frontmatter name (Claude's rule); a plugin's by `plugin:<file name>`, the name
        // hw's manifest prints and pstack's own pi extension resolves (setup/vendor/plugins/pstack/pi/config.ts)
        const agent = plugin ? `${plugin}:${base}` : def.name;
        if (seen.has(agent)) continue;
        seen.add(agent);
        if (!plugin && own.has(base)) { out.skipped.push(`${agent} (a .pi/agents copy provides it)`); continue; }
        const req: any = { version: 1, name: agent, definition: def.definition };
        pi.events.emit("pi-subagents:runtime-agent-register:v1", req);
        if (!req.result) { out.skipped.push(`${agent} (pi-subagents did not answer)`); continue; }
        if (!req.result.ok) { out.skipped.push(`${agent} (${String(req.result.error?.message ?? req.result.error).slice(0, 80)})`); continue; }
        out.registered.push(agent);
        const t = def.resolved.tier;
        out.runs.push(`${agent}→${t.model ? t.model + (t.thinking ? "|" + t.thinking : "") : "the executor's model"} (${def.resolved.why})`);
        out.disposers.push(() => req.result.registration?.dispose?.());
        for (const t of def.dropped) if (!out.dropped.includes(t)) out.dropped.push(t);
      } catch (e: any) { out.skipped.push(`${base} (${String(e?.message ?? e).slice(0, 80)})`); }
    }
  };
  for (const d of dirs) registerDir(d);
  for (const { plugin, dir } of pluginDirs) registerDir(dir, plugin);
  return out;
}

// ── 9. CAN A BACKGROUND SUBAGENT START? ─────────────────────────────────────
// A pure reader over two package roots: pi-subagents' and the host pi's. `ok` is false only where a child cannot start;
// `text` is the receipt row. pi-subagents < 0.78.0 required chord on every pi ("Background children require the host npm
// package", measured with 0.77.0); from 0.78.0 it is required only when the host manifest declares it.
function readJson(file: string): any { try { return JSON.parse(readFileSync(file, "utf8")); } catch { return null; } }
function versionLt(a: string, b: string): boolean {
  const pa = a.split(".").map((x) => parseInt(x, 10) || 0), pb = b.split(".").map((x) => parseInt(x, 10) || 0);
  for (let i = 0; i < 3; i++) { if ((pa[i] ?? 0) !== (pb[i] ?? 0)) return (pa[i] ?? 0) < (pb[i] ?? 0); }
  return false;
}
export function backgroundSubagentsStatus(subDir: string, hostRoot: string | null): { ok: boolean; text: string } {
  const sub = readJson(join(subDir, "package.json"));
  if (!sub) return { ok: false, text: `NOT AVAILABLE — pi-subagents is not installed at ${subDir}: this executor has no background (or foreground) subagent` };
  const subVersion = String(sub.version ?? "?");
  if (!hostRoot) return { ok: true, text: `pi-subagents ${subVersion}; the host pi package was not found, so chord was not checked` };
  const host = readJson(join(hostRoot, "package.json")) ?? {};
  const hostVersion = String(host.version ?? "?");
  const declared = [host.dependencies, host.optionalDependencies].some((d: any) => d && typeof d === "object" && Object.hasOwn(d, "@earendil-works/chord"));
  const present = existsSync(join(hostRoot, "node_modules", "@earendil-works", "chord", "package.json"));
  const fixHint = "run `pi update`, or install @earendil-works/chord at pi's own version into pi-coding-agent/node_modules/@earendil-works";
  if (versionLt(subVersion, "0.78.0")) {
    return { ok: false, text: `NOT ABLE TO START — pi-subagents ${subVersion} requires @earendil-works/chord on every pi and fails a background child with "Background children require the host npm package" (fixed in 0.78.0)${present ? "; chord is present, so it may still start" : "; chord is absent"}: update pi-subagents (\`pi update\`)` };
  }
  if (declared && !present) {
    return { ok: false, text: `NOT ABLE TO START — pi ${hostVersion} declares @earendil-works/chord and it is missing (\`pi update\` removes it), so pi-subagents ${subVersion} cannot start a background child: ${fixHint}` };
  }
  return { ok: true, text: `can start — pi-subagents ${subVersion} on pi ${hostVersion}; ${declared ? "chord is declared and present" : "the host declares no chord, so none is needed"}; the wake is pi-subagents' own (no second wake path)` };
}

// ── 7. A SHELL COMMAND THAT WAKES THE SESSION WHEN IT ENDS ──────────────────
// pi's `bash` tool has no run_in_background (dist/core/tools/bash.js: `command`, `timeout`), so a pi executor
// that must wait on its own long run had nothing to wait WITH: MEASURED 2026-11-04 (executor
// effort-help-matches-code). It hand-rolled `nohup bash -c 'while kill -0 $PID; do sleep 20; done; …' &`
// plus a foreground `timeout 3000 tail -f log | grep -m1 GATE-DONE`, and its own reasoning named pi's
// `bg_wait` — which waits for background SUBAGENTS, not for a shell job.
//
// The shape is the one gentle-pi (MIT, read as a reference, 2026-11-04) settled on and pi has no native
// version of: a job registry that owns live jobs, one notice per job when it ends on its own, a bounded tail
// (the last 20 lines), the full output in a file the `read` tool can open, and a cap on how many run at once.
// What is NOT ours, on purpose: the command runs through pi's OWN shell operations
// (`createLocalBashOperations` from the host package) rather than a hand-built spawn, so the configured shell,
// the stdin/argv command transport, the detached process group and the process-tree kill are the ones native
// `bash` uses; and the wake is pi's own `sendMessage` with triggerTurn, the call the turn end below already
// makes. What differs from gentle-pi: our write and secret guards judge the command before it starts (its
// registry leaves that to its callers), the notice travels as an `hw-bg-exit` custom message instead of
// through its delivery router, and there is no `monitor`/`job_list` — a line-per-event tool and a second
// source of truth for ids the model already holds are not what this executor was missing.
const BG_TOOL = "bash_background";
const BG_STOP_TOOL = "job_stop";
const BG_LIST_TOOL = "job_list";
const MONITOR_TOOL = "monitor";
const BG_MAX_RUNNING = 25;      // gentle-pi's cap; a runaway fleet is not a wait
const BG_TAIL_LINES = 20;

interface BgJob {
  id: string; label: string; command: string; cwd: string; log: string;
  started: number; status: "running" | "exited" | "stopped" | "failed" | "timeout";
  monitor?: MonitorState;
  exit?: number | null; error?: string; ended?: number;
  tail: string[]; partial: string; abort: any; dropped: number;
}

// A MONITOR is a background job whose matching OUTPUT LINES are events: each (coalesced) batch wakes the session, and the
// job ends on its own exit, on its mandatory timeout, or on its event cap. Gentle-pi's `monitor` (MIT, read as a reference):
// line events with a required timeout; our own version, over the same job registry, the same guards and the same output filter.
export interface MonitorState {
  pattern: RegExp | null; timeoutMs: number; maxEvents: number;
  events: number; pending: string[]; flushAt: ReturnType<typeof setTimeout> | null; timer: ReturnType<typeof setTimeout> | null;
}
export const MONITOR_MAX_TIMEOUT_S = 3600;
export const MONITOR_DEFAULT_MAX_EVENTS = 50;
export const MONITOR_MAX_EVENTS = 500;
const MONITOR_BATCH_MS = 1000;       // lines of one second are ONE wake (a burst is not fifty turns)
const MONITOR_BATCH_LINES = 20;

// The jobs this process started. Module level because the turn end reads them too; only the executor's
// session registers the tools, and a subagent's child is not the executor (see the factory).
const BG_JOBS = new Map<string, BgJob>();
let bgSeq = 0;   // module level: a re-registration must not reuse an id a live job still answers to

export function bgLogDir(dir: string | null): string {
  return dir ? join(dir, "bg") : join(tmpdir(), "hw-pi-bg");
}

// The line the messages quote: a command's first non-empty line, capped.
export function firstLine(text: string, cap = 120): string {
  const one = String(text).split("\n").map((l) => l.trim()).filter(Boolean)[0] ?? "";
  return one.length > cap ? one.slice(0, cap - 1) + "…" : one;
}

// ONE output stream, judged line by line, for BOTH readers of it: the wake's tail and the log file. A
// recursive `grep -r KEY .` that names no secret path is allowed by the guard on purpose — the foreground
// path then removes the protected file's lines from the tool result (`redactSecretLines`, the `tool_result`
// handler). A background job has no tool result, and its log outlives the turn, so the same filter runs
// HERE: a line that a protected file answered with never reaches the log or the notice (Judgment Day round
// 1, judge A). `sink.partial` is capped, so newline-free output cannot grow the process's heap.
export function pushTail(job: { tail: string[]; partial: string; dropped: number }, chunk: string, cwd: string, write: (line: string) => void): void {
  const keep = (raw: string) => {
    const line = raw.endsWith("\r") ? raw.slice(0, -1) : raw;
    const { text, dropped } = redactSecretLines(line, cwd, undefined);
    if (dropped) { job.dropped += dropped; return; }
    write(text + "\n");                                    // the log IS the output: never truncated
    job.tail.push(text.length > 2000 ? "…" + text.slice(-2000) : text);   // the wake's tail is the display
    if (job.tail.length > BG_TAIL_LINES) job.tail.shift();
  };
  const parts = (job.partial + chunk).split("\n");
  job.partial = parts.pop() ?? "";
  for (const part of parts) keep(part);
  if (job.partial.length > 64 * 1024) { keep(job.partial); job.partial = ""; }   // one endless line
}

function bgTail(job: BgJob): string[] {
  // an unterminated last line is output too (judges of round 2, both): it is shown, shortened like the rest
  const lines = job.partial ? [...job.tail, job.partial.length > 2000 ? "…" + job.partial.slice(-2000) : job.partial] : job.tail;
  return lines.slice(-BG_TAIL_LINES);
}

function bgRunning(): number {
  let n = 0;
  for (const j of BG_JOBS.values()) if (j.status === "running") n++;
  return n;
}

// What the session is told when a job ends: what ran, how it ended, its last lines and where the whole
// output lives — everything needed to act without polling.
export function bgNotice(job: { id: string; label: string; command: string; exit?: number | null; status: string; error?: string; log: string; seconds: number; lines: string[] }): string {
  const outcome = job.status === "stopped" ? "was stopped"
    : job.status === "timeout" ? `ended: ${job.error ?? "its timeout"}`
    : job.status === "failed" ? `could not run: ${job.error ?? "unknown error"}`
    : `exited with code ${job.exit ?? "?"}`;
  const head = `[hw background] ${job.id} ("${firstLine(job.label, 60)}") ${outcome} after ${job.seconds}s.\ncommand: ${firstLine(job.command)}`;
  const tail = job.lines.length ? `last ${job.lines.length} line(s) of output:\n${job.lines.join("\n")}` : "no output.";
  const dropped = (job as { dropped?: number }).dropped ? `\n[hw] ${(job as { dropped?: number }).dropped} line(s) of a protected secret file were removed from this log (secret read guard).` : "";
  return `${head}\n${tail}${dropped}\nfull output: ${job.log} (read it with the read tool if you need more)`;
}

// THE HOST'S OWN SHELL OPERATIONS. An extension loaded from a worktree has no node_modules to resolve
// `@earendil-works/pi-coding-agent` from, so the package is found the way the running process knows it:
// from the entry script (`argv[1]`, the package's dist/bundle/cli.js) up to the package root. A tree whose
// host does not expose it falls back to a plain /bin/bash spawn and says so in the receipt.
const BG_SHELL_ENV = "HW_PI_SHELL_MODULE";   // a test's pointer AT A DIRECTORY (its package name is checked, below)
let bgOps: { exec: (command: string, cwd: string, options: any) => Promise<{ exitCode: number | null }> } | null = null;
let bgOpsSource = "";
// The package root of a path that must be pi itself: the name in its package.json is the identity, never the
// presence of a file (an inherited env var that names an arbitrary module would decide the shell and the kill).
function piPackageRoot(from: string): string | null {
  let dir = from;
  for (let i = 0; i < 6 && dir && dir !== "/"; i++, dir = dirname(dir)) {
    const pkg = join(dir, "package.json");
    if (!existsSync(pkg)) continue;
    try {
      if (String(JSON.parse(readFileSync(pkg, "utf8")).name) === "@earendil-works/pi-coding-agent") return dir;
    } catch { /* not a readable package */ }
  }
  return null;
}

async function shellOperations(): Promise<typeof bgOps> {
  if (bgOpsSource) return bgOps;
  const roots: string[] = [];
  try { const r = piPackageRoot(dirname(realpathSync(process.argv[1] || ""))); if (r) roots.push(r); } catch { /* argv[1] is not a path we can read */ }
  const hinted = process.env[BG_SHELL_ENV];
  if (hinted) { try { const r = piPackageRoot(isAbsolute(hinted) ? hinted : resolve(hinted)); if (r) roots.push(r); } catch { /* ignored */ } }
  const tries = roots.map((r) => join(r, "dist", "index.js"));
  for (const entry of tries) {
    try {
      const mod: any = await import(pathToFileURL(entry).href);
      if (typeof mod.createLocalBashOperations === "function") {
        bgOps = mod.createLocalBashOperations();
        bgOpsSource = entry;
        receiptRow("pi_background_shell", `native — createLocalBashOperations from ${entry}`, "hw-executor.ts bash_background");  // # MUTATION-ANCHOR: 1235-M01
        return bgOps;
      }
    } catch { /* try the next one */ }
  }
  bgOps = null;
  bgOpsSource = "fallback";
  receiptRow("pi_background_shell", `NOT NATIVE — no host createLocalBashOperations found (tried ${tries.join(", ") || "nothing"}); a plain /bin/bash spawn runs background jobs`, "hw-executor.ts bash_background");
  return null;
}

// The fallback, only for a host that did not hand us its shell operations: same contract.
function fallbackOps(log: string) {
  return {
    exec: (command: string, cwd: string, options: { onData: (d: Buffer) => void; signal?: AbortSignal }) => new Promise<{ exitCode: number | null }>((done, fail) => {
      const win = process.platform === "win32";
      const child = spawn(win ? process.env.ComSpec || "cmd.exe" : "/bin/bash", [win ? "/c" : "-c", command], { cwd, detached: !win, stdio: ["ignore", "pipe", "pipe"] });
      const killTree = () => { try { process.kill(-(child.pid as number), "SIGKILL"); } catch { try { child.kill("SIGKILL"); } catch { /* gone */ } } };
      if (options.signal?.aborted) killTree();   // a stop that landed before the child existed still kills it
      else options.signal?.addEventListener("abort", killTree, { once: true });
      child.stdout?.on("data", options.onData);
      child.stderr?.on("data", options.onData);
      child.on("error", (e) => fail(e));
      child.on("close", (code) => done({ exitCode: code }));
    }),
  };
}

// Starts one job (a plain background command, or a monitor when `mon` is given) and returns its record at once. The caller has
// checked the cap and the arguments; the call was judged by the guards before this runs.
function startJob(pi: any, dir: string | null, ctx: any, command: string, label: string, mon?: { pattern: RegExp | null; timeoutMs: number; maxEvents: number }): BgJob {
  const dirOf = bgLogDir(dir);
  const cwd = ctx?.cwd || process.cwd();
  const id = `bg-${++bgSeq}`;
  const log = join(dirOf, `${id}.log`);
  const job: BgJob = { id, label, command, cwd, log, started: Date.now(), status: "running", tail: [], partial: "", abort: new AbortController(), dropped: 0 };
  if (mon) job.monitor = { ...mon, events: 0, pending: [], flushAt: null, timer: null };
  try { mkdirSync(dirOf, { recursive: true }); writeFileSync(log, ""); } catch { /* the stream below will say why */ }
  BG_JOBS.set(id, job);
  const wake = (customType: string, content: string, details: Record<string, unknown>) => {
    // THE WAKE. deliverAs steer: it joins a turn already running, and triggerTurn starts one when the session is idle — the same
    // call the turn end makes for a queued ruling, and the one pi-subagents makes for a background child.
    try {
      Promise.resolve(pi.sendMessage({ customType, content, display: true, details }, { triggerTurn: true, deliverAs: "steer" }))
        .catch(() => { /* the session went away with the job: the .json and the receipt carry the fact */ });
    } catch { /* same */ }
  };
  const flushEvents = () => {
    const m = job.monitor;
    if (!m) return;
    if (m.flushAt) { clearTimeout(m.flushAt); m.flushAt = null; }
    if (!m.pending.length) return;
    const lines = m.pending.splice(0, m.pending.length);
    const shown = lines.slice(0, MONITOR_BATCH_LINES);
    wake("hw-monitor-lines", `[hw monitor] ${id} ("${firstLine(label, 60)}") — ${lines.length} new line(s):\n${shown.join("\n")}${lines.length > shown.length ? `\n… and ${lines.length - shown.length} more (full output: ${log})` : ""}`,
      { id, label, lines: lines.length, log });
  };
  const finish = (status: "exited" | "failed" | "timeout" | "stopped", exit: number | null, error?: string) => {
    if (job.status !== "running") return;  // an abort already said "stopped"
    job.status = status; job.exit = exit; job.error = error; job.ended = Date.now();
    if (job.monitor?.timer) { clearTimeout(job.monitor.timer); job.monitor.timer = null; }
    flushEvents();
    const seconds = Math.max(0, Math.round((job.ended - job.started) / 1000));
    try { writeAtomic(join(dirOf, `${id}.json`), JSON.stringify({ id, label, command, log, exit, status, error, seconds, ended: job.ended, ...(job.monitor ? { monitor_events: job.monitor.events } : {}) }) + "\n"); } catch { /* evidence, not a dependency */ }
    receiptRow("pi_background", `${id} ${status === "exited" ? "exit " + exit : status === "timeout" ? "monitor timeout" : status === "stopped" ? "stopped" : "failed: " + (error ?? "")} after ${seconds}s — ${firstLine(command)} (log ${log})${job.monitor ? ` [monitor, ${job.monitor.events} event(s)]` : ""}`, `hw-executor.ts ${job.monitor ? MONITOR_TOOL : BG_TOOL}`);
    if (status === "stopped") return;  // whoever stopped it already knows
    wake("hw-bg-exit", bgNotice({ ...job, seconds, lines: bgTail(job) }) + (job.monitor ? `\n[hw monitor] ${job.monitor.events} event line(s) were sent.` : ""),
      { id, label, exit, status, log, seconds });
    if (status === "timeout") { try { job.abort.abort(); } catch { /* gone */ } }
  };
  const onLine = (line: string) => {
    const m = job.monitor;
    if (!m || job.status !== "running") return;
    const text = line.replace(/\n$/, "");
    if (m.pattern && !m.pattern.test(text)) return;
    m.events++;
    m.pending.push(text.length > 500 ? text.slice(0, 499) + "…" : text);
    if (m.events >= m.maxEvents) {
      finish("timeout", null, `event cap (${m.maxEvents}) reached`);
      return;
    }
    if (m.pending.length >= MONITOR_BATCH_LINES) flushEvents();
    else if (!m.flushAt) m.flushAt = setTimeout(flushEvents, MONITOR_BATCH_MS);
  };
  if (job.monitor) job.monitor.timer = setTimeout(() => { try { finish("timeout", null, `its timeout (${Math.round((job.monitor?.timeoutMs ?? 0) / 1000)} s)`); } catch { /* ended */ } }, job.monitor.timeoutMs);  // # MUTATION-ANCHOR: 1251-M01
  void (async () => {
    const ops = (await shellOperations()) ?? fallbackOps(log);
    const out = createWriteStream(log, { flags: "a" });
    out.on("error", () => { /* a lost log must not crash the session: the tail survives */ });
    const decoder = new TextDecoder("utf-8");
    const sink = (line: string) => { out.write(line); onLine(line); };
    try {
      const { exitCode } = await ops.exec(command, cwd, {
        signal: job.abort.signal,
        onData: (data: Buffer) => {
          const text = decoder.decode(data, { stream: true });
          if (text) pushTail(job, text, cwd, sink);
        },
      });
      const rest = decoder.decode();
      if (rest) pushTail(job, rest, cwd, sink);
      if (job.partial) { pushTail(job, "\n", cwd, sink); }   // the last, unterminated line
      await new Promise<void>((r) => out.end(r));
      finish("exited", exitCode);
    } catch (e: any) {
      if (job.partial) { pushTail(job, "\n", cwd, sink); }   // a stopped or failed job keeps its last line too
      await new Promise<void>((r) => out.end(r));
      if (job.status === "running") finish("failed", null, String(e?.message ?? e));
    }
  })();
  return job;
}

function registerBackgroundTools(pi: any, dir: string | null) {
  pi.registerTool({
    name: BG_TOOL,
    label: "Bash (background)",
    // pi takes the schema as plain JSON Schema (an extension here has no typebox to import), and the model sees
    // this text: the tool has to say what happens next, or the agent polls.
    parameters: {
      type: "object",
      properties: {
        command: { type: "string", description: "The shell command to run in the session's working directory." },
        label: { type: "string", description: "Short human-readable label, e.g. \"fast gate for this branch\"." },
      },
      required: ["command"],
      additionalProperties: false,
    },
    description: "Run a shell command in the background and return at once with a job id. When the command exits this session is woken ONCE, automatically, with its exit code, its last 20 output lines and the path of its full output (pi's answer to Claude's `run_in_background=true`). Do not poll, sleep, `tail -f` or delegate a subagent to wait: the notice is the wait. Put the wait condition inside the command so its exit is the event — `timeout 600 bash -c 'until curl -sf localhost:3000/health; do sleep 2; done'` (the `timeout` is what makes it a bounded wait; an unbounded poll loop or a `tail -f` is refused even here), or `gh run watch <id> --exit-status`. Use it for a long or unknown-duration run — the fast gate, a heavy job, a build, a service — and use `bash` for a finite check you expect within 30 s. Stop it with `job_stop`; at most " + BG_MAX_RUNNING + " run at once.",
    promptSnippet: "Run a shell command in the background; the session is woken when it exits",
    async execute(_id: string, params: any, _signal: any, _onUpdate: any, ctx: any) {
      const command = typeof params?.command === "string" ? params.command : "";
      const label = typeof params?.label === "string" && params.label.trim() ? params.label.trim() : command;
      if (!command.trim()) {
        return { content: [{ type: "text", text: "bash_background needs a `command`." }], isError: true };
      }
      if (bgRunning() >= BG_MAX_RUNNING) {
        return { content: [{ type: "text", text: `${BG_MAX_RUNNING} background jobs are already running; stop one with job_stop first.` }], isError: true };
      }
      const job = startJob(pi, dir, ctx, command, label);
      return {
        content: [{ type: "text", text:
          `Started ${job.id} ("${firstLine(label, 60)}") in the background.\nlog: ${job.log}\n` +
          "You will be woken once, when it exits, with its exit code and its last lines — do not poll, sleep or `tail -f` for it. End your turn, or do other work." }],
        details: { id: job.id, label, log: job.log },
      };
    },
  });

  // MONITOR: a job whose output LINES are events. Same guards (the call is judged before it starts, as a background tool),
  // same output filter (every line passes redactSecretLines before it reaches the log or an event), same registry and cap.
  pi.registerTool({
    name: MONITOR_TOOL,
    label: "Monitor (line events)",
    parameters: {
      type: "object",
      properties: {
        command: { type: "string", description: "The shell command whose output lines are the events, e.g. `tail -n0 -F build.log` or `gh run watch <id>`." },
        timeout_seconds: { type: "number", description: `REQUIRED. How long the monitor may run, in seconds (1 to ${MONITOR_MAX_TIMEOUT_S}). It ends at its timeout whether or not the command did.` },
        pattern: { type: "string", description: "Optional regular expression: only output lines matching it are events. Without it every line is." },
        label: { type: "string", description: "Short human-readable label." },
        max_events: { type: "number", description: `Stop after this many event lines (default ${MONITOR_DEFAULT_MAX_EVENTS}, at most ${MONITOR_MAX_EVENTS}).` },
      },
      required: ["command", "timeout_seconds"],
      additionalProperties: false,
    },
    description: `Watch a long command's output for lines you care about, without polling: it starts the command in the background and returns at once; each batch of matching output lines (one second's worth is ONE batch) WAKES this session as a message, and a final message says why the monitor ended (the command exited, its timeout, or its event cap). A timeout is mandatory, so a monitor cannot outlive the question it was started for. Use it when the lines themselves are the signal (a build's errors, a deploy's phases); use bash_background when only the exit matters. Stop it early with job_stop; list jobs with job_list; at most ${BG_MAX_RUNNING} jobs run at once.`,
    promptSnippet: "Watch a command's output lines as events; the session is woken per batch, with a mandatory timeout",
    async execute(_id: string, params: any, _signal: any, _onUpdate: any, ctx: any) {
      const command = typeof params?.command === "string" ? params.command : "";
      if (!command.trim()) return { content: [{ type: "text", text: "monitor needs a `command`." }], isError: true };
      const t = Number(params?.timeout_seconds);
      if (!Number.isFinite(t) || t <= 0 || t > MONITOR_MAX_TIMEOUT_S) {
        return { content: [{ type: "text", text: `monitor needs a \`timeout_seconds\` between 1 and ${MONITOR_MAX_TIMEOUT_S}: a monitor with no deadline is an unbounded wait, which this executor does not start.` }], isError: true };
      }
      let pattern: RegExp | null = null;
      if (typeof params?.pattern === "string" && params.pattern !== "") {
        try { pattern = new RegExp(params.pattern); } catch (e: any) { return { content: [{ type: "text", text: `monitor: \`pattern\` is not a regular expression (${String(e?.message ?? e)}).` }], isError: true }; }
      }
      const cap = Number(params?.max_events);
      const maxEvents = Number.isFinite(cap) && cap >= 1 ? Math.min(Math.floor(cap), MONITOR_MAX_EVENTS) : MONITOR_DEFAULT_MAX_EVENTS;
      if (bgRunning() >= BG_MAX_RUNNING) {
        return { content: [{ type: "text", text: `${BG_MAX_RUNNING} background jobs are already running; stop one with job_stop first.` }], isError: true };
      }
      const label = typeof params?.label === "string" && params.label.trim() ? params.label.trim() : command;
      const job = startJob(pi, dir, ctx, command, label, { pattern, timeoutMs: Math.round(t * 1000), maxEvents });
      return {
        content: [{ type: "text", text:
          `Monitoring ${job.id} ("${firstLine(label, 60)}") for up to ${t}s${pattern ? `, events = lines matching /${pattern.source}/` : ""}, at most ${maxEvents} events.\nlog: ${job.log}\n` +
          "You will be woken with each batch of event lines and once more when the monitor ends — do not poll or sleep. End your turn, or do other work." }],
        details: { id: job.id, label, log: job.log, timeout_seconds: t },
      };
    },
  });

  // LIST: what this session started, with the state the notices would have said. A read, never a write.
  pi.registerTool({
    name: BG_LIST_TOOL,
    label: "List background jobs",
    parameters: { type: "object", properties: { status: { type: "string", enum: ["running", "all"], description: "running (default) or all, including the jobs that ended." } }, additionalProperties: false },
    description: "List the background jobs and monitors this session started (bash_background and monitor): id, status, age, label, command, log path and the last output line. Use it to find a job id for job_stop, or to see what is still running before ending a turn.",
    async execute(_id: string, params: any) {
      const all = params?.status === "all";
      const rows = [...BG_JOBS.values()].filter((j) => all || j.status === "running");
      if (!rows.length) return { content: [{ type: "text", text: all ? "No background job was started in this session." : "No background job is running." }], details: { jobs: [] } };
      const now = Date.now();
      const text = rows.map((j) => {
        const secs = Math.max(0, Math.round(((j.ended ?? now) - j.started) / 1000));
        const last = bgTail(j).slice(-1)[0];
        const kind = j.monitor ? `monitor ${j.monitor.events} event(s)` : "job";
        return `${j.id} ${j.status}${j.status === "exited" ? ` (exit ${j.exit ?? "?"})` : ""} ${secs}s [${kind}] "${firstLine(j.label, 60)}" — ${firstLine(j.command)}\n  log: ${j.log}${last ? `\n  last: ${firstLine(last, 160)}` : ""}`;
      }).join("\n");
      return { content: [{ type: "text", text }], details: { jobs: rows.map((j) => ({ id: j.id, status: j.status, label: j.label, log: j.log })) } };
    },
  });

  // STOP: a job nobody stops can outlive a whole task. The abort is pi's own — its shell operations kill the
  // job's whole process tree on that signal, the same kill native `bash` gives the Stop button.
  pi.registerTool({
    name: BG_STOP_TOOL,
    label: "Stop background job",
    parameters: { type: "object", properties: { job_id: { type: "string", description: "The id bash_background returned, e.g. bg-2." } }, required: ["job_id"], additionalProperties: false },
    description: "Stop a background job started with bash_background, killing its whole process tree. A stopped job sends no exit notice (whoever stops it already knows), and its log stays on disk.",
    async execute(_id: string, params: any) {
      const wanted = typeof params?.job_id === "string" ? params.job_id : "";
      const job = BG_JOBS.get(wanted);
      if (!job) return { content: [{ type: "text", text: `No background job ${wanted || "(none named)"} in this session.` }], isError: true };
      if (job.status !== "running") return { content: [{ type: "text", text: `${job.id} was already ${job.status}.` }], details: { id: job.id, status: job.status } };
      job.status = "stopped"; job.ended = Date.now();
      if (job.monitor?.timer) { clearTimeout(job.monitor.timer); job.monitor.timer = null; }
      if (job.monitor?.flushAt) { clearTimeout(job.monitor.flushAt); job.monitor.flushAt = null; }
      try { job.abort.abort(); } catch { /* the tree is already gone */ }
      receiptRow("pi_background", `${job.id} stopped by the executor after ${Math.max(0, Math.round((job.ended - job.started) / 1000))}s — ${firstLine(job.command)}`, "hw-executor.ts job_stop");
      return { content: [{ type: "text", text: `Stopped ${job.id} ("${firstLine(job.label, 60)}"). Its log stays at ${job.log}.` }], details: { id: job.id, status: "stopped" } };
    },
  });
}

// THE LIVE JOBS, for hw's turn end: a turn that ends while one runs is legal (ended_awaiting_child), which is
// what lets the wake start the next turn instead of hw refusing the handoff.
export function liveBackgroundTasks(): Array<{ type: string; status: string; command: string }> {
  return [...BG_JOBS.values()].filter((j) => j.status === "running")
    .map((j) => ({ type: j.monitor ? "monitor" : "shell", status: "running", command: firstLine(j.command) }));
}

// Stop what this session started: a pi process that ends must not leave its background shells running.
export function stopBackgroundJobs(): number {
  let n = 0;
  for (const j of BG_JOBS.values()) {
    if (j.status !== "running") continue;
    j.status = "stopped"; j.ended = Date.now(); n++;
    if (j.monitor?.timer) { clearTimeout(j.monitor.timer); j.monitor.timer = null; }
    if (j.monitor?.flushAt) { clearTimeout(j.monitor.flushAt); j.monitor.flushAt = null; }
    try { j.abort.abort(); } catch { /* already gone */ }
  }
  return n;
}

const ROOT_SESSION = Symbol.for("hw.pi.root-session");

// ── the exploration budget ──────────────────────────────────────────────────
// MEASURED 2026-11-04: a pi executor calls a subagent in ~35% of sessions where a Claude executor does in ~100%, and
// reads file after file inline instead. The IDEA — a per-request budget of discovery results, steered ONCE toward the
// explorer — was read as a reference from gentle-pi (MIT, 078a7eb). NO CODE is copied from it: this is our own minimal
// version, on pi's own tools (read/grep/find/ls; no bash phase inference, no memory tools) with its own state,
// thresholds and text, and it steers through pi's native sendMessage(deliverAs:"steer"). A lane with no explorer is
// never nudged: hw sets HW_PI_EXPLORE_AGENT only when pi-subagents and the explorer are both there.
const DISCOVERY_TOOLS = new Set(["read", "grep", "find", "ls"]);
const EXPLORATION_LOOKUPS = 3, EXPLORATION_BYTES = 40_000;
function explorationNotice(agent: string, lookups: number, bytes: number): string {
  return `[hw] Exploration budget: ${lookups} lookups and ${bytes.toLocaleString("en-US")} bytes read inline this turn. ` +
    `Past this point, delegate the reading: call the \`subagent\` tool with agent "${agent}" and work from its answer — it runs on the cheap tier and returns facts, not files. ` +
    `Read only what you need to make your own edit.`;
}

// The account's global rules file: $CLAUDE_CONFIG_DIR/CLAUDE.md, default ~/.claude/CLAUDE.md. "" when absent or empty.
export function globalRulesFile(): string {
  const file = join(process.env.CLAUDE_CONFIG_DIR || join(homedir(), ".claude"), "CLAUDE.md");
  try { const st = statSync(file); return st.isFile() && st.size > 0 ? file : ""; } catch { return ""; }
}

// THE GUARDS AS PI EVENTS, registered by every pi process that must be held to the policy: the executor's default
// export below, and a plugin's children (setup/pi/hw-child-guard.ts), which start pi with discovery ON and would
// otherwise find no guard at all. `before` is an extra refusal the caller owns; a child passes none.
// `pi.on("tool_result")` returns nothing at any point (the redaction is a result rewrite, not a refusal).
// A BASH CALL THAT SETS NO TIMEOUT IS BOUNDED. pi's `bash` has "no default timeout" (dist/core/tools/bash.js), and a
// call that blocks cannot be steered: MEASURED 2026-11-04 in pi-theme-and-models, two foreground probes hung 83 min
// (`script … pi` 28 min; `tail -f /dev/null | … timeout 60 pi` 55 min, the `tail -f` keeping the pipe open past pi's
// own timeout) until a ruling arrived. The default is 600 s: the longest a Claude Bash call may run (its own maximum),
// 4.5x the longest probe an executor legitimately needs in the foreground, and far under the 28 min that already
// cost an hour. Anything longer belongs in `bash_background`, which wakes the session when it ends. A call that
// sets ANY timeout keeps it, `bash_background` is untouched, and a session with no HW_* environment is not an executor.
// HW_PI_BASH_DEFAULT_TIMEOUT_S overrides the number (the probe and the tests use a short one); 0 turns it off.
const BASH_DEFAULT_TIMEOUT_S = 600;
function bashDefaultTimeoutS(): number {
  const raw = process.env.HW_PI_BASH_DEFAULT_TIMEOUT_S;
  if (raw === undefined || raw === "") return BASH_DEFAULT_TIMEOUT_S;
  const n = Number(raw);
  return Number.isFinite(n) && n >= 0 ? n : BASH_DEFAULT_TIMEOUT_S;
}
const defaultedBashCalls = new Map<string, number>(); // toolCallId -> the seconds hw put on a call that set none
export function applyBashDefaultTimeout(event: any): void {
  if (event?.toolName !== "bash" || !isHwExecutor()) return;
  const input = event?.input;
  if (input === null || typeof input !== "object" || Array.isArray(input)) return;
  if (input.timeout !== undefined && input.timeout !== null) return;  // the model chose one: it keeps it
  const secs = bashDefaultTimeoutS();
  if (secs <= 0) return;
  input.timeout = secs;  // # MUTATION-ANCHOR: 1243-M01
  if (typeof event.toolCallId === "string") defaultedBashCalls.set(event.toolCallId, secs);
}
export function bashTimeoutNote(event: any): string | null {
  const id = event?.toolCallId;
  if (event?.toolName !== "bash" || typeof id !== "string") return null;
  const secs = defaultedBashCalls.get(id);
  if (secs === undefined) return null;
  defaultedBashCalls.delete(id);
  const text = Array.isArray(event?.content) ? event.content.map((c: any) => (typeof c?.text === "string" ? c.text : "")).join("\n") : "";
  if (!text.includes(`Command timed out after ${secs} seconds`)) return null;
  return `[hw] this bash call set no timeout, so hw bounded it at ${secs} s: a foreground command that never ends holds your whole turn, and a ruling cannot reach you while it blocks (two probes hung 83 min, 2026-11-04). ` +
    `Run anything long or open-ended with ${BG_TOOL} (it returns at once and wakes you when the command ends), or pass an explicit timeout.`;
}

export function registerGuards(pi: any, before?: (event: any, ctx: any) => any): void {
  pi.on("tool_call", async (event: any, ctx: any) => {
    const first = before?.(event, ctx);
    if (first) return first;
    applyBashDefaultTimeout(event);
    const reason = await judge(event?.toolName, event?.input, ctx?.cwd || process.cwd());
    return reason ? { block: true, reason } : undefined;
  });

  pi.on("tool_result", (event: any, ctx: any) => {
    if ((event?.toolName !== "grep" && event?.toolName !== "bash") || !Array.isArray(event?.content)) return;
    const timeoutNote = bashTimeoutNote(event);
    const cwd = ctx?.cwd || process.cwd();
    let dropped = 0;
    const content = event.content.map((c: any) => {
      if (c?.type !== "text" || typeof c.text !== "string") return c;
      const r = redactSecretLines(c.text, cwd, event?.input?.path);  // # MUTATION-ANCHOR: 989-M06
      dropped += r.dropped;
      return { ...c, text: r.text };
    });
    const notes: any[] = [];
    if (dropped) notes.push({ type: "text", text: `[hw] ${dropped} matching line(s) of a protected secret file were removed from this result (secret read guard).` });
    if (timeoutNote) notes.push({ type: "text", text: timeoutNote });
    if (!notes.length) return;
    return { content: [...content, ...notes] };
  });
}

export default function (pi: any) {
  // THE BACKGROUND SHELL IS A PLAIN TOOL, not executor state: every pi session that loads this file gets it, so a
  // probe with no HW_* environment can use it too (its log then lives under the temp dir). A pi-subagents child
  // is skipped: it is not the executor, its turn is its parent's foreground call, and hw runs specialists
  // foreground-only. Registering twice is harmless (pi keys tools by name), which keeps a reload simple.
  if (process.env.PI_SUBAGENT_CHILD !== "1" && typeof pi.registerTool === "function") {
    registerBackgroundTools(pi, runDir());
    // the async subagents in flight, for the sleep rule (see activeWork): pi-subagents announces each launch and each completion
    try {
      pi.events?.on?.("subagent:async-started", (d: any) => { if (d?.id) ASYNC_SUBAGENTS.add(String(d.id)); });
      pi.events?.on?.("subagent:async-complete", (d: any) => { if (d?.id) ASYNC_SUBAGENTS.delete(String(d.id)); if (d?.runId) ASYNC_SUBAGENTS.delete(String(d.runId)); });
    } catch { /* no event bus: the jobs alone are counted */ }
    // A session with no HW_* gets the tools too (its log lives in the temp dir), so its shutdown must stop its
    // jobs as well: the executor's handler below is past an early return (Judgment Day round 1, judge B).
    pi.on("session_shutdown", () => { try { stopBackgroundJobs(); } catch { /* nothing left to stop */ } ASYNC_SUBAGENTS.clear(); });
  }

  registerGuards(pi, (event) => {
    // A SUBAGENT'S CHILD IS GUARDED ONLY IF THIS FILE WAS REGISTERED AS A REQUIRED CHILD EXTENSION (session_start,
    // below). When that did not happen — the API is missing in this pi-subagents, the session had no id, the call
    // threw — the child would run with no guard at all, and a scan of the tool's arguments cannot see what a child
    // does. So the tool is refused rather than trusted: the guard fails closed.
    if (isHwExecutor() && /^subagents?$/.test(String(event?.toolName ?? "")) && !(globalThis as any)[Symbol.for("hw.pi.required-child")]) {  // # MUTATION-ANCHOR: 988-M05
      return { block: true, reason: "Blocked: the subagent tool is refused because this executor's write guard is NOT registered as a required child extension of pi-subagents (see the pi_child_guard row of the receipt), so a child would run unguarded." };
    }
    return undefined;
  });

  if (!isHwExecutor()) return;

  const G = globalThis as any;
  // THE PROXY IS REGISTERED ONCE, BY THE ROOT. A pi-subagents child loads this file too (that is how the
  // guard reaches it), and its extension set has no MCP connector: pi refuses a child that registers
  // a server nothing connects ("MCP server hw_engram is registered, but no loaded extension connects
  // MCP servers" — measured live, the first subagent died at startup on exactly that). The root
  // factory runs first and sets the flag; a child's finds it set and skips. A reload of the root
  // clears it at shutdown, so the next factory run registers again.
  // pi-subagents names its children in the environment (PI_SUBAGENT_CHILD=1), in-process or detached:
  // the case a process-wide flag cannot see, because a detached runner is another process.
  const MCP_FLAG = Symbol.for("hw.pi.mcp-registered");
  const SPECIALISTS_FLAG = Symbol.for("hw.pi.specialists-registered");
  const subagentChild = process.env.PI_SUBAGENT_CHILD === "1";
  const server = engramProxyServer();
  if (server && typeof pi.registerMcpServer === "function" && !G[MCP_FLAG] && !subagentChild) {
    try { pi.registerMcpServer("hw_engram", server); G[MCP_FLAG] = true; } catch { /* reported by pi */ }  // # MUTATION-ANCHOR: 986-M12
  }

  // THE ROOT SESSION IS THE ONE THAT OPENED THE PROCESS. pi-subagents' children are sessions of
  // this same process and load this file too (that is how the guard reaches them); a child's
  // turn end, inputs and model are not the executor's. The first session to start claims the
  // process-wide slot; the same instance re-claims it when its own session changes.
  let isRoot = false;
  const sessionOf = (ctx: any): string | null => {
    try { return ctx?.sessionManager?.getSessionFile?.() || ctx?.sessionManager?.getSessionId?.() || null; } catch { return null; }
  };
  const ensureRoot = (ctx: any): boolean => {
    if (subagentChild) return false; // pi-subagents' own word for "not the executor": never root
    const s = sessionOf(ctx);
    if (!s) return isRoot;
    if (G[ROOT_SESSION] === undefined || isRoot) { G[ROOT_SESSION] = s; isRoot = true; }
    else isRoot = G[ROOT_SESSION] === s;  // # MUTATION-ANCHOR: 988-M02
    return isRoot;
  };

  const dir = runDir() as string;
  let turns = 0, inputs = 0, switchedTo: string | null = null;
  let lastError: { message: string; model: string } | null = null;
  let inputSinceSettle = false;
  let exploration = { lookups: 0, bytes: 0, notified: false };

  const modelOf = (ctx: any): string => (ctx?.model ? `${ctx.model.provider}/${ctx.model.id}` : "");
  const thinkingOf = (ctx: any): string => {
    try { return String(pi.getThinkingLevel?.() ?? ctx?.thinkingLevel ?? ""); } catch { return String(ctx?.thinkingLevel ?? ""); }
  };
  const ctxUsage = (ctx: any) => {
    try {
      const u = ctx?.getContextUsage?.();
      if (!u) return null;
      const n = (v: unknown) => (typeof v === "number" && Number.isFinite(v) ? v : null);
      return { tokens: n(u.tokens), window: n(u.contextWindow), pct: n(u.percent) };
    } catch { return null; }
  };
  const publishRuntime = (ctx: any) => {
    if (!ensureRoot(ctx)) return;
    try {
      mkdirSync(dir, { recursive: true });
      const model = modelOf(ctx);
      writeAtomic(join(dir, "pi-runtime.json"), JSON.stringify({
        session_file: sessionOf(ctx), session_id: (() => { try { return ctx.sessionManager.getSessionId(); } catch { return null; } })(),
        provider: ctx?.model?.provider ?? null, model: ctx?.model?.id ?? null, model_ref: model || null,
        thinking: thinkingOf(ctx) || null, ctx: ctxUsage(ctx), turns, fallback: switchedTo,
        at: new Date().toISOString(),
      }) + "\n");
    } catch { /* evidence, not a dependency of the work */ }
  };

  // THE ACCOUNT'S GLOBAL RULES. pi loads <agent dir>/AGENTS.md|CLAUDE.md and the ones in the working directory's
  // ancestors, never the account's ~/.claude/CLAUDE.md that a Claude executor starts with. pi's own slot for "user
  // instructions" is systemPromptOptions.contextFiles, which an extension may add to in before_agent_start: the file is
  // read, not copied, and nothing is written under ~/.pi. The executor only (a child is not this session).
  pi.on("before_agent_start", (event: any, ctx: any) => {
    if (!ensureRoot(ctx)) return;
    const file = globalRulesFile();
    const files = event?.systemPromptOptions?.contextFiles;
    if (!file || !Array.isArray(files) || files.some((f: any) => f?.path === file)) return;
    try {
      files.unshift({ path: file, content: readFileSync(file, "utf8") });
    } catch { /* unreadable: the receipt row says what was found at session start */ }
  });

  pi.on("session_start", async (event: any, ctx: any) => {
    if (!ensureRoot(ctx)) return;
    publishRuntime(ctx);
    const rules = globalRulesFile();
    receiptRow("pi_global_rules", rules ? `${rules} (added to the system prompt as a context file by hw-executor.ts)` : "none: no CLAUDE.md in the account's config dir", "hw-executor.ts before_agent_start");
    // The guard travels with the subagents (see the header). Only where pi-subagents is loaded.
    try {
      const sub = process.env.HW_PI_SUBAGENTS_DIR
        || join(process.env.PI_CODING_AGENT_DIR || join(homedir(), ".pi", "agent"), "npm", "node_modules", "pi-subagents");
      const entry = join(sub, "src", "api", "required-child-extensions.js");
      const sessionId = ctx.sessionManager.getSessionId();
      if (existsSync(entry) && sessionId && !G[Symbol.for("hw.pi.required-child")]) {
        const mod = await import(pathToFileURL(entry).href);
        G[Symbol.for("hw.pi.required-child")] = mod.registerRequiredChildExtensions({
          sessionId, extensions: [{ id: "hw-executor", path: SELF }], requireForAllRunners: true,
        });
      }
    } catch (e: any) {
      receiptRow("pi_child_guard", `NOT REGISTERED — ${String(e?.message ?? e).slice(0, 200)}`, "hw-executor.ts session_start");
    }
    // Can a background child start here? The row nothing wrote when `pi update` removed a hand-installed chord.
    try {
      const sub = process.env.HW_PI_SUBAGENTS_DIR
        || join(process.env.PI_CODING_AGENT_DIR || join(homedir(), ".pi", "agent"), "npm", "node_modules", "pi-subagents");
      let hostRoot: string | null = null;
      try { hostRoot = piPackageRoot(dirname(realpathSync(process.argv[1] || ""))); } catch { /* argv[1] is not a path we can read */ }
      const st = backgroundSubagentsStatus(sub, hostRoot);
      receiptRow("pi_background_subagents", st.text, "hw-executor.ts session_start");
    } catch (e: any) {
      receiptRow("pi_background_subagents", `NOT CHECKED — ${String(e?.message ?? e).slice(0, 200)}`, "hw-executor.ts session_start");
    }
    // The lane's `.claude/agents`, registered where pi-subagents is loaded and the guard travels with the children.
    try {
      const sub = process.env.HW_PI_SUBAGENTS_DIR
        || join(process.env.PI_CODING_AGENT_DIR || join(homedir(), ".pi", "agent"), "npm", "node_modules", "pi-subagents");
      if ((process.env.HW_PI_CLAUDE_AGENT_DIRS || process.env.HW_PI_PLUGIN_AGENT_DIRS) && G[Symbol.for("hw.pi.required-child")] && !G[SPECIALISTS_FLAG]) {
        const r = await registerClaudeSpecialists(pi, sub);
        G[SPECIALISTS_FLAG] = r.disposers;
        receiptRow("pi_specialists", `registered ${r.registered.join(" ") || "none"}${r.runs.length ? `; runs ${r.runs.join(", ")}` : ""}${r.skipped.length ? `; skipped ${r.skipped.join(", ")}` : ""}${r.dropped.length ? `; tools with no pi counterpart dropped: ${r.dropped.join(" ")}` : ""}`, "hw-executor.ts session_start");
      }
    } catch (e: any) {
      receiptRow("pi_specialists", `NOT REGISTERED — ${String(e?.message ?? e).slice(0, 200)}`, "hw-executor.ts session_start");
    }
  });
  pi.on("session_shutdown", () => {
    if (!isRoot) return; // a child's shutdown is not the process's
    try { stopBackgroundJobs(); } catch { /* nothing left to stop */ }
    try { G[Symbol.for("hw.pi.required-child")]?.dispose?.(); } catch { /* idempotent */ }
    G[Symbol.for("hw.pi.required-child")] = undefined;
    for (const d of G[SPECIALISTS_FLAG] ?? []) { try { d(); } catch { /* idempotent */ } }
    G[SPECIALISTS_FLAG] = undefined;
    G[MCP_FLAG] = undefined;
  });

  // THE `processed` PROOF. The first thing a prompt does inside pi is arrive here, so what lands in the
  // session is hashed as it lands; hw compares it with the brief's own hash (receipt `brief_sha256`).
  pi.on("input", (event: any, ctx: any) => {
    if (!ensureRoot(ctx)) return;
    inputSinceSettle = true;
    quotaReported = false; // a new prompt (a re-task, a resume) is a new chance to be told
    const text = typeof event?.text === "string" ? event.text : "";
    try {
      mkdirSync(dir, { recursive: true });
      appendFileSync(join(dir, "pi-inputs.jsonl"), JSON.stringify({
        n: ++inputs, at: new Date().toISOString(), source: event?.source ?? null,
        sha256: sha256(text), normalized_sha256: sha256(normalizeInput(text)), bytes: Buffer.byteLength(text, "utf8"),
        session_file: sessionOf(ctx),
      }) + "\n");
    } catch { /* evidence */ }
  });

  pi.on("agent_start", (_e: any, ctx: any) => { lastError = null; exploration = { lookups: 0, bytes: 0, notified: false }; publishRuntime(ctx); });
  // The budget is counted per request (agent_start resets it) and steered once. It is the root executor's: a child's
  // reads are what the explorer is for, and a nested call is one lookup, not the tool it wrapped.
  pi.on("tool_result", async (event: any, ctx: any) => {
    const agent = process.env.HW_PI_EXPLORE_AGENT;
    if (!agent || event?.parentToolCallId || event?.isError || !DISCOVERY_TOOLS.has(String(event?.toolName ?? ""))) return;
    if (!ensureRoot(ctx)) return;
    exploration.lookups++;
    for (const c of Array.isArray(event?.content) ? event.content : []) {
      if (c?.type === "text" && typeof c.text === "string") exploration.bytes += Buffer.byteLength(c.text, "utf8");
    }
    if (exploration.notified || (exploration.lookups <= EXPLORATION_LOOKUPS && exploration.bytes <= EXPLORATION_BYTES)) return;
    exploration.notified = true;
    let outcome = "HELD (the run was idle)";
    try {
      if (typeof ctx?.isIdle !== "function" || !ctx.isIdle()) {
        // the API is promise-returning (a rejected delivery would be an unhandled rejection and end the session):
        // await it, the way the background shell's wake does
        await Promise.resolve(pi.sendMessage({ customType: "hw.exploration-budget", content: explorationNotice(agent, exploration.lookups, exploration.bytes), display: true }, { deliverAs: "steer", triggerTurn: true }));
        outcome = "SENT";
      }
    } catch (e: any) { outcome = `FAILED (${String(e?.message ?? e).slice(0, 80)})`; /* advisory only */ }
    receiptRow("pi_exploration_budget", `${exploration.lookups} lookups, ${exploration.bytes} bytes; steer ${outcome} → ${agent}`, "hw-executor.ts tool_result");
  });
  pi.on("model_select", (_e: any, ctx: any) => publishRuntime(ctx));
  pi.on("thinking_level_select", (_e: any, ctx: any) => publishRuntime(ctx));
  pi.on("message_end", (event: any, ctx: any) => {
    const m = event?.message;
    if (m?.role === "assistant" && m.stopReason === "error") {
      lastError = { message: String(m.errorMessage ?? ""), model: m.provider && m.model ? `${m.provider}/${m.model}` : modelOf(ctx) };
    }
    publishRuntime(ctx);
  });
  pi.on("turn_end", (_e: any, ctx: any) => { if (ensureRoot(ctx)) turns++; publishRuntime(ctx); });

  pi.on("agent_before_settle", async (event: any, ctx: any) => {
    if (!ensureRoot(ctx)) return; // a child's boundary is not the executor's
    // A MODEL ERROR IS THE ROSTER'S WAY OUT; A SPENT QUOTA IS NOT (see modelErrorFallback): the cap is
    // shared dollars, so there is no other Go model to go to. The executor is told, in the receipt, and
    // the run ends: reporting it blocked is the brainer's/executor's call, not a silent switch.
    if (event?.outcome === "error" && lastError) {
      const current = lastError.model || modelOf(ctx);
      const err = lastError.message.replace(/\s+/g, " ").slice(0, 200);
      if (QUOTA_ERROR.test(lastError.message)) {
        receiptRow("model_fallback", `NOT SWITCHED — ${current} hit a quota error and opencode-go's cap is shared dollars for the whole key, so another Go model draws from the same cupo: ${err}`, "hw-executor.ts agent_before_settle");
        receiptRow("provider_quota", `SPENT — report this task blocked (done-invoker --blocked): ${err.slice(0, 120)}`, "hw-executor.ts agent_before_settle");
        await reportQuotaBlocked(err);
        lastError = null;
        return;
      }
      const to = modelErrorFallback(lastError.message, current, process.env.HW_PI_FALLBACK_MODEL);
      if (to) {
        const [prov, ...rest] = to.split("/");
        const target = ctx?.modelRegistry?.find?.(prov, rest.join("/"));
        const prevThinking = thinkingOf(ctx);
        const wanted = process.env.HW_PI_FALLBACK_THINKING;
        let ok = false;
        try { ok = !!target && (await pi.setModel(target)) === true; } catch { ok = false; }
        if (ok) {
          const level = wanted && THINKING_LEVELS.has(wanted) ? wanted : prevThinking;
          try { if (level) pi.setThinkingLevel(level); } catch { /* clamped by pi */ }
          switchedTo = to;
          lastError = null;
          const src = "hw-executor.ts agent_before_settle: the model's terminal error, then pi.setModel";
          receiptRow("model_fallback", `${current} → ${to} (thinking ${level || "unchanged"}) — model error: ${err}`, src);
          receiptRow("model_running", to, src);
          receiptRow("model_verdict", `FALLBACK — asked ${process.env.HW_PI_ASKED_MODEL || current}, it answered a model error (${err.slice(0, 80)}); the roster's fallback ${to} runs`, src);
          publishRuntime(ctx);
          return {
            entries: [{ type: "custom_message", customType: "hw-model-fallback", display: true,
              content: `[hw] The model ${current} failed with an error of its own, so this executor now runs ${to} (the roster's fallback for this kind). Continue the work from where it stopped; do not start over.` }],
            continue: true,
          };
        }
        receiptRow("model_fallback", `NOT SWITCHED — ${current} failed and ${to} could not be selected`, "hw-executor.ts agent_before_settle");
      }
    }
    // An abort is a person at the keyboard (Esc), and an error is a provider
    // that failed: continuing either would loop against the thing that stopped it.
    if (event?.outcome && event.outcome !== "completed") return;
    const hadInput = inputSinceSettle;
    inputSinceSettle = false;
    const usage = ctxUsage(ctx);
    // A TURN THAT ENDS WHILE A BACKGROUND SHELL RUNS IS LEGAL. hw's turn-end predicate refuses a "handback" — a turn
    // ending with no fact on disk — and the one legal end it cannot read off disk is a live child (core/t3_turnend.go,
    // background_tasks). Claude's Stop payload carries it; pi's event does not, so the adapter hands over the jobs its
    // own tool started. Without this the executor would be forced to keep the turn alive, and the wake would land
    // after the work it was meant to deliver.
    const live = liveBackgroundTasks();
    const reason = await runTurnEnd(hadInput, {
      session_id: sessionOf(ctx), model: modelOf(ctx) || null, thinking: thinkingOf(ctx) || null,
      ...(usage ? { ctx: usage } : {}),
      ...(live.length ? { background_tasks: live } : {}),
    });
    if (!reason) return;
    return {
      entries: [{ type: "custom_message", customType: "hw-turn-end", content: reason, display: true }],
      continue: true,
    };
  });
}

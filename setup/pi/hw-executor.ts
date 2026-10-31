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
//      copied or rewritten on disk.
//
// Inert outside an hw executor (HW_TASK/HW_RUN/HW_WORKDIR), except the guard,
// which is policy for any session that loads this file.

import { execFile, spawnSync } from "node:child_process";
import { createHash } from "node:crypto";
import { appendFileSync, existsSync, mkdirSync, readFileSync, readdirSync, realpathSync, renameSync, statSync, writeFileSync } from "node:fs";
import { homedir, userInfo } from "node:os";
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
function homeDir(): string { return userInfo().homedir; }
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
  const commandIn = (command: string): string | null => {
    const g = globalHookVerdict(command);
    if (g) return g;
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
  for (const c of commands) { const r = commandIn(c); if (r) return r; }
  for (const p of paths) { const r = pathIn(p); if (r) return r; }
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
// What hw's own specialist list leaves out (`_specialist_agents`): the framework and review agents.
const NOT_SPECIALIST = /^(?:sdd-|speckit-|review-|jd-)/;

export function mapClaudeTools(list: string[]): { tools: string[]; dropped: string[] } {
  const tools: string[] = [], dropped: string[] = [];
  for (const t of list) {
    const to = CLAUDE_TOOLS[t.toLowerCase()];
    if (!to) dropped.push(t);
    else if (!tools.includes(to)) tools.push(to);
  }
  return { tools, dropped };
}

// A Claude agent file → pi-subagents' runtime definition. `parse` is pi-subagents' own frontmatter reader, so the
// file is read the way a pi agent file is. `model` is left out on purpose: a Claude alias (sonnet, opus) is not a pi
// model, and a definition without one follows the executor's. No `tools:` line means every tool, as in Claude.
export function claudeAgentDefinition(content: string, parse: any) {
  const { frontmatter: fm, body } = parse.parseFrontmatter(content);
  if (!fm?.name || !fm?.description) return null;
  const declared = parse.parseFrontmatterList(fm.tools);
  const mapped = declared ? mapClaudeTools(declared) : { tools: undefined, dropped: [] as string[] };
  return {
    name: String(fm.name),
    definition: { description: String(fm.description), systemPrompt: body, defaultAsync: false, ...(mapped.tools ? { tools: mapped.tools } : {}) },
    dropped: mapped.dropped,
  };
}

// Registers every specialist of HW_PI_CLAUDE_AGENT_DIRS and returns what it did, for the receipt. A name a `.pi/agents`
// copy already provides (PI_SUBAGENT_EXTRA_AGENT_DIRS) is left to that copy.
async function registerClaudeSpecialists(pi: any, sub: string): Promise<{ registered: string[]; skipped: string[]; dropped: string[]; disposers: Array<() => void> }> {
  const out = { registered: [] as string[], skipped: [] as string[], dropped: [] as string[], disposers: [] as Array<() => void> };
  const dirs = (process.env.HW_PI_CLAUDE_AGENT_DIRS || "").split(":").filter(Boolean);
  if (!dirs.length || typeof pi.events?.emit !== "function") return out;
  const parse = await import(pathToFileURL(join(sub, "src", "agents", "frontmatter.js")).href);
  const own = new Set<string>();
  for (const d of (process.env.PI_SUBAGENT_EXTRA_AGENT_DIRS || "").split(":").filter(Boolean)) {
    try { for (const f of readdirSync(d)) if (f.endsWith(".md")) own.add(f.slice(0, -3)); } catch { /* absent */ }
  }
  for (const d of dirs) {
    let files: string[] = [];
    try { files = readdirSync(d).filter((f) => f.endsWith(".md")).sort(); } catch { continue; }
    for (const f of files) {
      const base = f.slice(0, -3);
      if (NOT_SPECIALIST.test(base)) continue;
      try {
        const def = claudeAgentDefinition(readFileSync(join(d, f), "utf8"), parse);
        if (!def) { out.skipped.push(`${base} (no name/description)`); continue; }
        if (own.has(base)) { out.skipped.push(`${def.name} (a .pi/agents copy provides it)`); continue; }
        const req: any = { version: 1, name: def.name, definition: def.definition };
        pi.events.emit("pi-subagents:runtime-agent-register:v1", req);
        if (!req.result) { out.skipped.push(`${def.name} (pi-subagents did not answer)`); continue; }
        if (!req.result.ok) { out.skipped.push(`${def.name} (${String(req.result.error?.message ?? req.result.error).slice(0, 80)})`); continue; }
        out.registered.push(def.name);
        out.disposers.push(() => req.result.registration?.dispose?.());
        for (const t of def.dropped) if (!out.dropped.includes(t)) out.dropped.push(t);
      } catch (e: any) { out.skipped.push(`${base} (${String(e?.message ?? e).slice(0, 80)})`); }
    }
  }
  return out;
}

const ROOT_SESSION = Symbol.for("hw.pi.root-session");

export default function (pi: any) {
  pi.on("tool_call", async (event: any, ctx: any) => {
    // A SUBAGENT'S CHILD IS GUARDED ONLY IF THIS FILE WAS REGISTERED AS A REQUIRED CHILD EXTENSION (session_start,
    // below). When that did not happen — the API is missing in this pi-subagents, the session had no id, the call
    // threw — the child would run with no guard at all, and a scan of the tool's arguments cannot see what a child
    // does. So the tool is refused rather than trusted: the guard fails closed.
    if (isHwExecutor() && /^subagents?$/.test(String(event?.toolName ?? "")) && !(globalThis as any)[Symbol.for("hw.pi.required-child")]) {  // # MUTATION-ANCHOR: 988-M05
      return { block: true, reason: "Blocked: the subagent tool is refused because this executor's write guard is NOT registered as a required child extension of pi-subagents (see the pi_child_guard row of the receipt), so a child would run unguarded." };
    }
    const reason = await judge(event?.toolName, event?.input, ctx?.cwd || process.cwd());
    return reason ? { block: true, reason } : undefined;
  });

  pi.on("tool_result", (event: any, ctx: any) => {
    if ((event?.toolName !== "grep" && event?.toolName !== "bash") || !Array.isArray(event?.content)) return;
    const cwd = ctx?.cwd || process.cwd();
    let dropped = 0;
    const content = event.content.map((c: any) => {
      if (c?.type !== "text" || typeof c.text !== "string") return c;
      const r = redactSecretLines(c.text, cwd, event?.input?.path);  // # MUTATION-ANCHOR: 989-M06
      dropped += r.dropped;
      return { ...c, text: r.text };
    });
    if (!dropped) return;
    return { content: [...content, { type: "text", text: `[hw] ${dropped} matching line(s) of a protected secret file were removed from this result (secret read guard).` }] };
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

  pi.on("session_start", async (event: any, ctx: any) => {
    if (!ensureRoot(ctx)) return;
    publishRuntime(ctx);
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
    // The lane's `.claude/agents`, registered where pi-subagents is loaded and the guard travels with the children.
    try {
      const sub = process.env.HW_PI_SUBAGENTS_DIR
        || join(process.env.PI_CODING_AGENT_DIR || join(homedir(), ".pi", "agent"), "npm", "node_modules", "pi-subagents");
      if (process.env.HW_PI_CLAUDE_AGENT_DIRS && G[Symbol.for("hw.pi.required-child")] && !G[SPECIALISTS_FLAG]) {
        const r = await registerClaudeSpecialists(pi, sub);
        G[SPECIALISTS_FLAG] = r.disposers;
        receiptRow("pi_specialists", `registered ${r.registered.join(" ") || "none"}${r.skipped.length ? `; skipped ${r.skipped.join(", ")}` : ""}${r.dropped.length ? `; tools with no pi counterpart dropped: ${r.dropped.join(" ")}` : ""}`, "hw-executor.ts session_start");
      }
    } catch (e: any) {
      receiptRow("pi_specialists", `NOT REGISTERED — ${String(e?.message ?? e).slice(0, 200)}`, "hw-executor.ts session_start");
    }
  });
  pi.on("session_shutdown", () => {
    if (!isRoot) return; // a child's shutdown is not the process's
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

  pi.on("agent_start", (_e: any, ctx: any) => { lastError = null; publishRuntime(ctx); });
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
    const reason = await runTurnEnd(hadInput, {
      session_id: sessionOf(ctx), model: modelOf(ctx) || null, thinking: thinkingOf(ctx) || null,
      ...(usage ? { ctx: usage } : {}),
    });
    if (!reason) return;
    return {
      entries: [{ type: "custom_message", customType: "hw-turn-end", content: reason, display: true }],
      continue: true,
    };
  });
}

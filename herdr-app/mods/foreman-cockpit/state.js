// The reader of cockpit/schema.json v1. Nothing here draws and nothing imports
// the app, so it runs under plain node against the contract's own table,
// cockpit/fixtures/index.json (state.test.mjs). A field it does not know is
// ignored; a field it reads that is missing or mistyped makes the file dead —
// except the two an older writer may omit (subagents, rules.no_progress_after_ms).

// The contract's two ages (x-state-classes): older than the first the file is
// stale, older than the second it is dead and its rows are not drawn.
const STALE_AFTER = 15000;
const MAX_AGE = 60000;
// The contract's ceiling: a file over 1 MiB is dead. Counted in characters here
// because the file arrives decoded, so it is the same number for ASCII state.
const MAX_BYTES = 1048576;

const ATTENTION = new Set(["challenge", "ask", "blocked", "report", "working", "idle", "gone"]);
const SUB_STATE = new Set(["running", "done", "stale"]);

const isMs = (x) => typeof x === "number" && Number.isFinite(x) && x >= 0;
const isStr = (x) => typeof x === "string";
const orNull = (f) => (x) => x === null || f(x);
const isObj = (x) => x !== null && typeof x === "object" && !Array.isArray(x);

// Why the folder cannot be read, in the person's words. The app lets a mod read only under its own home, after symlinks
// are resolved, and nothing in `mod.json` widens that (measured 2026-11-10: `capabilities.fs.read` there is ignored), so the
// host's "outside every granted read root" says where to put the folder, not what the app allows.
export const folderError = (e, home) =>
  /outside every granted/.test(String(e))
    ? `no state folder: the app reads files only under its own home (${home}), links resolved; put the work root's .cockpit there (${e})`
    : `no state folder: ${e}`;

// Text drawn by the panel goes through here: a control character or an escape
// sequence (a report summary or a brief's title can carry one) makes the engine
// refuse the whole tree.
export const scrub = (text) =>
  String(text)
    .replace(/\x1b\[[0-9;?]*[ -/]*[@-~]|\x1b[@-_]/g, " ")
    .replace(/[\x00-\x1f\x7f-\x9f]/g, " ")
    .replace(/ {2,}/g, " ")
    .trim();

export const cut = (text, max) => {
  const t = scrub(text);
  return t.length <= max ? t : `${t.slice(0, max - 1)}…`;
};

// The app's own terminal for an executor's pane id, or null. Executors live in the TUI
// `herdr server` (ids like `w1:p11`) which the app cannot reach (KNOWN-LIMITATIONS L29);
// only an executor started in the app's core (`hc`, id = HC_TERMINAL_ID) matches.
export const terminalFor = (pane, terminals) => terminals.find((x) => x.address === pane)?.address ?? null;

// The lane a brainer's rows belong to: the state carries it per row (`project`),
// so a brainer that dispatched across lanes is named by all of them.
export const laneOf = (rows) => [...new Set(rows.map((r) => r.project))].sort().join(", ");

// The name of a brainer's section: the lane the writer was opened for (`lane`, which `bin/brain`
// hands it), else the lanes of its rows (a file from a writer older than the field), else its pane id.
export const brainerLane = (state) =>
  (isStr(state.lane) && state.lane.trim() !== "" ? state.lane.trim() : "") || laneOf(state.rows) || state.invoker_pane;

// A duration the way a person reads it: `41m`, `3h12m`, `2d4h`; under a minute, seconds.
export function dur(ms) {
  const s = Math.max(0, Math.floor(ms / 1000));
  if (s < 60) return `${s}s`;
  const m = Math.floor(s / 60);
  if (m < 60) return `${m}m`;
  const h = Math.floor(m / 60);
  if (h < 24) return `${h}h${String(m % 60).padStart(2, "0")}m`;
  return `${Math.floor(h / 24)}d${h % 24}h`;
}

// Tokens the way the statusline writes them: 900, 237.5k, 1.2M.
function tokensText(n) {
  const f = (x, u) => `${x.toFixed(1).replace(/\.0$/, "")}${u}`;
  return n >= 1000000 ? f(n / 1000000, "M") : n >= 1000 ? f(n / 1000, "k") : `${n}`;
}

// A percent only when the window is known; the tokens alone when it is not; n/a when nothing is.
export function ctxMeta(c) {
  if (c.tokens === null) return "ctx n/a";
  return c.pct === null ? `ctx ${tokensText(c.tokens)}` : `ctx ${Math.round(c.pct)}%`;
}

function okSub(x) {
  return isObj(x) && isStr(x.type) && isStr(x.description) && orNull(isStr)(x.model) &&
    SUB_STATE.has(x.state) && isMs(x.started_ms) && isMs(x.last_ms);
}

// One row: the fields this reader draws or reasons about, each present and right.
function okRow(r) {
  return isObj(r) &&
    isStr(r.id) && isStr(r.project) && isStr(r.task) && orNull(isStr)(r.title) &&
    isStr(r.vendor) && orNull(isStr)(r.model) && orNull(isStr)(r.effort) &&
    ["idle", "working", "unknown"].includes(r.agent_status) &&
    ATTENTION.has(r.attention) && orNull(isMs)(r.attention_since) && isMs(r.dispatched_at) &&
    orNull(isMs)(r.turn_started_at) && orNull(isMs)(r.last_progress_at) &&
    orNull((c) => isMs(c.count) && orNull(isMs)(c.last_at))(r.commits) &&
    orNull((g) => isStr(g.what) && orNull(isMs)(g.since))(r.gate) &&
    orNull(isStr)(r.children_running) &&
    isObj(r.ctx) && orNull(isMs)(r.ctx.pct) && orNull(isMs)(r.ctx.tokens) &&
    isObj(r.rulings_pending) && isMs(r.rulings_pending.count) && orNull(isMs)(r.rulings_pending.oldest_at) &&
    orNull((p) => ["done", "blocked"].includes(p.status) && ["delivered", "undelivered"].includes(p.state) &&
      isMs(p.at) && isStr(p.summary))(r.report) &&
    orNull((a) => ["ask", "challenge"].includes(a.kind) && isStr(a.seq) &&
      ["delivered", "undelivered"].includes(a.state) && orNull(isStr)(a.pending_reply))(r.ask) &&
    (r.subagents === undefined || (Array.isArray(r.subagents) && r.subagents.every(okSub)));
}

// One state file's text → its document, or null when a reader treats it as dead.
// A missing file is the caller's null.
export function parseState(text) {
  if (typeof text !== "string" || text.length > MAX_BYTES) return null;
  let s;
  try {
    s = JSON.parse(text);
  } catch {
    return null;
  }
  if (!isObj(s) || s.schema !== 1) return null;
  if (!isMs(s.generated_at) || !isStr(s.invoker_pane)) return null;
  if (!(s.lane === undefined || orNull(isStr)(s.lane))) return null;
  if (!isObj(s.herdr) || typeof s.herdr.ok !== "boolean") return null;
  const t = s.totals;
  if (!isObj(t)) return null;
  for (const k of ["executors", "omitted", "working", "idle", "asks", "reports", "blocked", "challenges", "rulings_pending"]) {
    if (!isMs(t[k])) return null;
  }
  const r = s.rules;
  // A file from a writer older than the field is still fresh (fixtures/index.json,
  // fresh-older-writer.json): without the threshold nothing is marked stalled.
  if (!isObj(r) || !(r.no_progress_after_ms === undefined || isMs(r.no_progress_after_ms))) return null;
  if (!isObj(r.cut) || typeof r.cut.running !== "boolean" || !orNull(isStr)(r.cut.holder)) return null;
  if (!Array.isArray(s.rows) || !s.rows.every(okRow)) return null;
  return s;
}

// The one threshold of the "no progress" mark, or Infinity when the file predates it.
export const noProgressAfterMs = (state) =>
  isMs(state?.rules?.no_progress_after_ms) ? state.rules.no_progress_after_ms : Infinity;

// The contract's own classification, first match wins. `rows` is what the panel
// draws: none for a dead file, a down herdr, or a missing one.
export function classify(state, now) {
  if (state === null) return { cls: "dead", why: "no state file", rows: [] };
  const age = now - state.generated_at;
  if (age < 0) return { cls: "dead", age, why: "the writer's clock is ahead", rows: [] };
  if (age > MAX_AGE) return { cls: "dead", age, why: `last ${dur(age)} ago`, rows: [] };
  if (age > STALE_AFTER) return { cls: "stale", age, rows: state.rows };
  if (!state.herdr.ok) return { cls: "herdr-down", age, why: cut(state.herdr.error ?? "no reason given", 80), rows: [] };
  return { cls: "fresh", age, rows: state.rows };
}

// The status at a row's top right: the state and how long it has held it. The
// mod draws its colored dot beside it, so the text carries no glyph.
export function statusText(r, now) {
  if (r.attention === "working") {
    return `working ${dur(now - r.dispatched_at)}${r.turn_started_at === null ? "" : ` · turn ${dur(now - r.turn_started_at)}`}`;
  }
  const label = r.attention === "report" ? "reported" : r.attention;
  const since = r.report !== null ? r.report.at : r.attention_since;
  return `${label}${since === null ? "" : ` ${dur(now - since)}`}`;
}

// The brief's title under the task's name, or null when it only repeats the name
// (a brief without a heading of its own is titled `Brief: <task>`).
export function titleOf(r) {
  const title = r.title?.trim();
  if (!title) return null;
  const bare = title.replace(/^brief:\s*/i, "");
  return bare === r.task ? null : title;
}

// The model as an id (`claude-sonnet-5-5`, `openai/gpt-5.6-sol`), or null: hw writes null when it read
// none, and a sentence where it has no reader ("n/a — hw has no model reader"); neither is a model to draw.
export const modelOf = (r) => (typeof r.model === "string" && /^[\w.@:/[\]-]{1,64}$/.test(r.model) ? r.model : null);

// Vendor, model, effort and context on one line; an unknown field is left out, not drawn as a dash.
export function metaText(r) {
  return [r.vendor, modelOf(r), r.effort, ctxMeta(r.ctx)]
    .filter((x) => x !== null && x !== undefined && x !== "").join(" · ");
}

// How far the task has come: its commits over its base, the suite running for it, the children it waits on.
export function progressText(r, now) {
  const parts = [];
  if (r.commits !== null) {
    parts.push(`${r.commits.count} commit${r.commits.count === 1 ? "" : "s"}`);
    if (r.commits.last_at !== null) parts.push(`last ${dur(now - r.commits.last_at)} ago`);
  }
  if (r.gate !== null) parts.push(`gate ${cut(r.gate.what, 40)}${r.gate.since === null ? "" : ` ${dur(now - r.gate.since)}`}`);
  if (r.children_running !== null) parts.push(`children: ${r.children_running}`);
  return parts.length === 0 ? null : cut(parts.join(" · "), 200);
}

// What waits on the brainer, one reason each; empty when nothing does.
export function attentionParts(r, now, thresholdMs) {
  const parts = [];
  if (r.attention === "working" && r.last_progress_at !== null && now - r.last_progress_at > thresholdMs) {
    parts.push(`no progress ${dur(now - r.last_progress_at)}`);
  }
  if (r.rulings_pending.count > 0) {
    parts.push(`${r.rulings_pending.count} ruling queued${r.rulings_pending.oldest_at === null ? "" : ` ${dur(now - r.rulings_pending.oldest_at)}`}`);
  }
  if (r.ask !== null && r.ask !== undefined) {
    parts.push(`${r.ask.kind} ${r.ask.seq} ${r.ask.state === "delivered" ? "unanswered" : "undelivered"}`);
  }
  if (r.report !== null && r.report.status === "blocked") parts.push(`blocked: ${cut(r.report.summary, 200)}`);
  if (r.report !== null && r.report.state === "undelivered") parts.push("report undelivered");
  return parts;
}

// `subagents: 1 running · 8 done`, or null when the row has none.
export function subagentsSummary(subs) {
  if (subs.length === 0) return null;
  const n = (s) => subs.filter((x) => x.state === s).length;
  return `subagents: ${[[n("running"), "running"], [n("done"), "done"], [n("stale"), "stale"]]
    .filter(([c]) => c > 0)
    .map(([c, l]) => `${c} ${l}`)
    .join(" · ")}`;
}

export function subagentLine(x, now) {
  const when = x.state === "running" ? dur(now - x.started_ms) : x.state === "done" ? "done" : `stale ${dur(now - x.last_ms)}`;
  return cut(`↳ ${x.type} · ${modelOf(x) ?? "—"} · ${x.description} · ${when}`, 200);
}

// ---- the attention queue, the context meter and the state filter ----

const WAITS = new Set(["challenge", "ask", "blocked", "report"]);

// Since when a row has held its state; null when nothing says.
export const heldSince = (r) => (r.attention === "working" ? r.dispatched_at : r.report?.at ?? r.attention_since ?? r.dispatched_at);

// What needs the person, across brainers, oldest first (ties by task so the order never jumps).
// `rows`: [{ row, ... }] or plain rows; returns the same items sorted.
export function waitingQueue(items, rowOf = (x) => x) {
  return items.filter((x) => WAITS.has(rowOf(x).attention))
    .sort((a, b) => (heldSince(rowOf(a)) ?? Infinity) - (heldSince(rowOf(b)) ?? Infinity) || rowOf(a).task.localeCompare(rowOf(b).task));
}

// The meter's band for a context percent: null when unknown, then quiet < 60 <= mid < 80 <= warn < 90 <= alarm.
export const CTX_WARN = 80;
export const CTX_ALARM = 90;
export const CTX_QUIET = 60;
export function ctxLevel(pct) {
  if (typeof pct !== "number" || !Number.isFinite(pct)) return null;
  return pct >= CTX_ALARM ? "alarm" : pct >= CTX_WARN ? "warn" : pct >= CTX_QUIET ? "mid" : "quiet";
}

// The filter's choices and what each keeps. `done`: a report that finished (a blocked one needs you).
export const FILTERS = [{ id: "all", label: "all" }, { id: "needs", label: "needs you" }, { id: "working", label: "working" }, { id: "done", label: "done" }];
export const filterId = (x) => (FILTERS.some((f) => f.id === x) ? x : "all");
export function keeps(filter, r) {
  switch (filterId(filter)) {
    case "needs": return WAITS.has(r.attention);
    case "working": return r.attention === "working";
    case "done": return r.attention === "report" && r.report?.status !== "blocked";
    default: return true;
  }
}

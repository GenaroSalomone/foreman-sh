// The mod's reader against the contract's own table: cockpit/fixtures/index.json
// says, for each example file, the class and the number of rows a reader draws.
// Run by setup/tests/1272-herdr-cockpit-mod-reads-the-contract.sh.
import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { brainerLane, classify, folderError, parseState, dur, laneOf, metaText, modelOf, modelsByPane, scrub, terminalFor, titleOf } from "./state.js";

const root = join(dirname(fileURLToPath(import.meta.url)), "../../..");
const fixtures = join(root, "cockpit/fixtures");
const index = JSON.parse(readFileSync(join(fixtures, "index.json"), "utf8"));

let failed = 0;
let checks = 0;
const fail = (name, got, want) => {
  failed += 1;
  console.error(`not ok - ${name}: got ${got}, want ${want}`);
};

// A case's bytes: its file, a missing file (null), or the >1 MiB case the note names.
function bytes(c) {
  if (c.file === null) return null;
  const text = readFileSync(join(fixtures, c.file), "utf8");
  return c.note?.includes("> 1 MiB") ? text.padEnd(1048577, " ") : text;
}

for (const c of index.cases) {
  const name = c.file === null ? "missing file" : `${c.file}${c.note ? ` (${c.note})` : ""}`;
  checks += 1;
  const state = parseState(bytes(c) ?? "");
  const got = classify(state, c.now_ms);
  if (got.cls !== c.class) fail(`${name} class`, got.cls, c.class);
  if (got.rows.length !== c.rows_rendered) fail(`${name} rows`, got.rows.length, c.rows_rendered);
}

// A missing file is the caller's null, and it classifies dead with no rows.
checks += 1;
const missing = classify(null, index.cases[0].now_ms);
if (missing.cls !== "dead" || missing.rows.length !== 0) fail("null state", missing.cls, "dead");

checks += 5;
for (const [ms, want] of [[0, "0s"], [59000, "59s"], [60000, "1m"], [3600000, "1h00m"], [90000000, "1d1h"]]) {
  if (dur(ms) !== want) fail(`dur(${ms})`, dur(ms), want);
}

checks += 3;
// The brainer's name is its lane, and a state file carries it per row.
for (const [rows, want] of [
  [[{ project: "setup" }, { project: "setup" }], "setup"],
  [[{ project: "zeta" }, { project: "alpha" }], "alpha, zeta"],
  [[], ""],
]) {
  if (laneOf(rows) !== want) fail(`laneOf(${JSON.stringify(rows)})`, laneOf(rows), want);
}

checks += 6;
// A brainer is named by its lane, whether or not it has executors; a file from a writer older than the field falls back to
// its rows, then to its pane id.
for (const [state, want] of [
  [{ lane: "setup", rows: [], invoker_pane: "w3Z:p2" }, "setup"],
  [{ lane: "setup", rows: [{ project: "demo" }], invoker_pane: "w3Z:p2" }, "setup"],
  [{ lane: null, rows: [{ project: "demo" }], invoker_pane: "w3Z:p2" }, "demo"],
  [{ rows: [{ project: "zeta" }, { project: "alpha" }], invoker_pane: "w3Z:p2" }, "alpha, zeta"],
  [{ lane: "  ", rows: [], invoker_pane: "w3Z:p2" }, "w3Z:p2"],
  [{ rows: [], invoker_pane: "w41:pX0" }, "w41:pX0"],
]) {
  if (brainerLane(state) !== want) fail(`brainerLane(${JSON.stringify(state)})`, brainerLane(state), want);
}

checks += 4;
// `lane` is optional (older writers), a string or null; any other type is a dead file.
{
  const base = JSON.parse(readFileSync(join(fixtures, "fresh-empty.json"), "utf8"));
  for (const [lane, ok] of [[undefined, true], [null, true], ["setup", true], [7, false]]) {
    if (lane === undefined || lane === 7 || lane === null) {
      const doc = { ...base };
      if (lane === undefined) delete doc.lane; else doc.lane = lane;
      if ((parseState(JSON.stringify(doc)) !== null) !== ok) fail(`parseState lane=${lane}`, !ok, ok);
    }
  }
  if (parseState(JSON.stringify(base))?.lane !== "setup") fail("parseState keeps lane", "other", "setup");
}

checks += 5;
// A title that only repeats the task's name is not drawn; a real one is, whole.
for (const [title, want] of [
  ["Brief: herdr-app-cockpit", null],
  ["herdr-app-cockpit", null],
  [null, null],
  ["  ", null],
  ["Daily lead assignment", "Daily lead assignment"],
]) {
  const got = titleOf({ task: "herdr-app-cockpit", title });
  if (got !== want) fail(`titleOf(${JSON.stringify(title)})`, got, want);
}

checks += 1;
// An unknown effort is left out, not drawn as a dash.
const meta = metaText({ vendor: "claude", model: "claude-sonnet-5-5", effort: null, ctx: { tokens: 1000, pct: 34 }, agent_status: "working" });
if (meta !== "claude · claude-sonnet-5-5 · ctx 34%") fail("metaText", meta, "claude · claude-sonnet-5-5 · ctx 34%");

checks += 1;
// An unknown agent status is not a meta field: the card read "ctx 16% · unknown" (the CLI card drops it too).
const idle = { vendor: "claude", model: "claude-sonnet-5-5", effort: null, ctx: { tokens: 1000, pct: 16 }, agent_status: "unknown" };
if (metaText(idle) !== "claude · claude-sonnet-5-5 · ctx 16%") fail("metaText(agent_status unknown)", metaText(idle), "claude · claude-sonnet-5-5 · ctx 16%");

checks += 5;
// No model to draw: hw's null (none read) and its "n/a — …" sentence (no reader) both leave the field out, never the words.
const lost = { vendor: "pi", model: "n/a — hw has no model reader", effort: null, ctx: { tokens: 1000, pct: 34 }, agent_status: "working" };
if (modelOf(lost) !== null) fail("modelOf(n/a sentence)", modelOf(lost), null);
if (metaText(lost) !== "pi · ctx 34%") fail("metaText(n/a sentence)", metaText(lost), "pi · ctx 34%");
if (modelOf({ model: null }) !== null) fail("modelOf(null)", modelOf({ model: null }), null);
for (const id of ["claude-sonnet-5-5", "openai/gpt-5.6-sol"]) if (modelOf({ model: id }) !== id) fail(`modelOf(${id})`, modelOf({ model: id }), id);

checks += 1;
// A line shows its text scrubbed, scrubbed like the line itself (an escape would make the engine refuse the tree).
const tip = scrub("a\x1b[31mb\x00c" + "x".repeat(200));
if (tip !== "a b c" + "x".repeat(200)) fail("scrub for the tooltip", tip, "a b c + 200 x");

checks += 3;
// A click reaches only a terminal the app's core holds; a TUI pane id (w1:p11) is unreachable (L29).
const held = [{ address: "t1b" }, { address: "t7" }];
// rc1-app-live-qa F4: the agents panel gets pane → model from the same rows and the same field (modelOf) as the cockpit row;
// a row with no model (hw's null, its "n/a —" sentence) or no pane is left out, never drawn as a word.
const mp = modelsByPane([{ pane: "t5", model: "claude-sonnet-5-5" }, { pane: "t6", model: null }, { pane: "t7", model: "n/a — hw has no model reader" }, { pane: "", model: "claude-opus-5-5" }, { pane: "t8", model: "openai/gpt-5.6-sol" }]);
if (JSON.stringify(mp) !== JSON.stringify({ t5: "claude-sonnet-5-5", t8: "openai/gpt-5.6-sol" })) fail("modelsByPane", JSON.stringify(mp), '{"t5":"claude-sonnet-5-5","t8":"openai/gpt-5.6-sol"}');

checks += 1;
for (const [pane, want] of [["t7", "t7"], ["w1:p11", null], ["", null]]) {
  if (terminalFor(pane, held) !== want) fail(`terminalFor(${pane})`, terminalFor(pane, held), want);
}

// QA5 G3: the host's refusal names the app's home, so the person knows where the folder has to be.
{
  checks += 1;
  const got = folderError(new TypeError("'/tmp/x/.cockpit' is outside every granted read root; add its directory to capabilities.fs.read"), "/tmp/h");
  if (!got.includes("only under its own home (/tmp/h)")) fail("folderError names the app's home", got, "a message naming /tmp/h");
  checks += 1;
  if (folderError(new Error("boom"), "/tmp/h") !== "no state folder: Error: boom") fail("folderError leaves other errors alone", folderError(new Error("boom"), "/tmp/h"), "no state folder: Error: boom");
}

if (failed > 0) process.exit(1);
console.log(`ok - ${checks} assertions`);

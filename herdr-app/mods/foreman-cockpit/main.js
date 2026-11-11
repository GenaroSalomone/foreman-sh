import { View, div } from "gpui-kit";
import { h_flex, v_flex, Scrollbar } from "gpui-base";
import { focused, home, layout, props, readTextFile, run, setShared, shared, terminals, theme } from "herdr";
import * as fs from "fs/promises";
import {
  attentionParts, brainerLane, classify, cut, dur, metaText, noProgressAfterMs, parseState, progressText,
  ctxLevel, filterId, FILTERS, folderError, keeps, modelOf, scrub, statusText, subagentLine, subagentsSummary, terminalFor, waitingQueue,
} from "./state.js";
import { activeChild, LOOK_LAYOUTS, shownLook, SWITCH_ROW2, switchOf } from "./switcher.js";
import { ageText, lineOf, parseUsage, sortAccounts } from "./usage.js";
import { alarm, button, kindOf, colorOf, lookOf, mark, SPACE, TEXT } from "./tokens.js";

// The foreman cockpit as the herdr app draws it. The data is the file
// bin/cockpit-state writes for each brainer pane (contract: cockpit/schema.json;
// its reader is state.js); this mod only reads it, and draws hw's order and
// counts as they come, like the Claude Code cockpit does.
//
// The app has no brainer pane of its own, so it draws every brainer whose file is
// live in the folder its block names (`props.dir`: the work root's `.cockpit`) —
// one writer per brainer pane, and the pane id names its section: what waits on a
// brainer belongs to that brainer.
//
// One mod, two blocks: `props.switch` is the right column's switch, anything else the cockpit body.
// `props.look` (c, d, g) picks the grammar of its mockup. The window around them is the app's own
// mods (spaces, agents, tabs, pane, workspace-chrome, workspace-status), styled by the layout. The
// body (`props.dir`) publishes hw's counts as shared("foreman.cockpit") for the switch's live count.

// The four attention states that need the brainer: their status text in its color, bold.
const LOUD = new Set(["challenge", "ask", "blocked", "report"]);
const QUEUE_MAX = 8;
const URGENT = ["blocked", "waits", "working", "idle"];
// No hover: the app's tooltip is a plain label as wide as its text, so a report on hover drew a bar
// across the window. A row shows what it has instead: its report and what waits wrap under it.
// How long a row has held its state, the way the mockups' age column reads it: 0m, 7m, 2h, 3d.
const since = (r) => (r.attention === "working" ? r.dispatched_at : r.report?.at ?? r.attention_since ?? r.dispatched_at);
const age = (r, now) => dur(now - since(r)).replace(/^(\d+[hd])\d+[mh]$/, "$1");
const model = (r) => modelOf(r)?.split("/").pop().replace(/^claude-/, "") ?? "—";
const plural = (n, w) => `${n} ${w}${n === 1 ? "" : "s"}`;
// The switch's live count: "3 brainers · 9 executors".
const liveText = (brainers, executors) => `${plural(brainers, "brainer")} · ${plural(executors, "executor")}`;
const commits = (r) => (r.commits === null ? null : r.commits.count);
// The report's own words, or what waits: the one line D and G show under a row.
const detailOf = (r, attention) => (r.report !== null && r.report.status !== "blocked" && r.report.summary.trim() !== "" ? r.report.summary : attention);

export default class Cockpit extends View {
  init(_props, cx) {
    this.states = [];
    this.usage = [];
    this.error = null;
    this.open = new Set();
    this.folded = new Set();
    this.cockpitDir = null;
    this.reading = false;
    this.held = [];
    this.picked = null;
    this.published = "";
    this.filter = filterId(shared("foreman.cockpit.filter")); // remembered by the app across restarts and windows
    const p = props();
    if (switchOf(p)) return;
    this.refresh(cx);
    // One read per beat, not one per write: eight writers beat every 5 s, and every
    // notify redraws the whole window. It is also what makes a dead writer's file
    // go stale (and its rows dim) without a write that never comes.
    cx.timer.every(5000, (cx) => this.refresh(cx));
  }

  refresh(cx) {
    if (this.reading) return;
    cx.spawn(async (cx) => {
      try {
        if (this.cockpitDir === null) this.cockpitDir = this.folder();
        await this.read();
      } catch (e) {
        this.error = folderError(e, home());
      }
      this.publish();
      cx.notify();
    });
  }

  // The folder the writers put their files in. The app cannot ask the writer, so
  // the block names it in props.dir.
  folder() {
    const dir = props()?.dir;
    if (typeof dir !== "string" || !dir) throw new Error("the layout must pass props.dir (the cockpit state folder)");
    return dir.startsWith("~/") ? `${home()}/${dir.slice(2)}` : dir;
  }

  // Every reader-readable state in the folder, one per brainer pane. A window
  // with no executors anywhere says so instead of drawing an empty section each.
  async read() {
    const dir = this.cockpitDir;
    if (dir === null) return;
    this.reading = true;
    try {
      const names = await fs.readdir(dir).catch(() => []);
      const states = [];
      for (const name of names) {
        if (!name.endsWith(".json")) continue;
        const text = await readTextFile(`${dir}/${name}`).catch(() => null);
        const state = text === null ? null : parseState(text);
        if (state !== null) states.push(state);
      }
      this.states = states;
      this.error = null;
      // The accounts' usage, written by bin/statusline-usage-tee; a missing folder is no line, never an error.
      const udir = `${home()}/.cache/hw/claude-usage`;
      const us = [];
      for (const name of await fs.readdir(udir).catch(() => [])) {
        if (!name.endsWith(".json")) continue;
        const u = parseUsage(await readTextFile(`${udir}/${name}`).catch(() => ""));
        if (u !== null) us.push(u);
      }
      this.usage = sortAccounts(us);
    } finally {
      this.reading = false;
    }
  }

  // hw's counts for the switch's live count. Set only when it changed: every setShared redraws every mod.
  publish() {
    const groups = this.groups(Date.now());
    const executors = groups.reduce((n, g) => n + (g.state.totals.executors ?? 0), 0);
    const text = liveText(groups.length, executors);
    if (text === this.published) return;
    this.published = text;
    setShared("foreman.cockpit", text);
  }

  // One line of a C row, clipped at the panel's width; `wrap`: the whole text, wrapped (a report, what waits).
  line(text, color, wrap = false) {
    return div().w_full().min_w(0).when(!wrap, (e) => e.truncate()).text_size(TEXT.label).text_color(color).child(text);
  }

  // A brainer's counts as one line, in hw's order: "6 working · 1 reported".
  counts(state) {
    const tt = state.totals;
    return [[tt.working, "working"], [tt.idle, "idle"], [tt.challenges, "challenge"], [tt.blocked, "blocked"],
      [tt.asks, "ask"], [tt.reports, "reported"]].filter(([n]) => n > 0).map(([n, l]) => `${n} ${l}`)
      .concat(tt.rulings_pending > 0 ? [`${tt.rulings_pending} ruling queued`] : [])
      .concat(tt.omitted > 0 ? [`${tt.omitted} not listed`] : [])
      .concat(state.rules.cut.running ? [`CUT running${state.rules.cut.holder === null ? "" : ` (${cut(state.rules.cut.holder, 30)})`}`] : [])
      .join(" · ");
  }

  // Click: focus the executor's terminal when the app's core holds it; else nothing (L29).
  clickable(el, r) {
    const target = terminalFor(r.pane, this.held);
    return el.when(target !== null, (e) => e.cursor_pointer().on_click(() => run("pane.focus", { terminal: target })));
  }

  // The context meter: a 36 px bar filling with use, quiet below 60 %, the warning color from 80 %, the alarm from 90 %;
  // the percent is written only once it warns. No window known (pct null): no meter, the meta line still says the tokens.
  meter(r, t, dim) {
    const lv = ctxLevel(r.ctx.pct);
    if (lv === null) return null;
    const color = dim || lv === "quiet" ? t.dim : lv === "mid" ? t.muted : lv === "warn" ? t.ansi3 ?? t.working : t.blocked;
    const w = Math.max(2, Math.round((36 * Math.min(100, r.ctx.pct)) / 100));
    return h_flex().id(`ctx-${r.id}`).flex_none().gap(5).items_center()
      .child(div().w(36).h(3).rounded(2).bg(t.hover).child(div().w(w).h(3).rounded(2).bg(color)))
      .when(lv === "warn" || lv === "alarm", (e) => e.child(div().text_size(TEXT.label).text_color(color).font_semibold().child(`${Math.round(r.ctx.pct)}%`)));
  }

  // One row. C: the task, its status and its lines (meta, progress, report, subagents, what waits),
  // a left bar in the signal color when it waits. D: Linear's one-line row with a label and an age,
  // the report under it. G: Zed's 24 px tree row under an indent guide, the report in a hover card.
  card(r, now, t, dim, thresholdMs, look) {
    const kind = dim ? "idle" : kindOf(r.attention);
    const loud = !dim && LOUD.has(r.attention);
    const color = colorOf(kind, t);
    const attention = attentionParts(r, now, thresholdMs).join(" · ");
    const detail = detailOf(r, attention);
    const name = (size, weight) => div().flex_1().min_w(0).truncate().text_size(size).font_weight(weight)
      .text_color(dim || kind === "idle" ? t.muted : t.foreground).child(r.task);
    if (look.id === "d") {
      const label = commits(r) ? `${commits(r)} commits` : model(r);
      return this.clickable(v_flex().id(`row-${r.id}`).w_full().min_w(0).flex_none().border_b(1).border_color(t.border), r)
        .child(h_flex().h(36).px(14).gap(10).text_size(13)
          .child(mark(look, kind, t, 12))
          .child(name(13, 400))
          .when(Boolean(label), (e) => e.child(div().flex_none().h(18).px(7).rounded(10).border_1().border_color(t.border)
            .text_size(TEXT.label).text_color(t.muted).flex().items_center().child(label)))
          .children([this.meter(r, t, dim)].filter((x) => x !== null))
          .child(div().flex_none().w(34).text_right().text_size(12).text_color(t.dim).child(age(r, now))))
        .when(detail !== "" && (loud || attention !== ""), (e) => e.child(div().pl(38).pr(14).pb(10).text_size(12).line_height(1.5)
          .text_color(r.attention === "blocked" || r.attention === "challenge" ? t.blocked : t.muted).child(cut(detail, 200))));
    }
    if (look.id === "g") {
      const meta = [model(r), commits(r) ? `${commits(r)}c` : null].filter(Boolean).join(" · ");
      const at = terminalFor(r.pane, this.held);
      const sel = at !== null && at === focused()?.terminal;
      return v_flex().id(`row-${r.id}`).w_full().min_w(0).flex_none()
        .child(this.clickable(h_flex().h(24).relative().pl(22).pr(12).gap(8).text_size(13).border_1()
          .border_color(sel ? t.input : t[look.dock]).when(sel, (e) => e.bg(t.selected)).when(!sel, (e) => e.hover((s) => s.bg(t.hover))), r)
          .child(div().absolute().top(0).bottom(0).left(10).w(1).bg(t.hover))
          .child(mark(look, kind, t, 11))
          .child(name(13, 400))
          .child(div().flex_none().text_size(12).text_color(t.dim).child(meta))
          .children([this.meter(r, t, dim)].filter((x) => x !== null))
          .child(div().flex_none().w(34).text_right().text_size(12).text_color(t.dim).child(age(r, now))))
        .when(detail !== "" && (loud || attention !== ""), (e) => e.child(div().ml(22).mr(12).mt(2).mb(6).px(8).py(6).rounded(4)
          .bg(t.background).border_1().border_color(t.border).font_family(look.mono).text_size(12).line_height(1.5)
          .text_color(r.attention === "blocked" || r.attention === "challenge" ? t.blocked : t.muted).child(cut(detail, 200))));
    }
    // C.
    const lines = [];
    const meta = [metaText(r), progressText(r, now)].filter(Boolean).join(" · ");
    lines.push(this.line(cut(meta, 200), t.muted));
    if (r.report !== null && r.report.status !== "blocked" && r.report.summary.trim() !== "") {
      lines.push(this.line(cut(r.report.summary, 200), loud ? t.ansi7 : t.muted, true));
    }
    const subs = r.subagents ?? [];
    const folded = subagentsSummary(subs);
    if (folded !== null) {
      const unfolded = this.open.has(r.id);
      lines.push(div().id(`subs-${r.id}`).w_full().min_w(0).truncate().text_size(TEXT.label).text_color(t.muted).cursor_pointer()
        .hover((s) => s.text_color(t.foreground))
        .on_click((_e, cx) => {
          cx.stop_propagation(); // the row's own click focuses the executor
          if (unfolded) this.open.delete(r.id);
          else this.open.add(r.id);
          cx.notify();
        })
        .child(`${unfolded ? "▾" : "▸"} ${folded}`));
      if (unfolded) for (const x of subs) lines.push(this.line(subagentLine(x, now), x.state === "running" && !dim ? t.working : t.muted));
    }
    if (attention !== "") {
      lines.push(this.line(`⚠ ${cut(attention, 300)}`, dim ? t.dim : LOUD.has(r.attention) ? color : t.ansi3 ?? t.working, true)
        .when(loud, (e) => e.font_semibold()));
    }
    const body = v_flex().w_full().min_w(0).gap(2).py(8).px(14)
      .child(h_flex().w_full().min_w(0).gap(10)
        .child(mark(look, kind, t, 12))
        .child(name(12.5, kind === "idle" ? 400 : 700))
        .children([this.meter(r, t, dim)].filter((x) => x !== null))
        .child(div().flex_none().text_size(TEXT.label).text_color(loud ? color : t.muted)
          .when(loud, (e) => e.font_bold())
          .child(statusText(r, now))))
      .child(v_flex().w_full().min_w(0).pl(24).gap(2).children(lines));
    // The signal bar is its own element: each element takes one border color, and the row's is the hairline.
    return this.clickable(div().id(`row-${r.id}`).w_full().min_w(0).flex_none().border_b(1).border_color(t.border), r)
      .child(loud ? div().w_full().border_l(2).border_color(color).child(body.pl(12)) : body);
  }

  // The brainers whose file is live now, by lane name: a group that moves as a
  // card's status changes is noise, so the order never depends on the rows.
  groups(now) {
    return this.states
      .map((state) => ({ state, ...classify(state, now) }))
      .filter((g) => g.cls === "fresh" || g.cls === "stale")
      .sort((a, b) => brainerLane(a.state).localeCompare(brainerLane(b.state)) || a.state.invoker_pane.localeCompare(b.state.invoker_pane));
  }

  // One brainer with executors: its lane and hw's counts, then its rows; the lane says which brainer,
  // so the pane id is not repeated. Another brainer is another section, because an ask waits on that
  // brainer and nobody else.
  section(g, now, t, look) {
    const dim = g.cls === "stale";
    const kept = g.rows.filter((r) => keeps(this.filter, r));
    const thresholdMs = noProgressAfterMs(g.state);
    const lane = cut(brainerLane(g.state), 40);
    const counts = this.counts(g.state);
    const kinds = new Set(kept.map((r) => kindOf(r.attention)));
    const urgent = URGENT.find((k) => kinds.has(k)) ?? "idle";
    const key = g.state.invoker_pane;
    const shut = this.folded.has(key);
    const rows = shut && look.id === "g" ? [] : kept.map((r) => this.card(r, now, t, dim, thresholdMs, look));
    const head = look.id === "d"
      ? h_flex().h(34).mt(6).px(14).gap(8).bg(t.surface).border_t(1).border_b(1).border_color(t.border).text_size(12.5)
        .child(mark(look, dim ? "idle" : urgent, t, 12))
        .child(div().flex_none().font_weight(500).text_color(dim ? t.dim : t.foreground).child(lane))
        .child(div().flex_none().text_color(t.dim).child(String(kept.length)))
        .child(div().flex_1().min_w(0).truncate().text_right().text_size(12).text_color(t.dim).child(counts))
      : look.id === "g"
        ? h_flex().id(`sec-${key}`).h(26).mt(4).pl(4).pr(6).gap(6).cursor_pointer().text_size(12.5).text_color(t.muted)
          .on_click((_e, cx) => { if (this.folded.has(key)) this.folded.delete(key); else this.folded.add(key); cx.notify(); })
          .child(div().flex_none().w(12).text_center().text_size(9).text_color(t.dim).child(shut ? "▸" : "▾"))
          .child(div().flex_none().font_weight(500).text_color(dim ? t.dim : t.foreground).child(lane))
          .child(div().flex_none().child(String(kept.length)))
          .child(div().flex_1().min_w(0).truncate().text_right().pr(6).text_size(12).text_color(t.dim).child(counts))
        : h_flex().w_full().min_w(0).gap(10).pt(16).px(14).pb(6).border_b(1).border_color(t.border).text_size(TEXT.label)
          .child(div().flex_none().text_size(13).font_bold().text_color(dim ? t.dim : t.foreground).child(lane))
          .child(div().flex_1().min_w(0).truncate().text_right().text_color(dim ? t.dim : t.foreground).child(counts));
    return v_flex().w_full().min_w(0).flex_none()
      .child(head)
      .children(rows);
  }

  // One line per Claude account, above everything: its name and its 5-hour and 7-day windows, amber from 80 %,
  // red from 95 %, dimmed (and aged) when no session has refreshed the file for 15 min.
  usageLines(now, t, look) {
    if (this.usage.length === 0) return null;
    const color = (lv) => (lv === "crit" ? t.blocked : lv === "warn" ? t.ansi3 ?? t.working : t.muted);
    const rows = this.usage.map((u) => {
      const l = lineOf(u, Math.floor(now / 1000));
      return h_flex().id(`usage-${l.account}`).w_full().min_w(0).h(24).gap(10).items_center().text_size(TEXT.label)
        .child(div().flex_none().size(6).rounded(3).bg(l.stale ? t.dim : color(l.level)))
        .child(div().flex_none().w(60).truncate().text_color(l.stale ? t.dim : t.foreground).child(scrub(l.account)))
        .children(l.parts.map((p) => div().flex_none().text_color(l.stale ? t.dim : color(p.level))
          .when(p.level !== "ok" && !l.stale, (e) => e.font_semibold()).child(`${p.label} ${p.text}`)))
        .when(l.stale, (e) => e.child(div().flex_1().min_w(0).truncate().text_color(t.dim).child(`${ageText(l.ageS)} ago`)));
    });
    return v_flex().id("usage").w_full().flex_none().px(14).py(6).border_b(1).border_color(t.border).children(rows);
  }


  // The brainers with no executors: one line of their lanes, not a section each (eight idle brainers
  // filled the panel with eight headers and eight "No executors."). Same lane names, no new copy.
  idle(groups, t, look) {
    const names = groups.map((g) => cut(brainerLane(g.state), 24)).join(" · ");
    const dimmed = groups.every((g) => g.cls === "stale");
    const row = h_flex().w_full().min_w(0).flex_none();
    if (look.id === "d") {
      return row.h(34).mt(6).px(14).gap(8).bg(t.surface).border_t(1).border_b(1).border_color(t.border).text_size(12.5)
        .child(mark(look, "idle", t, 12))
        .child(div().flex_1().min_w(0).truncate().text_color(t.dim).child(names));
    }
    if (look.id === "g") {
      return row.h(26).mt(4).pl(4).pr(6).gap(6).text_size(12.5)
        .child(div().flex_none().w(12))
        .child(div().flex_1().min_w(0).truncate().text_color(t.dim).child(names));
    }
    return row.gap(10).pt(16).px(14).pb(6).border_b(1).border_color(t.border).text_size(TEXT.label)
      .child(div().flex_1().min_w(0).truncate().text_color(dimmed ? t.dim : t.muted).child(names));
  }

  // The queue of what needs the person, above everything, oldest first (`waitingQueue`): a title with the counts, then one
  // entry per waiting executor; a click on an entry focuses that executor's pane (`pane.focus`, the app's own API). C: the
  // full-width signal block, D: the inbox card, G: the outlined callout.
  callout(rows, now, t, look) {
    if (rows.length === 0) return null;
    const by = (a) => rows.filter((r) => r.attention === a).length;
    const title = [[by("report"), "report", "to read"], [by("ask"), "ask", "to answer"], [by("challenge"), "challenge", "to rule"],
      [by("blocked"), "blocked", ""]].filter(([n]) => n > 0)
      .map(([n, l, what]) => (l === "blocked" ? `${n} blocked` : `${n} ${l}${n === 1 ? "" : "s"} ${what}`)).join(" · ");
    const shown = rows.slice(0, QUEUE_MAX);
    const c = look.id === "c";
    const entry = (r) => {
      const k = kindOf(r.attention);
      return this.clickable(h_flex().id(`q-${r.id}`).w_full().min_w(0).gap(8).items_center().h(c ? 20 : 22).text_size(c ? TEXT.label : 12)
        .when(!c, (e) => e.px(4).rounded(4).text_color(t.muted).hover((s) => s.bg(t.hover).text_color(t.foreground)))
        .when(c, (e) => e.hover((s) => s.font_bold())), r)
        .child(c ? div().flex_none().child("■") : mark(look, k, t, 11))
        .child(div().flex_1().min_w(0).truncate().child(`${r.task}`))
        .child(div().flex_none().when(!c, (e) => e.text_color(t.dim)).child(`${r.project} · ${r.attention === "report" ? "reported" : r.attention}`))
        .child(div().flex_none().w(34).text_right().when(!c, (e) => e.text_color(t.dim)).child(age(r, now)));
    };
    const more = rows.length > shown.length ? [div().text_size(TEXT.label).when(!c, (e) => e.text_color(t.dim)).child(`+${rows.length - shown.length}`)] : [];
    if (c) {
      return v_flex().w_full().flex_none().px(14).py(12).gap(3).bg(t.done).text_color(t.background)
        .child(div().text_size(14).font_bold().child(title))
        .children([...shown.map(entry), ...more]);
    }
    const kind = rows.some((r) => r.attention === "blocked" || r.attention === "challenge") ? "blocked" : "waits";
    const box = look.id === "d"
      ? v_flex().mt(10).mx(12).mb(4).px(12).py(10).rounded(8).border_1().border_color(t.border).bg(t.raised)
      : v_flex().m(8).px(10).py(8).rounded(6).border_1().border_color(t.input).bg(t.background);
    return box.flex_none().gap(2)
      .child(h_flex().gap(look.id === "d" ? 10 : 8).items_center().child(mark(look, kind, t, 12))
        .child(div().flex_1().min_w(0).truncate().text_size(13).font_weight(look.id === "d" ? 600 : 500).text_color(t.foreground).child(title)))
      .children([...shown.map(entry), ...more]);
  }

  // The state filter: all / needs you / working / done; the choice is the app's shared state, so it survives a restart.
  filters(t, look) {
    return h_flex().id("cockpit-filter").w_full().flex_none().min_w(0).px(look.id === "c" ? 14 : 12).py(6).gap(look.id === "g" ? 2 : 4)
      .border_b(1).border_color(t.border)
      .children(FILTERS.map((f) => button(`filter-${f.id}`, f.label, t, this.filter === f.id, look)
        .on_click((_e, cx) => { this.filter = f.id; setShared("foreman.cockpit.filter", f.id); cx.notify(); })));
  }

  // The right column's switch: one label per stack child; the app's own `layout.select`. C: words in
  // a 46 px bar, the active one bold; D: Linear's pill tabs; G: Zed's tab bar. hw's live count beside them.
  tabs(sw, look) {
    const t = theme();
    const shown = this.picked ?? activeChild(layout(), sw.stack) ?? sw.items[0].child;
    const live = shared("foreman.cockpit") ?? "no state";
    const pick = (i) => (_e, cx) => { this.picked = i.child; run("layout.select", { stack: sw.stack, child: i.child }); cx.notify(); };
    const label = (i) => {
      const on = i.child === shown;
      const el = div().id(`tab-${i.child}`).flex_shrink_1().min_w(0).flex().items_center().cursor_pointer().on_click(pick(i)).child(i.label);
      if (look.id === "c") return el.text_size(13).text_color(on ? t.foreground : t.muted).when(on, (e) => e.font_bold())
        .when(!on, (e) => e.hover((s) => s.text_color(t.foreground)));
      if (look.id === "d") return el.h(26).px(8).rounded(6).border_1().border_color(on ? t.border : t[look.dock]).text_size(12.5)
        .text_color(on ? t.foreground : t.muted).when(on, (e) => e.bg(t.raised)).when(!on, (e) => e.hover((s) => s.text_color(t.foreground)));
      return el.h_full().px(10).text_size(13).border_r(1).border_b(on ? 0 : 1).border_color(t.border)
        .text_color(on ? t.foreground : t.muted).when(!on, (e) => e.hover((s) => s.bg(t.hover)));
    };
    // Two rows, so nothing is ever squeezed out: the labels on the first (they share the width and may shrink, never clip a
    // neighbour), the live count and the look switcher on the second. One row held five labels, the count and C D G only
    // while the column was wide: in D at 392 px the switcher was cut to "C" and the count vanished.
    const row1 = look.id === "g"
      ? h_flex().w_full().flex_1().min_h(0).items_stretch().bg(t[look.dock]).children(sw.items.map(label))
      : h_flex().w_full().flex_1().min_h(0).items_center().px(look.id === "c" ? 14 : 8).gap(look.id === "c" ? 10 : 2).bg(t[look.dock]).children(sw.items.map(label));
    const row2 = h_flex().w_full().flex_none().h(SWITCH_ROW2).min_w(0).items_center().px(look.id === "c" ? 14 : 12).gap(8).bg(t[look.dock]).border_b(1).border_color(t.border)
      .child(div().flex_1().min_w(0).truncate().text_size(look.id === "d" ? 12 : TEXT.label).text_color(t.muted).child(live))
      .child(this.looks(t, look));
    return v_flex().size_full().min_w(0).overflow_hidden().child(row1).child(row2);
  }

  // The look switcher, C · D · G: each label shows its layout (`layout.switch {name}`), the shown one marked.
  // cmd-shift-l still cycles every layout (the app's own key).
  looks(t, look) {
    const shown = shownLook(shared("layout"));
    return h_flex().flex_none().items_center().gap(look.id === "c" ? 6 : 2).text_size(12)
      .children(LOOK_LAYOUTS.map((l) => {
        const on = l.name === shown;
        return div().id(`look-${l.name}`).flex_none().h(20).px(5).flex().items_center().cursor_pointer()
          .when(look.id !== "c", (e) => e.rounded(4))
          .when(on, (e) => (look.id === "c" ? e.bg(t.foreground).text_color(t.background) : e.bg(t.selected).text_color(t.foreground)).font_bold())
          .when(!on, (e) => e.text_color(t.muted).hover((s) => s.text_color(t.foreground)))
          .on_click(() => run("layout.switch", { name: l.name }))
          .child(l.label);
      }));
  }

  render() {
    const p = props();
    const look = lookOf(p);
    const sw = switchOf(p);
    if (sw) return this.tabs(sw, look);
    const t = theme();
    const now = Date.now();
    this.held = terminals(); // once per frame, not once per row
    const groups = this.groups(now);
    const drawn = groups.filter((g) => g.rows.length > 0);
    const empty = groups.filter((g) => g.rows.length === 0);
    const stale = groups.some((g) => g.cls === "stale");
    const loud = waitingQueue(groups.filter((g) => g.cls === "fresh").flatMap((g) => g.rows));
    // Nothing live: the freshest file's own class says why, and one line stands for every brainer.
    const freshest = this.states.reduce((best, s) => (best === null || s.generated_at > best.generated_at ? s : best), null);
    const silent = groups.length === 0 ? classify(freshest, now) : null;
    const clock = new Date(now).toTimeString().slice(0, 5);
    return v_flex().size_full().min_h(0).overflow_hidden().bg(t[look.dock]).text_color(t.foreground).text_size(TEXT.body)
      .when(this.error !== null, (e) => e.child(alarm(this.error, null, t).text_size(TEXT.label)))
      .when(silent !== null, (e) =>
        e.child(alarm(silent.cls === "herdr-down"
          ? `HERDR UNREACHABLE — rows unknown (${silent.why})`
          : `NO STATE — no writer's file is live (${silent.why})`, null, t).text_size(TEXT.label)))
      .children([this.usageLines(now, t, look), this.callout(loud, now, t, look)].filter((x) => x !== null))
      .when(drawn.length > 0, (e) => e.child(this.filters(t, look)))
      .when(stale, (e) => e.child(div().flex_none().px(14).py(4).text_size(TEXT.label).text_color(t.dim).child("stale — a writer has not beaten for 15 s; its rows are dimmed")))
      .when(silent === null && drawn.length === 0, (e) => e.child(div().px(14).py(SPACE.sm).text_size(TEXT.label).text_color(t.dim).child("no executors")))
      .child(
        div().flex_1().min_h(0).relative()
          .child(
            v_flex().id("cockpit-rows").absolute().inset_0().overflow_y_scroll()
              .children([...drawn.filter((g) => g.rows.some((r) => keeps(this.filter, r))).map((g) => this.section(g, now, t, look)), ...(empty.length > 0 ? [this.idle(empty, t, look)] : [])]),
          )
          .child(Scrollbar.vertical("cockpit-rows").absolute().inset_0()),
      )
      .when(look.id === "g", (e) => e.child(div().flex_none().h(26).px(10).flex().items_center().border_t(1).border_color(t.hover)
        .text_size(12).text_color(t.dim).child(`state ${clock} · every 5s`)));
  }
}

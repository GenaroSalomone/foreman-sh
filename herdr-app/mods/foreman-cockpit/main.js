import { View, div } from "gpui-kit";
import { h_flex, v_flex, Scrollbar } from "gpui-base";
import { focused, home, layout, props, readTextFile, run, setShared, shared, terminals, theme } from "herdr";
import * as fs from "fs/promises";
import {
  attentionParts, classify, cut, dur, laneOf, metaText, noProgressAfterMs, parseState, progressText,
  folderError, modelOf, statusText, subagentLine, subagentsSummary, terminalFor,
} from "./state.js";
import { activeChild, LOOK_LAYOUTS, shownLook, switchOf } from "./switcher.js";
import { alarm, kindOf, colorOf, lookOf, mark, SPACE, TEXT } from "./tokens.js";

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
    this.error = null;
    this.open = new Set();
    this.folded = new Set();
    this.cockpitDir = null;
    this.reading = false;
    this.held = [];
    this.picked = null;
    this.published = "";
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
      .sort((a, b) => laneOf(a.rows).localeCompare(laneOf(b.rows)) || a.state.invoker_pane.localeCompare(b.state.invoker_pane));
  }

  // One brainer: its lane, its pane id and hw's counts, then its rows. Another brainer is another
  // section, because an ask waits on that brainer and nobody else.
  section(g, now, t, look) {
    const dim = g.cls === "stale";
    const thresholdMs = noProgressAfterMs(g.state);
    const lane = cut(laneOf(g.rows) || g.state.invoker_pane, 40);
    const counts = this.counts(g.state);
    const right = [g.state.invoker_pane, counts].filter(Boolean).join(" · ");
    const kinds = new Set(g.rows.map((r) => kindOf(r.attention)));
    const urgent = URGENT.find((k) => kinds.has(k)) ?? "idle";
    const key = g.state.invoker_pane;
    const shut = this.folded.has(key) || g.rows.length === 0;
    const rows = shut && look.id === "g" ? [] : g.rows.map((r) => this.card(r, now, t, dim, thresholdMs, look));
    const head = look.id === "d"
      ? h_flex().h(34).mt(6).px(14).gap(8).bg(t.surface).border_t(1).border_b(1).border_color(t.border).text_size(12.5)
        .child(mark(look, dim ? "idle" : urgent, t, 12))
        .child(div().flex_none().font_weight(500).text_color(dim ? t.dim : t.foreground).child(lane))
        .child(div().flex_none().text_color(t.dim).child(String(g.rows.length)))
        .child(div().flex_1().min_w(0).truncate().text_right().text_size(12).text_color(t.dim).child(right))
      : look.id === "g"
        ? h_flex().id(`sec-${key}`).h(26).mt(4).pl(4).pr(6).gap(6).cursor_pointer().text_size(12.5).text_color(t.muted)
          .on_click((_e, cx) => { if (this.folded.has(key)) this.folded.delete(key); else this.folded.add(key); cx.notify(); })
          .child(div().flex_none().w(12).text_center().text_size(9).text_color(t.dim).child(shut ? "▸" : "▾"))
          .child(div().flex_none().font_weight(500).text_color(dim ? t.dim : t.foreground).child(lane))
          .child(div().flex_none().child(String(g.rows.length)))
          .child(div().flex_1().min_w(0).truncate().text_right().pr(6).text_size(12).text_color(t.dim).child(right))
        : h_flex().w_full().min_w(0).gap(10).pt(16).px(14).pb(6).border_b(1).border_color(t.border).text_size(TEXT.label)
          .child(div().flex_none().text_size(13).font_bold().text_color(dim ? t.dim : t.foreground).child(lane))
          .child(div().flex_none().text_color(t.muted).child(cut(g.state.invoker_pane, 24)))
          .child(div().flex_1().min_w(0).truncate().text_right().text_color(dim ? t.dim : t.foreground).child(counts));
    return v_flex().w_full().min_w(0).flex_none()
      .child(head)
      .when(look.id === "d" && g.rows.length === 0, (e) => e.child(div().pl(38).pr(14).pt(10).pb(12).text_size(12.5).text_color(t.dim).child("No executors.")))
      .children(rows);
  }

  // What waits on the brainers, above everything: C's full-width signal block, D's inbox card, G's outlined callout.
  callout(rows, now, t, look) {
    if (rows.length === 0) return null;
    const by = (a) => rows.filter((r) => r.attention === a).length;
    const title = [[by("report"), "report", "to read"], [by("ask"), "ask", "to answer"], [by("challenge"), "challenge", "to rule"],
      [by("blocked"), "blocked", ""]].filter(([n]) => n > 0)
      .map(([n, l, what]) => (l === "blocked" ? `${n} blocked` : `${n} ${l}${n === 1 ? "" : "s"} ${what}`)).join(" · ");
    const items = rows.map((r) => (look.id === "c" ? `${r.task} (${r.project}, ${age(r, now)})` : `${r.task} · ${r.project} · ${age(r, now)}`));
    if (look.id === "c") {
      return v_flex().w_full().flex_none().px(14).py(12).gap(3).bg(t.done).text_color(t.background)
        .child(div().text_size(14).font_bold().child(title))
        .child(div().text_size(TEXT.label).child(items.join(" · ")));
    }
    const kind = rows.some((r) => r.attention === "blocked" || r.attention === "challenge") ? "blocked" : "waits";
    const box = look.id === "d"
      ? h_flex().mt(10).mx(12).mb(4).px(12).py(10).rounded(8).border_1().border_color(t.border).bg(t.raised)
      : h_flex().m(8).px(10).py(8).rounded(6).border_1().border_color(t.input).bg(t.background);
    return box.flex_none().gap(look.id === "d" ? 10 : 8).items_start()
      .child(mark(look, kind, t, 12))
      .child(v_flex().flex_1().min_w(0).gap(1)
        .child(div().text_size(13).font_weight(look.id === "d" ? 600 : 500).text_color(t.foreground).child(title))
        .children(look.id === "d" ? [div().text_size(12).line_height(1.5).text_color(t.muted).child(items.join("  /  "))]
          : items.map((x) => div().text_size(12).line_height(1.5).text_color(t.muted).child(x))));
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
      const el = div().id(`tab-${i.child}`).flex_none().flex().items_center().cursor_pointer().on_click(pick(i)).child(i.label);
      if (look.id === "c") return el.text_size(13).text_color(on ? t.foreground : t.muted).when(on, (e) => e.font_bold())
        .when(!on, (e) => e.hover((s) => s.text_color(t.foreground)));
      if (look.id === "d") return el.h(26).px(10).rounded(6).border_1().border_color(on ? t.border : t[look.dock]).text_size(12.5)
        .text_color(on ? t.foreground : t.muted).when(on, (e) => e.bg(t.raised)).when(!on, (e) => e.hover((s) => s.text_color(t.foreground)));
      return el.h_full().px(12).text_size(13).border_r(1).border_b(on ? 0 : 1).border_color(t.border)
        .text_color(on ? t.foreground : t.muted).when(!on, (e) => e.hover((s) => s.bg(t.hover)));
    };
    if (look.id === "g") {
      return h_flex().size_full().items_stretch().bg(t[look.dock])
        .children(sw.items.map(label))
        .child(h_flex().flex_1().justify_end().pr(8).border_b(1).border_color(t.border).child(this.looks(t, look)));
    }
    return h_flex().size_full().px(look.id === "c" ? 14 : 12).gap(look.id === "c" ? 14 : 6).bg(t[look.dock]).border_b(1).border_color(t.border)
      .children(sw.items.map(label))
      .child(look.id === "d"
        ? h_flex().flex_1().min_w(0).justify_end().child(div().min_w(0).h(24).px(8).rounded(6).border_1().border_color(t.border)
          .text_size(12).text_color(t.muted).flex().items_center().child(div().min_w(0).truncate().child(live)))
        : div().flex_1().min_w(0).truncate().text_size(TEXT.label).text_color(t.muted).child(live))
      .child(this.looks(t, look));
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
    const drawn = look.id === "c" ? groups.filter((g) => g.rows.length > 0) : groups;
    const stale = groups.some((g) => g.cls === "stale");
    const loud = groups.filter((g) => g.cls === "fresh").flatMap((g) => g.rows).filter((r) => LOUD.has(r.attention));
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
      .children([this.callout(loud, now, t, look)].filter((x) => x !== null))
      .when(stale, (e) => e.child(div().flex_none().px(14).py(4).text_size(TEXT.label).text_color(t.dim).child("stale — a writer has not beaten for 15 s; its rows are dimmed")))
      .when(silent === null && drawn.length === 0, (e) => e.child(div().px(14).py(SPACE.sm).text_size(TEXT.label).text_color(t.dim).child("no executors")))
      .child(
        div().flex_1().min_h(0).relative()
          .child(
            v_flex().id("cockpit-rows").absolute().inset_0().overflow_y_scroll()
              .children(drawn.map((g) => this.section(g, now, t, look))),
          )
          .child(Scrollbar.vertical("cockpit-rows").absolute().inset_0()),
      )
      .when(look.id === "g", (e) => e.child(div().flex_none().h(26).px(10).flex().items_center().border_t(1).border_color(t.hover)
        .text_size(12).text_color(t.dim).child(`state ${clock} · every 5s`)));
  }
}

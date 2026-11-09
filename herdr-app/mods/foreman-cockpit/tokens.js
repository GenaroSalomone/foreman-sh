// The foreman mods' one look: near-monochrome theme surfaces, one accent, hairline
// separators, one type scale, dense rows. Measured from Herdr's own sidebar (spaces).
// The same file sits in every foreman-* mod, because the app refuses an import from
// outside a mod's folder; edit one, copy it to the others — setup/tests/1277 fails
// while the copies differ.
import { div } from "gpui-kit";
import { props, shared } from "herdr";
import { h_flex, v_flex } from "gpui-base";

export const TEXT = { label: 11.5, body: 12 };
export const SPACE = { xs: 4, sm: 8, md: 12 };
export const RADIUS = 6;
export const ROW = 28; // one dense list row
const BAR = 36; // a panel's header bar

// A status: a small dot in its color; the text beside it stays neutral.
export const dot = (color) => div().flex_none().size(6).rounded(3).bg(color);
export const status = (color, label) => h_flex().flex_none().gap(SPACE.xs + 1).items_center().child(dot(color)).child(label);
// Where text under a dotted line starts: past the dot and its gap.
export const INDENT = 6 + SPACE.sm;

// A panel's header bar: one hairline below, nothing else.
export const bar = (t) =>
  h_flex().h(BAR).flex_none().px(SPACE.md).gap(SPACE.sm).items_center().border_b(1).border_color(t.border).text_size(TEXT.body);

// A quiet text button: no fill until hovered; `active` (a chosen filter or tab) sits on the selection color.
// D: Linear's outlined pill; G: Zed's 22 px segment.
export const button = (id, label, t, active = false, look = lookOf(props())) =>
  div().id(id).h(look.id === "c" ? 22 : look.id === "d" ? 24 : 22).flex_none().px(look.id === "g" ? 7 : SPACE.sm)
    .rounded(look.id === "g" ? 4 : RADIUS).flex().items_center().cursor_pointer().text_size(look.id === "c" ? TEXT.label : 12)
    .when(look.id === "d", (el) => el.border_1().border_color(t.border))
    .when(active, (el) => el.bg(look.id === "d" ? t.raised : t.selected).text_color(t.foreground))
    .when(!active, (el) => el.text_color(t.muted).hover((s) => s.bg(t.hover).text_color(t.foreground)))
    .child(label);

// Something failed: a blocked-colored line, its detail under it, a hairline below.
export const alarm = (title, detail, t) =>
  v_flex().w_full().min_w(0).flex_none().px(SPACE.md).py(SPACE.sm).gap(2).border_b(1).border_color(t.border)
    .child(h_flex().w_full().min_w(0).gap(SPACE.sm).items_center().text_color(t.blocked).child(dot(t.blocked)).child(title))
    .when(Boolean(detail), (el) => el.child(div().pl(INDENT).text_size(TEXT.label).text_color(t.dim).child(detail)));

// Each look's grammar, from its approved mockup (looks C, D and G in herdr-app/README.md): the
// layout passes `props.look` to every foreman block. `dock`: the theme color the look's panels
// sit on. `font`: the mono face for code-like text (G's hover card). Marks: Linear/Zed status
// circles in D and G, C's square for "waits on you"; color is the theme's status color.
export const LOOKS = {
  c: { id: "c", dock: "chrome", mono: "Geist Mono", marks: { working: "●", idle: "○", waits: "■", blocked: "■" }, pulse: 0.25 },
  d: { id: "d", dock: "background", mono: "JetBrains Mono", marks: { working: "◐", idle: "○", waits: "●", blocked: "✕" }, pulse: 0.35 },
  g: { id: "g", dock: "surface", mono: "Lilex", marks: { working: "◐", idle: "○", waits: "●", blocked: "✕" }, pulse: 0.4 },
};
// A block's own `props.look`, else the shown layout's (`foreman-d`, `foreman-g`; `foreman` is C):
// a pane header or a viewer pane gets no props from the layout.
const known = (id) => (Object.hasOwn(LOOKS, id ?? "") ? LOOKS[id] : null);
export const lookOf = (p) => known(p?.look) ?? known(/^foreman-([a-z])$/.exec(String(shared("layout") ?? ""))?.[1]) ?? LOOKS.c;

// A row's kind from hw's attention: an ask and a report wait on the brainer, a challenge or a
// blocked report is an alarm.
export const kindOf = (attention) =>
  attention === "challenge" || attention === "blocked" ? "blocked"
    : attention === "ask" || attention === "report" ? "waits" : attention === "working" ? "working" : "idle";
// The theme color of a kind; `waits` is `done`, the app's own "finished while you weren't looking".
export const colorOf = (kind, t) => ({ working: t.working, idle: t.idle, waits: t.done, blocked: t.blocked })[kind];
// The status glyph in its color; working pulses natively (no timer).
export const mark = (look, kind, t, size = TEXT.label) =>
  div().flex_none().w(14).text_center().text_size(size).text_color(colorOf(kind, t)).child(look.marks[kind])
    .when(kind === "working", (el) => el.loop("opacity", { from: look.pulse, to: 1, period: 1400 }));

// A list's group header: D's tinted 34 px band, G's chevron row; C has none (null).
export const groupHead = (look, t, title, count, right) =>
  look.id === "c" ? null
    : look.id === "d"
      ? h_flex().flex_none().h(34).mt(6).px(14).gap(8).bg(t.surface).border_t(1).border_b(1).border_color(t.border).text_size(12.5)
        .child(div().flex_none().font_weight(500).child(title)).child(div().flex_none().text_color(t.dim).child(String(count)))
        .child(div().flex_1().text_right().text_size(12).text_color(t.dim).child(right))
      : h_flex().flex_none().h(26).mt(4).pl(4).pr(12).gap(6).text_size(12.5).text_color(t.muted)
        .child(div().flex_none().w(12).text_center().text_size(9).text_color(t.dim).child("▾"))
        .child(div().flex_none().font_weight(500).text_color(t.foreground).child(title)).child(div().flex_none().child(String(count)))
        .child(div().flex_1().text_right().text_size(12).text_color(t.dim).child(right));
// A list row's frame: C's rounded 28 px row, D's 38 px row on a hairline, G's flush row (two lines: 44 px).
export const listRow = (look, t, id, lines = 1) =>
  h_flex().id(id).w_full().flex_none().gap(look.id === "c" ? SPACE.sm : 10).cursor_pointer().hover((s) => s.bg(t.hover))
    .h(look.id === "c" ? ROW : look.id === "d" ? 38 : lines === 2 ? 44 : 24).px(look.id === "c" ? SPACE.sm : look.id === "d" ? 14 : 12)
    .text_size(look.id === "c" ? TEXT.body : 13)
    .when(look.id === "c", (e) => e.rounded(RADIUS)).when(look.id === "d", (e) => e.border_b(1).border_color(t.border));

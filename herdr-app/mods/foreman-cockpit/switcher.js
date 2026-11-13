// The right column's switch (Cockpit / Issues / Ports / History), without the app: plain
// node runs it, switcher.test.mjs pins it. The app's own mechanism is a layout
// `stack` plus `layout.select {stack, child}`; this only says which child shows.

// The `active` child of the stack named `id` in the shown layout's tree, or null.
export function activeChild(node, id) {
  if (node === null || typeof node !== "object") return null;
  if (node.id === id) return typeof node.active === "string" ? node.active : null;
  for (const c of Array.isArray(node.children) ? node.children : []) {
    const found = activeChild(c, id);
    if (found !== null) return found;
  }
  return null;
}

// The labels to draw, from the block's props `{ stack, items: [{ child, label }] }`;
// null when the props are not a switch (the block is the cockpit itself).
export function switchOf(p) {
  const s = p?.switch;
  if (!s || typeof s.stack !== "string" || !Array.isArray(s.items)) return null;
  const items = s.items.filter((i) => typeof i?.child === "string" && typeof i?.label === "string");
  return items.length > 0 ? { stack: s.stack, items } : null;
}

// The looks, as the layouts that draw them: the switcher's labels, in order. `layout.switch {name}` shows one.
export const LOOK_LAYOUTS = [{ label: "C", name: "foreman" }, { label: "D", name: "foreman-d" }, { label: "G", name: "foreman-g" }];
// The look the shown layout draws (`shared("layout")`), or null when it is none of them.
export const shownLook = (layoutName) => LOOK_LAYOUTS.find((l) => l.name === layoutName)?.name ?? null;

// The right column's switch bar is two rows: the labels (`SWITCH_ROW1`, each look's own height) and the live count with the look
// switcher (`SWITCH_ROW2`). The layouts' `right-tabs` block must be their sum; switcher.test.mjs holds it.
export const SWITCH_ROW1 = { foreman: 36, "foreman-d": 44, "foreman-g": 32 };
export const SWITCH_ROW2 = 26;
export const switchBarPx = (layoutName) => SWITCH_ROW1[layoutName] + SWITCH_ROW2;

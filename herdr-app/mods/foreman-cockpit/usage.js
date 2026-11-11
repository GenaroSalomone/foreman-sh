// The reader of an account's usage file, written by bin/statusline-usage-tee under
// ~/.cache/hw/claude-usage/<account>.json from Claude Code's status line stdin
// (`rate_limits`: 5-hour and 7-day windows, used_percentage and resets_at in epoch
// seconds). Nothing here draws and nothing imports the app, so it runs under plain
// node (usage.test.mjs). A file that is not that shape is dead, never a number.

// The thresholds the person asked for: amber from 80 %, red from 95 %.
export const WARN = 80;
export const CRIT = 95;
// The file is written on every status line refresh of a live session; older than this the
// number is a guess about a window that may have reset, so it is dimmed and says its age.
export const STALE_AFTER_S = 15 * 60;

const isPct = (x) => typeof x === "number" && Number.isFinite(x) && x >= 0;
const isObj = (x) => x !== null && typeof x === "object" && !Array.isArray(x);

const windowOf = (w) =>
  isObj(w) && isPct(w.used_percentage)
    ? { pct: w.used_percentage, resetsAt: typeof w.resets_at === "number" && Number.isFinite(w.resets_at) ? w.resets_at : null }
    : null;

// The file's text -> { account, writtenAt, five, seven } or null (unreadable / another shape).
export function parseUsage(text) {
  let j;
  try { j = JSON.parse(text); } catch { return null; }
  if (!isObj(j) || typeof j.account !== "string" || !/^[A-Za-z0-9_-]+$/.test(j.account)) return null;
  if (typeof j.written_at !== "number" || !isObj(j.rate_limits)) return null;
  const five = windowOf(j.rate_limits.five_hour);
  const seven = windowOf(j.rate_limits.seven_day);
  if (five === null && seven === null) return null;
  return { account: j.account, writtenAt: j.written_at, five, seven };
}

// A window whose reset has passed shows no number: Claude Code only rewrites the file when a session
// refreshes, and the old percentage belongs to a window that is over.
export const live = (w, nowS) => (w === null || (w.resetsAt !== null && w.resetsAt <= nowS) ? null : w);

// `pct` is the number the line draws (rounded): the color must never disagree with the text.
export const level = (pct) => (pct >= CRIT ? "crit" : pct >= WARN ? "warn" : "ok");
const RANK = { ok: 0, warn: 1, crit: 2 };

// One line of text per account: "personal   5h 96%   7d 41%". Its level is the worst window's.
export function lineOf(u, nowS) {
  const parts = [["5h", live(u.five, nowS)], ["7d", live(u.seven, nowS)]]
    .map(([l, w]) => ({ label: l, text: w === null ? "—" : `${Math.round(w.pct)}%`, level: w === null ? "ok" : level(Math.round(w.pct)) }));
  const worst = parts.reduce((a, p) => (RANK[p.level] > RANK[a] ? p.level : a), "ok");
  const ageS = Math.max(0, nowS - u.writtenAt);
  return { account: u.account, parts, level: worst, stale: ageS > STALE_AFTER_S, ageS };
}

// Sorted by account name; the default account first, as the brainers use it.
export const sortAccounts = (us) => [...us].sort((a, b) => (a.account === "default" ? -1 : b.account === "default" ? 1 : a.account.localeCompare(b.account)));

export const ageText = (s) => (s < 3600 ? `${Math.max(1, Math.round(s / 60))}m` : s < 86400 ? `${Math.round(s / 3600)}h` : `${Math.round(s / 86400)}d`);

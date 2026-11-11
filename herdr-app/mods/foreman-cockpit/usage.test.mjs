// usage.js against the shape bin/statusline-usage-tee writes. Plain node.
import { ageText, level, lineOf, parseUsage, sortAccounts } from "./usage.js";

let failed = 0;
let checks = 0;
const eq = (name, got, want) => {
  checks += 1;
  if (JSON.stringify(got) !== JSON.stringify(want)) { failed += 1; console.error(`not ok - ${name}: got ${JSON.stringify(got)}, want ${JSON.stringify(want)}`); }
};
const file = (o = {}) => JSON.stringify({ account: "personal", written_at: 1000, model: "m", cwd: "/x",
  rate_limits: { five_hour: { used_percentage: 96.5, resets_at: 5000 }, seven_day: { used_percentage: 41, resets_at: 9000 } }, ...o });

const u = parseUsage(file());
eq("parses both windows", [u.account, u.five.pct, u.seven.resetsAt], ["personal", 96.5, 9000]);
eq("not json", parseUsage("nope"), null);
eq("no windows", parseUsage(file({ rate_limits: {} })), null);
eq("account with a slash is dead", parseUsage(file({ account: "../x" })), null);
eq("no written_at", parseUsage(file({ written_at: "now" })), null);
eq("one window is enough", parseUsage(file({ rate_limits: { seven_day: { used_percentage: 3 } } })).five, null);

eq("level 79.9", level(79.9), "ok");
eq("level 80", level(80), "warn");
eq("level 94.9", level(94.9), "warn");
eq("level 95", level(95), "crit");

const a = lineOf(u, 2000);
eq("text", a.parts.map((p) => p.text), ["97%", "41%"]);
eq("worst window sets the level", a.level, "crit");
eq("fresh", lineOf(u, 1500).stale, false);
const near = (a, b) => lineOf(parseUsage(file({ rate_limits: { five_hour: { used_percentage: a, resets_at: 5000 }, seven_day: { used_percentage: b, resets_at: 9000 } } })), 2000);
eq("94.6 draws 95% and is red: the color follows the number shown", [near(94.6, 1).parts[0].text, near(94.6, 1).parts[0].level], ["95%", "crit"]);
eq("79.6 draws 80% and is amber", [near(79.6, 1).parts[0].text, near(79.6, 1).parts[0].level], ["80%", "warn"]);
eq("79.4 draws 79% and is not colored", [near(79.4, 1).parts[0].text, near(79.4, 1).parts[0].level], ["79%", "ok"]);
eq("a window past its reset shows no number", lineOf(u, 6000).parts.map((p) => p.text), ["—", "41%"]);
eq("and stops counting toward the level", lineOf(u, 6000).level, "ok");
eq("stale after 15 min", lineOf(u, 1000 + 901).stale, true);
eq("sort: default first", sortAccounts([{ account: "personal" }, { account: "default" }]).map((x) => x.account), ["default", "personal"]);
eq("age", [ageText(30), ageText(600), ageText(7200), ageText(200000)], ["1m", "10m", "2h", "2d"]);

if (failed > 0) { console.error(`${failed} of ${checks} usage checks failed`); process.exit(1); }
console.log(`usage: ${checks} checks hold`);

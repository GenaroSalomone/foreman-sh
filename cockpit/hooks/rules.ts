// The rule band: which Bash commands are an `hw` dispatch, and the verdict
// `hw preflight --json` printed for one. Pure; the mod only displays a verdict.

export type Rule = { id: string; level: string; msg: string }
export type Verdict = { level: 'ok' | 'warn' | 'block'; rules: Rule[] }

// hw words that are not `hw <project> <task>`.
const NOT_A_DISPATCH = new Set([
  'status', 'receipt', 'log', 'outbox', 'reports', 'output', 'stage', 'train', 'done', 'preview',
  'ruling', 'next', 'wait', 'reap', 'unstick', 'handle', 'revive', 'help', 'preflight',
  'review-range', 'review-delta', 'cockpit-state', 'human-boundary', 'executor-turn-end', 'decisions',
  'sweep', 'suite', 'worktrees', 'gentle-home', 'wait-one', 'wait-monitor', 'chaining-lease-start', 'chaining-lease-expire',
])

// Splits a command into words the way a shell would for plain quoting. Returns
// null for anything a shell would do more with (pipes, substitution, redirects,
// lists): such a command is not observed, never guessed at.
export function words(command: string): string[] | null {
  const out: string[] = []
  let cur = ''
  let has = false
  let q: '"' | "'" | null = null
  for (let i = 0; i < command.length; i++) {
    const c = command[i] as string
    if (q === "'") {
      if (c === "'") q = null
      else cur += c
    } else if (q === '"') {
      if (c === '"') q = null
      else if (c === '$' || c === '`' || c === '\\') return null
      else cur += c
    } else if (c === "'" || c === '"') {
      q = c
      has = true
    } else if (/\s/.test(c)) {
      if (has || cur !== '') out.push(cur)
      cur = ''
      has = false
    } else if ('|&;<>()$`\\*?[]{}!#~'.includes(c)) {
      return null
    } else {
      cur += c
    }
  }
  if (q !== null) return null
  if (has || cur !== '') out.push(cur)
  return out
}

// The argv of the dispatch WITHOUT its leading `hw`, or null when the command is
// not one: `hw <project> <task> …` with neither of the first two a flag or a verb.
export function dispatchArgv(command: string): string[] | null {
  const w = words(command.trim())
  if (w === null || w.length < 3) return null
  const [bin, a, b] = w
  if (bin !== 'hw' && !(bin as string).endsWith('/hw')) return null
  if ((a as string).startsWith('-') || (b as string).startsWith('-')) return null
  if (NOT_A_DISPATCH.has(a as string)) return null
  return w.slice(1)
}

// hw's verdict, or null when what it printed is not one (preflight is then
// unavailable, and the band says so; it never blocks anything).
export function parseVerdict(text: string): Verdict | null {
  let v: any
  try {
    v = JSON.parse(text)
  } catch {
    return null
  }
  if (typeof v !== 'object' || v === null || v.schema !== 1) return null
  if (v.level !== 'ok' && v.level !== 'warn' && v.level !== 'block') return null
  if (!Array.isArray(v.rules)) return null
  const rules: Rule[] = []
  for (const r of v.rules) {
    if (typeof r !== 'object' || r === null) return null
    if (typeof r.id !== 'string' || typeof r.level !== 'string' || typeof r.msg !== 'string') return null
    rules.push({ id: r.id, level: r.level, msg: r.msg })
  }
  return { level: v.level, rules }
}

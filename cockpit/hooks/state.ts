// The reader's half of the contract in cockpit/schema.json: parse the state
// file and classify it against the reader's own clock. Pure; no `$` in here.

export const FRESH_MS = 15_000
export const DEAD_MS = 60_000
export const MAX_BYTES = 1_048_576

export type Row = {
  id: string
  project: string
  task: string
  pane: string
  vendor: string
  model: string | null
  agent_status: string
  turn_state: string | null
  children_running: string | null
  ctx: { pct: number | null; tokens: number | null; source: string | null }
  rulings_pending: { count: number; oldest_at: number | null }
  attention: string
  attention_since: number | null
  report: {
    status: string
    state: string
    at: number
    summary: string
  } | null
  actions: {
    verify: boolean
    ruling: boolean
    done: boolean
    receipt: boolean
    why_not?: Partial<Record<'verify' | 'ruling' | 'done' | 'receipt', string>>
  }
  ask: {
    kind: string
    seq: string
    state: string
    at: number
    pending_reply?: string | null
  } | null
}

export type State = {
  schema: 1
  generated_at: number
  herdr: { ok: boolean; error: string | null }
  totals: Record<string, number>
  rules: { cut: { running: boolean; since_ms: number | null; holder: string | null } }
  rows: Row[]
}

export type Class = 'fresh' | 'stale' | 'dead' | 'herdr-down'

export type Model = {
  cls: Class
  state: State | null
  ageMs: number | null
  why: string
}

const isObj = (v: unknown): v is Record<string, unknown> =>
  typeof v === 'object' && v !== null && !Array.isArray(v)
const isInt = (v: unknown): v is number => typeof v === 'number' && Number.isInteger(v)
const isStr = (v: unknown): v is string => typeof v === 'string'
const nullOr = (v: unknown, ok: (x: unknown) => boolean) => v === null || ok(v)

// The first field the mod uses that is missing or mistyped, or null.
function badRow(r: unknown, i: number): string | null {
  const at = `rows[${i}]`
  if (!isObj(r)) return `${at} is not an object`
  for (const k of ['id', 'project', 'task', 'pane', 'vendor', 'agent_status', 'attention']) {
    if (!isStr(r[k])) return `${at}.${k}`
  }
  if (!nullOr(r.model, isStr)) return `${at}.model`
  if (!nullOr(r.turn_state, isStr)) return `${at}.turn_state`
  if (!nullOr(r.children_running, isStr)) return `${at}.children_running`
  if (!nullOr(r.attention_since, isInt)) return `${at}.attention_since`
  const ctx = r.ctx
  if (!isObj(ctx) || !nullOr(ctx.pct, x => typeof x === 'number')) return `${at}.ctx`
  const rp = r.rulings_pending
  if (!isObj(rp) || !isInt(rp.count)) return `${at}.rulings_pending`
  const rep = r.report
  if (rep !== null) {
    if (!isObj(rep) || !isStr(rep.status) || !isStr(rep.state) || !isInt(rep.at) || !isStr(rep.summary)) {
      return `${at}.report`
    }
  }
  const act = r.actions
  if (!isObj(act)) return `${at}.actions`
  for (const k of ['verify', 'ruling', 'done', 'receipt']) {
    if (typeof act[k] !== 'boolean') return `${at}.actions.${k}`
  }
  if (act.why_not !== undefined) {
    if (!isObj(act.why_not)) return `${at}.actions.why_not`
    for (const v of Object.values(act.why_not)) if (!isStr(v)) return `${at}.actions.why_not`
  }
  const ask = r.ask
  if (ask !== null) {
    if (!isObj(ask) || !isStr(ask.kind) || !isStr(ask.seq) || !isStr(ask.state) || !isInt(ask.at)) {
      return `${at}.ask`
    }
    if (ask.pending_reply !== undefined && ask.pending_reply !== null && !isStr(ask.pending_reply)) return `${at}.ask.pending_reply`
  }
  return null
}

// A state object, or why the bytes are not one. `size` is the file's own.
export function parse(text: string, size: number): { state: State } | { error: string } {
  if (size > MAX_BYTES) return { error: `file is ${size} bytes, over ${MAX_BYTES}` }
  let v: unknown
  try {
    v = JSON.parse(text)
  } catch {
    return { error: 'unparsable' }
  }
  if (!isObj(v)) return { error: 'not an object' }
  if (v.schema !== 1) return { error: 'schema is not 1' }
  if (!isInt(v.generated_at)) return { error: 'generated_at' }
  const herdr = v.herdr
  if (!isObj(herdr) || typeof herdr.ok !== 'boolean') return { error: 'herdr.ok' }
  if (!nullOr(herdr.error, isStr)) return { error: 'herdr.error' }
  const totals = v.totals
  if (!isObj(totals)) return { error: 'totals' }
  for (const k of ['executors', 'omitted', 'working', 'idle', 'asks', 'reports', 'blocked', 'challenges', 'rulings_pending']) {
    if (!isInt(totals[k])) return { error: `totals.${k}` }
  }
  const cut = isObj(v.rules) ? v.rules.cut : undefined
  if (!isObj(cut) || typeof cut.running !== 'boolean' || !nullOr(cut.holder, isStr)) {
    return { error: 'rules.cut' }
  }
  if (!Array.isArray(v.rows)) return { error: 'rows' }
  for (let i = 0; i < v.rows.length; i++) {
    const bad = badRow(v.rows[i], i)
    if (bad !== null) return { error: bad }
  }
  return { state: v as unknown as State }
}

// x-state-classes, in order, first match wins. `state` is null when the file
// is missing or did not parse; `why` then says so.
export function classify(state: State | null, why: string, now: number): Model {
  if (state === null) return { cls: 'dead', state: null, ageMs: null, why }
  const age = now - state.generated_at
  if (age < 0) return { cls: 'dead', state: null, ageMs: age, why: 'generated_at is in the future' }
  if (age > DEAD_MS) return { cls: 'dead', state: null, ageMs: age, why: 'writer down' }
  if (age > FRESH_MS) return { cls: 'stale', state, ageMs: age, why: '' }
  if (!state.herdr.ok) return { cls: 'herdr-down', state: null, ageMs: age, why: state.herdr.error ?? 'herdr is unreachable' }
  return { cls: 'fresh', state, ageMs: age, why: '' }
}

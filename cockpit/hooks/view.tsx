// Pure tree builders: a classified model in, an element tree out. The mod has
// no logic about executors; hw's order and counts are drawn as they come.

import type { Model, Row, Subagent } from './state'

type Els = { Box: any; Text: any; Button: any; Input: any }

// What a press does, handed in by the module: the view draws, it decides nothing.
export type Acts = {
  done: (r: Row) => void
  receipt: (r: Row) => void
  dismiss: (r: Row) => void
  verify: (r: Row) => void
  ruling: (r: Row, text: string) => void
  reply: (r: Row, text: string) => void
  subagents: (r: Row) => void
}
// Which cards have their subagent list open, by row id. Absent = folded.
export type Open = Set<string>
// hw's last answer for a row, shown verbatim under its card until the row goes or its [x] is pressed.
export type Notes = Record<string, { ok: boolean; text: string; verb: string }>

const NOTE_MAX = 1500


const SUMMARY_MAX = 200
const BAND_SUMMARY_MAX = 80

export function span(ms: number): string {
  const s = Math.max(0, Math.round(ms / 1000))
  if (s < 60) return `${s}s`
  if (s < 3600) return `${Math.floor(s / 60)}m`
  if (s < 86400) return `${Math.floor(s / 3600)}h`
  return `${Math.floor(s / 86400)}d`
}

// A duration to the minute, the way a person reads how long something has run: `41m`, `3h12m`,
// `2d4h`; under a minute, seconds. `span` above says only the largest unit and serves `ago`.
export function dur(ms: number): string {
  const s = Math.max(0, Math.floor(ms / 1000))
  if (s < 60) return `${s}s`
  const m = Math.floor(s / 60)
  if (m < 60) return `${m}m`
  const h = Math.floor(m / 60)
  if (h < 24) return `${h}h${String(m % 60).padStart(2, '0')}m`
  return `${Math.floor(h / 24)}d${h % 24}h`
}

// Every string drawn goes through here: a control character or an escape sequence
// (a report summary or a brief's title can carry one) makes the engine refuse the whole tree.
const scrub = (text: string) =>
  text.replace(/\x1b\[[0-9;?]*[ -/]*[@-~]|\x1b[@-_]/g, ' ').replace(/[\x00-\x1f\x7f-\x9f]/g, ' ').replace(/ {2,}/g, ' ').trim()
const cut = (text: string, max: number) => {
  const t = scrub(text)
  return t.length <= max ? t : `${t.slice(0, max - 1)}…`
}

// The colour of the existing attention palette (an ask) that marks a card gone quiet.
const STALL_COLOR = 'yellow'

const LOUD = new Set(['challenge', 'ask', 'blocked', 'report'])

function tone(attention: string): string | undefined {
  if (attention === 'challenge' || attention === 'blocked') return 'red'
  if (attention === 'ask') return 'yellow'
  if (attention === 'report') return 'green'
  return undefined
}

// The one-line summary of a fresh or stale file, from hw's own totals.
export function totalsLine(m: Model): string {
  const s = m.state
  if (s === null) return ''
  // herdr.ok false zeroes the totals: they are unknown, not zero.
  if (!s.herdr.ok) return 'HERDR UNREACHABLE: counts unknown'
  const t = s.totals
  const parts = [`${t.executors} executor${t.executors === 1 ? '' : 's'}`]
  const add = (n: number, label: string) => {
    if (n > 0) parts.push(`${n} ${label}`)
  }
  add(t.working, 'working')
  add(t.idle, 'idle')
  add(t.challenges, 'challenge')
  add(t.asks, 'ask')
  add(t.blocked, 'blocked')
  add(t.reports, 'report')
  add(t.rulings_pending, 'ruling queued')
  add(t.omitted, 'not listed')
  if (s.rules.cut.running) {
    parts.push(`CUT running${s.rules.cut.holder === null ? '' : ` (${s.rules.cut.holder})`}`)
  }
  return parts.join(' · ')
}

// The line that stands in for rows when there are none to trust.
export function banner(m: Model): string | null {
  switch (m.cls) {
    case 'dead': {
      const last = m.ageMs !== null && m.ageMs >= 0 ? `last ${span(m.ageMs)} ago` : m.why
      return `NO STATE — writer down (${last})`
    }
    case 'herdr-down':
      return `HERDR UNREACHABLE: rows unknown (${m.why})`
    case 'stale':
      return `STALE ${Math.round((m.ageMs ?? 0) / 1000)} s`
    default:
      return null
  }
}

function rowKey(r: Row): string {
  return `row:${r.id}`
}

const BADGE: Record<string, { color: string | undefined; dim?: boolean }> = {
  working: { color: 'green' },
  idle: { color: undefined, dim: true },
  blocked: { color: 'red' },
  challenge: { color: 'red' },
  ask: { color: 'yellow' },
  report: { color: 'blue' },
  gone: { color: undefined, dim: true },
}

// A six-cell bar of hw's own ctx percentage.
function ctxBar(pct: number): string {
  const n = Math.max(0, Math.min(6, Math.round((pct / 100) * 6)))
  return '▰'.repeat(n) + '▱'.repeat(6 - n)
}

// Tokens the way the statusline writes them: 900, 237.5k, 1.2M.
function tokensText(n: number): string {
  const f = (x: number, u: string) => `${x.toFixed(1).replace(/\.0$/, '')}${u}`
  return n >= 1_000_000 ? f(n / 1_000_000, 'M') : n >= 1000 ? f(n / 1000, 'k') : `${n}`
}

// A percent only when the window is known; the tokens alone when it is not; n/a when nothing is.
function ctxMeta(r: Row): string {
  if (r.ctx.tokens === null) return 'ctx n/a'
  return r.ctx.pct === null ? `ctx ${tokensText(r.ctx.tokens)}` : `ctx ${Math.round(r.ctx.pct)}% ${ctxBar(r.ctx.pct)}`
}

// The one decision about a stalled card: a working row whose last progress is older than the
// threshold hw published (rules.no_progress_after_ms). How long, or null when it is not stalled.
function noProgress(r: Row, now: number, thresholdMs: number): number | null {
  if (r.attention !== 'working' || r.last_progress_at === null) return null
  const age = now - r.last_progress_at
  return age > thresholdMs ? age : null
}

// The status at the card's top right: the state and how long it has held it. A working card counts from its
// dispatch (and says its turn); a reported one from its report; the others from hw's attention_since.
function statusText(r: Row, now: number): string {
  if (r.attention === 'working') {
    return `● working ${dur(now - r.dispatched_at)}${r.turn_started_at === null ? '' : ` · turn ${dur(now - r.turn_started_at)}`}`
  }
  const label = r.attention === 'report' ? 'reported' : r.attention
  const since = r.report !== null ? r.report.at : r.attention_since
  return `● ${label}${since === null ? '' : ` ${dur(now - since)}`}`
}

// The folded line of a card's subagents, or null when it has none: `subagents: 1 running · 8 done`.
export function subagentsSummary(subs: Subagent[]): string | null {
  if (subs.length === 0) return null
  const n = (s: string) => subs.filter(x => x.state === s).length
  const parts = [[n('running'), 'running'], [n('done'), 'done'], [n('stale'), 'stale']].filter(([c]) => (c as number) > 0).map(([c, l]) => `${c} ${l}`)
  return `subagents: ${parts.join(' · ')}`
}

// One subagent when unfolded: how long it has run (or how long it ran), `done`, or `stale`.
export function subagentLine(x: Subagent, now: number): string {
  const when = x.state === 'running' ? dur(now - x.started_ms) : x.state === 'done' ? 'done' : `stale ${dur(now - x.last_ms)}`
  return cut(`↳ ${x.type} · ${x.model ?? '—'} · ${x.description} · ${when}`, 200)
}

// How far the task has come: its commits over its base, the suite running for it, the children its turn waits on.
function progressText(r: Row, now: number): string | null {
  const parts: string[] = []
  if (r.commits !== null) {
    parts.push(`${r.commits.count} commit${r.commits.count === 1 ? '' : 's'}`)
    if (r.commits.last_at !== null) parts.push(`last ${dur(now - r.commits.last_at)} ago`)
  }
  if (r.gate !== null) parts.push(`gate running${r.gate.since === null ? '' : ` ${dur(now - r.gate.since)}`}`)
  if (r.children_running !== null) parts.push(`children: ${r.children_running}`)
  return parts.length === 0 ? null : cut(parts.join(' · '), 200)
}

// What needs the brainer, one reason each; empty when nothing does, and then the card has no such line.
function attentionParts(r: Row, now: number, stalled: number | null): string[] {
  const parts: string[] = []
  if (stalled !== null) parts.push(`no progress ${dur(stalled)}`)
  if (r.rulings_pending.count > 0) {
    parts.push(`${r.rulings_pending.count} ruling${r.rulings_pending.count === 1 ? '' : 's'} queued${r.rulings_pending.oldest_at === null ? '' : ` ${dur(now - r.rulings_pending.oldest_at)}`}`)
  }
  if (r.ask !== null) parts.push(`${r.ask.kind} ${r.ask.seq} ${r.ask.state === 'delivered' ? 'unanswered' : 'undelivered'}`)
  if (r.report !== null && r.report.status === 'blocked') parts.push(`blocked: ${cut(r.report.summary, SUMMARY_MAX)}`)
  if (r.report !== null && r.report.state === 'undelivered') parts.push('report undelivered')
  return parts
}

// The card, top to bottom: what the task is and how it stands; its model and context; how far it has come; what
// needs the brainer (only when something does); the actions that would run. An action hw refuses is not drawn,
// and neither is its reason: the reason stays in the state file and in `hw receipt`, never on the card.
function RowCard({ els, r, now, dim, act, notes, open, thresholdMs, slug }: { els: Els; r: Row; now: number; dim: boolean; act: Acts; notes: Notes; open: Open; thresholdMs: number; slug: string }) {
  const { Box, Text, Button, Input } = els
  const badge = BADGE[r.attention] ?? { color: undefined, dim: true }
  const stalled = noProgress(r, now, thresholdMs)
  const color = dim || badge.dim ? undefined : stalled !== null ? STALL_COLOR : badge.color
  const meta = [cut(`${r.vendor || '—'} · ${r.model || '—'} · ${r.effort || '—'}`, 120), ctxMeta(r), r.agent_status === 'unknown' ? 'unknown' : null]
    .filter(x => x !== null)
    .join(' · ')
  const lines: unknown[] = []
  lines.push(
    <Box key="head" justifyContent="space-between" columnGap={2}>
      <Text key="id" bold dimColor={dim}>{cut(r.title === null ? slug : `${slug} · ${r.title}`, 160)}</Text>
      <Text key="badge" bold={!dim && LOUD.has(r.attention)} dimColor={dim || badge.dim} color={color}>{statusText(r, now)}</Text>
    </Box>,
  )
  lines.push(<Text key="meta" dimColor>{meta}</Text>)
  const progress = progressText(r, now)
  if (progress !== null) lines.push(<Text key="progress" dimColor={dim}>{progress}</Text>)
  if (r.report !== null && r.report.status !== 'blocked') {
    lines.push(
      <Text key="report" dimColor={dim}>
        {`report: ${cut(r.report.summary, SUMMARY_MAX)}`}
      </Text>,
    )
  }
  const subs = r.subagents ?? []
  const folded = subagentsSummary(subs)
  if (folded !== null) {
    const unfolded = open.has(r.id)
    // The only line of the card that is a toggle; running ones keep the colour that already marks what runs.
    lines.push(
      <Box key={`subs:${r.id}`}>
        <Button key={`subs-toggle:${r.id}`} label={`${unfolded ? '▾' : '▸'} ${folded}`} onPress={() => act.subagents(r)} />
      </Box>,
    )
    if (unfolded) {
      for (let i = 0; i < subs.length; i++) {
        lines.push(
          <Text key={`sub:${r.id}:${i}`} dimColor={dim || subs[i].state !== 'running'} color={!dim && subs[i].state === 'running' ? 'green' : undefined}>
            {subagentLine(subs[i], now)}
          </Text>,
        )
      }
    }
  }
  const attention = attentionParts(r, now, stalled)
  if (attention.length > 0) {
    const loud = r.attention === 'challenge' || r.attention === 'blocked'
    lines.push(
      <Text key="attention" bold={!dim} color={dim ? undefined : loud ? 'red' : STALL_COLOR}>
        {`⚠ ${attention.join(' · ')}`}
      </Text>,
    )
  }
  // The reply: an idle executor holding an ask or a challenge. The writer sets
  // pending_reply exactly when the hold is valid and addressed to this pane, so
  // the mod only draws what it is given. A challenge is answered with the
  // ruling verb, an ask with the report verb (the contract's `reply` and
  // `challenge-reply`).
  if (!dim && r.ask !== null && typeof r.ask.pending_reply === 'string') {
    lines.push(
      <Input key={`reply:${r.id}`} label="reply " placeholder={`answer this ${r.ask.kind}`} submitLabel="send" onSubmit={(text: string) => act.reply(r, text)} />,
    )
  }
  // The actions. Which ones exist is hw's call, read from `actions`: a true one is
  // drawn, a false one is not. A stale state draws none.
  const a = r.actions
  if (!dim) {
    const verbs: ['verify' | 'receipt' | 'done', (r: Row) => void][] = [['verify', act.verify], ['receipt', act.receipt], ['done', act.done]]
    const buttons = verbs.filter(([v]) => a[v]).map(([v, run]) => <Button key={`${v}:${r.id}`} label={v} onPress={() => run(r)} />)
    if (buttons.length > 0) {
      lines.push(
        <Box key={`buttons:${r.id}`} gap={1} justifyContent="flex-end">
          {buttons}
        </Box>,
      )
    }
    lines.push(
      a.ruling ? (
        <Input key={`ruling:${r.id}`} label="ruling " placeholder="correction for this executor" submitLabel="queue" onSubmit={(text: string) => act.ruling(r, text)} />
      ) : null,
    )
  }
  const note = notes[r.id]
  if (note !== undefined) {
    lines.push(
      <Box key={`note:${r.id}`} gap={1}>
        <Text color={note.ok ? undefined : 'red'}>{cut(note.text, NOTE_MAX)}</Text>
        <Button key={`close:${r.id}`} label="x" onPress={() => act.dismiss(r)} />
      </Box>,
    )
  }
  return (
    <Box key={rowKey(r)} flexDirection="column" borderStyle="round" borderColor={color} borderDimColor={dim || color === undefined} paddingX={1}>
      {lines.filter(l => l !== null)}
    </Box>
  )
}

export function band(els: Els, m: Model, now: number, open: () => void) {
  const { Box, Text, Button } = els
  const note = banner(m)
  if (note !== null) {
    return (
      <Box key="band">
        <Text key="line" color={m.cls === 'stale' ? 'yellow' : 'red'}>
          {`cockpit  ${note}`}
        </Text>
        {m.cls === 'stale' ? <Text key="totals" dimColor>{`  ${totalsLine(m)}`}</Text> : null}
      </Box>
    )
  }
  const top = m.state?.rows[0]
  const topLoud = top !== undefined && LOUD.has(top.attention)
  return (
    <Box key="band" flexDirection="column">
      <Box key="head">
        <Text key="line">{`cockpit  ${totalsLine(m)} `}</Text>
        <Button key="open" label="open" onPress={open} />
      </Box>
      {topLoud ? (
        <Text key="top" color={tone(top.attention)}>
          {`▸ ${top.attention} ${top.id}${top.report === null ? '' : `: ${cut(top.report.summary, BAND_SUMMARY_MAX)}`}`}
        </Text>
      ) : null}
    </Box>
  )
}

export function pane(els: Els, m: Model, now: number, act: Acts, notes: Notes, open: Open = new Set()) {
  const { Box, Text } = els
  const note = banner(m)
  // classify leaves `state` null for dead and herdr-down: there are no rows to draw.
  const rows = m.state?.rows ?? []
  const dim = m.cls === 'stale'
  // The task's slug names the card; the project only when the rows span more than one.
  const oneProject = new Set(rows.map(r => r.project)).size <= 1
  const t = m.state?.totals
  // hw's own counts as chips; unknown counts (herdr down) are said, not zeroed.
  const chips: [number | undefined, string, string | undefined][] = [
    [t?.working, 'working', 'green'],
    [t?.idle, 'idle', undefined],
    [t?.challenges, 'challenge', 'red'],
    [t?.blocked, 'blocked', 'red'],
    [t?.asks, 'ask', 'yellow'],
    [t?.reports, 'reported', 'blue'],
  ]
  return (
    <Box key="pane" flexDirection="column" gap={1}>
      {note !== null ? (
        <Box key="bannerbox" borderStyle="bold" borderColor={m.cls === 'stale' ? 'yellow' : 'red'} paddingX={1}>
          <Text key="banner" bold color={m.cls === 'stale' ? 'yellow' : 'red'}>
            {note}
          </Text>
        </Box>
      ) : null}
      {m.state !== null && !m.state.herdr.ok ? <Text key="totals" dimColor={dim}>{totalsLine(m)}</Text> : null}
      {m.state !== null && m.state.herdr.ok ? (
        <Box key="chips" flexWrap="wrap" columnGap={2}>
          <Text key="n" bold dimColor={dim}>{`${m.state.totals.executors} executor${m.state.totals.executors === 1 ? '' : 's'}`}</Text>
          {chips.map(([n, label, color]) =>
            n === undefined || n === 0 ? null : (
              <Text key={label} color={dim ? undefined : color} dimColor={dim || color === undefined}>{`● ${n} ${label}`}</Text>
            ),
          )}
          {(t?.rulings_pending ?? 0) > 0 ? <Text key="rp" dimColor>{`${t?.rulings_pending} ruling queued`}</Text> : null}
          {(t?.omitted ?? 0) > 0 ? <Text key="om" dimColor>{`${t?.omitted} not listed`}</Text> : null}
          {m.state.rules.cut.running ? <Text key="cut" color={dim ? undefined : 'yellow'} dimColor={dim}>{`CUT running${m.state.rules.cut.holder === null ? '' : ` (${m.state.rules.cut.holder})`}`}</Text> : null}
        </Box>
      ) : null}
      {m.cls === 'fresh' && rows.length === 0 ? <Text key="empty" dimColor>no executors</Text> : null}
      {rows.length > 0 ? (
        <Box key="cards" flexDirection="column">
          {rows.map(r => RowCard({ els, r, now, dim, act, notes, open, thresholdMs: m.state?.rules.no_progress_after_ms ?? Infinity, slug: oneProject ? r.task : r.id }))}
        </Box>
      ) : null}
    </Box>
  )
}

// The rule band: hw preflight's verdict for the dispatch just observed.
// Displayed, never enforced.
export function ruleLine(els: Els, v: { level: string; rules: { id: string; msg: string }[] } | 'unavailable') {
  const { Text } = els
  if (v === 'unavailable') return <Text key="rules" dimColor>{'rules  preflight unavailable (dispatch not checked)'}</Text>
  if (v.level === 'ok') return null
  const head = v.level === 'block' ? 'rules  hw would refuse this dispatch: ' : 'rules  warn: '
  return (
    <Text key="rules" color={v.level === 'block' ? 'red' : 'yellow'}>
      {cut(head + v.rules.map(r => r.msg).join('; '), 400)}
    </Text>
  )
}

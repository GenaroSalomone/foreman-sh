// Pure tree builders: a classified model in, an element tree out. The mod has
// no logic about executors; hw's order and counts are drawn as they come.

import type { Model, Row } from './state'

type Els = { Box: any; Text: any; Button: any; Input: any }

// What a press does, handed in by the module: the view draws, it decides nothing.
export type Acts = {
  done: (r: Row) => void
  receipt: (r: Row) => void
  verify: (r: Row) => void
  ruling: (r: Row, text: string) => void
  reply: (r: Row, text: string) => void
}
// hw's last answer for a row, shown verbatim under its card until the row goes.
export type Notes = Record<string, { ok: boolean; text: string }>

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

// Every string drawn goes through here: a control character or an escape sequence
// (hw's why_not texts carry newlines) makes the engine refuse the whole tree.
const scrub = (text: string) =>
  text.replace(/\x1b\[[0-9;?]*[ -/]*[@-~]|\x1b[@-_]/g, ' ').replace(/[\x00-\x1f\x7f-\x9f]/g, ' ').replace(/ {2,}/g, ' ').trim()
const cut = (text: string, max: number) => {
  const t = scrub(text)
  return t.length <= max ? t : `${t.slice(0, max - 1)}…`
}
const WHY_MAX = 200

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

function ctxMeta(r: Row): string {
  return r.ctx.pct === null ? 'ctx n/a' : `ctx ${Math.round(r.ctx.pct)}% ${ctxBar(r.ctx.pct)}`
}

function RowCard({ els, r, now, dim, act, notes }: { els: Els; r: Row; now: number; dim: boolean; act: Acts; notes: Notes }) {
  const { Box, Text, Button, Input } = els
  const badge = BADGE[r.attention] ?? { color: undefined, dim: true }
  const color = dim || badge.dim ? undefined : badge.color
  const meta = [
    cut(`${r.vendor || '—'} · ${r.model || '—'} · ${r.effort || '—'}`, 120),
    ctxMeta(r),
    r.agent_status === 'unknown' ? 'unknown' : null,
    r.report !== null ? `reported ${span(now - r.report.at)} ago` : r.attention_since === null ? null : `${span(now - r.attention_since)}`,
  ]
    .filter(x => x !== null)
    .join(' · ')
  const lines: unknown[] = []
  lines.push(
    <Box key="head" gap={1}>
      <Text key="id" bold dimColor={dim}>{cut(r.id, 120)}</Text>
      <Text key="badge" bold={!dim && LOUD.has(r.attention)} dimColor={dim || badge.dim} color={color}>{`[${r.attention}]`}</Text>
    </Box>,
  )
  lines.push(
    <Text key="meta" dimColor>{meta}</Text>,
  )
  if (r.report !== null) {
    lines.push(
      <Text key="report" dimColor={dim}>
        {`report ${r.report.status} (${r.report.state}): ${cut(r.report.summary, SUMMARY_MAX)}`}
      </Text>,
    )
  }
  if (r.ask !== null) {
    lines.push(
      <Text key="ask" dimColor={dim}>
        {`${r.ask.kind} ${r.ask.seq} (${r.ask.state})`}
      </Text>,
    )
  }
  if (r.children_running !== null) {
    lines.push(
      <Text key="children" dimColor>
        {`children: ${r.children_running}`}
      </Text>,
    )
  }
  if (r.rulings_pending.count > 0) {
    lines.push(
      <Text key="rulings" dimColor={dim}>
        {`${r.rulings_pending.count} ruling${r.rulings_pending.count === 1 ? '' : 's'} queued`}
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
  // The buttons. Which ones exist is hw's call, read from `actions`; a false one
  // shows hw's own reason beneath the row. A stale state draws none.
  const a = r.actions
  if (!dim) {
    const verbs: ['verify' | 'receipt' | 'done', (r: Row) => void][] = [['verify', act.verify], ['receipt', act.receipt], ['done', act.done]]
    const refused = verbs.filter(([v]) => !a[v])
    lines.push(
      <Box key={`buttons:${r.id}`} gap={1}>
        {verbs.filter(([v]) => a[v]).map(([v, run]) => <Button key={`${v}:${r.id}`} label={v} onPress={() => run(r)} />)}
      </Box>,
    )
    for (const [v] of refused) {
      lines.push(<Text key={`${v}:${r.id}`} dimColor>{cut(`[${v}] ${a.why_not?.[v] ?? 'not available'}`, WHY_MAX)}</Text>)
    }
    lines.push(
      a.ruling ? (
        <Input key={`ruling:${r.id}`} label="ruling " placeholder="correction for this executor" submitLabel="queue" onSubmit={(text: string) => act.ruling(r, text)} />
      ) : (
        <Text key={`ruling:${r.id}`} dimColor>{cut(`[ruling] ${a.why_not?.ruling ?? 'not available'}`, WHY_MAX)}</Text>
      ),
    )
  }
  const note = notes[r.id]
  if (note !== undefined) {
    lines.push(
      <Text key={`note:${r.id}`} color={note.ok ? undefined : 'red'}>
        {cut(note.text, NOTE_MAX)}
      </Text>,
    )
  }
  return (
    <Box key={rowKey(r)} flexDirection="column" borderStyle="round" borderColor={color} borderDimColor={dim || color === undefined} paddingX={1}>
      {lines}
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

export function pane(els: Els, m: Model, now: number, act: Acts, notes: Notes) {
  const { Box, Text } = els
  const note = banner(m)
  // classify leaves `state` null for dead and herdr-down: there are no rows to draw.
  const rows = m.state?.rows ?? []
  const dim = m.cls === 'stale'
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
          {rows.map(r => RowCard({ els, r, now, dim, act, notes }))}
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

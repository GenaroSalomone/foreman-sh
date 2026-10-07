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

const cut = (text: string, max: number) =>
  text.length <= max ? text : `${text.slice(0, max - 1)}…`

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

function ctxText(r: Row): string {
  return r.ctx.pct === null ? 'ctx n/a' : `ctx ${Math.round(r.ctx.pct)}%`
}

function rowKey(r: Row): string {
  return `row:${r.id}`
}

function RowCard({ els, r, now, dim, act, notes }: { els: Els; r: Row; now: number; dim: boolean; act: Acts; notes: Notes }) {
  const { Box, Text, Button, Input } = els
  const since = r.attention_since === null ? '' : ` ${span(now - r.attention_since)}`
  const model = r.model === null ? '' : ` ${r.model}`
  const lines: unknown[] = []
  lines.push(
    <Text key="head" dimColor={dim} color={dim ? undefined : tone(r.attention)} bold={!dim && LOUD.has(r.attention)}>
      {`${r.attention.padEnd(10)}${r.id}  ${r.vendor}${model}  ${r.agent_status}${since}  ${ctxText(r)}`}
    </Text>,
  )
  if (r.report !== null) {
    lines.push(
      <Text key="report" dimColor={dim}>
        {`  report ${r.report.status} (${r.report.state}): ${cut(r.report.summary, SUMMARY_MAX)}`}
      </Text>,
    )
  }
  if (r.ask !== null) {
    lines.push(
      <Text key="ask" dimColor={dim}>
        {`  ${r.ask.kind} ${r.ask.seq} (${r.ask.state})`}
      </Text>,
    )
  }
  if (r.children_running !== null) {
    lines.push(
      <Text key="children" dimColor>
        {`  children: ${r.children_running}`}
      </Text>,
    )
  }
  if (r.rulings_pending.count > 0) {
    lines.push(
      <Text key="rulings" dimColor={dim}>
        {`  ${r.rulings_pending.count} ruling${r.rulings_pending.count === 1 ? '' : 's'} queued`}
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
  // shows hw's own reason. A stale state draws none.
  const a = r.actions
  if (!dim) {
    const btn = (verb: 'done' | 'receipt' | 'verify', label: string, run: (r: Row) => void) =>
      a[verb] ? (
        <Button key={`${verb}:${r.id}`} label={label} onPress={() => run(r)} />
      ) : (
        <Text key={`${verb}:${r.id}`} dimColor>{`[${label}] ${a.why_not?.[verb] ?? 'not available'}`}</Text>
      )
    lines.push(
      <Box key={`buttons:${r.id}`}>
        {btn('verify', 'verify', act.verify)}
        {btn('receipt', 'receipt', act.receipt)}
        {btn('done', 'done', act.done)}
      </Box>,
    )
    lines.push(
      a.ruling ? (
        <Input key={`ruling:${r.id}`} label="ruling " placeholder="correction for this executor" submitLabel="queue" onSubmit={(text: string) => act.ruling(r, text)} />
      ) : (
        <Text key={`ruling:${r.id}`} dimColor>{`[ruling] ${a.why_not?.ruling ?? 'not available'}`}</Text>
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
    <Box key={rowKey(r)} flexDirection="column">
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
  return (
    <Box key="pane" flexDirection="column">
      {note !== null ? (
        <Text key="banner" bold color={m.cls === 'stale' ? 'yellow' : 'red'}>
          {note}
        </Text>
      ) : null}
      {m.state !== null ? (
        <Text key="totals" dimColor={dim}>{totalsLine(m)}</Text>
      ) : null}
      {m.cls === 'fresh' && rows.length === 0 ? <Text key="empty" dimColor>no executors</Text> : null}
      {rows.map(r => RowCard({ els, r, now, dim, act, notes }))}
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

import type { Register } from 'claude-code'

import { classify, parse, type Model, type Row, type State } from './state'
import { dispatchArgv, parseVerdict, type Verdict } from './rules'
import { argvOf, VERBS, type Names } from './verbs'
import { band, pane, ruleLine, type Acts, type Notes } from './view'

const PANE = 'cockpit'
const TICK_MS = 1000
const RELATIVE_TICK_MS = 5000
const MAX_BYTES = 1_048_576
const PREFLIGHT_MS = 3000
const VERB_MS = 120_000
const RULE_TTL_MS = 300_000

// The session's view of the file. Module variables, kept nowhere else: a reload
// starts them over and the first render reads the file again. The mod keeps
// nothing but the pilot's line (recordAction).
type Loaded = { state: State | null; why: string }

type Rule = { v: Verdict | 'unavailable'; at: number }

const S: {
  loaded: Loaded | null
  sig: string
  lastClass: string
  lastBucket: number
  busy: boolean
  notes: Notes
  rule: Rule | null
} = {
  loaded: null,
  notes: {},
  rule: null,
  sig: '',
  lastClass: '',
  lastBucket: -1,
  busy: false,
}

async function statePath($: any): Promise<string | null> {
  const direct = await $.env.get('HW_COCKPIT_STATE')
  if (direct) return direct
  const work = (await $.env.get('HW_COCKPIT_WORK')) || (await $.env.get('WORK'))
  const paneId = await $.env.get('HERDR_PANE_ID')
  return work && paneId ? `${work}/.cockpit/${paneId}.json` : null
}

// Reads the file only when its size or mtime moved. Returns whether what the
// reader holds changed.
async function refresh($: any): Promise<boolean> {
  const path = await statePath($)
  if (path === null) {
    const changed = S.loaded === null || S.sig !== 'nopath'
    S.sig = 'nopath'
    S.loaded = { state: null, why: 'no state path' }
    return changed
  }
  let next = 'missing'
  let size = 0
  try {
    const st = await $.fs.stat(path)
    size = st.size
    next = `${st.size}:${st.mtimeMs}`
  } catch {
    next = 'missing'
  }
  if (S.loaded !== null && next === S.sig) return false
  S.sig = next
  if (next === 'missing') {
    S.loaded = { state: null, why: 'state file missing' }
    return true
  }
  if (size > MAX_BYTES) {
    // Refused unread: dead, never a partial view.
    S.loaded = { state: null, why: 'state file too large' }
    return true
  }
  try {
    const text = await $.fs.read(path)
    const parsed = parse(text, size)
    S.loaded = 'state' in parsed ? { state: parsed.state, why: '' } : { state: null, why: parsed.error }
  } catch {
    S.loaded = { state: null, why: 'state file unreadable' }
  }
  return true
}

async function model($: any): Promise<{ m: Model; now: number }> {
  if (S.loaded === null) await refresh($)
  const now: number = await $.clock.now()
  const l = S.loaded as Loaded
  return { m: classify(l.state, l.why, now), now }
}

const redraw = ($: any) => $.ui.invalidate('ui.render')

const ACTIONS_CAP = 1_048_576

// The pilot's other half: one line per button press, `{at, pane, verb}`, in
// cockpit-actions.jsonl beside the state file (bin/cockpit-state --pilot reads it
// next to the writer's cockpit-events.jsonl). The mod's fs rewrites a whole file,
// so it owns this one alone. Capped by rotation to .1; a failure here never
// reaches the verb.
async function recordAction($: any, pane: string, verb: string): Promise<void> {
  try {
    const state = await statePath($)
    if (state === null) return
    const file = `${state.slice(0, state.lastIndexOf('/'))}/cockpit-actions.jsonl`
    let old = ''
    try {
      old = await $.fs.read(file)
    } catch {
      old = ''
    }
    if (old.length >= ACTIONS_CAP) {
      await $.fs.write(`${file}.1`, old)
      old = ''
    }
    await $.fs.write(file, `${old}${JSON.stringify({ at: await $.clock.now(), pane, verb })}\n`)
  } catch {
    // the recorder must not fail a verb
  }
}

// Starts one verb of the contract and keeps hw's answer, verbatim, for the row.
// A non-zero exit is shown as it came and the card stays; nothing is retried.
async function runVerb($: any, verb: string, id: string, names: Names, stdin?: string): Promise<void> {
  const argv = argvOf(verb, names)
  if (argv === null) return
  await recordAction($, names.pane ?? '', verb)
  let note: { ok: boolean; text: string }
  try {
    const out = await $.process.run(argv, stdin === undefined ? { timeoutMs: VERB_MS } : { stdin, timeoutMs: VERB_MS })
    const text = (out.exitCode === 0 ? out.stdout : `${out.stderr}${out.stdout}`).trim()
    note = { ok: out.exitCode === 0, text: text === '' ? (out.exitCode === 0 ? `hw ${verb}: ok` : `hw ${verb}: exit ${out.exitCode}`) : text }
  } catch (err) {
    note = { ok: false, text: `hw ${verb} did not run: ${String((err as Error)?.message ?? err)}` }
  }
  S.notes[id] = note
  $.ui.toast(note.ok ? `hw ${verb} ${id}: done` : `hw ${verb} ${id}: failed`)
  redraw($)
}

const namesOf = (r: Row): Names => ({
  project: r.project,
  task: r.task,
  pane: r.pane,
  ...(typeof r.ask?.pending_reply === 'string' ? { pending_reply: r.ask.pending_reply } : {}),
})

function acts($: any): Acts {
  return {
    done: async r => {
      if (VERBS.done!.confirm) {
        let answer = ''
        try {
          answer = await $.ui.ask(`Close ${r.project}/${r.task}? hw done refuses what is not safe to close.`, ['Close it', 'Keep it'])
        } catch {
          return
        }
        if (answer !== 'Close it') return
      }
      await runVerb($, 'done', r.id, namesOf(r))
    },
    receipt: r => runVerb($, 'receipt', r.id, namesOf(r)),
    ruling: async (r, text) => {
      if (text.trim() === '') return
      await runVerb($, 'ruling', r.id, namesOf(r), text)
    },
    reply: async (r, text) => {
      if (text.trim() === '' || r.ask === null || typeof r.ask.pending_reply !== 'string') return
      await runVerb($, r.ask.kind === 'challenge' ? 'challenge-reply' : 'reply', r.id, namesOf(r), text)
    },
    verify: async r => {
      await $.prompt.fill({
        text: `Verify the report of ${r.project}/${r.task} against its evidence before accepting it.`,
        mode: 'replace',
      })
    },
  }
}

// Asks hw what its rules say about a dispatch about to run. Fails open: no
// answer is "unavailable", never a refusal.
async function checkDispatch($: any, argv: string[]): Promise<void> {
  const cmd = argvOf('preflight', {}, argv)
  let v: Verdict | 'unavailable' = 'unavailable'
  try {
    const out = await $.process.run(cmd as string[], { timeoutMs: PREFLIGHT_MS })
    const parsed = out.exitCode === 0 ? parseVerdict(out.stdout) : null
    if (parsed !== null) v = parsed
  } catch {
    v = 'unavailable'
  }
  S.rule = { v, at: await $.clock.now() }
  redraw($)
}

async function tick($: any): Promise<void> {
  if (S.busy) return
  S.busy = true
  try {
    const changed = await refresh($)
    const { m, now } = await model($)
    if (m.state !== null) {
      const ids = new Set(m.state.rows.map(r => r.id))
      for (const id of Object.keys(S.notes)) if (!ids.has(id)) delete S.notes[id]
    }
    const bucket = Math.floor(now / RELATIVE_TICK_MS)
    if (changed || m.cls !== S.lastClass || bucket !== S.lastBucket) {
      S.lastClass = m.cls
      S.lastBucket = bucket
      $.ui.invalidate('ui.render')
    }
  } finally {
    S.busy = false
  }
}

export const register: Register = on => {
  S.loaded = null
  S.sig = ''
  S.lastClass = ''
  S.lastBucket = -1
  S.busy = false
  S.notes = {}
  S.rule = null

  on('session.start', async ($, e, next) => {
    await $.command.register({ name: 'cockpit', description: 'Show the brainer cockpit pane' })
    $.clock.every(TICK_MS, () => {
      void tick($)
    })
    return next(e)
  })

  on('command.run', { command: 'cockpit' }, async $ => {
    await $.ui.open({ id: PANE, title: 'Cockpit' })
    return { text: 'Cockpit pane opened.' }
  })

  on('ui.render', { component: 'AbovePrompt' }, async ($, e, next) => {
    if (e.props.hasSurvey) return next(e)
    const { m, now } = await model($)
    const els = $.ui.resolve(e)
    const main = band(els, m, now, () => {
      void $.ui.open({ id: PANE, title: 'Cockpit' })
    })
    const rule = S.rule !== null && now - S.rule.at < RULE_TTL_MS ? ruleLine(els, S.rule.v) : null
    if (rule === null) return main
    const { Box } = els
    return (
      <Box key="stack" flexDirection="column">
        {main}
        {rule}
      </Box>
    )
  })

  // The rule band only looks: hw's verdict is shown and the dispatch always goes on.
  on('tool.call', { tool: 'Bash' }, async ($, e, next) => {
    const argv = dispatchArgv(e.command)
    if (argv !== null) await checkDispatch($, argv)
    return next(e)
  }).catch(($, e, next) => next(e))

  on('ui.render', { component: 'Pane', requestId: PANE }, async ($, e) => {
    const { m, now } = await model($)
    return pane($.ui.resolve(e), m, now, acts($), S.notes)
  })
}

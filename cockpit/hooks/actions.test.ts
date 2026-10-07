import { expect, mock, test } from 'claude-code/testing'
import { FIXTURES } from './fixtures.gen'

const NOW = 1_790_000_000_000
const PATH = '/state/cockpit.json'
const VERBS_ALLOWED = ['done', 'ruling', 'receipt', 'preflight']
const REPLY_FLAGS = ['--report', '--ruling']

type Run = { argv: string[]; stdin?: string }
type Reply = { exitCode: number; stdout: string; stderr: string } | 'throw'

// The state file, the clock, the host's processes and the person's answers,
// all from memory beneath the mod.
function world(on: any, file: string, opts: { reply?: Reply; ask?: string | 'dismiss'; files?: Record<string, string> } = {}) {
  const w = { timeouts: [] as (number | undefined)[], runs: [] as Run[], fills: [] as string[], asks: [] as string[], reply: opts.reply ?? ({ exitCode: 0, stdout: 'ok', stderr: '' } as Reply), ask: opts.ask ?? 'Close it', toolRan: 0, files: {} as Record<string, string>, ...(opts.files ? { files: { ...opts.files } } : {}) }
  mock.env(on, { HW_COCKPIT_STATE: PATH })
  mock.clock(on, { now: NOW })
  on('fs.stat', async (_$: any, e: any) =>
    e.path === PATH ? { value: { kind: 'file', size: file.length, mtimeMs: 1, isLink: false } } : { deny: 'ENOENT' })
  on('fs.read', async (_$: any, e: any) => (e.path === PATH ? { value: file } : e.path in w.files ? { value: w.files[e.path] } : { deny: 'ENOENT' }))
  on('fs.write', async (_$: any, e: any) => {
    w.files[e.path] = e.text
    return { value: undefined }
  })
  on('ui.invalidate', async () => ({ value: undefined }))
  on('ui.toast', async () => ({ value: undefined }))
  on('command.register', async () => ({ value: {} }))
  on('session.start', async (_$: any, e: any) => ({ cwd: e.cwd }))
  on('process.run', async (_$: any, e: any) => {
    w.runs.push({ argv: [...e.argv], stdin: e.init?.stdin })
    w.timeouts.push(e.init?.timeoutMs)
    if (w.reply === 'throw') return { deny: 'timed out' }
    return { value: w.reply }
  })
  on('prompt.fill', async (_$: any, e: any) => {
    w.fills.push(e.text)
    return { isFilled: true }
  })
  on('tool.call', { tool: 'AskUserQuestion' }, async (_$: any, e: any) => {
    w.asks.push(e.questions[0].question)
    if (w.ask === 'dismiss') return { deny: 'dismissed' }
    return { result: { questions: e.questions, answers: { [e.questions[0].question]: w.ask } } }
  })
  on('tool.call', { tool: 'Bash' }, async () => {
    w.toolRan += 1
    return { result: { stdout: '', stderr: '', interrupted: false } }
  })
  return w
}

const VIEWPORT = { columns: 120, rows: 40 } as any
async function drawPane($: any) {
  return $.ui.mount({ plugin: 'cockpit', surface: 'terminal', component: 'Pane', requestId: 'cockpit', props: { title: 'Cockpit', isFocused: false, bodyColumns: 120, placement: 'inline', scroll: {}, view: {} } as any, viewport: VIEWPORT })
}
async function drawBand($: any) {
  return $.ui.mount({ plugin: 'cockpit', surface: 'terminal', component: 'AbovePrompt', props: { hasSurvey: false, isWorking: false, maxRows: 40, bodyColumns: 120, scroll: {}, view: {} } as any, viewport: VIEWPORT })
}
const texts = async (ui: any): Promise<string[]> => (await ui.findAll({ type: 'Text' })).map((t: any) => t.text as string)
const keys = async (ui: any, type: string): Promise<string[]> => (await ui.findAll({ type })).map((t: any) => t.key as string)
const allowed = (w: { runs: Run[] }) =>
  w.runs.every(r => (r.argv[0] === 'hw' && VERBS_ALLOWED.includes(r.argv[1] as string)) || (r.argv[0] === 'channel-send' && REPLY_FLAGS.includes(r.argv[1] as string)))

// A9: each button starts one verb, with the row's own names.
test('done asks first, then runs hw done with the row’s project and task', async ($, on) => {
  const w = world(on, FIXTURES['fresh.json']!)
  const ui = await drawPane($)
  await ui.press({ key: 'done:demo:task-04' })
  expect(w.asks.length).toBe(1)
  expect(w.runs).toEqual([{ argv: ['hw', 'done', 'demo', 'task-04'], stdin: undefined }])
})

for (const ask of ['dismiss', 'Keep it']) {
  test(`a question answered ${ask} runs nothing`, async ($, on) => {
    const w = world(on, FIXTURES['fresh.json']!, { ask })
    const ui = await drawPane($)
    await ui.press({ key: 'done:demo:task-04' })
    expect(w.asks.length).toBe(1)
    expect(w.runs.length).toBe(0)
  })
}

test('hw’s refusal is shown verbatim and the card stays', async ($, on) => {
  const w = world(on, FIXTURES['fresh.json']!, { reply: { exitCode: 1, stdout: '', stderr: 'hw: refusing: a pane WORKING right now\n' } })
  const ui = await drawPane($)
  await ui.press({ key: 'done:demo:task-04' })
  expect(w.runs.length).toBe(1)
  await ui.unmount()
  const again = await drawPane($)
  expect((await texts(again)).some(t => t === 'hw: refusing: a pane WORKING right now')).toBe(true)
  expect(await keys(again, 'Button')).toContain('done:demo:task-04')
  expect(await keys(again, 'Box')).toContain('row:demo:task-04')
})

test('a verb that cannot start is a shown failure, not a crash', async ($, on) => {
  const w = world(on, FIXTURES['fresh.json']!, { reply: 'throw' })
  const ui = await drawPane($)
  await ui.press({ key: 'receipt:demo:task-01' })
  expect(w.runs.length).toBe(1)
  await ui.unmount()
  expect((await texts(await drawPane($))).some(t => t.startsWith('hw receipt did not run'))).toBe(true)
})

test('receipt runs hw receipt for the row', async ($, on) => {
  const w = world(on, FIXTURES['fresh.json']!, { reply: { exitCode: 0, stdout: 'measured: 3 files', stderr: '' } })
  const ui = await drawPane($)
  await ui.press({ key: 'receipt:demo:task-01' })
  expect(w.runs).toEqual([{ argv: ['hw', 'receipt', 'demo', 'task-01'], stdin: undefined }])
  await ui.unmount()
  expect((await texts(await drawPane($))).some(t => t === 'measured: 3 files')).toBe(true)
})

// The pilot: a button press leaves `{at, pane, verb}` beside the state file, before the verb runs.
const ACTIONS = '/state/cockpit-actions.jsonl'
test('a button press is recorded for the pilot, and a failing verb is recorded too', async ($, on) => {
  const w = world(on, FIXTURES['fresh.json']!, { reply: { exitCode: 1, stdout: '', stderr: 'refused' } })
  const ui = await drawPane($)
  await ui.press({ key: 'receipt:demo:task-01' })
  const lines = w.files[ACTIONS]!.trim().split('\n').map(l => JSON.parse(l))
  expect(lines).toEqual([{ at: NOW, pane: expect.any(String), verb: 'receipt' }])
  expect(lines[0].pane).not.toBe('')
})

test('the action file is appended to', async ($, on) => {
  const old = '{"at":1,"pane":"w1:p1","verb":"done"}\n'
  const w = world(on, FIXTURES['fresh.json']!, { files: { [ACTIONS]: old } })
  await (await drawPane($)).press({ key: 'receipt:demo:task-01' })
  expect(w.files[ACTIONS]!.startsWith(old)).toBe(true)
  expect(w.files[ACTIONS]!.trim().split('\n')).toHaveLength(2)
})

test('the action file is rotated at 1 MiB', async ($, on) => {
  const old = '{"at":1,"pane":"w1:p1","verb":"done"}\n'
  const big = old.repeat(Math.ceil(1_048_576 / old.length))
  const w2 = world(on, FIXTURES['fresh.json']!, { files: { [ACTIONS]: big } })
  await (await drawPane($)).press({ key: 'receipt:demo:task-01' })
  expect(w2.files[`${ACTIONS}.1`]).toBe(big)
  expect(w2.files[ACTIONS]!.trim().split('\n')).toHaveLength(1)
})

test('ruling takes its text on stdin, never in argv', async ($, on) => {
  const w = world(on, FIXTURES['fresh.json']!)
  const ui = await drawPane($)
  const text = 'use `rg`, not $HOME grep\nsecond line; "quoted"'
  await ui.input({ key: 'ruling:demo:task-03', text })
  expect(w.runs).toEqual([{ argv: ['hw', 'ruling', 'w1:p13', '-'], stdin: text }])
})

test('an empty ruling runs nothing', async ($, on) => {
  const w = world(on, FIXTURES['fresh.json']!)
  const ui = await drawPane($)
  await ui.input({ key: 'ruling:demo:task-03', text: '   ' })
  expect(w.runs.length).toBe(0)
})

test('verify only pre-fills the prompt: no process, no turn', async ($, on) => {
  const w = world(on, FIXTURES['fresh.json']!)
  const ui = await drawPane($)
  await ui.press({ key: 'verify:demo:task-04' })
  expect(w.fills.length).toBe(1)
  expect(w.fills[0]).toContain('demo/task-04')
  expect(w.runs.length).toBe(0)
})

test('the mod never decides: a false action draws hw’s reason, not a button', async ($, on) => {
  world(on, FIXTURES['fresh.json']!)
  const ui = await drawPane($)
  const b = await keys(ui, 'Button')
  expect(b).not.toContain('done:demo:task-01')
  expect(b).toContain('done:demo:task-04')
  expect(b).not.toContain('verify:demo:task-01')
  const t = await texts(ui)
  expect(t).toContain('[done] task still working')
  expect(t.some(x => x.startsWith('[ruling] pane is idle'))).toBe(true)
  expect(await keys(ui, 'Input')).toEqual(['reply:demo:task-01', 'reply:demo:task-02', 'ruling:demo:task-03', 'ruling:demo:task-05'])
})

test('a stale state draws no buttons and no input', async ($, on) => {
  world(on, FIXTURES['stale.json']!)
  const ui = await drawPane($)
  expect(await keys(ui, 'Button')).toEqual([])
  expect(await keys(ui, 'Input')).toEqual([])
})

test('an ask holding a valid reply gets an input that runs the reply verb, text on stdin', async ($, on) => {
  const w = world(on, FIXTURES['fresh.json']!)
  const ui = await drawPane($)
  expect(await keys(ui, 'Input')).toContain('reply:demo:task-02')
  const text = 'yes, use `rg`\nsecond line; "quoted"'
  await ui.input({ key: 'reply:demo:task-02', text })
  expect(w.runs.length).toBe(1)
  expect(w.runs[0]!.argv.slice(0, 4)).toEqual(['channel-send', '--report', '--reply-hold', '/srv/demo/.hw/run-02/pending-reply-1'])
  expect(w.runs[0]!.argv.slice(4, 5)).toEqual(['herdr'])
  expect(w.runs[0]!.argv.slice(-2)).toEqual(['-', '-'])
  expect(w.runs[0]!.stdin).toBe(text)
  expect(w.runs[0]!.argv.join(' ')).not.toContain('second line')
})

test('a challenge is answered with the ruling flavour of the reply verb', async ($, on) => {
  const w = world(on, FIXTURES['fresh.json']!)
  const ui = await drawPane($)
  await ui.input({ key: 'reply:demo:task-01', text: 'the prohibition stands' })
  expect(w.runs[0]!.argv.slice(0, 4)).toEqual(['channel-send', '--ruling', '--reply-hold', '/srv/demo/.hw/run-01/pending-reply-1'])
  expect(w.runs[0]!.stdin).toBe('the prohibition stands')
})

test('an empty reply runs nothing', async ($, on) => {
  const w = world(on, FIXTURES['fresh.json']!)
  const ui = await drawPane($)
  await ui.input({ key: 'reply:demo:task-02', text: '  ' })
  expect(w.runs.length).toBe(0)
})

test('no reply input where the row has no pending_reply', async ($, on) => {
  const base = JSON.parse(FIXTURES['fresh.json']!)
  base.rows.find((r: any) => r.id === 'demo:task-02').ask.pending_reply = null
  world(on, JSON.stringify(base))
  expect(await keys(await drawPane($), 'Input')).not.toContain('reply:demo:task-02')
})

test('every process the mod starts is a verb of the contract', async ($, on) => {
  const w = world(on, FIXTURES['fresh.json']!)
  const ui = await drawPane($)
  await ui.press({ key: 'receipt:demo:task-01' })
  await ui.press({ key: 'done:demo:task-04' })
  await ui.input({ key: 'ruling:demo:task-05', text: 'x' })
  expect(w.runs.length).toBe(3)
  expect(allowed(w)).toBe(true)
})

// A10: the rule band shows hw's verdict and never denies.
const DISPATCH = 'hw demo task-a --brief b.md'
const PREFLIGHT_ARGV = ['hw', 'preflight', '--json', '--', 'demo', 'task-a', '--brief', 'b.md']

for (const level of ['ok', 'warn', 'block'] as const) {
  test(`preflight ${level} is shown and the dispatch still goes on`, async ($, on) => {
    const stdout = FIXTURES[`preflight-${level}.json`]!
    const w = world(on, FIXTURES['fresh.json']!, { reply: { exitCode: 0, stdout, stderr: '' } })
    const r: any = await $.tool.call({ tool: 'Bash', command: DISPATCH })
    expect(r.deny).toBeUndefined()
    expect(w.toolRan).toBe(1)
    expect(w.runs).toEqual([{ argv: PREFLIGHT_ARGV, stdin: undefined }])
    expect(w.timeouts).toEqual([3000])
    const lines = await texts(await drawBand($))
    const rules = lines.filter(t => t.startsWith('rules'))
    if (level === 'ok') expect(rules).toEqual([])
    else expect(rules.length).toBe(1)
    if (level === 'warn') expect(rules[0]).toContain('a suite cut is running')
    if (level === 'block') expect(rules[0]).toContain('hw would refuse this dispatch')
  })
}

for (const [name, reply] of [
  ['a non-zero exit', { exitCode: 2, stdout: '', stderr: 'boom' }],
  ['unparsable output', { exitCode: 0, stdout: 'not json', stderr: '' }],
  ['a wrong schema', { exitCode: 0, stdout: '{"schema":2,"level":"block","rules":[]}', stderr: '' }],
  ['a timeout', 'throw'],
] as [string, Reply][]) {
  test(`preflight with ${name} fails open: unavailable, the dispatch goes on`, async ($, on) => {
    const w = world(on, FIXTURES['fresh.json']!, { reply })
    const r: any = await $.tool.call({ tool: 'Bash', command: DISPATCH })
    expect(r.deny).toBeUndefined()
    expect(w.toolRan).toBe(1)
    const lines = await texts(await drawBand($))
    expect(lines.some(t => t.includes('preflight unavailable'))).toBe(true)
  })
}

test('only an hw dispatch is checked', async ($, on) => {
  const w = world(on, FIXTURES['fresh.json']!)
  for (const command of ['ls -la', 'hw status', 'hw done demo task-a', 'hw demo task-a | tee x', 'hw demo "$(id)"', 'hw --help', 'echo hw demo task-a', 'hw worktrees setup --migrate', 'hw wait-one w1:p1 now']) {
    await $.tool.call({ tool: 'Bash', command })
  }
  expect(w.runs.length).toBe(0)
  expect(w.toolRan).toBe(9)
})

test('a quoted dispatch is checked with its words intact', async ($, on) => {
  const w = world(on, FIXTURES['fresh.json']!, { reply: { exitCode: 0, stdout: FIXTURES['preflight-ok.json']!, stderr: '' } })
  await $.tool.call({ tool: 'Bash', command: `hw demo task-a --why 'two words' --brief "b c.md"` })
  expect(w.runs[0]!.argv).toEqual(['hw', 'preflight', '--json', '--', 'demo', 'task-a', '--why', 'two words', '--brief', 'b c.md'])
})

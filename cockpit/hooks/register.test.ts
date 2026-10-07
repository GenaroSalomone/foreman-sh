import { expect, mock, test } from 'claude-code/testing'
import { FIXTURES } from './fixtures.gen'

const NOW = 1_790_000_000_000
const PATH = '/state/cockpit.json'
const INDEX = JSON.parse(FIXTURES['index.json']!) as {
  cases: { file: string | null; now_ms: number; class: string; rows_rendered: number; note?: string }[]
}

type World = { file: string | null; mtime: number; reads: number; stats: number; invalidates: number }

// The file system, the clock and the environment the mod sees, answered from
// memory beneath it. `file: null` is a missing file.
function world(on: any, file: string | null, now: number) {
  const w: World = { file, mtime: 1, reads: 0, stats: 0, invalidates: 0 }
  mock.env(on, { HW_COCKPIT_STATE: PATH })
  const clock = mock.clock(on, { now })
  on('fs.stat', async (_$: any, e: any) => {
    w.stats += 1
    if (e.path !== PATH || w.file === null) return { deny: 'ENOENT' }
    return { value: { kind: 'file', size: w.file.length, mtimeMs: w.mtime, isLink: false } }
  })
  on('fs.read', async (_$: any, e: any) => {
    w.reads += 1
    if (e.path !== PATH || w.file === null) return { deny: 'ENOENT' }
    return { value: w.file }
  })
  on('ui.invalidate', async () => {
    w.invalidates += 1
    return { value: undefined }
  })
  on('command.register', async () => ({ value: {} }))
  on('session.start', async (_$: any, e: any) => ({ cwd: e.cwd }))
  return { w, clock }
}

const PROPS = { hasSurvey: false, isWorking: false, maxRows: 40, bodyColumns: 120 } as any
const VIEWPORT = { columns: 120, rows: 40 } as any

async function drawPane($: any) {
  return $.ui.mount({ plugin: 'cockpit', surface: 'terminal', component: 'Pane', requestId: 'cockpit', props: { title: 'Cockpit', isFocused: false, bodyColumns: 120, placement: 'inline', scroll: {}, view: {} } as any, viewport: VIEWPORT })
}
async function drawBand($: any) {
  return $.ui.mount({ plugin: 'cockpit', surface: 'terminal', component: 'AbovePrompt', props: { ...PROPS, scroll: {}, view: {} }, viewport: VIEWPORT })
}
async function texts(ui: any): Promise<string[]> {
  return (await ui.findAll({ type: 'Text' })).map((t: any) => t.text as string)
}
async function rows(ui: any): Promise<string[]> {
  const boxes = await ui.findAll({ type: 'Box' })
  return boxes.map((b: any) => b.key as string).filter((k: string | undefined) => k !== undefined && k.startsWith('row:'))
}
const has = (lines: string[], re: RegExp) => lines.some(l => re.test(l))

// A1-A3: every fixture of the contract draws the class and the number of rows
// its own bytes earn, on every surface the pane is raised on.
for (const c of INDEX.cases) {
  const name = `${c.file ?? 'missing file'}${c.note ? ` (${c.note})` : ''} draws ${c.class} with ${c.rows_rendered} rows`
  test(name, async ($, on) => {
    let file: string | null = c.file === null ? null : FIXTURES[c.file]!
    if (c.note?.includes('1048577') && file !== null) file = file.padEnd(1_048_577, ' ')
    const { w } = world(on, file, c.now_ms)
    for (const surface of ['terminal', 'desktop'] as const) {
      const ui = await $.ui.mount({ plugin: 'cockpit', surface, component: 'Pane', requestId: 'cockpit', props: { title: 'Cockpit', isFocused: false, bodyColumns: 120, placement: 'inline', scroll: {}, view: {} } as any, viewport: VIEWPORT })
      const lines = await texts(ui)
      expect((await rows(ui)).length).toBe(c.rows_rendered)
      expect(has(lines, /^STALE \d+ s/)).toBe(c.class === 'stale')
      expect(has(lines, /^NO STATE — writer down/)).toBe(c.class === 'dead')
      expect(has(lines, /^HERDR UNREACHABLE: rows unknown/)).toBe(c.class === 'herdr-down')
      await ui.unmount()
    }
    // An over-size file is refused unread.
    if (c.note?.includes('1048577')) expect(w.reads).toBe(0)
  })
}

test('a dead state never shows the old rows, and says how long ago it was', async ($, on) => {
  world(on, FIXTURES['dead-old.json']!, NOW)
  const ui = await drawPane($)
  const lines = await texts(ui)
  expect(has(lines, /NO STATE — writer down \(last \d+[smhd] ago\)/)).toBe(true)
  expect(has(lines, /demo:/)).toBe(false)
})

test('a missing file is NO STATE, not an empty fresh panel', async ($, on) => {
  world(on, null, NOW)
  const ui = await drawPane($)
  const lines = await texts(ui)
  expect(has(lines, /NO STATE/)).toBe(true)
  expect(has(lines, /no executors/)).toBe(false)
})

// The thresholds are the contract's: 15 s fresh, 60 s stale, past that dead.
for (const [age, cls] of [[15_000, 'fresh'], [15_001, 'stale'], [60_000, 'stale'], [60_001, 'dead'], [-1, 'dead']] as const) {
  test(`state ${age} ms old is ${cls}`, async ($, on) => {
    const base = JSON.parse(FIXTURES['fresh.json']!)
    base.generated_at = NOW - age
    world(on, JSON.stringify(base), NOW)
    const lines = await texts(await drawPane($))
    expect(has(lines, /^STALE/)).toBe(cls === 'stale')
    expect(has(lines, /^NO STATE/)).toBe(cls === 'dead')
  })
}

test('the band says the same, and a stale one hides the top card', async ($, on) => {
  world(on, FIXTURES['stale.json']!, NOW)
  const lines = await texts(await drawBand($))
  expect(has(lines, /cockpit {2}STALE \d+ s/)).toBe(true)
  expect(has(lines, /^▸/)).toBe(false)
})

test('a fresh band shows hw’s totals and the top card, with a button that opens the pane', async ($, on) => {
  world(on, FIXTURES['fresh.json']!, NOW)
  const ui = await drawBand($)
  const lines = await texts(ui)
  expect(has(lines, /7 executors · 1 working · 1 idle · 1 challenge · 1 ask · 1 blocked · 1 report/)).toBe(true)
  expect(has(lines, /^▸ challenge demo:task-01/)).toBe(true)
  expect(await ui.find({ type: 'Button', key: 'open' })).toBeDefined()
})

test('the cut running is on the band', async ($, on) => {
  world(on, FIXTURES['fresh-cut-running.json']!, NOW)
  expect(has(await texts(await drawBand($)), /CUT running/)).toBe(true)
})

test('opencode rows show ctx n/a and every vendor draws a row', async ($, on) => {
  world(on, FIXTURES['vendor-mix.json']!, NOW)
  const lines = await texts(await drawPane($))
  expect(has(lines, /opencode.*ctx n\/a/)).toBe(true)
  expect(has(lines, /codex/)).toBe(true)
  expect(has(lines, /unknown/)).toBe(true)
})

test('a reply input appears only on an ask row that carries pending_reply', async ($, on) => {
  const base = JSON.parse(FIXTURES['fresh.json']!)
  base.rows.find((r: any) => r.ask !== null).ask.pending_reply = 'run/pending-ask.json'
  world(on, JSON.stringify(base), NOW)
  const ui = await drawPane($)
  expect(await ui.find({ type: 'Input', key: 'reply:demo:task-02' })).toBeDefined()
})

test('the state path comes from HW_COCKPIT_WORK and the pane id when no file is named', async ($, on) => {
  const file = '/work/.cockpit/w2:p1.json'
  mock.env(on, { HW_COCKPIT_WORK: '/work', HERDR_PANE_ID: 'w2:p1' })
  mock.clock(on, { now: NOW })
  on('fs.stat', async (_$: any, e: any) =>
    e.path === file ? { value: { kind: 'file', size: FIXTURES['fresh.json']!.length, mtimeMs: 1, isLink: false } } : { deny: 'ENOENT' })
  on('fs.read', async (_$: any, e: any) => (e.path === file ? { value: FIXTURES['fresh.json']! } : { deny: 'ENOENT' }))
  on('ui.invalidate', async () => ({ value: undefined }))
  on('command.register', async () => ({ value: {} }))
  on('session.start', async (_$: any, e: any) => ({ cwd: e.cwd }))
  const lines = await texts(await drawPane($))
  expect(has(lines, /no state path|writer down/i)).toBe(false)
  expect(has(lines, /demo:task-0/)).toBe(true)
})

test('no reply without the field', async ($, on) => {
  const base = JSON.parse(FIXTURES['fresh.json']!)
  for (const r of base.rows) if (r.ask !== null) r.ask.pending_reply = null
  world(on, JSON.stringify(base), NOW)
  const ui = await drawPane($)
  expect(await ui.find({ text: /reply/ })).toBeUndefined()
})

test('a stale state hides the reply even when the field is there', async ($, on) => {
  const base = JSON.parse(FIXTURES['stale.json']!)
  const ask = base.rows.find((r: any) => r.ask !== null)
  ask.ask.pending_reply = 'run/pending-ask.json'
  world(on, JSON.stringify(base), NOW)
  expect(await (await drawPane($)).find({ text: /reply/ })).toBeUndefined()
})

test('40 rows draw in under 50 ms and stay inside the element limits', async ($, on) => {
  world(on, FIXTURES['rows-40.json']!, NOW)
  const t0 = Date.now()
  const ui = await drawPane($)
  const spent = Date.now() - t0
  expect((await rows(ui)).length).toBe(40)
  for (const line of await texts(ui)) expect(line.length).toBeLessThan(1000)
  expect(spent).toBeLessThan(50)
})

const START = { cwd: '/', surface: 'terminal', isInteractive: true } as any

// A5: with the file unchanged the mod stats every second, reads nothing more,
// and redraws only when the 5 s relative-time bucket turns.
test('an unchanged file costs no reads and one redraw per 5 s', async ($, on) => {
  const { w, clock } = world(on, FIXTURES['fresh.json']!, NOW - 5000)
  await $.session.start(START)
  await clock.advance(1000)
  const reads0 = w.reads
  const inv0 = w.invalidates
  for (let i = 0; i < 30; i++) await clock.advance(1000)
  // The writer is not heard from, so after 30 s the state is stale: the class
  // changes once, and there are at most 6 relative-time ticks.
  expect(w.reads).toBe(reads0)
  expect(w.invalidates - inv0).toBeLessThanOrEqual(8)
  expect(w.stats).toBeGreaterThan(20)
})

// A5: a writer rewriting at 4/s for 60 s makes the mod redraw at most once a second.
test('a writer rewriting 4 times a second makes at most 1 redraw a second', async ($, on) => {
  const { w, clock } = world(on, FIXTURES['fresh.json']!, NOW - 1000)
  await $.session.start(START)
  await clock.advance(1000)
  const inv0 = w.invalidates
  for (let i = 0; i < 60; i++) {
    for (let q = 0; q < 4; q++) {
      const s = JSON.parse(w.file!)
      s.generated_at = clock.now()
      s.writer.seq += 1
      w.file = JSON.stringify(s)
      w.mtime += 1
      await clock.advance(250)
    }
  }
  expect(w.invalidates - inv0).toBeLessThanOrEqual(60)
  expect(w.invalidates - inv0).toBeGreaterThan(30)
})

test('the state going stale is noticed without the file changing', async ($, on) => {
  const { w, clock } = world(on, FIXTURES['fresh.json']!, NOW)
  await $.session.start(START)
  await clock.advance(1000)
  const ui = await drawBand($)
  expect(has(await texts(ui), /STALE|NO STATE/)).toBe(false)
  const before = w.invalidates
  await clock.advance(20_000)
  expect(w.invalidates).toBeGreaterThan(before)
  expect(has(await texts(await drawBand($)), /STALE \d+ s/)).toBe(true)
})

test('a herdr-down file that has gone stale does not read as zero executors', async ($, on) => {
  const base = JSON.parse(FIXTURES['herdr-down.json']!)
  base.generated_at = NOW - 30_000
  world(on, JSON.stringify(base), NOW)
  const lines = [...(await texts(await drawPane($))), ...(await texts(await drawBand($)))]
  expect(has(lines, /^STALE/)).toBe(true)
  expect(has(lines, /HERDR UNREACHABLE: counts unknown/)).toBe(true)
  expect(has(lines, /0 executors/)).toBe(false)
})

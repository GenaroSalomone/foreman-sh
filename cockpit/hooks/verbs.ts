// The only argv the mod ever starts: the lines of cockpit/verbs.txt, spelled the
// same way (the contract test compares them). The mod never builds a shell
// string; a verb that is not in this table cannot be called.

export type Verb = { template: string; confirm: boolean; stdin: boolean }

export const VERBS: Record<string, Verb> = {
  done: { template: 'hw done <project> <task>', confirm: true, stdin: false },
  ruling: { template: 'hw ruling <pane> -', confirm: false, stdin: true },
  reply: { template: 'channel-send --report --reply-hold <pending_reply> herdr <pane> - -', confirm: false, stdin: true },
  'challenge-reply': { template: 'channel-send --ruling --reply-hold <pending_reply> herdr <pane> - -', confirm: false, stdin: true },
  receipt: { template: 'hw receipt <project> <task>', confirm: false, stdin: false },
  preflight: { template: 'hw preflight --json -- <argv...>', confirm: false, stdin: false },
}

export type Names = Record<string, string>

// A template's argv: each `<name>` becomes exactly one element taken from
// `names`; `<argv...>` becomes the elements of `argv`. Null when a name has no value.
export function argvOf(verb: string, names: Names, argv: readonly string[] = []): string[] | null {
  const v = Object.prototype.hasOwnProperty.call(VERBS, verb) ? VERBS[verb] : undefined
  if (v === undefined) return null
  const out: string[] = []
  for (const part of v.template.split(' ')) {
    if (part === '<argv...>') {
      out.push(...argv)
    } else if (part.startsWith('<') && part.endsWith('>')) {
      const value = names[part.slice(1, -1)]
      if (typeof value !== 'string' || value === '') return null
      out.push(value)
    } else {
      out.push(part)
    }
  }
  return out
}

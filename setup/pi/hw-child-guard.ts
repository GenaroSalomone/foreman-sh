// THE GUARD FOR A PLUGIN'S CHILDREN. pstack's `agent` tool starts its children with `--extension <plugin>/pi/index.ts`
// and no `-ne` (setup/vendor/plugins/pstack/pi/agents.ts), so the child runs pi with extension discovery ON: the private
// agent directory a plugin dispatch gets (core/t6_pi_shadow.go) is where it looks, and hw writes a shim beside it that
// exports this file. A pi-subagents child is guarded by hw-executor.ts through pi-subagents' required-child-extensions
// API instead; this is the other door, and it registers the same `judge` and the same secret-read redaction, nothing
// else — the default export of hw-executor.ts would claim the child as the root session, register its MCP server and
// run the executor's turn end, none of which belongs to a child.
import { registerGuards } from "./hw-executor.ts";

export default function (pi: any): void {
  registerGuards(pi);
}

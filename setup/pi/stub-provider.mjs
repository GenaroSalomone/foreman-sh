// A local, scripted OpenAI-compatible provider for REAL pi probes: no quota, no network, deterministic.
//
//   import { startStubProvider } from "./stub-provider.mjs";
//   const stub = await startStubProvider({ command: "sleep 20; echo LIVE-DONE", final: "FINISHED" });
//   stub.agentDir      // a PI_CODING_AGENT_DIR whose models.json adds the provider `stub` (model `stub/stub`)
//   stub.requests      // every chat-completions request body pi sent, in order
//   await stub.close();
//
// The script is two turns: while the conversation holds no tool result the model calls pi's `bash` tool with `command`
// (and no timeout, which is the point of the probes that use it); once a tool result is in the conversation it answers
// with the `final` text. Whatever else is in the request (a steering message that arrived mid-turn) is recorded in
// `requests`, which is how a probe proves WHERE a message landed.
import { createServer } from "node:http";
import { mkdirSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { mkdtempSync } from "node:fs";

export async function startStubProvider({ command, final = "FINISHED", port = 0 } = {}) {
  const requests = [];
  const sse = (res, chunks) => {
    res.writeHead(200, { "content-type": "text/event-stream", "cache-control": "no-cache", connection: "keep-alive" });
    for (const c of chunks) res.write(`data: ${typeof c === "string" ? c : JSON.stringify(c)}\n\n`);
    res.end();
  };
  const base = { id: "stub-1", object: "chat.completion.chunk", created: 1, model: "stub" };
  const server = createServer((req, res) => {
    let body = "";
    req.on("data", (d) => { body += String(d); });
    req.on("end", () => {
      let json = {};
      try { json = JSON.parse(body); } catch { /* not JSON */ }
      if (req.url?.endsWith("/chat/completions")) {
        requests.push(json);
        const messages = Array.isArray(json.messages) ? json.messages : [];
        const hasToolResult = messages.some((m) => m.role === "tool");
        if (!hasToolResult && command) {
          sse(res, [
            { ...base, choices: [{ index: 0, delta: { role: "assistant", content: null, tool_calls: [{ index: 0, id: "call_stub_1", type: "function", function: { name: "bash", arguments: JSON.stringify({ command }) } }] }, finish_reason: null }] },
            { ...base, choices: [{ index: 0, delta: {}, finish_reason: "tool_calls" }], usage: { prompt_tokens: 10, completion_tokens: 10, total_tokens: 20 } },
            "[DONE]",
          ]);
        } else {
          sse(res, [
            { ...base, choices: [{ index: 0, delta: { role: "assistant", content: final }, finish_reason: null }] },
            { ...base, choices: [{ index: 0, delta: {}, finish_reason: "stop" }], usage: { prompt_tokens: 10, completion_tokens: 2, total_tokens: 12 } },
            "[DONE]",
          ]);
        }
        return;
      }
      res.writeHead(404); res.end();
    });
  });
  await new Promise((ok) => server.listen(port, "127.0.0.1", ok));
  const url = `http://127.0.0.1:${server.address().port}/v1`;
  const agentDir = mkdtempSync(join(tmpdir(), "stub-pi-agent-"));
  mkdirSync(agentDir, { recursive: true });
  writeFileSync(join(agentDir, "models.json"), JSON.stringify({
    providers: {
      stub: {
        name: "stub", baseUrl: url, api: "openai-completions", apiKey: "stub-key",
        compat: { supportsDeveloperRole: false, supportsReasoningEffort: false, supportsUsageInStreaming: true, maxTokensField: "max_tokens" },
        models: [{ id: "stub", name: "stub", reasoning: false, input: ["text"], contextWindow: 32000, maxTokens: 2000,
                   cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0 } }],
      },
    },
  }, null, 2));
  return { url, agentDir, requests, close: () => new Promise((ok) => server.close(() => ok())) };
}

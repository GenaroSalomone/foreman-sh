// Serves setup/pi/stub-provider.mjs until killed, for a probe that runs pi in ANOTHER process (a herdr pane).
//   node --experimental-strip-types stub-provider-serve.mjs "<bash command the model runs>" <info.json> <requests.jsonl>
// <info.json> gets {url, agentDir} once it listens; <requests.jsonl> gets one line per chat-completions request pi sends.
import { appendFileSync, writeFileSync } from "node:fs";
import { startStubProvider } from "./stub-provider.mjs";

const [command, infoFile, requestsFile] = process.argv.slice(2);
if (!command || !infoFile || !requestsFile) { console.error("usage: stub-provider-serve.mjs <command> <info.json> <requests.jsonl>"); process.exit(2); }
const stub = await startStubProvider({ command, final: "FINISHED" });
writeFileSync(requestsFile, "");
let seen = 0;
setInterval(() => {
  while (seen < stub.requests.length) {
    const r = stub.requests[seen++];
    appendFileSync(requestsFile, JSON.stringify({ at: Date.now(), messages: r.messages }) + "\n");
  }
}, 100);
writeFileSync(infoFile, JSON.stringify({ url: stub.url, agentDir: stub.agentDir }));
process.on("SIGTERM", async () => { await stub.close(); process.exit(0); });

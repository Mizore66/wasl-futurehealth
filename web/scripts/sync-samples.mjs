// Copies docs/api-samples/*.json into web/data/api/ so fixtures mode works without the Go API.
// Also copies docs/api-samples/agent/*.json into web/data/api-agent/ (the separate r1agent10 lane, never merged into data/api).
import { readdirSync, readFileSync, writeFileSync, copyFileSync, mkdirSync, rmSync } from "node:fs";
import { join, dirname } from "node:path";
import { fileURLToPath } from "node:url";

const web = join(dirname(fileURLToPath(import.meta.url)), "..");
const src = join(web, "..", "docs", "api-samples");

function copyJson(from, to) {
  rmSync(to, { recursive: true, force: true });
  mkdirSync(to, { recursive: true });
  const files = readdirSync(from).filter((f) => f.endsWith(".json")).sort();
  for (const f of files) copyFileSync(join(from, f), join(to, f));
  return files.length;
}

console.log(`sync-samples: ${copyJson(src, join(web, "data", "api"))} files -> data/api`);
console.log(`sync-samples: ${copyJson(join(src, "agent"), join(web, "data", "api-agent"))} files -> data/api-agent`);

// The exact agent disclosure, copied (not retyped) from docs/DEMO-RECORDING-GUIDE.md line 43.
const guide = readFileSync(join(web, "..", "docs", "DEMO-RECORDING-GUIDE.md"), "utf8").split("\n");
const quoted = /“([^”]+)”/.exec(guide[42] ?? "");
if (!quoted || !quoted[1].startsWith("Development passed;")) {
  throw new Error("sync-samples: DEMO-RECORDING-GUIDE.md line 43 no longer holds the agent disclosure in curly quotes");
}
writeFileSync(
  join(web, "data", "api-agent", "disclosure.json"),
  JSON.stringify({ source: "docs/DEMO-RECORDING-GUIDE.md", line: 43, text: quoted[1] }, null, 2) + "\n",
);
console.log("sync-samples: disclosure -> data/api-agent/disclosure.json");

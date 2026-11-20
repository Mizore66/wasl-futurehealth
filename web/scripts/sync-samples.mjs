// Copies docs/api-samples/*.json into web/data/api/ so fixtures mode works without the Go API.
import { readdirSync, copyFileSync, mkdirSync, rmSync } from "node:fs";
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

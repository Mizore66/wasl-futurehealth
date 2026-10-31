// Copies docs/api-samples/*.json into web/data/api/ so fixtures mode works without the Go API.
import { readdirSync, copyFileSync, mkdirSync, rmSync } from "node:fs";
import { join, dirname } from "node:path";
import { fileURLToPath } from "node:url";

const web = join(dirname(fileURLToPath(import.meta.url)), "..");
const src = join(web, "..", "docs", "api-samples");
const dst = join(web, "data", "api");

rmSync(dst, { recursive: true, force: true });
mkdirSync(dst, { recursive: true });
const files = readdirSync(src).filter((f) => f.endsWith(".json")).sort();
for (const f of files) copyFileSync(join(src, f), join(dst, f));
console.log(`sync-samples: ${files.length} files -> data/api`);

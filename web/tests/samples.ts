import { readFileSync, readdirSync } from "node:fs";
import path from "node:path";

export const SAMPLES_DIR = path.join(__dirname, "..", "..", "docs", "api-samples");

export function sampleFiles(): string[] {
  return readdirSync(SAMPLES_DIR).filter((f) => f.endsWith(".json")).sort();
}

export function loadSample(file: string): unknown {
  return JSON.parse(readFileSync(path.join(SAMPLES_DIR, file), "utf8"));
}

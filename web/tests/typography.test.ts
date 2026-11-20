import { readFileSync, readdirSync, statSync } from "node:fs";
import { join, relative } from "node:path";
import { describe, expect, it } from "vitest";

/**
 * Typography roles (issue #55). Mono is for actual code, hashes and audit entry bodies only.
 * Case IDs, report IDs, event IDs and registration numbers are ordinary clinical identifiers and use
 * the default sans role. Every `font-mono` in the source must match an entry here. Entries with a
 * `line` pattern must still match something, so the list cannot go stale.
 */
const MONO_ALLOWED: { file: string; line?: RegExp; why: string }[] = [
  { file: "app/globals.css", line: /^\s*--font-mono:/, why: "the mono token itself" },
  // Hashes and signatures
  { file: "components/case/audit-export.tsx", line: /\{h\.value\}/, why: "full export content and comparison hashes" },
  // Code: system names, raw records, versions and machine traces
  { file: "app/app/evidence/page.tsx", line: /<code className="break-all font-mono text-\[13px\]">\{text\}<\/code>/, why: "claim ledger reproduce command" },
  { file: "app/app/evidence/page.tsx", line: /<code className="font-mono">(git clone|make verify)/, why: "verify commands a judge types" },
  { file: "components/review/ifhas-label-composer.tsx", line: /<code className="font-mono[^"]*">\{label\}<\/code>/, why: "the exact IFHAS evidence label the controls compose" },
];

const ROOT = join(import.meta.dirname, "..");
const DIRS = ["app", "components", "lib"];

function sources(dir: string): string[] {
  return readdirSync(dir).flatMap((name) => {
    const path = join(dir, name);
    if (statSync(path).isDirectory()) return sources(path);
    return /\.(tsx?|css)$/.test(name) ? [path] : [];
  });
}

const files = DIRS.flatMap((d) => sources(join(ROOT, d))).map((path) => ({ file: relative(ROOT, path), text: readFileSync(path, "utf8") }));

const allows = (a: (typeof MONO_ALLOWED)[number], h: { file: string; line: string }) => a.file === h.file && (!a.line || a.line.test(h.line));

describe("typography roles", () => {
  const hits = files.flatMap(({ file, text }) =>
    text.split("\n").flatMap((line, i) => (/\bfont-mono\b/.test(line) ? [{ file, n: i + 1, line }] : [])),
  );

  it("uses font-mono only for code, hashes and audit entries on the allow-list", () => {
    const stray = hits.filter((h) => !MONO_ALLOWED.some((a) => allows(a, h)));
    expect(stray.map((h) => `${h.file}:${h.n}: ${h.line.trim()}`)).toEqual([]);
  });

  it("has no stale allow-list entries", () => {
    const stale = MONO_ALLOWED.filter((a) => a.line && !hits.some((h) => allows(a, h)));
    expect(stale.map((a) => `${a.file} (${a.why})`)).toEqual([]);
  });

  it("keeps ordinary numerals as the document default", () => {
    const css = readFileSync(join(ROOT, "app/globals.css"), "utf8");
    expect(css).not.toMatch(/(^|[\s,}])(html|body|:root)\s*\{[^}]*font-variant-numeric\s*:\s*tabular-nums/);
  });
});

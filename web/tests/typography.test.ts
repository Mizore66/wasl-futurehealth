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
  { file: "components/wasl/sealed-chain.tsx", line: /\{n\.hash\}/, why: "full audit hash" },
  { file: "components/review/review-desk.tsx", line: /done\.events\.at\(-1\)\?\.hash/, why: "audit hash of the saved review event" },
  { file: "components/review/review-desk.tsx", line: /\{e\.hash\.slice/, why: "audit hash of each appended review event" },
  { file: "app/app/patients/[id]/page.tsx", line: /\{e\.hash\.slice/, why: "audit entry hash" },
  { file: "components/review/double-review/read-form.tsx", line: /\{result\.recorded\.hash\.slice/, why: "audit hash of the saved double-review read" },
  { file: "components/agent/chain.tsx", line: /shortHash\(e\.hash\)/, why: "agent ledger entry hash" },
  { file: "components/agent/chain.tsx", line: /shortHash\(e\.prev\)/, why: "agent ledger previous hash" },
  { file: "components/case/audit-view.tsx", line: /title=\{value\}/, why: "Hash component (chain, signature and head hashes)" },
  { file: "components/case/audit-view.tsx", line: /prev \{e\.prev\}/, why: "full previous hash" },
  { file: "components/case/audit-view.tsx", line: /hash \{e\.hash\}/, why: "full entry hash" },
  { file: "components/case/audit-view.tsx", line: /^\s*<pre\b/, why: "audit entry body (JSON)" },
  { file: "components/case/audit-view.tsx", line: /\{s\.decision_id\}/, why: "signed decision id, full value" },
  { file: "components/case/audit-view.tsx", line: /stored \{s\.sig\}/, why: "full stored signature" },
  { file: "components/case/audit-view.tsx", line: /recomputed \{s\.recomputed\}/, why: "full recomputed signature" },
  { file: "components/case/audit-view.tsx", line: /^\s*<code className="break-all font-mono">$/, why: "stream head, full value" },
  // Code: system names, raw records, versions and machine traces
  { file: "app/(site)/experts/page.tsx", line: /whitespace-pre-wrap break-all .*font-mono text-\[14px\]/, why: "HL7 v2 message code block (<pre>)" },
  { file: "app/app/review/page.tsx", line: /<code className="font-mono">review_hold<\/code>/, why: "system name of the queue" },
  { file: "components/wasl/tech-term.tsx", line: /<code\b/, why: "system identifier behind a plain term" },
  { file: "components/pipeline/technical.tsx", line: /\{children\}/, why: "technical details disclosure (machine trace)" },
  { file: "components/agent/banner.tsx", line: /\{meta\.run\}/, why: "run id" },
  { file: "components/agent/decision.tsx", line: /\{reads\.join/, why: "raw read codes beside their plain words" },
  { file: "components/agent/investigation.tsx", line: /refWords\(c\.arg\)/, why: "tool-call argument reference" },
  { file: "components/agent/investigation.tsx", line: /\{prov\.(model|prompt_ver|schema_ver)\}/, why: "model, prompt and schema versions" },
  { file: "components/case/sections.tsx", line: /\{h\.(state|reason)\}/, why: "system state and reason codes" },
  { file: "components/case/sections.tsx", line: /\{l\.linker\}/, why: "linker version" },
  { file: "components/case/sections.tsx", line: /^\s*<p className="m-0 mt-1 break-all font-mono">$/, why: "raw link edge references in linker details" },
  { file: "components/case/sections.tsx", line: /JSON\.stringify\(o\.raw\)/, why: "raw message record" },
  { file: "components/case/sections.tsx", line: /\{s\.item\}/, why: "raw reviewer record item" },
  { file: "components/case/demo-cases.tsx", line: /AHS-sr-1079<\/code>/, why: "full system id format, as typed in an address" },
  // Run metadata (run id and revision values); this file is edited by a parallel change, so it is allowed whole.
  { file: "components/shell/run-meta.tsx", why: "run id and code revisions" },
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

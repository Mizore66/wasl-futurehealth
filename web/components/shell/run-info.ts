import { apiMode, getDashboard } from "@/lib/api/source";
import { runStripCopy } from "./run-meta-copy";

const SOURCE = {
  live: "Live Wasl API",
  fixtures: "Published snapshot of the API",
} as const;

/** What the run notice in every page header says, read once per request by the app layout. */
export type RunInfo =
  | { ok: true; data: string; when: string; loadedAt: string; note: string; rows: [string, string][] }
  | { ok: false; source: string };

export async function runInfo(): Promise<RunInfo> {
  const mode = apiMode();
  const source = SOURCE[mode];
  try {
    const { meta } = await getDashboard();
    const copy = runStripCopy(meta, mode);
    return {
      ok: true,
      ...copy,
      loadedAt: meta.loaded_at,
      note: meta.note,
      rows: [
        ["Data source", source],
        ["Run", meta.run],
        ["Policy", meta.policy],
        ["Reader model", meta.reader_model],
        ["API", meta.api],
      ],
    };
  } catch {
    return { ok: false, source };
  }
}

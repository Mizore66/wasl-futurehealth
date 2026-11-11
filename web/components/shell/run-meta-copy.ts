import { formatUtcInstant } from "@/lib/domain/day";

type StripMeta = { data_mode: string; loaded_at: string };

/**
 * The always-visible words of the run strip (issue #50): how real the data is and when it was taken, in plain language.
 * Run, policy, reader model and API version stay in the strip's expanded technical panel.
 */
export function runStripCopy(meta: StripMeta, mode: "live" | "fixtures"): { data: string; when: string } {
  // The API only emits "simulated" today; any other value is shown as-is rather than relabelled.
  const data = meta.data_mode === "simulated" ? "Simulated data" : `Data mode: ${meta.data_mode}`;
  const at = formatUtcInstant(meta.loaded_at);
  return { data, when: mode === "live" ? `Run loaded ${at}` : `Snapshot ${at}` };
}

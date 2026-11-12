/*
 * Ledger `at` values are Go RFC3339Nano strings. Go drops trailing zeros from the fraction, and drops the fraction
 * entirely at zero nanoseconds, so text order is not time order: "…:05Z" sorts after "…:05.1Z" because "Z" > ".".
 * Compare parsed instants instead, at full nanosecond precision (Date.parse keeps only milliseconds).
 */

const RFC3339 = /^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2})(?:\.(\d{1,9}))?(Z|[+-]\d{2}:\d{2})$/i;

/** `[epoch ms, nanoseconds within that ms]`, or null when `at` is not an RFC3339 timestamp. */
export function instant(at: string): [number, number] | null {
  const m = RFC3339.exec(at);
  if (!m) return null;
  const ms = Date.parse(`${m[1]}${m[3].toUpperCase()}`);
  if (Number.isNaN(ms)) return null;
  const frac = (m[2] ?? "").padEnd(9, "0");
  return [ms + Number(frac.slice(0, 3)), Number(frac.slice(3))];
}

/**
 * Orders two ledger timestamps by the instant they name. An unparseable timestamp sorts after every parseable one
 * (two unparseable ones keep text order), so a malformed value never jumps ahead of real events.
 */
export function compareInstants(a: string, b: string): number {
  const x = instant(a);
  const y = instant(b);
  if (!x || !y) return x ? -1 : y ? 1 : a < b ? -1 : a > b ? 1 : 0;
  return x[0] - y[0] || x[1] - y[1];
}

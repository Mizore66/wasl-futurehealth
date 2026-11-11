/*
 * Blinding guard for the auditor export. The export is built by the Go API with the shared auditor credential and
 * lists every protocol event, including a first read that its second reader must not see yet. The guard inspects the
 * bytes actually about to be streamed, so there is no check-then-fetch race: an export that contains r1 without r2 for
 * any (item, question) is refused; one that predates r1, or already holds r2/r3, is released unchanged.
 */
type Json = Record<string, unknown>;
const obj = (v: unknown): Json | undefined => (v && typeof v === "object" && !Array.isArray(v) ? (v as Json) : undefined);

/** True when the document shows an initial read that is still concealed, or cannot be read with confidence. */
export function exportConcealsInitialRead(doc: unknown): boolean {
  const root = obj(doc);
  if (!root) return true;
  const lists = [obj(root.audit)?.history, obj(root.detail)?.api_reviews];
  if (!lists.every(Array.isArray)) return true;
  const slots = new Map<string, Set<string>>();
  for (const list of lists as unknown[][]) {
    for (const raw of list) {
      const e = obj(raw);
      if (!e || typeof e.protocol !== "string") continue;
      const key = `${String(e.item ?? "")}\u0000${String(e.question ?? "")}`;
      const set = slots.get(key) ?? new Set<string>();
      set.add(String(e.read ?? ""));
      slots.set(key, set);
    }
  }
  return [...slots.values()].some((s) => s.has("r1") && !s.has("r2") && !s.has("r3"));
}

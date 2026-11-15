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

/*
 * Model-output guard for the browser audit page and the export. The auditor representations carry the image reader's
 * verdict (`image_reader`) in the evidence block of the chain, in every item's chain and in `detail.evidence`. A model
 * verdict may reach a viewer only after that viewer can see a first read of every evidence question it could belong to.
 * The browser has no per-photo or per-source association, so a representation that carries a verdict is released only
 * when EVERY evidence question the viewer knows of has a visible read that is still live; a read on another question,
 * another item or a frozen (superseded) state never authorises it, and nothing known means withheld.
 */
const MODEL_KEY = "image_reader";

/** True when the value holds an `image_reader` key anywhere, however deeply nested. */
export function containsModelOutput(value: unknown, depth = 0): boolean {
  if (depth > 64) return true;
  if (Array.isArray(value)) return value.some((v) => containsModelOutput(v, depth + 1));
  const o = obj(value);
  if (!o) return false;
  return Object.keys(o).some((k) => k === MODEL_KEY || containsModelOutput(o[k], depth + 1));
}

const visibleRead = (r: unknown) => {
  const e = obj(r);
  return !!e && e.submitted === true && typeof e.decision === "string" && e.concealed !== true;
};

/** The viewer's own case detail (its review items and per-question states), as the Go API projected it for that viewer. */
export function viewerMayReadModel(detail: unknown): boolean {
  const d = obj(detail);
  if (!d) return false;
  const items = Array.isArray(d.review_items) ? d.review_items : [];
  const states = (Array.isArray(d.api_review_states) ? d.api_review_states : []).map(obj);
  if (states.some((s) => !s)) return false;
  const need = new Set<string>();
  for (const raw of items) {
    const it = obj(raw);
    if (!it || typeof it.item !== "string") return false;
    if ((Array.isArray(it.kinds) && it.kinds.includes("evidence")) || containsModelOutput(it.chain)) need.add(it.item);
  }
  for (const s of states) if (s && s.question === "evidence" && typeof s.item === "string") need.add(s.item);
  if (need.size === 0) return false;
  return [...need].every((item) => {
    const st = states.find((s) => s && s.item === item && s.question === "evidence");
    return !!st && st.frozen !== true && Array.isArray(st.reads) && st.reads.some(visibleRead);
  });
}

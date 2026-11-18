/**
 * Maps `items` with at most `limit` calls in flight, preserving order. Each item settles on its own: a rejection becomes
 * `{ ok: false }` for that item only, so one slow or failed read never fails the rest.
 */
export async function mapSettledBounded<T, R>(
  items: readonly T[],
  limit: number,
  fn: (item: T, index: number) => Promise<R>,
): Promise<({ ok: true; value: R } | { ok: false; error: unknown })[]> {
  const out: ({ ok: true; value: R } | { ok: false; error: unknown })[] = new Array(items.length);
  let next = 0;
  const worker = async () => {
    while (next < items.length) {
      const i = next++;
      try {
        out[i] = { ok: true, value: await fn(items[i], i) };
      } catch (error) {
        out[i] = { ok: false, error };
      }
    }
  };
  await Promise.all(Array.from({ length: Math.max(1, Math.min(limit, items.length)) }, worker));
  return out;
}

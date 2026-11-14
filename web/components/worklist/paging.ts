import { PAGE_SIZE } from "./filters";

export type PagerControl = "first" | "prev" | "next" | "last";
export type Paging<T> = { rows: T[]; page: number; pages: number; total: number; from: number; to: number; size: number };

function checkSize(size: number) {
  if (!Number.isInteger(size) || size < 1) throw new RangeError("Page size must be a positive integer");
}

/** Slice only: callers retain their complete filtered order for counts and decisions. */
export function paginate<T>(rows: readonly T[], requested: number, size = PAGE_SIZE): Paging<T> {
  checkSize(size);
  const total = rows.length;
  const pages = Math.max(1, Math.ceil(total / size));
  const page = Number.isInteger(requested) && requested > 0 ? Math.min(requested, pages) : 1;
  const start = (page - 1) * size;
  return { rows: rows.slice(start, start + size), page, pages, total, from: total ? start + 1 : 0, to: Math.min(start + size, total), size };
}

export function pageOfId<T extends { id: string }>(rows: readonly T[], id: string | null | undefined, size = PAGE_SIZE): number | null {
  checkSize(size);
  if (id == null) return null;
  const index = rows.findIndex((row) => row.id === id);
  return index < 0 ? null : Math.floor(index / size) + 1;
}

export function pageWith<T extends { id: string }>(rows: readonly T[], openId: string | null, fallbackPage: number, size = PAGE_SIZE): number {
  return pageOfId(rows, openId, size) ?? paginate(rows, fallbackPage, size).page;
}

export function rangeText(p: Paging<unknown>, complete = true): string {
  return `Showing ${p.from}–${p.to}${complete ? ` of ${p.total}` : ""}`;
}

export function pageStatusText(p: Paging<unknown>, complete = true): string {
  return `Page ${p.page}${complete ? ` of ${p.pages}` : ""}, showing ${p.from} to ${p.to}${complete ? ` of ${p.total}` : ""}.`;
}

export function pagerTargets(p: Paging<unknown>): Record<PagerControl, number | null> {
  return { first: p.page > 1 ? 1 : null, prev: p.page > 1 ? p.page - 1 : null,
    next: p.page < p.pages ? p.page + 1 : null, last: p.page < p.pages ? p.pages : null };
}

/** Keep focus on the activated control, or its surviving opposite at an end. */
export function pagerFocusAfter(pressed: PagerControl, to: Paging<unknown>): PagerControl | null {
  const targets = pagerTargets(to);
  if (targets[pressed] !== null) return pressed;
  return (["prev", "next", "first", "last"] as const).find((key) => targets[key] !== null) ?? null;
}

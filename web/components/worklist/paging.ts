import { PAGE_SIZE } from "./filters";

/** A chevron, or a numbered page button. */
export type PagerControl = "prev" | "next" | number;
export type PageSlot = number | "gap";
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

export function pagerTargets(p: Paging<unknown>): Record<"prev" | "next", number | null> {
  return { prev: p.page > 1 ? p.page - 1 : null, next: p.page < p.pages ? p.page + 1 : null };
}

/**
 * The numbered pages to show: all of them up to five, otherwise always five slots with the first and last page
 * reachable, so the pager keeps one width while paging through a narrow column.
 */
export function pageWindow(page: number, pages: number): PageSlot[] {
  if (pages <= 5) return Array.from({ length: pages }, (_, i) => i + 1);
  if (page <= 3) return [1, 2, 3, "gap", pages];
  if (page >= pages - 2) return [1, "gap", pages - 2, pages - 1, pages];
  return [1, "gap", page, "gap", pages];
}

/** Keep focus on the activated control: a page number becomes the current page, a chevron disabled at an end hands over to the other. */
export function pagerFocusAfter(pressed: PagerControl, to: Paging<unknown>): PagerControl | null {
  if (typeof pressed === "number") return to.pages > 1 ? to.page : null;
  const targets = pagerTargets(to);
  const other = pressed === "prev" ? "next" : "prev";
  return targets[pressed] !== null ? pressed : targets[other] !== null ? other : null;
}

/**
 * The Next.js router stand-in for the referral inbox's render tests. Kept apart from the harness so a `vi.mock` factory
 * can import it without importing the inbox (which imports next/navigation, the module being mocked).
 */
import { useMemo, useSyncExternalStore } from "react";
import { vi } from "vitest";

export const push = vi.fn();
export const refresh = vi.fn();

const listeners = new Set<() => void>();
let patched = false;
export function patchHistory() {
  if (patched) return;
  patched = true;
  for (const k of ["pushState", "replaceState"] as const) {
    const original = history[k].bind(history);
    history[k] = (...args: Parameters<History["pushState"]>) => {
      original(...args);
      listeners.forEach((l) => l());
    };
  }
}
const subscribe = (l: () => void) => {
  listeners.add(l);
  return () => listeners.delete(l);
};

export const navigationMock = {
  useRouter: () => ({ push, refresh, replace: push }),
  useSearchParams: () => {
    const search = useSyncExternalStore(subscribe, () => location.search, () => "");
    return useMemo(() => new URLSearchParams(search), [search]);
  },
};


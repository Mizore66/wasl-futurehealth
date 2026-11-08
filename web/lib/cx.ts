/**
 * Plain class join for client modules. It does no conflict resolution, so use it only where the classes cannot clash
 * (no caller overrides a base utility). It lives apart from `cn` because importing `lib/utils` ships the whole
 * tailwind-merge table (about 160 kB) to the browser.
 */
export function cx(...parts: (string | false | null | undefined)[]): string {
  return parts.filter(Boolean).join(" ");
}

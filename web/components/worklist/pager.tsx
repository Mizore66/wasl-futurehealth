import Link from "next/link";
import { pagerTargets, rangeText, type PagerControl, type Paging } from "./paging";

const CONTROLS: [PagerControl, string][] = [["first", "First"], ["prev", "Previous"], ["next", "Next"], ["last", "Last"]];
const controlClass = "inline-flex min-h-11 items-center rounded-chip border border-control px-3 text-body text-ink no-underline hover:bg-active";
type Props = { paging: Paging<unknown>; label: string; testId: string };

export function Pager({ paging, label, testId, onPage }: Props & { onPage: (page: number, via: PagerControl) => void }) {
  if (paging.pages === 1) return null;
  const targets = pagerTargets(paging);
  return (
    <nav aria-label={label} className="flex flex-wrap items-center gap-2 px-4 py-3">
      <p data-testid={testId} className="m-0 w-full text-small text-muted">{rangeText(paging)}</p>
      {CONTROLS.map(([key, name]) => targets[key] === null ? null : (
        <button key={key} type="button" data-focus={`pager-${key}`} className={`${controlClass} cursor-pointer bg-transparent`} onClick={() => onPage(targets[key]!, key)}>{name}</button>
      ))}
    </nav>
  );
}

export function PagerLinks({ paging, label, testId, hrefFor, complete = true }: Props & { hrefFor: (page: number) => string; complete?: boolean }) {
  if (paging.pages === 1) return null;
  const targets = pagerTargets(paging);
  return (
    <nav aria-label={label} className="mt-4 flex flex-wrap items-center gap-2">
      <p data-testid={testId} className="m-0 w-full text-small text-muted">{rangeText(paging, complete)}</p>
      {CONTROLS.map(([key, name]) => targets[key] === null || (!complete && key === "last") ? null : (
        <Link key={key} href={hrefFor(targets[key]!)} rel={key === "prev" ? "prev" : key === "next" ? "next" : undefined} className={controlClass}>{name}</Link>
      ))}
    </nav>
  );
}

"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { ChevronRight } from "lucide-react";
import { Button } from "@/components/ui/button";
import { dayLabel } from "@/components/case/audit-model";
import { formatDayYear } from "@/lib/domain/day";
import { shortId } from "@/lib/domain/ids";
import { cn } from "@/lib/utils";
import { heldCount, TodayRecords } from "./today-records";

/*
 * The open case's supporting file on Today: its source records and source events, read lazily for the selected case
 * only (GET /app/today/records), in both modes. A failed read says why and offers Retry; a case without a detail record
 * in this demo says so instead of showing an empty list.
 */

const without = (reads: Record<string, RecordsRead>, id: string) => {
  const next = { ...reads };
  delete next[id];
  return next;
};

export type RecordsRead = { ok: true; records: TodayRecords } | { ok: false; error: string; missing: boolean };

/** One read per case while the board is open: a successful read is kept, a failed one is read again on selection or Retry. */
export function useTodayRecords(id: string | undefined) {
  const [reads, setReads] = useState<Record<string, RecordsRead>>({});
  const [attempt, setAttempt] = useState(0);
  const [readFor, setReadFor] = useState<string | undefined>(undefined);
  const kept = useRef(new Set<string>());
  // Selecting a case whose last read failed (but is not simply missing) reads it again; show "Reading…" meanwhile.
  if (id !== readFor) {
    setReadFor(id);
    const last = id ? reads[id] : undefined;
    if (id && last && !last.ok && !last.missing) setReads((prev) => without(prev, id));
  }
  useEffect(() => {
    if (!id || kept.current.has(id)) return;
    const ac = new AbortController();
    fetch(`/app/today/records?id=${encodeURIComponent(id)}`, { signal: ac.signal, cache: "no-store" })
      .then(async (r): Promise<RecordsRead> => {
        const body: unknown = await r.json().catch(() => null);
        if (r.ok) {
          const parsed = TodayRecords.safeParse(body);
          return parsed.success ? { ok: true, records: parsed.data } : { ok: false, error: "the records could not be read", missing: false };
        }
        const b = body as { error?: unknown; missing?: unknown } | null;
        return { ok: false, error: typeof b?.error === "string" ? b.error : "the system returned an error", missing: r.status === 404 && b?.missing === true };
      })
      .catch((): RecordsRead | null => (ac.signal.aborted ? null : { ok: false, error: "no response. Check your connection", missing: false }))
      .then((read) => {
        if (!read || ac.signal.aborted) return;
        if (read.ok || read.missing) kept.current.add(id);
        setReads((prev) => ({ ...prev, [id]: read }));
      });
    return () => ac.abort();
  }, [id, attempt]);
  const retry = useCallback(() => {
    if (!id) return;
    setReads((prev) => without(prev, id));
    setAttempt((n) => n + 1);
  }, [id]);
  return { read: id ? reads[id] : undefined, retry };
}

/** "18 Jul 2026" from an event's ISO day; the Audit page's undated words otherwise. */
export const eventDay = (date: string | null) => (date ? dayLabel(date) : "Date unrecorded");

export function heldOf(read: RecordsRead | undefined): number {
  return read?.ok ? heldCount(read.records) : 0;
}

export function RecordsPanel({ read, onRetry }: { read: RecordsRead | undefined; onRetry: () => void }) {
  if (!read) {
    return (
      <p role="status" className="m-0 mt-4 text-muted" data-testid="records-loading">
        Loading records…
      </p>
    );
  }
  if (!read.ok) {
    return read.missing ? (
      <p role="status" className="m-0 mt-4 text-muted" data-testid="records-missing">
        No records to show: {read.error}.
      </p>
    ) : (
      <div role="status" data-testid="records-failed" className="mt-3 flex min-h-11 flex-wrap items-center gap-x-gap">
        <span className="text-warning">Could not load the records: {read.error}</span>
        <Button variant="quiet" size="touch" onClick={onRetry}>
          Retry
        </Button>
      </div>
    );
  }
  const { sources, caseHeld } = read.records;
  if (sources.length === 0 && caseHeld.length === 0) {
    return (
      <p className="m-0 mt-4 text-muted" data-testid="records-empty">
        No records on this case yet.
      </p>
    );
  }
  return (
    <>
      {caseHeld.length > 0 && (
        <ul aria-label="Held on this case" className="m-0 mt-3 list-none p-0">
          {caseHeld.map((why) => (
            <li key={why} className="flex flex-wrap items-baseline gap-2 py-1.5 text-small">
              <HeldMark />
              <span className="text-ink-soft">{why}</span>
            </li>
          ))}
        </ul>
      )}
      <ul aria-label="Source records" className="m-0 mt-2 grid list-none grid-cols-1 p-0 lg:grid-cols-2 lg:gap-x-[clamp(15px,3vw,42px)]">
        {sources.map((s) => (
          <li key={s.id} data-testid="source-record" data-held={s.held.length > 0 || undefined} className="min-w-0 border-t border-rule first:border-t-0 lg:[&:nth-child(2)]:border-t-0">
            <details className="group">
              <summary className="grid min-h-11 cursor-pointer list-none grid-cols-[minmax(0,1fr)_auto] items-baseline gap-x-2 gap-y-0.5 py-2.5 [&::-webkit-details-marker]:hidden">
                <span className="flex min-w-0 items-baseline gap-1.5 font-medium text-ink">
                  <span className="min-w-0 break-words">{s.title}</span>
                  <ChevronRight aria-hidden className="size-3.5 flex-none self-center text-jade-600 transition-transform group-open:rotate-90" />
                </span>
                {s.held.length > 0 ? <HeldMark /> : <span className="text-small text-muted">{s.type}</span>}
                <span className="col-span-full text-small text-muted">
                  {s.provider} · {s.day === null ? "date not recorded" : formatDayYear(s.day)}
                </span>
                {s.held.map((why) => (
                  <span key={why} className="col-span-full text-small text-ink-soft" data-testid="held-reason">
                    {why}
                  </span>
                ))}
              </summary>
              <p className="m-0 mb-2.5 border-l-2 border-jade-200 pl-2.5 text-small text-ink-soft">
                {s.type} record <span dir="ltr" className="break-all">{shortId(s.id)}</span> from {s.provider}.
              </p>
            </details>
          </li>
        ))}
      </ul>
    </>
  );
}

function HeldMark({ className }: { className?: string }) {
  return <span className={cn("rounded-chip bg-ochre-wash px-1.5 py-px text-small font-medium text-ochre-ink", className)}>Held</span>;
}

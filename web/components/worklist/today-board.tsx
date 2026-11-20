"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { AnimatePresence, motion, useReducedMotion } from "motion/react";
import { ArrowRight, Check, ChevronDown, CircleAlert, Folder, IdCard, Phone, Search } from "lucide-react";
import { Button } from "@/components/ui/button";
import { isTodayQueue, publishTodayCounts, type TodayQueue } from "@/components/shell/today-counts";
import { formatDay } from "@/lib/domain/day";
import { caseHref } from "@/lib/domain/ids";
import { cn } from "@/lib/utils";
import { callbackChoice, fromNow } from "./callback";
import { byUrgency, clockWords } from "./days";
import { LedgerInvalidBanner, listLedgerInvalid } from "./ledger-banner";
import type { PersistedLedger, TrailEntry } from "./ledger-trail";
import type { View, WorkItem } from "./model";
import { BOOKING_NEEDED, DECLINED_CANCELLED_NEXT, DECLINED_RESERVED_NEXT, DECLINED_STATUS, HANDOFF_OWNER, noAnswerStatus, obligationAfterCommunication, outcomeStatus, ownerLabel, workItem } from "./model";
import { communicationWords, outcomeWords, reconciliationWords, retrievalOutcomeWords } from "@/components/case/words";
import { CommunicationForm } from "./communication-form";
import { ElsewhereForm } from "./elsewhere-form";
import { BookingControls } from "@/components/case/booking-controls";
import type { BookingProjection } from "@/app/app/patients/[id]/booking-actions";
import { CONSENT_WORDS, contactWords, taskSentence, taskTitle } from "./today-task";
import { eventDay, heldOf, RecordsPanel, useTodayRecords, type RecordsRead } from "./today-file";
import { TodayRecords } from "./today-records";
import { recordAction } from "@/app/app/today/actions";
import { ApiOverlay, Booking, CasePatient, CaseSummary, RETRIEVAL_OUTCOMES, type CaseAction } from "@/lib/api/schema";
import { z } from "zod";
import { Pager } from "./pager";
import { paginate, pageWith, pageStatusText, pagerFocusAfter, type PagerControl } from "./paging";

/*
 * Live API (`live`): every outcome, snooze and owner change is first appended to the API's coordinator ledger
 * (POST /api/cases/{id}/actions) and shown only after a 201. The ledger is append-only, so there is no Undo once saved;
 * the riskiest one-click writes (Snooze, Hand to …, Declined) wait `undoMs` before they are sent, with an Undo that
 * cancels the POST outright.
 * Fixtures mode: the same buttons keep session state in the browser, with Undo after the fact.
 *
 * Live API, after a reload: the board position comes from the persisted overlay in the case list (model.ts workItem),
 * and the open patient's trail from that ONE case's detail overlay (GET /app/today/ledger), never from session state.
 * If the API reports its ledger as not trustworthy (ledger_status: invalid), the board and trail say so and writes are disabled.
 */

/** One trail entry. `who` is the actor the API recorded (live) or the browser that holds it (fixtures): never a guessed name. */
type Contact = TrailEntry;
type ContactProjection = Pick<WorkItem, "contactBlocked" | "textsAllowed" | "textsStopped" | "consent">;
type Patch = Partial<ContactProjection> & { view?: View; status?: string; next?: string; snoozed?: boolean; noAnswer?: number; awaitingNext?: boolean; owner?: string; head?: string; reserved?: boolean; cancelled?: boolean; retrieval?: WorkItem["retrieval"]; communicationStep?: WorkItem["communicationStep"]; log: Contact[] };
type Stage = "reached" | "callback" | "elsewhere";
type Logged = { id: string; head: string; move: string; undo: Pick<State, "over" | "stage" | "last"> };
type State = {
  tab: View;
  /** The last `?queue=` this board saw: a new one from outside (sidebar, history) moves `tab`. */
  seen: View;
  page: number;
  /** Phone only: the selected row's inline case is folded away (from lg up the case always shows beside the queue). */
  collapsed: boolean;
  /** The case-details disclosure (person-card icon). */
  facts: boolean;
  /** Phone only: the Activity and Records panel under the inline case is open. */
  hist: boolean;
  /** The supporting-information tab under the open case. */
  file: "activity" | "records";
  /** The activity event whose note is open. */
  note: string | null;
  sel: string | null;
  q: string;
  calling: string | null;
  stage: Stage | null;
  /** Wall-clock ms when "Call back on…" or "Done elsewhere…" was opened: the one instant its dates are computed from. */
  callbackFrom: number;
  last: Logged | null;
  over: Record<string, Patch>;
};

type Row = WorkItem & { snoozed?: boolean; noAnswer?: number; awaitingNext?: boolean; log: Contact[] };
/** A live write held back for the Undo window: nothing has been sent yet. */
type Pending = { id: string; name: string; what: string; patch: Omit<Patch, "log">; head: string; move: string; action: CaseAction };

/** Live API: how long Snooze, Hand to … and Declined wait, with Undo, before the POST is sent. */
export const LIVE_UNDO_MS = 5000;

/** One case's ledger read on Today: the persisted trail, or why it could not be read (the route's own words). */
const LedgerResponse = z.object({
  status: z.enum(["ok", "invalid"]), detail: z.string(), applied: z.number().nullable(),
  entries: z.array(z.object({ what: z.string(), when: z.string(), who: z.string(), hash: z.string().optional() })),
  patient: CasePatient.optional(),
  bookingPanel: z.object({ booking: Booking, overlay: ApiOverlay, case: CaseSummary }).nullable().optional(),
  /** The case's source records and events (optional: an older web server's ledger read has none). */
  records: TodayRecords.optional(),
});
type LedgerSnapshot = z.infer<typeof LedgerResponse>;
type LedgerRead = LedgerSnapshot | { error: string };
const isLedger = (l: LedgerRead | undefined): l is LedgerSnapshot => !!l && !("error" in l);
const contactProjection = (item: WorkItem): ContactProjection => ({
  contactBlocked: item.contactBlocked, textsAllowed: item.textsAllowed, textsStopped: item.textsStopped, consent: item.consent,
});
/** Keep the row's booking workflow and action-driving flags on the same projection as its write head. */
const bookingRowProjection = (item: WorkItem) => ({
  view: item.view, status: item.status, next: item.next, head: item.head,
  snoozed: item.snoozed, noAnswer: item.noAnswer, awaitingNext: item.awaitingNext,
  reserved: item.reserved, cancelled: item.cancelled, retrieval: item.retrieval, communicationStep: item.communicationStep,
});
const withoutRead = (reads: Record<string, LedgerRead>, id: string) => {
  const next = { ...reads };
  delete next[id];
  return next;
};
/** A ledger read the route answered and refused: its message is shown as given. */
class ReadError extends Error {}

const TABS: [View, string][] = [
  ["call", "To call"],
  ["waiting", "Waiting"],
  ["none", "No owner"],
];

const OUTCOME_CHIP = { voicemail: "Voicemail", wrong_number: "Wrong number" } as const;
const RETRIEVAL_CHIP = { not_found: "Record not found", not_matching: "Another patient or procedure", non_closing: "Report does not close the loop" } as const;

const clock = () => new Date().toLocaleTimeString("en-GB", { hour: "2-digit", minute: "2-digit" });

/** Fixtures mode: nothing reaches a ledger, so the trail names the browser that holds the entry, not a person. */
const SESSION_WHO = "This browser";
/** Fixtures mode only: the demo persona. A live save shows the owner the API recorded, the verified coordinator. */
const SESSION_OWNER = "Hessa Al Ali";

const REFUSED: Record<number, string> = {
  403: "the coordinator role may not do this",
  404: "the API does not know this case",
  409: "someone changed this case first; reload to see it",
  500: "the case ledger failed validation, so the API refuses new actions",
  503: "no live API",
};

// Revision access is confined to effects and event handlers; render only constructs the chip callbacks.
function useReadFence() {
  const revisions = useRef<Record<string, number>>({});
  const revisionFor = useCallback((id: string) => revisions.current[id] ?? 0, []);
  const supersede = useCallback((id: string) => { revisions.current[id] = (revisions.current[id] ?? 0) + 1; }, []);
  return { revisionFor, supersede };
}

/**
 * One write held back for an Undo window. `take` cancels the timer and hands the job back (Undo drops it, Save now sends
 * it); unmounting inside the window sends it rather than losing what the user chose. Ref access stays in callbacks.
 */
function useHeldWrite<T>() {
  const held = useRef<{ job: T; timer: ReturnType<typeof setTimeout> } | null>(null);
  const sender = useRef<(job: T) => void>(() => {});
  const setSender = useCallback((fn: (job: T) => void) => { sender.current = fn; }, []);
  const send = useCallback((job: T) => sender.current(job), []);
  const isHeld = useCallback(() => held.current !== null, []);
  const hold = useCallback((job: T, ms: number, due: () => void) => { held.current = { job, timer: setTimeout(due, ms) }; }, []);
  const take = useCallback(() => {
    const h = held.current;
    if (!h) return null;
    clearTimeout(h.timer);
    held.current = null;
    return h.job;
  }, []);
  useEffect(() => () => {
    const job = take();
    if (job) send(job);
  }, [take, send]);
  return { setSender, send, isHeld, hold, take };
}

export function TodayBoard({
  items,
  now,
  head,
  aside,
  live = false,
  scope = "Al Mushrif Healthcare Centre",
  undoMs = LIVE_UNDO_MS,
  queue = "call",
}: {
  items: WorkItem[];
  now: number;
  /** The page header, drawn as the sheet's top band. */
  head?: React.ReactNode;
  aside: React.ReactNode;
  live?: boolean;
  scope?: string;
  /** Live API: the Undo window before a held-back write is sent; 0 sends at once. */
  undoMs?: number;
  /** The queue the server rendered from `?queue=`; the URL's current value wins once mounted. */
  queue?: TodayQueue;
}) {
  const params = useSearchParams();
  const fromUrl = params?.get("queue");
  /* The sidebar's queue links and this board's own moves both change `?queue=`; the URL is the one source. */
  const urlTab: View = isTodayQueue(fromUrl) ? fromUrl : queue;
  const [s, setS] = useState<State>({ tab: urlTab, seen: urlTab, page: 1, collapsed: false, facts: false, hist: false, file: "activity", note: null, sel: null, q: "", calling: null, stage: null, last: null, over: {}, callbackFrom: 0 });
  const [busy, setBusy] = useState(false);
  const [pending, setPending] = useState<Pending | null>(null);
  const heldWrite = useHeldWrite<Pending>();
  const [refusal, setRefusal] = useState<{ id: string; text: string } | null>(null);
  /**
   * Live API: each case's last ledger read. Every selection of a case reads it again (however it was selected: row
   * click, keyboard, tab change or search) and so does Retry; a successful read is shown until the new one lands, a
   * failed one is kept only to say why and is dropped as soon as the next read starts. A failure never retries by itself.
   */
  const [persisted, setPersisted] = useState<Record<string, LedgerRead>>({});
  /** Bumped by Retry so the same selected case is read again. */
  const [attempt, setAttempt] = useState(0);
  // A completed local write supersedes any read begun before it, even if fetch ignores abort.
  const { revisionFor, supersede } = useReadFence();
  const supersedeRead = (id: string) => {
    supersede(id);
    setAttempt((n) => n + 1);
  };
  /** The case the current read is for: when the selection moves, a read starts for the new one. */
  const [readFor, setReadFor] = useState<string | undefined>(undefined);
  /**
   * `ledger_status` of the most recent successful detail read, or null before the first. The API's flag is server-wide
   * and the latest read is the newest word on it, so it decides the board once there is one: a ledger an operator has
   * restored clears the warning on the next read, and one that fails again sets it.
   */
  const [lastStatus, setLastStatus] = useState<PersistedLedger["status"] | null>(null);

  const reduce = useReducedMotion();
  const headingRef = useRef<HTMLHeadingElement>(null);
  const searchRef = useRef<HTMLInputElement>(null);
  /* `/` focuses the queue search from anywhere on the page that is not already taking text. */
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== "/" || e.altKey || e.ctrlKey || e.metaKey || e.defaultPrevented) return;
      const t = e.target as HTMLElement | null;
      if (t && (t.isContentEditable || /^(INPUT|TEXTAREA|SELECT)$/.test(t.tagName))) return;
      e.preventDefault();
      searchRef.current?.focus();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  /* Where focus goes after the control that had it is removed. Resolved after the render that removes it. */
  const root = useRef<HTMLDivElement>(null);
  const [want, setWant] = useState<{ k: string } | null>(null);
  useEffect(() => {
    if (!want) return;
    root.current?.querySelector<HTMLElement>(`[data-focus="${want.k}"]`)?.focus();
  }, [want]);

  /* A queue change from outside (the sidebar, history) starts that queue at its top, as a tab change did. */
  if (s.seen !== urlTab) {
    setS((prev) =>
      prev.seen === urlTab ? prev
        : prev.tab === urlTab ? { ...prev, seen: urlTab }
          : { ...prev, seen: urlTab, tab: urlTab, page: 1, sel: null, collapsed: false, calling: null, stage: null, last: null },
    );
  }
  const tab = s.tab;
  /** Move to another queue from the board, keeping `sel` open there; the URL keeps the scope and records the queue. */
  const goQueue = (to: View, sel: string | null) => {
    setS((prev) => ({ ...prev, tab: to, page: 1, sel, collapsed: false }));
    const next = new URLSearchParams(window.location.search);
    next.set("queue", to);
    window.history.pushState(null, "", `${window.location.pathname}?${next.toString()}`);
  };

  /* An all-programmes queue names each row's programme; a single-programme queue says it once, in the scope. */
  const mixed = new Set(items.map((it) => it.programme)).size > 1;
  const all: Row[] = items.map((it) => {
    const o = s.over[it.id];
    return { ...it, ...o, log: o?.log ?? [] };
  });
  const inView = (v: View) => all.filter((p) => p.view === v).sort(byUrgency);
  const q = s.q.trim().toLowerCase();
  const matches = (x: Row) => !q || x.name.toLowerCase().includes(q) || x.short.toLowerCase().includes(q);
  const list = inView(tab).filter(matches);
  /*
   * The open patient is one of the visible results, so Call, Snooze and Hand-off never act on someone filtered out.
   * The one exception: a patient the last action moved to another tab stays open (named, with a link to that tab)
   * until the user moves on, so the outcome is seen where it was recorded. A search that excludes them lets go too.
   */
  const paging = paginate(list, pageWith(list, s.sel, s.page));
  const held = s.last ? all.find((x) => x.id === s.last!.id && x.view !== tab && matches(x)) : undefined;
  const p = held ?? paging.rows.find((x) => x.id === s.sel) ?? paging.rows[0];
  if (s.page !== paging.page) setS((prev) => (prev.page === paging.page ? prev : { ...prev, page: paging.page }));
  const fresh = { calling: null, stage: null, last: null };
  /* Fixtures mode: the open case's source records and events, read for the selected case only. Live, the ledger read carries them. */
  const { read: fixtureRecords, retry: retryFixtureRecords } = useTodayRecords(live ? undefined : p?.id);
  /* From lg up the case pane is its own scroller: a newly opened case starts at its top, not at the last case's offset. */
  useEffect(() => {
    root.current?.querySelector("#today-case")?.scrollTo({ top: 0 });
  }, [p?.id]);
  const goPage = (to: number, via: PagerControl) => {
    const target = paginate(list, to);
    const focus = pagerFocusAfter(via, target);
    if (focus) setWant({ k: `pager-${focus}` });
    setS((prev) => ({ ...prev, page: target.page, sel: target.rows[0]?.id ?? null, ...fresh }));
  };

  /* Only the selected case's detail is read (issue #47); every other row keeps its list summary. */
  const selId = live ? p?.id : undefined;
  /*
   * A read starts whenever the selected case changes, whichever way it was selected: drop that case's failed read now
   * (React's adjust-state-during-render pattern) so it shows "Reading…", not the old error and Retry, while it runs.
   */
  if (selId !== readFor) {
    setReadFor(selId);
    if (selId && persisted[selId] && !isLedger(persisted[selId])) setPersisted((prev) => withoutRead(prev, selId));
  }
  useEffect(() => {
    if (!selId) return;
    const ac = new AbortController();
    const revision = revisionFor(selId);
    const stale = () => ac.signal.aborted || revision !== revisionFor(selId);
    fetch(`/app/today/ledger?id=${encodeURIComponent(selId)}`, { signal: ac.signal, cache: "no-store" })
      .then(async (r) => {
        if (r.ok) {
          const parsed = LedgerResponse.safeParse(await r.json());
          if (!parsed.success) throw new ReadError("the web server returned an unrecognised ledger");
          const panel = parsed.data.bookingPanel;
          if (panel && (panel.case.id !== selId || panel.case.api_overlay?.head !== panel.overlay.head)) {
            throw new ReadError("the web server returned a ledger for a different case or head");
          }
          return parsed.data;
        }
        const body = (await r.json().catch(() => null)) as { error?: unknown } | null;
        throw new ReadError(typeof body?.error === "string" ? body.error : `HTTP ${r.status}`);
      })
      .then((ledger) => {
        if (stale()) return;
        setPersisted((prev) => ({ ...prev, [selId]: ledger }));
        setLastStatus(ledger.status);
        const panel = ledger.bookingPanel;
        // An invalid replay is only a prefix, never permission to resume contact. Keep patient activity/history
        // separate; only the validated current shared case projection may change these affordances and their head.
        if (ledger.status === "ok" && panel?.overlay.ledger_status === "ok") {
          const latest = workItem(panel.case, now, true);
          setS((prev) => {
            const before = prev.over[selId] ?? { log: [] };
            return {
              ...prev,
              over: {
                ...prev.over,
                [selId]: {
                  ...before,
                  // A new writable head must not retain booking flags/Next from an older local save. A same-head
                  // read can retain the just-saved presentation; contact permission is always refreshed below.
                  ...(before.head !== latest.head ? bookingRowProjection(latest) : {}),
                  ...contactProjection(latest), head: latest.head,
                },
              },
            };
          });
        }
      })
      .catch((e: unknown) => {
        if (stale()) return;
        const error = e instanceof ReadError ? e.message : "the web server did not answer";
        setPersisted((prev) => ({ ...prev, [selId]: { error } }));
      });
    return () => ac.abort();
  }, [selId, attempt, now, revisionFor]);
  /** Forget a failed read of `id` and read it again; a successful read is kept (selecting the case reads it again). */
  const retryLedger = (id: string) => {
    if (!persisted[id] || isLedger(persisted[id])) return;
    setPersisted((prev) => withoutRead(prev, id));
    setAttempt((n) => n + 1);
  };
  const ledger = selId ? persisted[selId] : undefined;
  const ledgerRead = isLedger(ledger) ? ledger : null;
  const bookingProjection = (id: string, projection: BookingProjection) => {
    if (projection.case.id !== id || projection.case.api_overlay?.head !== projection.overlay.head) return;
    supersedeRead(id);
    const original = items.find((item) => item.id === id);
    const latest = workItem(projection.case, now, original?.hasRecord ?? true, Date.now());
    setS((prev) => {
      const before = prev.over[id] ?? { log: [] };
      return {
        ...prev,
        over: {
          ...prev.over,
          [id]: {
            ...before, ...bookingRowProjection(latest),
            ...(projection.overlay.ledger_status === "ok" ? contactProjection(latest) : {}),
          },
        },
      };
    });
    setPersisted((prev) => {
      const existing = prev[id];
      return isLedger(existing) ? { ...prev, [id]: { ...existing, bookingPanel: projection } } : prev;
    });
  };
  const ledgerError = ledger && !isLedger(ledger) ? ledger.error : null;
  /* Live, the selected case's one detail read (GET /app/today/ledger) also carries its records, so the detail is read once. */
  const recordsRead: RecordsRead | undefined = !live
    ? fixtureRecords
    : ledger === undefined
      ? undefined
      : !isLedger(ledger)
        ? { ok: false, error: ledger.error, missing: false }
        : ledger.records
          ? { ok: true, records: ledger.records }
          : { ok: false, error: "the case read carried no source records", missing: true };
  const retryRecords = () => {
    if (!p) return;
    if (live) retryLedger(p.id);
    else retryFixtureRecords();
  };
  /*
   * An untrustworthy ledger is server-wide: if the latest detail read (or, before any, a listed case) says so, no row on
   * the board is authoritative and the API refuses every write. The board says so once; the open patient's detail says why.
   * The list overlay is from page load, so a newer detail read overrides it in either direction.
   */
  const boardInvalid = live && (lastStatus ? lastStatus === "invalid" : listLedgerInvalid(items));
  const ledgerInvalid = !!p && boardInvalid;

  /* The sidebar's queue counts follow this board, including actions logged in this browser. */
  const counts = { call: 0, waiting: 0, none: 0 };
  for (const r of all) counts[r.view] += 1;
  useEffect(() => {
    publishTodayCounts({ call: counts.call, waiting: counts.waiting, none: counts.none });
  }, [counts.call, counts.waiting, counts.none]);
  useEffect(() => () => publishTodayCounts(null), []);

  const apply = (row: Row, entry: Contact, patch: Omit<Patch, "log">, head: string, move: string, stage: Stage | null) => {
    setWant({ k: stage ? "chips" : "undo" });
    setS((prev) => {
      const before = prev.over[row.id] ?? { log: [] };
      return {
        ...prev,
        over: { ...prev.over, [row.id]: { ...before, ...patch, log: [...before.log, entry] } },
        sel: row.id,
        calling: null,
        stage,
        last: {
          id: row.id,
          head,
          move,
          undo: { over: prev.over, stage: prev.stage === "callback" ? "reached" : prev.stage, last: prev.last },
        },
      };
    });
  };

  /**
   * One action from the board. Live: append `action` to the ledger first and change the board only on success;
   * a refusal (for example a contact outcome without contact permission) is shown and nothing changes.
   */
  const log = (row: Row, what: string, patch: Omit<Patch, "log">, head: string, move: string, action: CaseAction, stage: Stage | null = null) => {
    setRefusal(null);
    if (!live) return apply(row, { what, when: `This session · ${clock()}`, who: SESSION_WHO }, patch, head, move, stage);
    if (busy) return;
    if (ledgerInvalid) return setRefusal({ id: row.id, text: `Not saved: ${REFUSED[500]}` });
    if (!row.head) return setRefusal({ id: row.id, text: `Not saved: ${REFUSED[409]}` });
    setBusy(true);
    recordAction(row.id, row.head, action)
      .then((res) => {
        if (res.ok) {
          supersedeRead(row.id);
          // A recorded notification or receipt shows the obligation the API computed next, not one guessed here.
          // Only those two actions: any other action on a case with a recorded fact keeps its own view and status.
          const communication = action.action === "initial_notification" || action.action === "receipt_acknowledgement";
          const next = communication && res.obligation ? obligationAfterCommunication(res.obligation) : undefined;
          const served = next ? { ...next, communicationStep: res.obligation === "no_acknowledgement" ? ("receipt" as const) : undefined } : {};
          apply(row, { what, when: `Saved · ${clock()} · ${res.hash.slice(0, 8)}`, who: res.actor, hash: res.hash }, { ...patch, ...served, ...(action.action === "assign_owner" && !action.owner ? { owner: ownerLabel(res.owner) } : {}), head: res.head }, head, move, stage);
        } else if (res.conflict === "consent") setRefusal({ id: row.id, text: "Not saved: this patient has not given contact permission" });
        else if (res.conflict === "reconciliation" || res.conflict === "communication" || res.conflict === "content" || res.conflict === "identity") setRefusal({ id: row.id, text: `Not saved: ${res.error}` });
        // 400 and 401 carry their own words: what was malformed, or whether the coordinator credential is missing or refused.
        else setRefusal({ id: row.id, text: `Not saved: ${res.status === 400 || res.status === 401 ? res.error : (REFUSED[res.status] ?? res.error)}` });
      })
      .catch(() => setRefusal({ id: row.id, text: "Not saved: the API did not answer" }))
      .finally(() => setBusy(false));
  };

  /*
   * Live API: Snooze, Hand to … and Declined are held for `undoMs` with Undo before anything is sent. Undo cancels the
   * timer, so no POST is made. Leaving the page inside the window sends the held write rather than dropping it.
   */
  // The timer sends with the latest render's `log` and rows, so a ledger read that refreshed the head in the window is used.
  const { setSender } = heldWrite;
  useEffect(() => {
    setSender((job) => {
      const row = all.find((x) => x.id === job.id);
      if (row) log(row, job.what, job.patch, job.head, job.move, job.action);
    });
  });
  const sendPending = () => {
    const job = heldWrite.take();
    if (!job) return;
    setPending(null);
    heldWrite.send(job);
  };
  const undoPending = () => {
    if (!heldWrite.take()) return;
    setPending(null);
    setWant({ k: "heading" });
  };
  /** A write held back for the Undo window in live mode; fixtures apply at once and keep their Undo after the fact. */
  const deferred = (row: Row, what: string, patch: Omit<Patch, "log">, head: string, move: string, action: CaseAction) => {
    if (!live || undoMs <= 0) return log(row, what, patch, head, move, action);
    setRefusal(null);
    if (busy || heldWrite.isHeld()) return;
    if (ledgerInvalid) return setRefusal({ id: row.id, text: `Not saved: ${REFUSED[500]}` });
    if (!row.head) return setRefusal({ id: row.id, text: `Not saved: ${REFUSED[409]}` });
    const job: Pending = { id: row.id, name: row.name, what, patch, head, move, action };
    heldWrite.hold(job, undoMs, sendPending);
    setPending(job);
    setWant({ k: "pending-undo" });
  };
  const writing = busy || !!pending;
  const pendingNotice = pending && (
    <div role="status" className="mb-4 flex min-h-11 flex-wrap items-center gap-x-gap rounded-card border border-warning px-3" data-testid="pending-write">
      <span className="flex-1">
        <span className="font-medium">{pending.what}</span> · {pending.name} · saves to the case ledger in {Math.round(undoMs / 1000)} seconds
      </span>
      <Button variant="quiet" size="touch" data-focus="pending-undo" onClick={undoPending}>
        Undo
      </Button>
      <Button variant="quiet" size="touch" onClick={sendPending}>
        Save now
      </Button>
    </div>
  );

  const last = s.last && p && s.last.id === p.id ? s.last : null;
  /* The last action moved its patient out of this result set and they are not held open: only the way back is shown. */
  const removed = s.last && !held && !list.some((x) => x.id === s.last!.id) ? s.last : null;
  const undoLast = (l: Logged) => {
    setWant({ k: "heading" });
    setS((prev) => ({ ...prev, sel: l.id, calling: null, ...l.undo }));
  };
  const removedNotice = removed && (
    <div role="status" className="mb-4 flex min-h-11 flex-wrap items-center gap-x-gap" data-testid="removed-notice">
      <span>
        <span className="font-medium">{removed.head}</span> · {removed.move}
        {live ? (
          <span data-focus="undo" tabIndex={-1} className="text-muted"> · saved to the case ledger</span>
        ) : (
          <>
            {" "}·{" "}
            <button data-focus="undo" onClick={() => undoLast(removed)} className="inline-flex min-h-11 cursor-pointer items-center bg-transparent px-1 text-body text-hue hover:text-ink">
              Undo
            </button>
          </>
        )}
      </span>
    </div>
  );
  /* A persisted Reached outcome keeps its follow-up choices until a later outcome, snooze or booking replaces it. */
  const stage: Stage | null = last ? s.stage : s.calling === p?.id ? (s.stage === "callback" || s.stage === "elsewhere" ? s.stage : null) : p?.awaitingNext ? "reached" : null;
  const first = p?.name.split(" ")[0] ?? "";
  const toEnd = `${first} moved to the end`;

  type Chip = [string, () => void];
  let chipsTitle = "";
  let chips: Chip[] = [];
  if (p?.retrieval) {
    const retrieval = p.retrieval;
    chipsTitle = "Record retrieval outcome";
    chips = RETRIEVAL_OUTCOMES.map((outcome): Chip => [
      RETRIEVAL_CHIP[outcome],
      () =>
        log(
          p,
          `Record retrieval ended: ${retrievalOutcomeWords(outcome)}`,
          { view: "call", status: "Record retrieval ended · follow-up resumes", next: "Call to arrange the procedure", retrieval: undefined, awaitingNext: false },
          `Logged: ${RETRIEVAL_CHIP[outcome]}`,
          `${first} moved to To call · ${retrieval.owner}'s retrieval task closed`,
          { action: "retrieval_outcome", outcome },
        ),
    ]);
  } else if (p && !p.contactBlocked) {
    if (!last && !stage) {
      chipsTitle = s.calling === p.id ? "How did it go?" : "After the call";
      chips = [
        [
          "Reached",
          () =>
            log(
              p,
              "Reached on the phone",
              { status: `Reached · ${p.next}`, snoozed: false, awaitingNext: true },
              "Logged: Reached",
              "Next: appointment agreed or a call back",
              { action: "contact_outcome", outcome: "reached", channel: "phone" },
              "reached",
            ),
        ],
        [
          "No answer",
          () => {
            const na = (p.noAnswer ?? 0) + 1;
            log(p, "Called, no answer", { noAnswer: na, status: noAnswerStatus(na), snoozed: true, awaitingNext: false }, "Logged: No answer", toEnd, {
              action: "contact_outcome",
              outcome: "no_answer",
              channel: "phone",
            });
          },
        ],
        ...(["voicemail", "wrong_number"] as const).map((outcome): Chip => [
          OUTCOME_CHIP[outcome],
          () =>
            log(
              p,
              outcomeWords(outcome, "phone"),
              { noAnswer: 0, status: outcomeStatus(outcome), snoozed: false, awaitingNext: false },
              `Logged: ${OUTCOME_CHIP[outcome]}`,
              outcomeStatus(outcome),
              { action: "contact_outcome", outcome, channel: "phone" },
            ),
        ]),
      ];
    } else if (stage === "reached") {
      chipsTitle = "What next?";
      chips = [
        [
          // The patient agreed to an appointment; no slot is chosen or reserved here, so the booking is still needed.
          "Appointment agreed",
          () =>
            log(
              p,
              "Appointment agreed",
              { view: "waiting", status: BOOKING_NEEDED, snoozed: false, awaitingNext: false },
              "Logged: Appointment agreed",
              `${first} moved to Waiting`,
              { action: "contact_outcome", outcome: "appointment_agreed", channel: "phone" },
            ),
        ],
        [
          "Call back on…",
          () => {
            setWant({ k: "chips" });
            setS((prev) => ({ ...prev, calling: last ? prev.calling : p.id, stage: "callback", callbackFrom: Date.now() }));
          },
        ],
        [
          "Done elsewhere…",
          () => {
            setWant({ k: "chips" });
            setS((prev) => ({ ...prev, calling: last ? prev.calling : p.id, stage: "elsewhere", callbackFrom: Date.now() }));
          },
        ],
        [
          "Declined",
          () =>
            deferred(
              p,
              "Declined the procedure",
              { view: "waiting", status: DECLINED_STATUS, snoozed: false, awaitingNext: false, ...(p.reserved ? { next: DECLINED_RESERVED_NEXT } : p.cancelled ? { next: DECLINED_CANCELLED_NEXT } : {}) },
              "Logged: Declined",
              `${first} moved to Waiting for the GP`,
              { action: "contact_outcome", outcome: "declined", channel: "phone" },
            ),
        ],
      ];
    } else if (stage === "callback") {
      chipsTitle = "Call back on";
      // One wall-clock `until` per choice drives both the label and the saved snooze (issue #37); never the run day.
      chips = [1, 2, 3].map((k) => {
        const { label: d, action } = callbackChoice(k, s.callbackFrom);
        return [d, () => log(p, `Asked for a call back on ${d}`, { status: `Call back ${d}`, snoozed: true, awaitingNext: false }, `Logged: Call back ${d}`, toEnd, action)] as Chip;
      });
    }
  }
  const chipsLive = !!p && (s.calling === p.id || !!stage);
  /* A held patient is no longer in the list: next is the first row on the page in view, so paging does not jump. */
  const nextUp = held ? paging.rows[0] : p ? list.find((x) => x.id !== p.id) : undefined;
  const callingNow = !!p && s.calling === p.id;

  /* Persisted ledger records first (live), then this session's; a record saved here and already read back shows once. */
  const kept = ledgerRead?.entries ?? [];
  const keptHashes = new Set(kept.map((e) => e.hash).filter(Boolean));
  const entries: Contact[] = p ? [...kept, ...p.log.filter((e) => !e.hash || !keptHashes.has(e.hash))] : [];
  const nothingYet = !live
    ? "No contact logged this session"
    : ledger === undefined
      ? "Reading the case ledger…"
      : ledgerError
        ? "Could not read the case ledger"
        : "No contact on the case ledger yet";
  /**
   * Activity, earliest to latest: the referral, the case's source events (once its records are read, in the Audit
   * page's words), every logged contact, then the next step. Each event opens its note.
   */
  type Activity = { id: string; what: string; when: string; note: string; current?: boolean };
  const sourceEvents = recordsRead?.ok ? recordsRead.records.events : [];
  const activity: Activity[] = p
    ? [
        // The source timeline names the referral itself when it carries it; otherwise the list summary does.
        ...(sourceEvents.some((e) => e.kind === "referral") ? [] : [{ id: "referral", what: "Referral issued", when: formatDay(p.referralDay), note: `${p.programmeName} · ${p.providerName}` }]),
        ...sourceEvents.map((e) => ({ id: e.key, what: e.label, when: eventDay(e.date), note: e.note })),
        ...entries.map((c, i) => ({ id: `log-${i}`, what: c.what, when: c.when, note: `Recorded by ${c.who}${c.hash ? ` · ledger ${c.hash.slice(0, 12)}` : ""}` })),
        { id: "next", what: `Next: ${p.next}`, when: entries.length ? "Now" : nothingYet, note: p.status, current: true },
      ]
    : [];
  /* The strip shows the latest few; older events fold under "Earlier activity" so the strip stays one line. */
  const STRIP = 4;
  const earlier = activity.length > STRIP + 1 ? activity.slice(0, activity.length - STRIP) : [];
  const strip = activity.slice(earlier.length);
  const heldNow = heldOf(recordsRead);

  const queueLabel = TABS.find(([id]) => id === tab)![1];
  const phone = () => !window.matchMedia("(min-width: 1024px)").matches;
  const pick = (r: Row) => {
    if (p?.id === r.id) {
      // On a phone the open row folds and unfolds; beside the queue the case simply stays open. Either way the row is
      // now the explicit selection, so a search or page change keeps it.
      const fold = phone();
      setS((prev) => ({ ...prev, sel: r.id, collapsed: fold ? !prev.collapsed : false }));
      return;
    }
    retryLedger(r.id);
    setS((prev) => ({ ...prev, sel: r.id, collapsed: false, note: null, ...fresh }));
  };
  const contact = p ? contactWords(p) : null;
  const caseId = "today-case";
  /*
   * In the DOM the open case follows its row, so a phone reads it inline. From lg up it is lifted beside the queue: its
   * containing block is the sheet, outside the queue's scroller, so the queue and the case each scroll on their own.
   */
  const openAt = !p || held ? -1 : paging.rows.findIndex((r) => r.id === p.id);
  const fold = s.collapsed ? "max-lg:hidden" : "";
  const side = "lg:absolute lg:inset-y-0 lg:right-0 lg:left-(--queue-w) lg:overflow-y-auto lg:overscroll-contain";

  const fileTabs: ["activity" | "records", string][] = [
    ["activity", "Activity"],
    ["records", "Records"],
  ];
  /** Arrow keys move between the two tabs (and select), as a tablist does. */
  const tabKey = (e: React.KeyboardEvent) => {
    if (!["ArrowLeft", "ArrowRight", "Home", "End"].includes(e.key)) return;
    e.preventDefault();
    const next = e.key === "Home" ? "activity" : e.key === "End" ? "records" : s.file === "activity" ? "records" : "activity";
    setS((prev) => ({ ...prev, file: next }));
    root.current?.querySelector<HTMLElement>(`#today-tab-${next}`)?.focus();
  };
  /** Fixtures mode only: no detail record, so the full record and its audit trail cannot open; say why, not just grey. */
  const noRecord = "showcase cases only in this demo";
  const renderEvent = (ev: Activity, i: number, list: Activity[]) => {
    const open = s.note === ev.id;
    return (
      <li key={ev.id} className="relative min-w-0 pb-3 pl-6 lg:pl-0 lg:pr-4 lg:pt-6">
        {/* The rail: down the side on a phone, across the top from lg up. */}
        <span aria-hidden className={cn("absolute bg-jade-300 max-lg:bottom-0 max-lg:left-[5px] max-lg:top-2 max-lg:w-px lg:left-0 lg:right-0 lg:top-[5px] lg:h-px", i === list.length - 1 && "max-lg:hidden lg:right-auto lg:w-2")} />
        <span aria-hidden className={cn("absolute left-0 top-1.5 size-[11px] rounded-full border border-jade-400 lg:top-0", ev.current ? "bg-jade-800 ring-2 ring-jade-200" : "bg-jade-50")} />
        <button
          type="button"
          aria-expanded={open}
          onClick={() => setS((prev) => ({ ...prev, note: prev.note === ev.id ? null : ev.id }))}
          className="block w-full min-w-0 cursor-pointer bg-transparent p-0 text-left"
        >
          <span className="block font-medium text-ink">{ev.what}</span>
          <span className="mt-0.5 flex min-h-6 items-center gap-1 text-small text-muted">
            {ev.when}
            <ChevronDown aria-hidden className={cn("size-3.5 flex-none transition-transform", open && "rotate-180")} />
          </span>
        </button>
        <p hidden={!open} className="m-0 mt-1 text-small text-ink-soft">
          {ev.note}
        </p>
      </li>
    );
  };
  const heldChip = heldNow > 0 && (
    <span data-testid="records-held" className="rounded-chip bg-ochre-wash px-1.5 py-px text-small font-medium text-ochre-ink">
      {heldNow} held
    </span>
  );

  const activityPane = p && (
    <section aria-label="Case history" className="mt-6 min-w-0 border-t border-line pt-2">
      <button
        type="button"
        aria-expanded={s.hist}
        aria-controls="today-activity"
        onClick={() => setS((prev) => ({ ...prev, hist: !prev.hist }))}
        className="flex min-h-11 w-full cursor-pointer items-center gap-2 bg-transparent text-left text-body font-medium text-jade-800 lg:hidden"
      >
        <ChevronDown aria-hidden className={cn("size-4 transition-transform", !s.hist && "-rotate-90")} />
        Activity and records
        {heldChip}
      </button>
      <div id="today-activity" className={cn(!s.hist && "max-lg:hidden")}>
        {/* Both halves carry the rule, so it stays under the tabs when the link wraps below them on a narrow phone. */}
        <div className="flex flex-wrap items-end justify-between [&>*]:border-b [&>*]:border-rule">
          <div role="tablist" aria-label="Case information" className="flex grow gap-5" onKeyDown={tabKey}>
            {fileTabs.map(([key, label]) => {
              const on = s.file === key;
              return (
                <button
                  key={key}
                  type="button"
                  role="tab"
                  id={`today-tab-${key}`}
                  aria-selected={on}
                  aria-controls={`today-panel-${key}`}
                  tabIndex={on ? 0 : -1}
                  data-focus={key === "activity" ? "contacts" : undefined}
                  onClick={() => setS((prev) => ({ ...prev, file: key }))}
                  className={cn(
                    "-mb-px inline-flex min-h-11 cursor-pointer items-center gap-2 border-b-2 bg-transparent py-2.5 text-body",
                    on ? "border-jade-700 font-semibold text-jade-900" : "border-transparent font-medium text-muted hover:text-jade-800",
                  )}
                >
                  {label}
                  {key === "records" && heldChip}
                </button>
              );
            })}
          </div>
          {s.file === "activity" ? (
            p.hasRecord ? (
              <Link href={`${caseHref("/app/patients", p.id)}/audit`} className="inline-flex min-h-11 items-center gap-1.5 pl-3 text-small font-medium text-jade-700">
                Audit trail
                <ArrowRight aria-hidden className="size-4" />
              </Link>
            ) : (
              <span data-testid="audit-unavailable" className="inline-flex min-h-11 items-center pl-3 text-small text-muted">Audit trail: {noRecord}</span>
            )
          ) : p.hasRecord ? (
            <Link href={caseHref("/app/patients", p.id)} className="inline-flex min-h-11 items-center gap-1.5 pl-3 text-small font-medium text-jade-700">
              <Folder aria-hidden className="size-4" strokeWidth={1.6} />
              Full case record
            </Link>
          ) : (
            <span data-testid="record-unavailable" className="inline-flex min-h-11 items-center pl-3 text-small text-muted">Full case record: {noRecord}</span>
          )}
        </div>
        <div role="tabpanel" id="today-panel-activity" aria-labelledby="today-tab-activity" hidden={s.file !== "activity"} tabIndex={0} className="outline-offset-2">
          {ledgerError && (
            <div role="status" data-testid="ledger-read-failed" className="mt-3 flex min-h-11 flex-wrap items-center gap-x-gap">
              <span className="text-warning">Could not read the case ledger: {ledgerError}</span>
              <Button
                variant="quiet"
                size="touch"
                onClick={() => {
                  // The button goes while the ledger is read again; focus stays in the Activity section.
                  setWant({ k: "contacts" });
                  retryLedger(p.id);
                }}
              >
                Retry
              </Button>
            </div>
          )}
          {/* Live, a failed read is the ledger read: its message and Retry are above. */}
          {!live && recordsRead && !recordsRead.ok && !recordsRead.missing && (
            <div role="status" data-testid="source-events-failed" className="mt-3 flex min-h-11 flex-wrap items-center gap-x-gap">
              <span className="text-warning">Source events not shown: {recordsRead.error}</span>
              <Button variant="quiet" size="touch" onClick={retryRecords}>
                Retry
              </Button>
            </div>
          )}
          <ol aria-live="polite" className="m-0 mt-4 list-none p-0 lg:grid lg:auto-cols-[minmax(180px,1fr)] lg:grid-flow-col lg:overflow-x-auto lg:pb-1 lg:pl-1.5 lg:pt-1">
            {strip.map((ev, i) => renderEvent(ev, i, strip))}
          </ol>
          {earlier.length > 0 && (
            <details data-testid="earlier-activity" className="group mt-1 border-t border-rule pt-1">
              <summary className="flex min-h-11 cursor-pointer list-none items-center gap-1.5 text-small font-semibold text-jade-700 [&::-webkit-details-marker]:hidden">
                <ChevronDown aria-hidden className="size-4 -rotate-90 transition-transform group-open:rotate-0" />
                Earlier activity ({earlier.length})
              </summary>
              <ol aria-label="Earlier activity, oldest first" className="m-0 mb-2 mt-1 grid list-none gap-2 p-0 lg:grid-cols-2 lg:gap-x-8">
                {earlier.map((ev) => (
                  <li key={ev.id} className="min-w-0 border-l-2 border-jade-200 pl-2.5">
                    <span className="font-medium text-ink">{ev.what}</span>
                    <span className="text-small text-muted"> · {ev.when}</span>
                    <p className="m-0 text-small text-ink-soft">{ev.note}</p>
                  </li>
                ))}
              </ol>
            </details>
          )}
        </div>
        <div role="tabpanel" id="today-panel-records" aria-labelledby="today-tab-records" hidden={s.file !== "records"} tabIndex={0} className="outline-offset-2">
          {s.file === "records" && <RecordsPanel read={recordsRead} onRetry={retryRecords} />}
        </div>
      </div>
    </section>
  );

  const casePane = p ? (
    <section
      key="case"
      id={caseId}
      aria-label="Selected case"
      data-case={p.short}
      className={cn("min-w-0 px-4 pb-4 pt-1 max-lg:bg-jade-50 max-lg:shadow-[inset_3px_0_var(--color-jade-600)] lg:px-[clamp(20px,2.4vw,40px)] lg:py-8", side, fold)}
    >
      {removedNotice}
      <p className="m-0 hidden text-small text-muted lg:block" data-testid="case-who">
        {p.named ? (
          <>
            {p.name}
            {p.nameAr && (
              <span lang="ar" dir="rtl" className="ml-2">
                {p.nameAr}
              </span>
            )}
            {" · "}
          </>
        ) : null}
        <span dir="ltr">{p.short}</span>
      </p>
      <div className="grid grid-cols-[minmax(0,1fr)_auto] items-start gap-x-3 lg:mt-2 lg:grid-cols-[minmax(0,max-content)_auto_1fr]">
        <h2
          ref={headingRef}
          data-focus="heading"
          tabIndex={-1}
          className="staff-display m-0 text-[clamp(30px,3vw,46px)] font-medium leading-[1.06] tracking-[-0.03em] text-jade-900 outline-none max-lg:sr-only lg:col-start-1 lg:row-start-1"
        >
          {taskTitle(p)}
        </h2>
        <div className="col-start-2 row-start-1 flex gap-1 rounded-control bg-limestone-100 p-0.5 lg:self-center" role="group" aria-label="Case">
          <button
            type="button"
            aria-label="Case details"
            aria-expanded={s.facts}
            aria-controls="today-facts"
            onClick={() => setS((prev) => ({ ...prev, facts: !prev.facts }))}
            className={cn("inline-flex size-11 cursor-pointer items-center justify-center rounded-control bg-transparent text-jade-700 hover:bg-jade-50 lg:size-9", s.facts && "bg-jade-100")}
          >
            <IdCard aria-hidden className="size-[18px]" strokeWidth={1.6} />
          </button>
          {p.hasRecord ? (
            <Link
              href={caseHref("/app/patients", p.id)}
              aria-label="Open full record"
              title="Open full record"
              className="inline-flex size-11 items-center justify-center rounded-control text-jade-700 hover:bg-jade-50 lg:size-9"
            >
              <Folder aria-hidden className="size-[18px]" strokeWidth={1.6} />
            </Link>
          ) : (
            // Fixtures mode only (the live API has a record for every case): focusable, so the reason can be read.
            <button
              type="button"
              aria-disabled="true"
              aria-label="Full record not available for this case"
              aria-describedby="today-no-record"
              title="Full record not available: this demo has full records only for the showcase cases"
              className="inline-flex size-11 cursor-not-allowed items-center justify-center rounded-control bg-transparent text-limestone-500 lg:size-9"
            >
              <Folder aria-hidden className="size-[18px]" strokeWidth={1.6} />
              <span id="today-no-record" className="sr-only">This demo has full records only for the showcase cases.</span>
            </button>
          )}
        </div>
        <p className="col-start-1 row-start-1 m-0 pt-2 text-body text-ink-soft lg:col-span-full lg:row-start-2 lg:mt-3 lg:max-w-[68ch] lg:pt-0">{taskSentence(p)}</p>
      </div>

      {(
        <dl id="today-facts" data-testid="case-facts" hidden={!s.facts} className="m-0 mt-3 grid max-w-[560px] grid-cols-1 gap-x-6 gap-y-1.5 rounded-card border border-line bg-surface p-3 text-small sm:grid-cols-2">
          <Fact label="Owner" value={p.owner} />
          <Fact label="Contact permission" value={p.contactBlocked ? "On hold · denial or opt-out recorded" : CONSENT_WORDS[p.consent]} warn={p.contactBlocked || p.consent !== "granted"} />
          <Fact label="Texts" value={p.textsStopped ? "Stopped by patient" : p.textsAllowed ? "Allowed" : "Not allowed yet"} />
          <Fact label="Case" value={p.short} ltr />
          {p.named && <Fact label="Patient" value={p.name} />}
          <Fact label="Preferred language" value={p.languageName} />
          <Fact label="Programme" value={`${p.programmeName} · ${p.providerName}`} />
          <Fact label="Referred" value={formatDay(p.referralDay)} />
          <Fact label="Deadline" value={p.deadline === null ? "Routine interval, no overdue clock" : `Day ${p.deadline} · ${formatDay(p.referralDay + p.deadline)}`} />
          <Fact label="Next" value={p.next} />
        </dl>
      )}

      <p className={cn("m-0 mt-2 text-small", clockWords(p).over ? "text-negative" : "text-muted")}>
        <span data-testid="lead-days" data-value={p.days} className="tabular-nums">
          {p.days}
        </span>{" "}
        days since referral · {clockWords(p).long}
      </p>

      <div className="mt-4 flex flex-wrap items-center gap-x-4 gap-y-2">
        <Button
          variant="cta"
          size="touch"
          disabled={p.contactBlocked || ledgerInvalid || !!p.retrieval}
          onClick={() => setS((prev) => ({ ...prev, calling: p.id, stage: null, last: null }))}
          className={cn("px-5", callingNow && "bg-ink-soft")}
        >
          <Phone aria-hidden strokeWidth={1.8} />
          {/* No dialer is connected: this only opens call logging, so it never says a call is active (issue #36). */}
          {callingNow ? (
            <span>Logging call · {first}</span>
          ) : (
            <span>
              Log call<span className="sr-only"> · {first}</span>
            </span>
          )}
        </Button>
        {contact && (
          <span data-testid="contact-status" className={cn("inline-flex items-center gap-1.5 text-small", contact.tone === "ok" ? "text-jade-700" : "font-medium text-warning")}>
            {contact.tone === "ok" ? <Check aria-hidden className="size-4" /> : <CircleAlert aria-hidden className="size-4" />}
            {contact.label}
          </span>
        )}
      </div>
      <div className="mt-3 flex flex-wrap gap-2">
        <Button
          variant="quiet"
          size="touch"
          disabled={writing || ledgerInvalid}
          onClick={() =>
            deferred(p, "Snoozed to tomorrow", { status: "Snoozed to tomorrow", snoozed: true, awaitingNext: false }, "Logged: Snoozed to tomorrow", toEnd, {
              action: "snooze",
              until: fromNow(1),
              reason: "Snoozed to tomorrow",
            })
          }
        >
          Snooze to tomorrow
        </Button>
        {p.view === "none" ? (
          <Button
            variant="quiet"
            size="touch"
            disabled={writing || ledgerInvalid}
            onClick={() =>
              log(p, "Took this patient", { view: "call", status: "Yours now", owner: SESSION_OWNER }, "Logged: Yours now", `${first} moved to To call`, {
                action: "assign_owner",
              })
            }
          >
            Take this patient
          </Button>
        ) : (
          <Button
            variant="quiet"
            size="touch"
            disabled={writing || ledgerInvalid}
            onClick={() =>
              deferred(
                p,
                `Handed to ${HANDOFF_OWNER}`,
                { view: "waiting", status: `With ${HANDOFF_OWNER}`, owner: HANDOFF_OWNER },
                `Logged: Handed to ${HANDOFF_OWNER}`,
                `${first} moved to Waiting`,
                { action: "assign_owner", owner: HANDOFF_OWNER },
              )
            }
          >
            Hand to {HANDOFF_OWNER}
          </Button>
        )}
      </div>

      {ledgerInvalid && (
        /* Not an alert: the board banner above is the one alert for this; this box only adds the open patient's detail. */
        <div data-testid="ledger-invalid" className="mt-4 rounded-card border-2 border-warning p-3">
          <p className="m-0 font-medium text-warning">Case ledger not trustworthy: this patient&rsquo;s state here is not authoritative</p>
          <p className="m-0 mt-1">
            The API reports its shared case ledger as not trustworthy, so the contacts and status shown here may not match it
            {ledgerRead?.applied != null ? ` (${ledgerRead.applied} ledger events applied)` : ""}, and the API refuses new actions until an
            operator restores the ledger.
          </p>
          {ledgerRead?.detail && <p className="m-0 mt-1 text-small text-muted">API: {ledgerRead.detail}</p>}
        </div>
      )}

      {refusal?.id === p.id && (
        <p role="alert" className="m-0 mt-4 text-warning" data-testid="action-refused">
          {refusal.text}
        </p>
      )}

      <AnimatePresence initial={false}>
        {last && (
          <motion.div
            key="logged"
            role="status"
            initial={reduce ? false : { opacity: 0, y: -4 }}
            animate={{ opacity: 1, y: 0 }}
            transition={{ duration: reduce ? 0 : 0.15 }}
            className="mt-4 flex min-h-11 flex-wrap items-center gap-x-gap"
          >
            <span>
              <span className="font-medium">{last.head}</span> · {last.move}
              {live ? (
                <span data-focus="undo" tabIndex={-1} className="text-muted"> · saved to the case ledger</span>
              ) : (
                <>
                  {" "}·{" "}
                  <button
                    data-focus="undo"
                    onClick={() => undoLast(last)}
                    className="inline-flex min-h-11 cursor-pointer items-center bg-transparent px-1 text-body text-hue hover:text-ink"
                  >
                    Undo
                  </button>
                </>
              )}
            </span>
            {held && held.id === p.id && (
              <Button
                variant="quiet"
                size="touch"
                data-testid="open-destination"
                onClick={() => {
                  setWant({ k: "heading" });
                  goQueue(held.view, held.id);
                }}
              >
                Open {TABS.find(([id]) => id === held.view)![1]}
              </Button>
            )}
            {!stage && nextUp && (
              <Button
                variant="quiet"
                size="touch"
                onClick={() => {
                  setWant({ k: "heading" });
                  setS((prev) => ({ ...prev, sel: nextUp.id, note: null, ...fresh }));
                }}
              >
                Next case: {nextUp.short}
              </Button>
            )}
          </motion.div>
        )}
      </AnimatePresence>

      {stage === "elsewhere" && !p.retrieval && (
        <div className="mt-4">
          <h3 data-focus="chips" tabIndex={-1} className="m-0 text-section font-medium">
            Done elsewhere
          </h3>
          <ElsewhereForm
            owner={p.owner === "No owner" ? "" : p.owner}
            procedure={p.procedure ?? ""}
            nowMs={s.callbackFrom}
            disabled={writing || ledgerInvalid}
            onCancel={() => setS((prev) => ({ ...prev, calling: null, stage: "reached" }))}
            onRecord={(action) => {
              const words = reconciliationWords(action, new Date().toISOString().slice(0, 10));
              log(
                p,
                `Reported done elsewhere: ${action.procedure} at ${action.provider}`,
                { view: "waiting", status: words.status, next: words.next, awaitingNext: false, retrieval: { owner: action.retrieval_owner, due: action.retrieval_due, overdue: words.overdue } },
                "Logged: Done elsewhere",
                `${first} moved to Waiting for the record`,
                action,
              );
            }}
          />
        </div>
      )}

      {p.communicationStep && !p.contactBlocked && !p.retrieval && stage !== "elsewhere" && (
        <CommunicationForm
          key={`${p.id}-${p.communicationStep}`}
          step={p.communicationStep}
          template={p.notificationTemplate}
          disabled={writing || ledgerInvalid}
          onRecord={(action) => {
            const notify = action.action === "initial_notification";
            log(
              p,
              communicationWords(action.action, action),
              notify
                ? { status: "Result told · receipt not confirmed yet", next: "Call to confirm they received the result", communicationStep: "receipt", awaitingNext: false }
                : { status: "Receipt confirmed · understanding not checked", communicationStep: undefined, awaitingNext: false },
              notify ? "Logged: Result notification" : "Logged: Receipt acknowledged",
              notify ? "Next: confirm they received it" : "Next: the following step on the referral",
              action,
            );
          }}
        />
      )}

      {chips.length > 0 && stage !== "elsewhere" && (
        <div className="mt-4">
          <h3 data-focus="chips" tabIndex={-1} className="m-0 text-section font-medium">
            {chipsTitle}
          </h3>
          <div role="group" aria-label={chipsTitle} className="mt-2 flex flex-wrap gap-2">
            {chips.map(([label, choose]) => (
              <Button key={label} variant="chip" size="touch" disabled={writing || ledgerInvalid} onClick={choose} className={chipsLive ? "border-ink" : "border-control"}>
                {label}
              </Button>
            ))}
          </div>
        </div>
      )}

      {p.teachBackCallback && (
        <div data-testid="teach-back-callback" className="mt-4 rounded-card border border-warning p-3">
          <p className="m-0 font-medium text-warning">
            {p.teachBackCallback.status} · patient ref <span dir="ltr">{p.teachBackCallback.ref}</span>
          </p>
          <p className="m-0 mt-1">
            Call to explain the next step. Logging a call does not resolve this task;{" "}
            {p.hasRecord ? (
              <Link href={`${caseHref("/app/patients", p.id)}#teach-back-callbacks`} className="underline">
                record the consultation outcome on the case page
              </Link>
            ) : (
              "record the consultation outcome on the case page"
            )}
            .
          </p>
        </div>
      )}

      {ledgerRead?.patient && (
        <section aria-labelledby="today-patient-journey" data-testid="today-patient-journey" className="mt-5 border-t border-rule pt-4">
          <h3 id="today-patient-journey" className="m-0 text-body font-medium">Patient self-service</h3>
          <p className="m-0 mt-1 text-muted">
            Teach-back {ledgerRead.patient.teach_back.status} · preparation {ledgerRead.patient.preparation.completed}/{ledgerRead.patient.preparation.total} · last patient contact choice: {ledgerRead.patient.contact_permission.status}
          </p>
          {ledgerRead.patient.slot_selection.status === "selected" && (
            <p className="m-0 mt-1">Patient selected a simulated slot · coordinator review needed. No appointment is reserved.</p>
          )}
        </section>
      )}

      {live && ledgerRead?.bookingPanel && (
        <div className="mt-6" aria-label="Simulated booking">
          <BookingControls key={p.id} caseId={p.id} initial={ledgerRead.bookingPanel} live retrievalPending={!!p.retrieval} onProjection={(projection) => bookingProjection(p.id, projection)} />
        </div>
      )}
      {activityPane}
    </section>
  ) : (
    <div
      key="case"
      data-testid="no-case"
      className={cn("min-w-0 px-4 py-4 lg:px-[clamp(20px,2.4vw,40px)] lg:py-8", side)}
    >
      {removedNotice}
      {items.length === 0 ? (
        <p className="m-0 text-body" role="status" data-testid="today-scope-empty">
          No open cases for {scope}. Choose another programme or provider.
        </p>
      ) : (
        <>
          <h2 className="staff-display m-0 text-[clamp(24px,2.2vw,32px)] font-medium text-jade-900">{q ? "No case matches" : "Nothing in this queue"}</h2>
          <p className="m-0 mt-2 text-muted">{q ? "Change or clear the search to choose a case." : "Choose another queue under Today."}</p>
        </>
      )}
    </div>
  );

  const rowEls = paging.rows.map((r) => {
    const on = p?.id === r.id;
    const title = taskTitle(r);
    const over = clockWords(r).over;
    return (
      <div key={r.id} className={cn("min-w-0 border-t border-rule", on && "lg:bg-jade-100")}>
        <button
          data-case-row={r.short}
          aria-expanded={on && !s.collapsed}
          aria-controls={on ? caseId : undefined}
          onClick={() => pick(r)}
          className={cn(
            "flex min-h-[52px] w-full cursor-pointer items-start gap-2 bg-transparent px-4 py-2.5 text-left hover:bg-jade-50",
            on && "bg-jade-50 shadow-[inset_3px_0_var(--color-jade-600)] lg:bg-jade-100",
          )}
        >
          <ChevronDown aria-hidden className={cn("mt-1 size-4 flex-none text-muted transition-transform lg:hidden", on && !s.collapsed && "rotate-180")} />
          <span className="min-w-0 flex-1">
            <span className="flex min-w-0 flex-wrap items-baseline justify-between gap-x-3">
              <span className="font-semibold text-ink">{title}</span>
              <span dir="ltr" className="whitespace-nowrap text-small tabular-nums text-muted">
                {r.short}
              </span>
            </span>
            {(r.named || r.status !== title || over) && (
              <span className="mt-0.5 block text-small text-muted">
                {r.named && (
                  <>
                    {r.name}
                    {r.nameAr && (
                      <span lang="ar" dir="rtl" className="ml-1.5">
                        {r.nameAr}
                      </span>
                    )}
                    {" · "}
                  </>
                )}
                {r.status !== title && <>{r.status}</>}
                {over && <span className="text-negative">{r.status !== title || r.named ? " · " : ""}{clockWords(r).short}</span>}
              </span>
            )}
            {r.teachBackCallback && (
              <span className="mt-0.5 block text-small text-warning" data-testid="row-callback">
                {r.status === r.teachBackCallback.status ? "" : `${r.teachBackCallback.status} · `}Patient ref <span dir="ltr">{r.teachBackCallback.ref}</span>
              </span>
            )}
            {mixed && (
              <span className="mt-0.5 block text-small text-muted" data-testid="row-programme">
                {r.programmeName} · {r.providerName}
              </span>
            )}
          </span>
        </button>
      </div>
    );
  });

  /* The open case, its history inside it, follows the open row (or leads the list when the open case was moved off it). */
  const at = openAt >= 0 ? openAt + 1 : 0;
  const opened = (
    <div key="opened" data-testid="today-open" className="contents">
      {casePane}
    </div>
  );
  const listBody = [...rowEls.slice(0, at), opened, ...rowEls.slice(at)];

  return (
    // From lg up the board fills the screen under the 54px top bar, so the queue and the case scroll, not the page.
    <div ref={root} className="lg:flex lg:h-[calc(100dvh_-_54px_-_2*var(--main-py))] lg:min-h-[480px] lg:flex-col">
      <div
        id="worklist"
        tabIndex={-1}
        data-testid="today-sheet"
        className="flex flex-col rounded-card border border-line bg-surface lg:min-h-0 lg:flex-1 lg:overflow-clip"
      >
        {head}
        {(boardInvalid || pendingNotice) && (
          <div className="px-4 pt-3">
            {boardInvalid && <LedgerInvalidBanner what="this board" className="mb-4" />}
            {pendingNotice}
          </div>
        )}
        <div className="relative flex min-h-0 flex-col [--queue-w:clamp(300px,32%,440px)] lg:flex-1">
          {/* Not positioned: the open case, lifted out with lg:absolute, must not scroll with the queue. */}
          <div className="flex min-w-0 flex-col lg:h-full lg:w-(--queue-w) lg:scroll-pt-20 lg:overflow-y-auto lg:overscroll-contain lg:border-r lg:border-line lg:bg-limestone-100">
            <div className="flex flex-wrap items-center gap-x-4 gap-y-2 px-4 py-3 lg:sticky lg:top-0 lg:z-10 lg:border-b lg:border-line lg:bg-limestone-100">
              <h2 id="today-queue" className="staff-display m-0 whitespace-nowrap text-[22px] font-medium text-jade-900">
                {queueLabel} <span className="sr-only">· {list.length}</span>
              </h2>
              <label className="flex min-w-[150px] flex-1 items-center gap-2 rounded-control border border-input bg-surface px-2.5 focus-within:border-hue focus-within:outline-2 focus-within:outline-offset-2 focus-within:outline-hue">
                <Search aria-hidden className="size-4 flex-none text-muted" />
                <span className="sr-only">Find a case or task</span>
                <input
                  ref={searchRef}
                  type="search"
                  placeholder="Find a case or task"
                  aria-keyshortcuts="/"
                  value={s.q}
                  onChange={(e) => setS((prev) => ({ ...prev, q: e.target.value, page: 1, collapsed: false }))}
                  className="h-11 min-w-0 flex-1 border-0 bg-transparent text-body text-ink outline-hidden placeholder:text-muted placeholder:opacity-100 lg:h-10"
                />
                <kbd aria-hidden className="hidden rounded-chip border border-line bg-limestone-100 px-1.5 text-small text-muted sm:inline">
                  /
                </kbd>
              </label>
            </div>
            <div className="contents">
              {listBody}
              {list.length === 0 && (
                <p className="m-0 border-t border-rule px-4 py-3 text-muted">
                  {q ? "No case matches this search." : "Nobody left in this queue."}
                </p>
              )}
            </div>
            <div className="border-t border-rule lg:mt-auto">
              <Pager paging={paging} label="Queue pages" testId="page-range" onPage={goPage} />
              {paging.pages > 1 && <p role="status" aria-live="polite" aria-atomic="true" className="sr-only" data-testid="page-status">{pageStatusText(paging)}</p>}
              <p className="m-0 px-4 py-3 text-small text-muted" data-testid="queue-summary">
                {tab === "call"
                  ? `${list.length} to call · ${list.filter((r) => clockWords(r).over).length} past their deadline`
                  : tab === "waiting"
                    ? `${list.length} waiting on someone else`
                    : `${list.length} with no owner`}
              </p>
            </div>
          </div>
        </div>
      </div>
      {aside && <div className="mt-2 lg:flex-none">{aside}</div>}
    </div>
  );
}

function Fact({ label, value, warn = false, ltr = false }: { label: string; value: string; warn?: boolean; ltr?: boolean }) {
  return (
    <div className="flex min-w-0 flex-wrap items-baseline justify-between gap-x-3">
      <dt className="text-muted">{label}</dt>
      <dd dir={ltr ? "ltr" : undefined} className={cn("m-0 text-right font-medium", warn ? "text-warning" : "text-ink")}>
        {value}
      </dd>
    </div>
  );
}

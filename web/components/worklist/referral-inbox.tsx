"use client";

import { useEffect, useId, useRef, useState, useSyncExternalStore } from "react";
import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { AnimatePresence, motion, useReducedMotion } from "motion/react";
import { ArrowRight, Check, ChevronDown, ChevronRight, Clock, FileText, FolderOpen, History, Hospital, House, Inbox, Undo2 } from "lucide-react";
import { Avatar } from "@/components/avatar/avatar";
import { Button } from "@/components/ui/button";
import { PageHeader } from "@/components/shell/page-header";
import { RecordAvatar } from "@/components/wasl/record-avatar";
import { HonestyLabel } from "@/components/wasl/honesty-label";
import { FilterMenu } from "@/components/ui/filters/filter-menu";
import { SearchField } from "@/components/ui/filters/search-field";
import { Toolbar } from "@/components/ui/filters/toolbar";
import { cn } from "@/lib/utils";
import { formatDay } from "@/lib/domain/day";
import { handoffActBy } from "@/lib/domain/handoff";
import type { ReferralAnswer, RespondResult } from "@/app/app/referrals/actions";
import { RETURN_NOTE_MAX, type ReferralCondition, type ReferralReturnReason } from "@/lib/api/schema";
import {
  CONDITION_LABELS,
  REASON_LABELS,
  STATUS_FILTERS,
  handoffOwner,
  handoffWords,
  referralStatus,
  statusFilterLabel,
  withSessionReturn,
  type Handoff,
  type Referral,
  type ReferralState,
  type ReferralStatus,
  type StatusTone,
} from "./referrals";
import { removeSessionReturn, saveSessionReturn, useSessionReturns } from "./session-returns";
import { inboxHref, readInboxUrl, shallowGo, type InboxUrl } from "./inbox-url";
import { PagerLinks } from "./pager";
import { paginate, pageOfId, pageStatusText } from "./paging";

/* With `respond` (live API), answers are appended to the case ledger and the inbox shows the server's projection after
   a refresh. Without it (fixtures), answers and prep grades are session state; a CRC return (reason and note) is also
   kept in this browser's sessionStorage (session-returns.ts), so the GP view in this browser can show and acknowledge
   it. Prep grading is always session state.
   The inbox state (view, search, status, page, open referral) lives in the URL (inbox-url.ts); session decisions live
   here, so they outlive every move between the hand-off and the full inbox. */

const ME = "Dr Layla Haddad";
const CONDITIONS = Object.entries(CONDITION_LABELS) as [ReferralCondition, string][];
const REASONS = Object.entries(REASON_LABELS) as [ReferralReturnReason, string][];
const GRADES: [string, string][] = [
  ["0", "Not seen"],
  ["1", "Partly seen"],
  ["2", "Mostly seen"],
  ["3", "All seen"],
];
/** Rows per page of the full inbox; the hand-off previews this many. */
export const INBOX_PAGE = 10;
const PREVIEW = 3;
const RANK: Record<ReferralState, number> = { new: 0, grade: 1, returned: 2, accepted: 3, done: 4 };

/** `saved`: appended to the ledger, so no local Undo and no session trail entry. */
type Override = { state: ReferralState; outcome: string; log: { who: string; what: string }[]; handoff?: Handoff; saved?: boolean };
const REFUSED: Record<number, string> = {
  403: "your clinician account is not assigned to this referral's unit.",
  404: "this referral could not be found.",
  409: "the referral changed since this page loaded. Reload and answer again.",
  500: "the case record could not be read. Try again later.",
};
type Mode = null | "cond" | "ret";

const TONE: Record<StatusTone, string> = {
  pending: "text-ochre-ink",
  held: "text-clay-ink",
  done: "text-jade-800",
};

export function ReferralInbox({
  referrals,
  window,
  now,
  q,
  scopeBar,
  respond,
  scope = "",
  referrer = "GP",
  heading = { title: "Referrals", indication: "" },
}: {
  referrals: Referral[];
  /** The server action that appends the receiving unit's answer; absent when there is no live API. */
  respond?: (caseId: string, head: string, response: ReferralAnswer) => Promise<RespondResult>;
  /** Days the unit has to answer (dashboard `sla.referral_response_days`). */
  window: number;
  /** The run day: a return is decided on it. */
  now: number;
  q: string;
  /** The page-level programme switcher and the referrer's view link, above the card. */
  scopeBar: React.ReactNode;
  /** The programme/provider query that every inbox URL keeps; empty for the CRC default. */
  scope?: string;
  /** Who returned referrals wait on, in words. */
  referrer?: string;
  /** The open referral's heading and programme chip. */
  heading?: { title: string; indication: string };
}) {
  const router = useRouter();
  const params = useSearchParams();
  const url = readInboxUrl(params, q);
  /* A decision moves its referral in the queue order; the hand-off keeps showing it until the URL moves on. Kept in
     state, not written to the URL: a history write here would make the router re-render and drop the focus the
     decision hands to Undo. */
  const [pinned, setPinned] = useState<string | null>(null);
  const urlKey = `${params.toString()}|${q}`;
  const [seenUrl, setSeenUrl] = useState(urlKey);
  if (seenUrl !== urlKey) {
    setSeenUrl(urlKey);
    setPinned(null);
  }
  const [mode, setMode] = useState<Mode>(null);
  const [picked, setPicked] = useState<string[]>([]);
  const [note, setNote] = useState("");
  const [tried, setTried] = useState(false);
  const [over, setOver] = useState<Record<string, Override>>({});
  const [packetOpen, setPacketOpen] = useState(false);
  const reduce = useReducedMotion();
  const uid = useId().replace(/:/g, "");
  const reasonError = `rf-reason-error-${uid}`;
  const noteHint = `rf-note-hint-${uid}`;
  /* Fixtures mode: returns and GP acknowledgements kept in this browser. Live mode never reads them. */
  const kept = useSessionReturns();

  /* Where focus goes after the control that had it is removed. Resolved after the render that removes it. */
  const root = useRef<HTMLDivElement>(null);
  const [want, setWant] = useState<{ k: string } | null>(null);
  useEffect(() => {
    if (want) root.current?.querySelector<HTMLElement>(`[data-focus="${want.k}"]`)?.focus();
  }, [want]);
  /* Moving between the hand-off and the full inbox focuses the new view's heading, or the row last opened. */
  const lastView = useRef(url.view);
  useEffect(() => {
    if (lastView.current === url.view) return;
    lastView.current = url.view;
    const el = root.current;
    (el?.querySelector<HTMLElement>('[data-focus="selected-row"]') ?? el?.querySelector<HTMLElement>('[data-focus="heading"]'))?.focus();
  }, [url.view]);

  const all = referrals
    .map((r) => (respond ? r : withSessionReturn(r, kept[r.id])))
    .map((r) => ({ ...r, state: over[r.id]?.state ?? r.state, handoff: over[r.id]?.handoff ?? r.handoff }));
  const sorted = [...all].sort((a, b) => RANK[a.state] - RANK[b.state] || b.waiting - a.waiting);
  const list = url.status ? sorted.filter((x) => referralStatus(x, referrer).key === url.status) : sorted;
  const r = url.caseId ? all.find((x) => x.id === url.caseId) : ((pinned ? all.find((x) => x.id === pinned) : undefined) ?? list[0]);

  /* An unconfirmed mode and picks belong to the referral shown when they were started. A search or a URL change can
     show another referral (or none) without any click here, so they are dropped when the displayed id changes.
     Committed decisions live in `over` and are untouched. */
  const [draftFor, setDraftFor] = useState<string | null>(null);
  const shownId = url.view === "case" ? (r?.id ?? null) : null;
  if (draftFor !== shownId) {
    setDraftFor(shownId);
    setMode(null);
    setPicked([]);
    setNote("");
    setTried(false);
  }

  const href = (s: Partial<InboxUrl>) => inboxHref(scope, { q: url.q, status: url.status, ...s });
  const caseLink = (id: string) => href({ caseId: id, from: url.view === "inbox" || url.from, page: url.page ?? undefined });

  const pin = (id: string) => {
    if (url.caseId !== id) setPinned(id);
  };
  const act = (id: string, state: ReferralState, outcome: string, entry: string, handoff?: Handoff, who = ME) => {
    pin(id);
    setWant({ k: "undo" });
    setMode(null);
    setPicked([]);
    setOver((o) => ({ ...o, [id]: { state, outcome, log: [...(o[id]?.log ?? []), { who, what: entry }], handoff } }));
  };
  const [busy, setBusy] = useState(false);
  const [refusal, setRefusal] = useState<{ id: string; text: string } | null>(null);
  /* Live: the answer is shown only once the ledger append succeeds, and the server's projection replaces it on refresh. */
  const answer = (x: Referral, request: ReferralAnswer, state: ReferralState, outcome: string, entry: string, handoff?: Handoff) => {
    setRefusal(null);
    if (!respond) return act(x.id, state, outcome, entry, handoff);
    if (busy) return;
    if (!x.head) return setRefusal({ id: x.id, text: "Not saved: this referral cannot be answered here. Nothing was sent." });
    setBusy(true);
    respond(x.id, x.head, request)
      .then((res) => {
        if (!res.ok) return setRefusal({ id: x.id, text: `Not saved: ${REFUSED[res.status] ?? res.error}` });
        pin(x.id);
        setWant({ k: "heading" });
        setMode(null);
        setPicked([]);
        setOver((o) => ({ ...o, [x.id]: { state, outcome: `${entry}. Saved to the case record.`, log: [{ who: ME, what: entry }], saved: true } }));
        router.refresh();
      })
      .catch(() => setRefusal({ id: x.id, text: "Not saved: no response. Try again." }))
      .finally(() => setBusy(false));
  };
  const accept = (x: Referral, conds: ReferralCondition[] = []) =>
    answer(
      x,
      conds.length ? { response: "accept_with_conditions", conditions: conds } : { response: "accept" },
      "accepted",
      conds.length ? `Accepted with conditions. ${conds.map((c) => CONDITION_LABELS[c]).join(", ")}. Your unit books her now.` : "Accepted. Your unit books her now.",
      conds.length ? `Accepted with ${conds.length} ${conds.length === 1 ? "condition" : "conditions"}` : "Accepted the referral",
    );
  const returnToGp = (x: Referral, reason: ReferralReturnReason, noteText: string) => {
    const handoff: Handoff = { status: "awaiting_referrer", actBy: handoffActBy(now, x.urgent) };
    const words = REASON_LABELS[reason].toLowerCase();
    const text = noteText.trim().slice(0, RETURN_NOTE_MAX);
    if (!respond && x.kind === "crc") {
      saveSessionReturn({ id: x.id, name: x.name, short: x.short, clinic: x.clinic, unit: x.unit, reason, ...(text && { note: text }), urgent: x.urgent, actBy: handoff.actBy });
    }
    answer(
      x,
      { response: "return", reason, ...(text && { note: text }) },
      "returned",
      `Returned to the ${x.clinic} GP. Reason: ${words}.${text ? " With a note to the GP." : ""} Act by ${formatDay(handoff.actBy)}.`,
      `Returned to the GP: ${words}`,
      handoff,
    );
  };
  const reRefer = (x: Referral) =>
    act(
      x.id,
      "returned",
      `Re-referral to ${x.unit} recorded on this page only. Nothing is saved or sent.`,
      `Re-referred to ${x.unit} (this page only)`,
      { status: "referrer_acted", actBy: x.handoff?.actBy ?? now },
      x.referrer,
    );
  const undo = (id: string) => {
    if (!respond) removeSessionReturn(id);
    pin(id);
    setWant({ k: "heading" });
    setOver((o) => Object.fromEntries(Object.entries(o).filter(([k]) => k !== id)));
  };
  const choose = (m: Exclude<Mode, null>) => {
    setWant({ k: "mode" });
    setMode(m);
    setPicked([]);
    setNote("");
    setTried(false);
  };
  const cancel = (opener: string) => {
    setMode(null);
    setPicked([]);
    setNote("");
    setTried(false);
    setWant({ k: opener });
  };

  const searchHref = (next: string) => href({ q: next.trim(), view: url.view === "inbox" ? "inbox" : undefined, caseId: undefined, page: undefined, from: url.from });

  const top = <div className="mb-3 flex flex-wrap items-center justify-between gap-x-6 gap-y-2">{scopeBar}</div>;

  if (url.view === "inbox") {
    return (
      <div ref={root}>
        {top}
        <Directory
          list={list}
          searched={sorted}
          url={url}
          scope={scope}
          href={href}
          caseLink={caseLink}
          referrer={referrer}
          window={window}
          search={(next) => router.push(searchHref(next))}
          clearSearch={searchHref("")}
        />
      </div>
    );
  }

  if (!r) {
    return (
      <div ref={root}>
        {top}
        {url.from && <BackLink href={href({ view: "inbox", caseId: url.caseId ?? undefined, page: url.page ?? undefined })} />}
        <section aria-label="Open referral" className="rounded-card border border-line bg-surface">
          <PageHeader
            attached
            title={<span data-focus="heading" tabIndex={-1} className="outline-none">{url.caseId ? "Referral unavailable" : heading.title}</span>}
          />
          <div className="p-4 sm:p-pad">
            <h2 className="m-0 text-section font-medium">{url.caseId ? "This referral is not in this view" : "Nothing to open"}</h2>
            <p className="m-0 mt-2 text-muted">
              {url.caseId ? "It may have been answered elsewhere, or the search no longer includes it." : "This view is empty."}{" "}
            </p>
            {(url.q || url.status || url.caseId) && (
              <Link href={inboxHref(scope, {})} className="mt-2 inline-flex min-h-11 items-center underline">
                Clear the search
              </Link>
            )}
          </div>
        </section>
      </div>
    );
  }

  const status = referralStatus(r, referrer);
  const returned = r.state === "returned";
  const late = r.state === "new" && r.waiting > window;
  const outcome = over[r.id]?.outcome;
  const fromName = r.referrer;
  const backUrl = href({ view: "inbox", caseId: r.id, page: url.page ?? undefined });
  const gpView = `/app/referrals/returned?${new URLSearchParams([...new URLSearchParams(scope), ["case", r.id], ["back", backUrl]]).toString()}`;

  /* The status line under the current party: status, case reference and how long it has waited. */
  const stateLine = (
    <div className="flex flex-wrap items-center justify-center gap-x-2.5 gap-y-1 max-md:justify-start">
      <StatusChip label={status.label} tone={status.tone} plain />
      <span className="border-l border-rule pl-2.5 text-small tabular-nums text-muted" title={r.id}>
        {r.short}
      </span>
      {(r.state === "new" || r.state === "grade") && (
        <span data-testid="lead-days" data-value={r.waiting} className={cn("border-l border-rule pl-2.5 text-small tabular-nums", late ? "font-medium text-negative" : "text-muted")}>
          {r.waiting} {r.waiting === 1 ? "day" : "days"} waiting{r.state === "new" && (late ? `, ${r.waiting - window} past the unit's ${window}` : `, answer within ${window}`)}
        </span>
      )}
    </div>
  );

  const outcomeBlock = (
    <>
      <AnimatePresence initial={false}>
        {outcome && (
          <motion.div key="outcome" role="status" initial={reduce ? false : { opacity: 0, y: -4 }} animate={{ opacity: 1, y: 0 }} exit={{ opacity: 0 }} transition={{ duration: reduce ? 0 : 0.15 }} className="mt-3 flex flex-wrap items-center justify-center gap-gap max-md:justify-start">
            <p className="m-0 max-w-prose">{outcome}</p>
            {!over[r.id]?.saved && (
              <Button variant="quiet" size="touch" data-focus="undo" onClick={() => undo(r.id)}>
                Undo
              </Button>
            )}
          </motion.div>
        )}
      </AnimatePresence>
      {refusal?.id === r.id && (
        <p role="alert" className="m-0 mt-3 text-negative">
          {refusal.text}
        </p>
      )}
    </>
  );

  const unitPanel = (
    <div className="mt-4 w-full">
      {r.flags.length > 0 && r.state !== "done" && (
        <p className="m-0 mb-3 text-body" data-testid="flags">
          <span className="font-medium">Check first:</span> <span className="text-muted">{r.flags.join(" · ")}</span>
        </p>
      )}
      {r.state === "new" && mode === null && (
        <div className="flex flex-wrap justify-center gap-2 max-md:justify-start">
          <Button variant="cta" size="touch" aria-label="Accept referral" onClick={() => accept(r)}>
            <Check aria-hidden /> Accept
          </Button>
          <Button variant="quiet" size="touch" data-focus="open-ret" aria-label="Return with reason" onClick={() => choose("ret")}>
            <Undo2 aria-hidden /> Return
          </Button>
          <Button variant="quiet" size="touch" data-focus="open-cond" onClick={() => choose("cond")}>
            Accept with conditions
          </Button>
        </div>
      )}
      {r.state === "new" && mode === "cond" && (
        <div className="mx-auto w-full max-w-[520px] text-start">
          <h3 data-focus="mode" tabIndex={-1} className="m-0 text-body font-semibold outline-none">
            Accept on these conditions
          </h3>
          <div role="group" aria-label="Conditions" className="mt-2 flex flex-wrap gap-2">
            {CONDITIONS.map(([c, label]) => (
              <button
                key={c}
                type="button"
                aria-pressed={picked.includes(c)}
                className={chip(picked.includes(c))}
                onClick={() => setPicked((p) => (p.includes(c) ? p.filter((x) => x !== c) : [...p, c]))}
              >
                {label}
              </button>
            ))}
          </div>
          <div className="mt-3 flex flex-wrap gap-2">
            <Button variant="cta" size="touch" onClick={() => accept(r, picked as ReferralCondition[])}>
              {picked.length ? `Accept with ${picked.length} ${picked.length === 1 ? "condition" : "conditions"}` : "Accept with no conditions"}
            </Button>
            <Button variant="quiet" size="touch" onClick={() => cancel("open-cond")}>
              Cancel
            </Button>
          </div>
        </div>
      )}
      {r.state === "new" && mode === "ret" && (
        <form
          noValidate
          aria-label="Return to the GP"
          className="mx-auto grid w-full max-w-[520px] gap-2 text-start"
          onSubmit={(e) => {
            e.preventDefault();
            setTried(true);
            if (!picked[0]) return;
            returnToGp(r, picked[0] as ReferralReturnReason, note);
          }}
        >
          <fieldset className="m-0 border-0 p-0" aria-describedby={tried && !picked[0] ? reasonError : undefined} aria-invalid={tried && !picked[0] ? true : undefined}>
            <legend data-focus="mode" tabIndex={-1} className="mb-1 text-body font-semibold outline-none">
              Reason for return
            </legend>
            <div className="grid gap-0.5">
              {REASONS.map(([c, label]) => (
                <label key={c} className="flex min-h-11 cursor-pointer items-center gap-2.5">
                  <input
                    type="radio"
                    name={`reason-${r.id}`}
                    value={c}
                    checked={picked[0] === c}
                    onChange={() => setPicked([c])}
                    className="size-5 flex-none cursor-pointer accent-jade-700"
                  />
                  {label}
                </label>
              ))}
            </div>
          </fieldset>
          {tried && !picked[0] && (
            <p id={reasonError} role="alert" className="m-0 font-medium text-negative">
              Choose a reason before you return it. Nothing was sent.
            </p>
          )}
          <label className="mt-1 grid gap-1">
            <span className="text-body font-semibold">
              Note to the GP <span className="font-normal text-muted">(optional)</span>
            </span>
            <textarea
              name="note"
              rows={3}
              maxLength={RETURN_NOTE_MAX}
              value={note}
              aria-describedby={noteHint}
              onChange={(e) => setNote(e.target.value)}
              className="w-full rounded-control border border-control bg-surface px-3 py-2 text-body text-ink"
            />
            <span id={noteHint} className="text-small text-muted">
              What the reason does not say, such as a missing report. {RETURN_NOTE_MAX - note.length} of {RETURN_NOTE_MAX} characters left.
            </span>
          </label>
          <p className="m-0 text-small text-muted">
            Returning hands the case back to the {r.clinic} GP, who must act by {formatDay(handoffActBy(now, r.urgent))}. Booking stays held.
          </p>
          <div className="flex flex-wrap gap-2">
            <Button type="submit" variant="cta" size="touch" disabled={busy}>
              Return to GP
            </Button>
            <Button type="button" variant="quiet" size="touch" onClick={() => cancel("open-ret")}>
              Cancel
            </Button>
          </div>
        </form>
      )}
      {r.state === "grade" && (
        <div className="text-start">
          <p className="m-0">The report does not state the prep or that the caecum was reached. Grade the prep from your procedure record.</p>
          <div role="group" aria-label="Boston grade" className="mt-2 flex flex-wrap gap-2">
            {GRADES.map(([n, w]) => (
              <button
                key={n}
                type="button"
                onClick={() => act(r.id, "grade", `Graded ${n}, ${w.toLowerCase()}. The case stays held until the report is complete.`, `Graded the prep ${n}`)}
                className="flex h-14 min-w-16 cursor-pointer flex-col items-center justify-center rounded-chip border border-control bg-surface px-3 hover:bg-ground"
              >
                <span className="font-medium">{n}</span>
                <span className="text-small text-muted">{w}</span>
              </button>
            ))}
          </div>
          <p className="m-0 mt-2 text-small text-muted">A grade never closes a case. Only a complete report does.</p>
        </div>
      )}
      {!outcome && (r.state === "accepted" || r.state === "done") && (
        <p className="m-0" role="status">
          {r.state === "done" ? "Closed with evidence. Nothing needed from you." : r.answer ? `${r.answer}. Nothing needed from you.` : "Accepted. Nothing needed from you."}
        </p>
      )}
      {outcomeBlock}
    </div>
  );

  const words = handoffWords(r.handoff, referrer);
  const referrerPanel = (
    <div className="mt-4 grid w-full justify-items-center gap-1 max-md:justify-items-start">
      <strong className="text-body font-semibold">{r.kind === "crc" ? "Booking held" : `Held with the ${referrer}`}</strong>
      {!outcome && <p className="m-0 text-muted">{r.answer ? `${r.answer}.` : `Returned to the ${referrer}.`}</p>}

      {(r.note || r.hasNote) && (
        <p className="m-0 max-w-prose whitespace-pre-line break-words text-start" data-testid="return-note">
          <span className="font-medium">Note to the {referrer}:</span> {r.note ?? "could not load this case's detail. Reload to try again."}
        </p>
      )}
      {r.acknowledged && (
        <small className="flex items-center gap-1.5 text-small text-jade-700">
          <Check aria-hidden className="size-4" /> The {referrer} acknowledged the return
        </small>
      )}
      {r.kind === "packet" && r.handoff && r.handoff.status !== "referrer_acted" && !outcome && (
        <div className="mt-2 grid justify-items-center gap-1 max-md:justify-items-start">
          <Button variant="quiet" size="touch" onClick={() => reRefer(r)}>
            Re-refer to {r.unit}
          </Button>
          <p className="m-0 text-small text-muted">No other unit offers this, so the referral goes back to the same unit. Recorded on this page only.</p>
        </div>
      )}
      {outcomeBlock}
      <p className="m-0 text-small text-muted">
        {handoffOwner(r.handoff, r.referrer)}: {words.what.toLowerCase()}, {words.when}
      </p>
      <Link href={gpView} className="inline-flex min-h-11 items-center gap-1 font-medium">
        Open the {referrer} view <ChevronRight aria-hidden className="size-4" />
      </Link>
      {!respond && r.kind === "crc" && r.kept === "browser" && (
        <p className="m-0 flex flex-wrap items-center justify-center gap-x-2 gap-y-1 text-small text-muted max-md:justify-start">
          <HonestyLabel plain kind="designed">Kept in this browser only</HonestyLabel>
          <span>The {referrer} view in this browser shows this return. Nothing is sent or saved.</span>
        </p>
      )}
      {!respond && r.kind === "crc" && over[r.id] && r.kept !== "browser" && (
        <p className="m-0 text-small text-muted">This browser could not keep the return, so the {referrer} view cannot show it. It stays on this page only.</p>
      )}
    </div>
  );

  const preview = previewOf(sorted.length === list.length ? sorted : list, r.id);
  const counted = list.length;
  const viewAll = url.from ? backUrl : href({ view: "inbox", caseId: r.id });

  return (
    <div ref={root}>
      {top}
      {url.from && <BackLink href={backUrl} />}
      <section aria-label="Open referral" data-case={r.id} className="overflow-hidden rounded-card border border-line bg-surface">
        <PageHeader
          attached
          title={
            <span data-focus="heading" tabIndex={-1} aria-label={`${heading.title}, ${r.short}`} className="outline-none">
              {heading.title}
            </span>
          }
          titleAside={
            <>
              <button
                type="button"
                aria-expanded={packetOpen}
                aria-controls="rf-packet"
                aria-label="Referral details"
                title="Referral details"
                onClick={() => setPacketOpen((o) => !o)}
                className={cn("grid size-11 cursor-pointer place-items-center rounded-control bg-transparent text-jade-700 hover:bg-jade-100", packetOpen && "bg-jade-100 text-jade-900")}
              >
                <FolderOpen aria-hidden className="size-5" strokeWidth={1.6} />
              </button>
              {heading.indication && <span className="rounded-full bg-jade-100 px-2.5 py-1 text-small font-semibold text-jade-800">{heading.indication}</span>}
            </>
          }
          sub={
            <>
              <span className="font-semibold text-ink">{r.name}</span>
              {r.nameAr && (
                <>
                  {" "}
                  <span lang="ar" dir="rtl">
                    {r.nameAr}
                  </span>
                </>
              )}
              {" · "}
              {[r.age, r.kind === "packet" ? `from ${r.clinic}` : `from ${r.clinic} Healthcare Centre`, `sent ${r.sent}`].filter(Boolean).join(" · ")}
            </>
          }
          aside={
            r.hasRecord ? (
              <RecordAvatar id={r.id} name={r.name} linked>
                <Avatar person={r.seed} size={36} label="" />
              </RecordAvatar>
            ) : undefined
          }
        />
        <dl id="rf-packet" hidden={!packetOpen} className="m-0 grid grid-cols-2 gap-x-6 gap-y-3 border-b border-line bg-limestone-100 px-[clamp(16px,2.8vw,42px)] py-3.5 sm:grid-cols-3 lg:grid-cols-4">
          {r.details.map(([k, v]) => (
            <div key={k} className="min-w-0">
              <dt className="text-small text-muted">{k}</dt>
              <dd className="m-0 break-words font-semibold">{v}</dd>
            </div>
          ))}
        </dl>
        <div
          role="group"
          aria-label={`Hand-off from ${returned ? r.unit : fromName} to ${returned ? fromName : r.unit}`}
          data-returned={returned}
          className="grid grid-cols-[minmax(0,1fr)_clamp(40px,8vw,120px)_minmax(0,1fr)] bg-gradient-to-b from-limestone-50 to-surface px-[clamp(20px,2.8vw,42px)] py-[clamp(24px,4.5vh,54px)] max-md:grid-cols-1 max-md:px-4 max-md:py-[18px]"
        >
          <Station name={fromName} icon="clinic" current={returned} line={returned ? stateLine : null}>
            {returned && referrerPanel}
          </Station>
          <div aria-hidden className="grid h-[clamp(70px,7vw,108px)] place-items-center self-start text-jade-600 max-md:h-9 max-md:w-11">
            <ArrowRight className={cn("size-6", returned ? "rotate-180 max-md:-rotate-90" : "max-md:rotate-90")} strokeWidth={1.7} data-testid="handoff-arrow" data-direction={returned ? "back" : "forward"} />
          </div>
          <Station name={r.unit} icon="hospital" current={!returned} line={!returned ? stateLine : null}>
            {!returned && unitPanel}
          </Station>
        </div>
        <div className="grid border-t border-line bg-limestone-100 md:grid-cols-[minmax(0,.85fr)_minmax(0,1.15fr)]">
          <section id="worklist" tabIndex={-1} aria-label="Referral inbox" className="flex min-w-0 flex-col outline-none max-md:order-2">
            <h2 className="m-0 flex min-h-12 items-center gap-2 px-[clamp(16px,2.8vw,42px)] text-body font-semibold text-jade-800">
              <Inbox aria-hidden className="size-[17px]" />
              Inbox <span className="text-small font-normal tabular-nums text-muted">{counted}</span>
              <Link href={viewAll} aria-label={`View all ${counted} referrals`} className="ml-auto inline-flex min-h-11 items-center gap-1.5 text-small font-semibold text-jade-700 no-underline hover:underline" onClick={shallow(viewAll)}>
                View all <ChevronRight aria-hidden className="size-4" />
              </Link>
            </h2>
            <ul className="m-0 grid flex-1 list-none content-start auto-rows-[minmax(56px,auto)] p-0">
              {preview.map((x) => {
                const s = referralStatus(x, referrer);
                const on = x.id === r.id;
                const target = caseLink(x.id);
                return (
                  <li key={x.id} className="flex">
                    <a
                      href={target}
                      aria-current={on ? "true" : undefined}
                      onClick={shallow(target)}
                      className={cn(
                        "flex w-full flex-wrap items-center gap-x-2.5 gap-y-1 border-t border-rule px-[clamp(16px,2.8vw,42px)] py-2.5 text-ink no-underline hover:bg-jade-50",
                        on ? "bg-jade-100 shadow-[inset_3px_0_var(--color-jade-600)]" : "bg-surface",
                      )}
                    >
                      <span className="font-semibold tabular-nums text-ink">{x.short}</span>
                      {s.key !== "needs" ? <small className="text-small text-jade-700">{s.label}</small> : <span className="sr-only">{s.label}</span>}
                      <span className="ml-auto whitespace-nowrap text-small text-muted">{x.sent}</span>
                    </a>
                  </li>
                );
              })}
            </ul>
          </section>
          <Activity r={r} over={over[r.id]} referrer={referrer} />
        </div>
      </section>
    </div>
  );
}

const chip = (on: boolean) => cn("h-11 cursor-pointer rounded-chip border px-4 text-body text-ink", on ? "border-ink bg-active" : "border-control bg-surface hover:bg-ground");

/** A plain left click moves within the inbox without a server round trip; a modified click opens a new tab as usual. */
function shallow(href: string) {
  return (e: React.MouseEvent<HTMLAnchorElement>) => {
    if (e.defaultPrevented || e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return;
    e.preventDefault();
    shallowGo(href);
  };
}

function BackLink({ href }: { href: string }) {
  return (
    <a href={href} onClick={shallow(href)} className="mb-2 inline-flex min-h-11 items-center gap-1.5 font-semibold text-jade-800 no-underline hover:underline">
      <ChevronRight aria-hidden className="size-4 rotate-180" /> Back to inbox
    </a>
  );
}

function StatusChip({ label, tone, plain = false }: { label: string; tone: StatusTone; plain?: boolean }) {
  return (
    <span
      data-tone={tone}
      className={cn(
        "inline-flex items-center gap-1.5 text-small font-semibold",
        TONE[tone],
        !plain && { pending: "bg-ochre-wash", held: "bg-clay-wash", done: "bg-jade-100" }[tone],
        !plain && "rounded-full px-2 py-0.5",
      )}
    >
      <i aria-hidden className="size-1.5 flex-none rounded-full bg-current" />
      {label}
    </span>
  );
}

function Station({ name, icon, current, line, children }: { name: string; icon: "clinic" | "hospital"; current: boolean; line: React.ReactNode; children?: React.ReactNode }) {
  const Icon = icon === "clinic" ? House : Hospital;
  return (
    <section
      aria-label={current ? `${name}, current` : name}
      className="flex min-w-0 flex-col items-center text-center max-md:grid max-md:grid-cols-[44px_minmax(0,1fr)] max-md:items-center max-md:gap-x-3 max-md:text-start"
    >
      <span className={cn("grid size-[clamp(70px,7vw,108px)] place-items-center rounded-[30%] max-md:size-11 max-md:self-start", current ? "bg-jade-100 text-jade-700" : "bg-limestone-100 text-limestone-700")}>
        <Icon aria-hidden className="size-[53%] max-md:size-[25px]" strokeWidth={1.25} />
      </span>
      <div className="mt-[15px] flex min-w-0 flex-col items-center gap-2 max-md:mt-0 max-md:items-start max-md:gap-1">
        <h2 className="staff-display m-0 break-words text-[clamp(22px,2.4vw,36px)] leading-[1.13] text-jade-900">{name}</h2>
        {line}
      </div>
      {children && <div className="flex w-full min-w-0 justify-center max-md:col-start-2 max-md:justify-start">{children}</div>}
    </section>
  );
}

/** Three rows around the open referral, so it is always in view; the full inbox pages through the rest. */
function previewOf<T extends { id: string }>(rows: T[], id: string): T[] {
  const i = Math.max(0, rows.findIndex((x) => x.id === id));
  const start = Math.min(Math.floor(i / PREVIEW) * PREVIEW, Math.max(0, rows.length - PREVIEW));
  return rows.slice(start, start + PREVIEW);
}

const PHONE = "(max-width: 767px)";
const isPhone = () => globalThis.matchMedia?.(PHONE).matches ?? false;
function subscribePhone(onChange: () => void) {
  const m = globalThis.matchMedia?.(PHONE);
  m?.addEventListener?.("change", onChange);
  return () => m?.removeEventListener?.("change", onChange);
}

type Entry = { id: string; icon: React.ComponentType<{ className?: string; "aria-hidden"?: boolean }>; what: string; who: string; when: string; now?: boolean };

function Activity({ r, over, referrer }: { r: Referral; over: Override | undefined; referrer: string }) {
  /* Open on desktop and closed on a phone until chosen: CSS decides first, so nothing moves on load. */
  const [open, setOpen] = useState<boolean | null>(null);
  const phone = useSyncExternalStore(subscribePhone, isPhone, () => false);
  const expanded = open ?? !phone;
  const entries: Entry[] = [
    { id: "ref", icon: FileText, what: "Referral sent", who: r.referrer, when: r.sent },
    ...(r.answer ? [{ id: "answer", icon: r.state === "returned" ? Undo2 : Check, what: r.answer, who: r.unit, when: r.answerWhen }] : []),
    ...(r.acknowledged ? [{ id: "ack", icon: Check, what: `The ${referrer} acknowledged the return`, who: r.referrer, when: r.kept === "browser" ? "Kept in this browser" : "Saved to the case record" }] : []),
    // A return kept in this browser is already the answer entry above; its session log line would repeat it.
    ...(over?.saved || r.kept === "browser" ? [] : (over?.log ?? [])).map(({ who, what }, i) => ({ id: `log-${i}`, icon: what.startsWith("Returned") ? Undo2 : Check, what, who, when: "This session" })),
    ...(r.state === "done"
      ? []
      : [
          {
            id: "now",
            icon: Clock,
            now: true,
            who: r.state === "returned" ? handoffOwner(r.handoff, r.referrer) : r.state === "grade" ? "You" : r.unit,
            what:
              r.state === "returned"
                ? handoffWords(r.handoff, referrer).what
                : { new: "Your answer", grade: over ? "Complete the report" : "Review the incomplete report", accepted: "Book and do the procedure", done: "", returned: "" }[r.state],
            when: r.state === "returned" ? handoffWords(r.handoff, referrer).when : `${r.waiting} ${r.waiting === 1 ? "day" : "days"} since referral`,
          },
        ]),
  ];
  return (
    <section aria-label="Activity" className="min-w-0 border-line px-[clamp(16px,2.8vw,42px)] md:border-l max-md:order-1 max-md:border-b">
      <h2 className="m-0">
        <button
          type="button"
          aria-expanded={expanded}
          aria-controls="rf-activity"
          onClick={() => setOpen(!expanded)}
          className="flex min-h-12 cursor-pointer items-center gap-2 bg-transparent text-body font-semibold text-jade-800"
        >
          <History aria-hidden className="size-[17px]" /> Activity <ChevronDown aria-hidden className={cn("size-4 transition-transform", expanded && "rotate-180")} />
        </button>
      </h2>
      <ol
        id="rf-activity"
        aria-label="Hand-off activity"
        aria-live="polite"
        className={cn("m-0 list-none p-0 pb-3", open === null ? "max-md:hidden" : open ? "" : "hidden")}
      >
        {entries.map((e) => (
          <li key={e.id} className="grid grid-cols-[19px_minmax(0,1fr)_auto] items-baseline gap-2 border-t border-rule py-[clamp(8px,1.25vh,16px)]">
            <e.icon aria-hidden className={cn("size-[17px] translate-y-[3px]", e.now ? "text-ochre-ink" : "text-jade-600")} />
            <span className="min-w-0">
              <span className="block font-semibold">{e.now ? `Now: ${e.what}` : e.what}</span>
              <span className="block text-small text-muted">{e.who}</span>
            </span>
            <span className="text-end text-small tabular-nums text-muted">{e.when}</span>
          </li>
        ))}
      </ol>
    </section>
  );
}

function Directory({
  list,
  searched,
  url,
  scope,
  href,
  caseLink,
  referrer,
  window,
  search,
  clearSearch,
}: {
  list: Referral[];
  /** Every referral the search leaves, before the status filter. */
  searched: Referral[];
  url: InboxUrl;
  scope: string;
  href: (s: Partial<InboxUrl>) => string;
  caseLink: (id: string) => string;
  referrer: string;
  window: number;
  search: (q: string) => void;
  /** The inbox URL without the search, opened when the field's committed search is cleared. */
  clearSearch: string;
}) {
  const requested = url.page ?? pageOfId(list, url.caseId, INBOX_PAGE) ?? 1;
  const paging = paginate(list, requested, INBOX_PAGE);
  const filtered = !!(url.q || url.status);
  const total = searched.length;
  const counts: Partial<Record<ReferralStatus, number>> = {};
  for (const x of searched) {
    const k = referralStatus(x, referrer).key;
    counts[k] = (counts[k] ?? 0) + 1;
  }
  return (
    <section id="worklist" tabIndex={-1} aria-label="Referral inbox" className="rounded-card border border-line bg-surface outline-none">
      <PageHeader
        attached
        title={
          <span data-focus="heading" tabIndex={-1} className="outline-none">
            Referral inbox
          </span>
        }
        sub={`${total} ${total === 1 ? "referral" : "referrals"}${url.q ? ` matching “${url.q}”` : ""}`}
      />
      <Toolbar label="Filter referrals" className="bg-limestone-100 px-[clamp(14px,2.4vw,32px)] py-3">
        <div className="flex min-w-0 flex-wrap items-center gap-2">
          <form
            role="search"
            action="/app/referrals"
            className="min-w-0 basis-full sm:max-w-[34rem] sm:flex-1 sm:basis-64"
            onSubmit={(e) => {
              e.preventDefault();
              search(String(new FormData(e.currentTarget).get("q") ?? ""));
            }}
          >
            {[...new URLSearchParams(scope), ["view", "inbox"]].map(([k, v]) => (
              <input key={k} type="hidden" name={k} value={v} />
            ))}
            <SearchField label="Search case id" name="q" defaultValue={url.q} clearHref={clearSearch} placeholder="Case id, then Enter" />
          </form>
          <FilterMenu
            label="Status"
            value={url.status || undefined}
            options={STATUS_FILTERS.map((k) => ({
              value: k,
              label: statusFilterLabel(k, referrer),
              count: counts[k] ?? 0,
              href: href({ view: "inbox", status: k, page: undefined, caseId: url.caseId ?? undefined }),
            }))}
            clearHref={href({ view: "inbox", status: "", page: undefined, caseId: url.caseId ?? undefined })}
          />
        </div>
      </Toolbar>
      {paging.rows.length === 0 ? (
        <div className="p-6 text-center">
          <p role="status" className="m-0 mb-2.5">
            No matching referrals
          </p>
          {filtered && (
            <Link href={href({ view: "inbox", q: "", status: "" })} className="inline-flex min-h-11 items-center rounded-control border border-control px-3 text-ink no-underline hover:bg-active">
              Clear filters
            </Link>
          )}
        </div>
      ) : (
        <table className="w-full table-fixed border-collapse text-start">
          <caption className="sr-only">Referral inbox, {pageStatusText(paging)}</caption>
          <thead>
            <tr className="bg-limestone-100 text-small text-muted">
              {["Referral", "From", "Status", "Sent"].map((h, i) => (
                <th key={h} scope="col" className={cn("border-b border-rule px-[clamp(8px,2.4vw,32px)] py-2.5 text-start font-medium", i === 1 && "max-md:hidden", i === 0 && "w-[34%]")}>
                  {h}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {paging.rows.map((x) => {
              const s = referralStatus(x, referrer);
              const on = x.id === url.caseId;
              const target = caseLink(x.id);
              const late = x.state === "new" && x.waiting > window;
              return (
                <tr key={x.id} className={cn("relative h-12 hover:bg-jade-50 focus-within:bg-jade-50", on && "bg-jade-100")}>
                  <th scope="row" className="border-b border-rule px-[clamp(8px,2.4vw,32px)] py-2.5 text-start align-middle font-normal">
                    <a
                      href={target}
                      onClick={shallow(target)}
                      data-focus={on ? "selected-row" : undefined}
                      aria-current={on ? "true" : undefined}
                      className="flex min-h-11 items-center font-semibold tabular-nums text-ink no-underline after:absolute after:inset-0 after:content-[''] focus-visible:outline-none focus-visible:after:outline-2 focus-visible:after:-outline-offset-2 focus-visible:after:outline-jade-600"
                    >
                      {x.short}
                    </a>
                    {x.name !== x.short && <span className="block truncate text-small text-muted">
                      {x.name}
                      {x.nameAr && (
                        <>
                          {" · "}
                          <span lang="ar" dir="rtl">
                            {x.nameAr}
                          </span>
                        </>
                      )}
                    </span>}
                  </th>
                  <td className="break-words border-b border-rule px-[clamp(8px,2.4vw,32px)] py-2.5 max-md:hidden">{x.referrer}</td>
                  <td className="border-b border-rule px-[clamp(8px,2.4vw,32px)] py-2.5">
                    <StatusChip label={s.label} tone={s.tone} />
                  </td>
                  <td className="border-b border-rule px-[clamp(8px,2.4vw,32px)] py-2.5 text-small text-muted">
                    {x.sent}
                    {(x.state === "new" || x.state === "grade") && <span className={cn("block tabular-nums", late && "font-medium text-negative")}>{x.waiting} {x.waiting === 1 ? "day" : "days"}</span>}
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      )}
      <div className="flex min-h-14 items-center px-[clamp(14px,2.4vw,32px)] py-2 text-small text-muted">
        {paging.pages > 1 ? (
          <PagerLinks
            paging={paging}
            label="Inbox pages"
            testId="page-range"
            summary={<span aria-live="polite" aria-atomic="true">{`${paging.from}–${paging.to} of ${list.length} referrals`}</span>}
            hrefFor={(p) => href({ view: "inbox", page: p, caseId: url.caseId ?? undefined })}
            onFollow={shallow}
          />
        ) : (
          <span data-testid="page-range" aria-live="polite" aria-atomic="true">
            {list.length} {list.length === 1 ? "referral" : "referrals"}
          </span>
        )}
      </div>
    </section>
  );
}


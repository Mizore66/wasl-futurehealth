"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { Button } from "@/components/ui/button";
import { HonestyLabel } from "@/components/wasl/honesty-label";
import { cn } from "@/lib/utils";
import { formatDay } from "@/lib/domain/day";
import type { ReferrerAnswer, RespondResult } from "@/app/app/referrals/actions";
import { INFO_LABELS, handoffWords } from "./referrals";
import { afterAnswerWords, DISPOSITION_LABELS, EMPTY_DRAFT, referrerRequest, sessionPacket, type Disposition, type Draft, type ReturnedPacket } from "./returned-packets";
import { acknowledgeSessionReturn, useSessionReturns } from "./session-returns";

const DISPOSITIONS = Object.entries(DISPOSITION_LABELS) as [Disposition, string][];
const REFUSED: Record<number, string> = {
  403: "your clinician account is not this patient's referring clinician.",
  409: "the referral changed since this page loaded. Reload and answer again.",
  500: "the case record could not be read. Try again later.",
};
const field = "w-full rounded-control border border-control bg-surface px-3 py-2 text-body text-ink";

/** A live case without a synthetic name falls back to its case ID; show that ID once so the heading is not read twice. */
export function packetHeading(p: Pick<ReturnedPacket, "name" | "short">): { name: string; caseId: string | null } {
  return { name: p.name, caseId: p.name.trim() === p.short ? null : p.short };
}

/**
 * Fixtures mode, CRC: the returns the receiving unit made in this browser session, which the GP may acknowledge here.
 * Both stay in this browser's sessionStorage (session-returns.ts); nothing is sent, saved or seen by another browser.
 */
export function SessionReferrerHandoffs({ referrer = "GP", focus }: { referrer?: string; focus?: string }) {
  const kept = useSessionReturns();
  const packets = Object.values(kept)
    .map(sessionPacket)
    .sort((a, b) => Number(a.acknowledged) - Number(b.acknowledged) || a.actBy - b.actBy);
  const shown = focus ? [...packets.filter((p) => p.id === focus), ...packets.filter((p) => p.id !== focus)] : packets;
  return (
    <section aria-label="Returns kept in this browser" className="flex flex-col gap-gap">
      <p className="m-0 flex flex-wrap items-center gap-x-2 gap-y-1 text-small text-muted">
        <HonestyLabel plain kind="designed">Kept in this browser only</HonestyLabel>
        <span>Returns made in the referral inbox in this browser. An acknowledgement here stays in this browser too. Nothing is sent to the {referrer} or saved.</span>
      </p>
      {packets.length ? (
        <ReferrerHandoffs packets={shown} referrer={referrer} focus={focus} />
      ) : (
        <p className="m-0 rounded-card border border-dashed border-control px-3 py-2.5 text-muted">No returns in this browser yet. Return a referral from the referral inbox to see it here.</p>
      )}
    </section>
  );
}

/** The referrer's returned referrals: acknowledge the exact packet, then resolve it with one structured answer. */
export function ReferrerHandoffs({
  packets,
  answer,
  referrer = "GP",
  focus,
}: {
  packets: ReturnedPacket[];
  /** The referral the user came from: its card is marked. */
  focus?: string;
  answer?: (caseId: string, head: string, packet: string, response: ReferrerAnswer) => Promise<RespondResult>;
  /** Who the packet waits on, in words: "GP" for CRC, "referrer" for lung work-up and DR. */
  referrer?: string;
}) {
  if (!packets.length) return <p className="m-0 rounded-card border border-dashed border-control px-3 py-2.5 text-muted">No returned referrals are waiting on you.</p>;
  return (
    <ul className="m-0 flex list-none flex-col gap-gap p-0">
      {packets.map((p) => (
        <li key={p.id}>
          <PacketCard p={p} answer={answer} referrer={referrer} focused={p.id === focus} />
        </li>
      ))}
    </ul>
  );
}

function PacketCard({
  p,
  answer,
  referrer,
  focused = false,
}: {
  p: ReturnedPacket;
  focused?: boolean;
  answer?: (caseId: string, head: string, packet: string, response: ReferrerAnswer) => Promise<RespondResult>;
  referrer: string;
}) {
  const router = useRouter();
  const [draft, setDraft] = useState<Draft>(EMPTY_DRAFT);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [saved, setSaved] = useState<string | null>(null);
  const [tried, setTried] = useState(false);
  const words = handoffWords({ status: p.status, actBy: p.actBy }, referrer);
  const built = referrerRequest(p, draft);
  const titleId = `packet-${p.short}`;
  const heading = packetHeading(p);

  const send = (request: ReferrerAnswer, done: string) => {
    if (!answer || busy) return;
    setBusy(true);
    setError(null);
    answer(p.id, p.head, p.packet, request)
      .then((res) => {
        if (!res.ok) return setError(`Not saved: ${REFUSED[res.status] ?? res.error}`);
        setSaved(done);
        router.refresh();
      })
      .catch(() => setError("Not saved: no response. Try again."))
      .finally(() => setBusy(false));
  };
  const submit = () => {
    setTried(true);
    if (built.ok) send(built.request, "Answer saved. The referral no longer waits on you.");
  };

  return (
    <article aria-labelledby={titleId} aria-current={focused ? "true" : undefined} className={cn("rounded-card border bg-surface p-4 sm:p-pad", focused ? "border-jade-600 shadow-[inset_3px_0_var(--color-jade-600)]" : "border-line")}>
      <h2 id={titleId} className="m-0 text-section font-semibold">
        {heading.name}
        {heading.caseId && (
          <>
            <span className="sr-only">, case</span> <span className="text-small font-normal text-muted">{heading.caseId}</span>
          </>
        )}
      </h2>
      <dl className="m-0 mt-3 grid grid-cols-1 gap-x-6 gap-y-2 sm:grid-cols-2">
        {(
          [
            ...(p.source === "canonical"
              ? ([["Recommendation", p.detailUnavailable ? "Could not load this case's detail. Reload to try again." : (p.recommendation ?? "Not recorded")]] as const)
              : []),
            ["Priority", p.urgent ? "Urgent" : "Routine"],
            ["Returned by", p.unit],
            ["Reason", p.reason],
            ...(p.source === "canonical" ? ([["Advice from the unit", p.detailUnavailable ? "Could not load this case's detail" : (p.advice ?? "Not recorded")]] as const) : []),
            ...(p.hasNote || p.note ? ([["Note from the unit", p.note ?? "Could not load this case's detail. Reload to try again."]] as const) : []),
            ["Information requested", p.infoRequested.length ? p.infoRequested.map((i) => INFO_LABELS[i]).join("; ") : "None"],
            ["Status", `${words.what}, ${words.when}`],
            ["Acknowledged", p.acknowledged ? "Yes" : "Not yet"],
          ] as const
        ).map(([k, v]) => (
          <div key={k} className={k === "Note from the unit" ? "sm:col-span-2" : undefined}>
            <dt className="text-small text-muted">{k}</dt>
            <dd className={cn("m-0", k === "Note from the unit" && "whitespace-pre-line break-words")}>{v}</dd>
          </div>
        ))}
      </dl>

      {p.answer ? (
        <p role="status" className="m-0 mt-4">
          {p.answer}. {afterAnswerWords(p.answerCode)}
        </p>
      ) : p.kept === "browser" ? (
        <div className="mt-4 grid justify-items-start gap-2">
          {p.acknowledged ? (
            <p role="status" className="m-0">
              Acknowledged in this browser. The return still waits on your answer.
            </p>
          ) : (
            <Button variant="quiet" size="touch" onClick={() => acknowledgeSessionReturn(p.id)}>
              Acknowledge the returned referral
            </Button>
          )}
          <p className="m-0 text-small text-muted">Kept in this browser only. Other answers are not available in this demo.</p>
        </div>
      ) : !answer ? (
        <p className="m-0 mt-4 text-muted">Answers are not available in this demo. Nothing can be recorded here.</p>
      ) : (
        <div className="mt-4">
          {!p.acknowledged && (
            <Button variant="quiet" size="touch" disabled={busy} onClick={() => send({ response: "acknowledge" }, "Acknowledged. The referral still waits on your answer.")}>
              Acknowledge the returned referral
            </Button>
          )}
          <fieldset className="m-0 mt-4 border-0 p-0">
            <legend className="text-section font-medium">Your answer</legend>
            <div className="mt-2 flex flex-col gap-1">
              {DISPOSITIONS.filter(([d]) => d !== "supply_info" || p.infoRequested.length > 0)
                .filter(([d]) => d !== "redirect" || p.others.length > 0)
                .map(([d, label]) => (
                  <label key={d} className="flex min-h-11 cursor-pointer items-center gap-2">
                    {/* WCAG 2.5.8: the radio itself is 24×24 px, and the whole 44 px row is its label. */}
                    <input type="radio" className="size-6 flex-none cursor-pointer accent-ink" name={`answer-${p.id}`} value={d} checked={draft.response === d} onChange={() => setDraft({ ...draft, response: d })} />
                    {d === "re_refer" ? `${label}: ${p.unit}` : label}
                  </label>
                ))}
            </div>
          </fieldset>
          {draft.response === "supply_info" &&
            p.infoRequested.map((i) => (
              <label key={i} className="mt-3 block">
                <span className="text-small text-muted">{INFO_LABELS[i]}</span>
                <textarea required maxLength={1000} className={cn(field, "mt-1")} value={draft.info[i] ?? ""} onChange={(e) => setDraft({ ...draft, info: { ...draft.info, [i]: e.target.value } })} />
              </label>
            ))}
          {draft.response === "redirect" && (
            <label className="mt-3 block">
              <span className="text-small text-muted">Unit to refer to</span>
              <select required className={cn(field, "mt-1 h-11")} value={draft.destination} onChange={(e) => setDraft({ ...draft, destination: e.target.value })}>
                <option value="">Choose a unit</option>
                {p.others.map((o) => (
                  <option key={o.id} value={o.id}>
                    {o.label}
                  </option>
                ))}
              </select>
            </label>
          )}
          {(draft.response === "manage_in_primary_care" || draft.response === "close") && (
            <label className="mt-3 block">
              <span className="text-small text-muted">Reason</span>
              <textarea required maxLength={200} className={cn(field, "mt-1")} value={draft.reason} onChange={(e) => setDraft({ ...draft, reason: e.target.value })} />
            </label>
          )}
          <div className="mt-3 flex flex-wrap items-center gap-gap">
            <Button variant="cta" size="touch" disabled={busy} onClick={submit}>
              Send answer
            </Button>
            {tried && !built.ok && <span role="alert" className="text-negative">{built.missing}</span>}
          </div>
          <p className="m-0 mt-2 text-small text-muted">Respond by {formatDay(p.actBy)}. Answering hands the referral back. It does not close the case or book anything.</p>
        </div>
      )}
      {saved && !error && (
        <p role="status" className="m-0 mt-3">
          {saved}
        </p>
      )}
      {error && (
        <p role="alert" className="m-0 mt-3 text-negative">
          {error}
        </p>
      )}
    </article>
  );
}

import { Mark, Pill } from "@/components/review/marks";
import { HonestyLabel } from "@/components/wasl/honesty-label";
import type { Audit, Programme, UndoProjection } from "@/lib/api/schema";
import type { ReviewAuthMode } from "@/lib/auth/oidc-config";
import { Card } from "./sections";
import { GovernanceControls, GovernanceIdentity, GovernanceSignIn } from "./governance-controls";
import { cn } from "@/lib/utils";
import { protocolRead, READ_LABEL } from "@/lib/domain/stage";
import { isNonbindingLegacyHistory } from "@/lib/domain/review-authority";
import { atWords, bodySummary, entryLabel, ledgerWords, reviewWords, roleWords } from "./words";

/**
 * The chain-check mark. An empty chain passes the server's check vacuously, so it reads as neutral, never as a
 * positive "0 of 0 intact"; a broken chain is negative whatever its length.
 */
export function chainCheck(audit: Pick<Audit, "chain" | "chain_length" | "chain_verified">): { tone: "positive" | "negative" | "muted"; shape: "tick" | "cross" | "dash"; label: string } {
  if (!audit.chain_verified) return { tone: "negative", shape: "cross", label: "Record altered" };
  if (audit.chain_length === 0) return { tone: "muted", shape: "dash", label: "Nothing recorded yet" };
  return { tone: "positive", shape: "tick", label: `Unchanged · ${audit.chain.length} of ${audit.chain_length} entries checked` };
}

/** Whether the server's check passed, with honest wording about what that check is. */
export function VerifiedCard({ audit }: { audit: Audit }) {
  const intact = audit.chain.length === audit.chain_length;
  const mark = chainCheck(audit);
  const empty = audit.chain_verified && audit.chain_length === 0;
  return (
    <section aria-labelledby="verified-h" className="rounded-card border border-line bg-surface p-4 md:p-pad">
      <h2 id="verified-h" className="m-0 text-section font-medium">
        Tamper check
      </h2>
      <p className="m-0 mt-3 flex flex-wrap items-center gap-3" role="status" data-testid="chain-verified">
        <Pill tone={mark.tone} shape={mark.shape}>
          {mark.label}
        </Pill>
        {!intact && (
          <span className="text-negative">
            Only {audit.chain.length} of {audit.chain_length} entries could be shown.
          </span>
        )}
      </p>
      <p className="m-0 mt-3 max-w-[72ch]">
        {empty
          ? "Nothing has been recorded for this case yet, so there is nothing to check."
          : audit.chain_verified
          ? "Every entry was checked against the one before it. No entry was changed or removed after it was recorded."
          : "At least one entry failed the check. Treat this case's record as altered until someone checks it."}
      </p>
      <p className="m-0 mt-2 max-w-[72ch] text-small text-muted">
        This shows the record is unaltered. It does not show the decisions were clinically right.
      </p>
    </section>
  );
}

/** Each sealed entry: what it was, what it says, and how it chains to the one before. */
export function ChainCard({ audit }: { audit: Audit }) {
  return (
    <Card id="chain" title="Recorded entries" note="Entries are locked in order. Changing one would break every entry after it.">
      <ol className="m-0 mt-3 list-none p-0" aria-label="Recorded entries">
        {audit.chain.map((e, i) => (
          <li key={e.hash} className="border-t border-rule py-3" data-entry={e.type}>
            <div className="flex items-baseline gap-3">
              <span className="w-5 flex-none text-right tabular-nums text-muted">{i + 1}</span>
              <div className="min-w-0 flex-1">
                <div className="font-medium">{entryLabel(e.type)}</div>
                <p className="m-0 mt-1">{bodySummary(e)}</p>
              </div>
            </div>
          </li>
        ))}
      </ol>
    </Card>
  );
}

/** Stored against recomputed, with a mark that is a word and a shape, not a colour. */
export function SignaturesCard({ audit }: { audit: Audit }) {
  const rows = audit.decision_signatures;
  const matching = rows.filter((s) => s.sig === s.recomputed).length;
  return (
    <Card
      id="signatures"
      title="Signed decisions"
      aside={
        <span className="text-small text-muted" data-testid="signature-count">
          {matching} of {rows.length} match
        </span>
      }
      note="Each decision is signed when it is made. It is checked again now, and the two must match."
    >
      {rows.length === 0 ? (
        <p className="m-0 mt-3 text-muted">No decisions are signed for this case.</p>
      ) : (
        <ol className="m-0 mt-3 list-none p-0" aria-label="Signed decisions">
          {rows.map((s, i) => {
            const ok = s.sig === s.recomputed;
            return (
              <li key={s.decision_id ?? `decision-${i}`} className="border-t border-rule py-3" data-match={ok}>
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <span className="text-muted">Decision {i + 1}</span>
                  {ok ? (
                    <Mark shape="tick" tone="positive">
                      Matches
                    </Mark>
                  ) : (
                    <Mark shape="cross" tone="negative">
                      Does not match
                    </Mark>
                  )}
                </div>
              </li>
            );
          })}
        </ol>
      )}
    </Card>
  );
}

/** Review actions that /undo accepts as targets; each keeps its own RBAC at the API. */
const GOVERNED_ACTIONS = new Set(["signoff_link", "signoff_evidence", "record_consent"]);

/**
 * `signedIn`: a reviewer-session token the API accepted for this case. Controls act only as that token.
 * `hostSignOut`: the host page already shows a sign-out for this session, so the history does not repeat it.
 */
export type HistoryGovernance = { caseId: string; projection: UndoProjection; signedIn: boolean; hostSignOut?: boolean; mode?: ReviewAuthMode; expiresAt?: string };

type LedgerRow = Audit["history"][number];

/**
 * Which events can be undone, contested or resolved. The page does not know the signed-in role: the API applies the
 * target action's RBAC and the independent-actor rules, and its refusal is shown as-is.
 */
export function ledgerGovernance(audit: Audit, governance?: HistoryGovernance) {
  const status = new Map(governance?.projection.events.map((event) => [event.hash, event.status]));
  const rejectedContests = new Set(
    audit.history.flatMap((event) => {
      if (event.ledger !== "compensation" || event.kind !== "contest_resolution" || event.resolution !== "rejected") return [];
      const contest = audit.history.find((candidate) => candidate.hash === event.target);
      return contest?.ledger === "compensation" ? [contest.target] : [];
    }),
  );
  const governable = (r: LedgerRow): "review" | "contest" | undefined => {
    if (!governance || (r.ledger !== "review" && r.ledger !== "compensation") || !GOVERNED_ACTIONS.has(r.action)) return undefined;
    const eventStatus = status.get(r.hash);
    if (r.ledger === "review" && !protocolRead(r) && eventStatus === "active") return "review";
    if (r.ledger === "compensation" && r.kind === "contest" && eventStatus === "open") return "contest";
    return undefined;
  };
  return { status, rejectedContests, governable, anyGovernable: audit.history.some((r) => governable(r)), controls: !!governance?.signedIn };
}

/** The event's name as the history lists it. */
export function LedgerTitle({ r, programme }: { r: LedgerRow; programme: Programme | "" }) {
  return r.ledger === "review" ? (
    <>
      {protocolRead(r) && <span className="mr-2 text-muted" data-protocol-read={protocolRead(r)}>{READ_LABEL[protocolRead(r)!]}:</span>}
      {isNonbindingLegacyHistory(r) ? "Older entry — not binding" : r.decision ? reviewWords(r.decision, programme) : roleWords(r.action)}
    </>
  ) : (
    /* Coordinator actions and undo/contest events share the chain: named by their real type/kind. */
    <span className={cn(r.ledger === "unknown" && "text-warning")}>{ledgerWords(r)}</span>
  );
}

/** Who, when, note, links and hashes of one ledger event. */
export function LedgerFacts({ audit, index, programme }: { audit: Audit; index: number; programme: Programme | "" }) {
  const r = audit.history[index];
  const target = r.ledger === "compensation" ? r.target : "";
  const resolving = audit.history.filter((event) => event.ledger === "compensation" && event.target === r.hash);
  return (
    <>
      {r.ledger === "review" && isNonbindingLegacyHistory(r) && (
        <p className="m-0 mt-1 text-small text-muted">Originally recorded as: {r.decision ? reviewWords(r.decision, programme) : "not recorded"}. This entry does not settle a review item.</p>
      )}
      {/* The auditor's redacted history never names the actor or quotes the note. */}
      <p className="m-0 text-muted">
        {r.actor ? `${r.actor} · ` : ""}
        {r.role ? roleWords(r.role) : "role not recorded"} · {r.at ? atWords(r.at) : "time not recorded"}
      </p>
      {r.ledger !== "unknown" && r.ledger !== "coordinator" && typeof r.note === "string" && r.note && <p className="m-0 mt-1">&ldquo;{r.note}&rdquo;</p>}
      {(target || resolving.length > 0) && (
        <p className="m-0 mt-1 flex flex-wrap gap-x-3 gap-y-1 text-small">
          {target && (
            <a href={`#review-event-${target}`} className="underline">
              {r.ledger === "compensation" && r.kind === "contest_resolution" ? "Contest event" : "Original event"}
            </a>
          )}
          {resolving.map((event) => (
            <a key={event.hash} href={`#review-event-${event.hash}`} className="underline">
              Resolved by: {event.ledger === "compensation" ? event.kind.replaceAll("_", " ") : "a later entry"}
            </a>
          ))}
        </p>
      )}
    </>
  );
}

/** The event's compensation status and, for a signed-in reviewer, its undo/contest/resolve controls. */
export function LedgerControls({ r, governance, gov }: { r: LedgerRow; governance?: HistoryGovernance; gov: ReturnType<typeof ledgerGovernance> }) {
  const eventStatus = gov.status.get(r.hash);
  return (
    <>
      {eventStatus && (
        <p className="m-0 mt-1 text-small font-medium" data-compensation-status={eventStatus}>
          {r.ledger === "review" ? "Review" : r.ledger === "compensation" && r.kind === "contest" ? "Contest" : "Correction"} status: {eventStatus}
        </p>
      )}
      {governance && gov.controls && gov.governable(r) === "review" && (
        <div className="mt-1 grid gap-2 sm:grid-cols-2">
          <GovernanceControls caseId={governance.caseId} target={r.hash} operation="undo" />
          {!gov.rejectedContests.has(r.hash) && <GovernanceControls caseId={governance.caseId} target={r.hash} operation="contest" />}
        </div>
      )}
      {governance && gov.controls && gov.governable(r) === "contest" && <GovernanceControls caseId={governance.caseId} target={r.hash} operation="resolve" />}
    </>
  );
}

/** The projection line and the governance sign-in or identity, shown once above the history. */
export function GovernanceHeader({ governance, gov }: { governance?: HistoryGovernance; gov: ReturnType<typeof ledgerGovernance> }) {
  if (!governance) return null;
  return (
    <>
      <p className="m-0 mt-3 max-w-[72ch] text-small text-muted" role="status">
        Review status: {governance.projection.case_status}. Original decision: {governance.projection.canonical_state}. Undoing or contesting a review does not change it.
      </p>
      {gov.anyGovernable &&
        (gov.controls ? (
          <GovernanceIdentity caseId={governance.caseId} signOut={!governance.hostSignOut} mode={governance.mode} expiresAt={governance.expiresAt} />
        ) : (
          <GovernanceSignIn caseId={governance.caseId} mode={governance.mode} />
        ))}
    </>
  );
}

/** Review actions appended through the API: append-only, each linked to the one before. */
export function HistoryCard({ audit, programme, governance }: { audit: Audit; programme: Programme | ""; governance?: HistoryGovernance }) {
  const gov = ledgerGovernance(audit, governance);
  return (
    <Card
      id="history"
      title="Review history"
      aside={<HonestyLabel plain kind="simulated">Demo actions, not clinical sign-offs</HonestyLabel>}
      note="Reviews, coordinator actions, undos and contests are listed in order. New entries are added below. Old ones are never edited."
    >
      <GovernanceHeader governance={governance} gov={gov} />
      {audit.history.length === 0 ? (
        <p className="m-0 mt-3 text-muted">
          No one has signed this case here yet. The simulated reviewers&rsquo; sign-offs are on the case page.
        </p>
      ) : (
        <ol className="m-0 mt-3 list-none p-0" aria-label="Review history">
          {audit.history.map((r, i) => (
            <li key={r.hash || `unknown-${i}`} id={r.hash ? `review-event-${r.hash}` : undefined} className="scroll-mt-4 border-t border-rule py-3">
              {r.ledger === "review" ? (
                <p className="m-0 font-medium">
                  <LedgerTitle r={r} programme={programme} />
                </p>
              ) : (
                <div className="font-medium">
                  <LedgerTitle r={r} programme={programme} />
                </div>
              )}
              <LedgerFacts audit={audit} index={i} programme={programme} />
              <LedgerControls r={r} governance={governance} gov={gov} />
            </li>
          ))}
        </ol>
      )}
    </Card>
  );
}

const STREAM_WORDS: Record<string, string> = { fhir: "Incoming records" };
const streamWords = (k: string) => STREAM_WORDS[k] ?? (/^out\.p\d+$/.test(k) ? `Outgoing messages, group ${k.slice(5)}` : k);

/** The daily heads the run reports, as the API gave them. */
export function AnchorsCard({ audit }: { audit: Audit }) {
  const entries = Object.entries(audit.anchors);
  return (
    <Card id="anchors" title="Daily record seals" aside={<HonestyLabel plain kind="designed">Designed, not checked here</HonestyLabel>}>
      {entries.length === 0 ? (
        <p className="m-0 mt-3 text-muted">No daily seals for this case.</p>
      ) : (
        <ul className="m-0 mt-3 list-none p-0" aria-label="Daily record seals">
          {entries.map(([k]) => (
            <li key={k} className="border-t border-rule py-2">
              <span className="block">{streamWords(k)}</span>
            </li>
          ))}
        </ul>
      )}
      {audit.anchor_note && (
        <p className="m-0 mt-3 max-w-[72ch] text-muted">{audit.anchor_note}.</p>
      )}
    </Card>
  );
}

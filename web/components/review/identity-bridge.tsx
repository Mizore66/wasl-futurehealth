"use client";

import { useId, useState } from "react";
import { Check, ChevronDown, FileText, TriangleAlert } from "lucide-react";
import { Avatar, type AvatarState } from "@/components/avatar/avatar";
import type { PersonSeed } from "@/lib/people/types";
import { cx } from "@/lib/cx";
import type { IdentityRow, ReviewView } from "./case-view";
import type { IdentityAnswer } from "./decision";
import { PAIR_WORD, pairChecks, pairState, type PairState } from "./pair-checks";

const ARABIC = /[؀-ۿ]/;
const LATIN = /[A-Za-z]/;

/**
 * A name "differ" between an Arabic-script and a Latin-script rendering may be one name transliterated, so the reviewer
 * is told to compare transliteration. The server's verdict is shown unchanged; this only adds a hint beside it.
 */
export function scriptHint(row: IdentityRow): string | undefined {
  if (row.field !== "Name" || row.verdict !== "differ") return undefined;
  const arabic = [row.referral, row.report].map((v) => ARABIC.test(v) && !LATIN.test(v));
  const latin = [row.referral, row.report].map((v) => LATIN.test(v) && !ARABIC.test(v));
  return (arabic[0] && latin[1]) || (latin[0] && arabic[1]) ? "different scripts, compare transliteration" : undefined;
}
type Shown = { person: PersonSeed; state: AvatarState; label: string };

/** The report's face follows the reviewer's answer: "?" until they choose, then the referral's face or a stranger's. */
function reportFace(view: ReviewView, answer: IdentityAnswer | undefined): Shown {
  const { referral, report } = view.faces;
  if (answer === "same") return { person: referral.person, state: report.state === "unknown" ? "resolving" : "known", label: `Same patient as ${view.name}` };
  if (answer === "different") return { person: { key: view.reportRegistration }, state: "resolving", label: "A different patient" };
  return { ...report, label: report.state === "unknown" ? "Not yet confirmed as this patient" : `Same patient as ${view.name}` };
}

const scriptOf = (v: string) => (ARABIC.test(v) ? { lang: "ar", dir: "rtl" as const } : {});

/** One side of the pair: its source, the recorded name and every identifier, differences marked and matches quiet. */
function RecordColumn({ side, view, face, second }: { side: "referral" | "report"; view: ReviewView; face: Shown; second?: boolean }) {
  const rows = view.identity;
  const nameRow = rows.find((r) => r.field === "Name");
  const name = (nameRow?.[side] || (side === "referral" ? view.referralName : view.reportName)) ?? "";
  const nameState = nameRow ? pairState(nameRow) : "unclear";
  return (
    <section
      aria-label={side === "referral" ? "Referral record" : "Report record"}
      data-record={side}
      className={cx(
        "flex min-w-0 flex-col gap-4 p-[var(--rv-pad)]",
        second && "border-t border-rule bg-limestone-100 @2xl:border-l @2xl:border-t-0",
      )}
    >
      <h3 className="m-0 flex min-w-0 items-center gap-2 text-small font-semibold text-ink">
        <FileText aria-hidden className="size-4 flex-none text-jade-700" strokeWidth={1.75} />
        <span className="min-w-0 break-words">{side === "referral" ? view.referralSource : view.reportSource}</span>
      </h3>
      <div className="flex min-w-0 items-center gap-3">
        <Avatar person={face.person} state={face.state} size={36} label={face.label} />
        <div className="min-w-0">
          <p
            {...scriptOf(name)}
            className={cx(
              "m-0 break-words text-[clamp(20px,2.3vw,32px)] leading-[1.15]",
              ARABIC.test(name) ? "font-arabic text-left" : "staff-display",
              nameState === "differ" ? "text-ochre-ink" : "text-jade-900",
            )}
          >
            {name || "not recorded"}
          </p>
          {nameState === "differ" && <p className="m-0 mt-0.5 text-small font-semibold text-ochre-ink">Name differs</p>}
        </div>
      </div>
      <dl className="m-0 border-t border-rule">
        {rows
          .filter((r) => r !== nameRow)
          .map((r) => {
            const state = pairState(r);
            const value = r[side];
            const absent = value.startsWith("not recorded") || value === "unknown";
            return (
              <div
                key={r.field}
                data-pair={state}
                className={cx(
                  "flex min-h-11 flex-wrap items-center justify-between gap-x-3 gap-y-0.5 border-b border-rule px-2 py-1.5",
                  state === "differ" && "bg-ochre-wash text-ochre-ink",
                )}
              >
                <dt className={cx("text-small", state === "differ" ? "font-semibold" : "text-muted")}>
                  {r.field}
                  {state === "differ" && <span className="ml-1.5">· differs</span>}
                </dt>
                <dd
                  {...scriptOf(value)}
                  className={cx(
                    "m-0 min-w-0 break-words text-right tabular-nums",
                    ARABIC.test(value) && "font-arabic",
                    r.verdict === "separate" ? "text-small text-muted" : absent ? "text-muted" : "font-semibold",
                  )}
                >
                  {value}
                </dd>
              </div>
            );
          })}
      </dl>
    </section>
  );
}

const CHECK_TONE: Record<PairState, string> = {
  differ: "text-ochre-ink",
  match: "text-ink",
  missing: "text-muted",
  unclear: "text-muted",
  separate: "text-muted",
};

/** The pair-checks summary: one line per identifier, the word always beside its mark. */
export function PairChecks({ view }: { view: ReviewView }) {
  const id = useId();
  const checks = pairChecks(view.identity);
  return (
    <section aria-labelledby={`${id}-h`} className="min-w-0 p-[var(--rv-pad)]" data-testid="pair-checks">
      <h3 id={`${id}-h`} className="m-0 text-small font-semibold text-ink">
        Checks
      </h3>
      <dl className="m-0 mt-1 grid grid-cols-1">
        {checks.map((k) => (
          <div key={k.field} data-pair={k.state} className="flex min-h-11 flex-wrap items-center justify-between gap-x-3 border-t border-rule">
            <dt className="text-muted">{k.field}</dt>
            <dd className={cx("m-0 inline-flex items-center gap-1.5 font-semibold", CHECK_TONE[k.state])}>
              {k.state === "match" && <Check aria-hidden className="size-4 text-jade-600" strokeWidth={2} />}
              {k.state === "differ" && <TriangleAlert aria-hidden className="size-4" strokeWidth={2} />}
              {PAIR_WORD[k.state]}
            </dd>
          </div>
        ))}
      </dl>
    </section>
  );
}

/** Link score on a 0.95 to 1.00 scale, with the auto-close line. */
function ScoreTrack({ score, line }: { score: number; line: number }) {
  const at = (v: number) => `${Math.min(100, Math.max(0, ((v - 0.95) / 0.05) * 100))}%`;
  const above = score >= line;
  return (
    <div
      role="img"
      aria-label={`Link score ${score.toFixed(3)} on a scale from 0.95 to 1.00, ${above ? "at or above" : "below"} the ${line} auto-close line.`}
      className="relative mt-2 h-[52px] tabular-nums"
    >
      <div className="absolute inset-x-0 top-[22px] h-1 rounded-sm bg-control" />
      <div className="absolute top-3.5 h-5 w-0.5 -ml-px bg-ink" style={{ left: at(line) }} />
      <div
        className={cx("absolute -top-0.5 whitespace-nowrap text-small", line > 0.975 ? "mr-1.5 text-right" : "ml-1.5")}
        style={line > 0.975 ? { right: `calc(100% - ${at(line)})` } : { left: at(line) }}
      >
        auto-close line {line}
      </div>
      <div className="absolute top-[19px] -ml-[5px] size-2.5 rounded-full bg-hue" style={{ left: at(score) }} />
      <div className="absolute left-0 top-[34px] text-small text-muted">0.95</div>
      <div className="absolute right-0 top-[34px] text-small text-muted">1.00</div>
    </div>
  );
}

/**
 * The record pair: the two records side by side (stacked on a narrow screen), the pair-checks summary beside the
 * answer form (`children`), and the source notes beneath. The desk renders it only when the viewer may see identifiers.
 */
export function IdentityBridge({ view, answer, children }: { view: ReviewView; answer?: IdentityAnswer; children?: React.ReactNode }) {
  const [why, setWhy] = useState(false);
  const suffix = view.items.link ?? "case";
  return (
    <section aria-label="Record pair" className="@container min-w-0 [--rv-pad:14px] @2xl:[--rv-pad:clamp(17px,2.4cqi,32px)]">
      <div className="grid grid-cols-1 @2xl:grid-cols-2">
        <RecordColumn side="referral" view={view} face={{ ...view.faces.referral, label: view.name }} />
        <RecordColumn side="report" view={view} face={reportFace(view, answer)} second />
      </div>
      <div className="border-t border-rule px-[var(--rv-pad)] py-3">
        {view.identity.flatMap((r) => {
          const hint = scriptHint(r);
          return hint ? [<p key={r.field} className="m-0 mb-2 text-small" data-testid="script-hint">{r.field}: {hint}.</p>] : [];
        })}
        <p className="m-0 font-medium">{view.identitySummary}</p>
        {view.identityGuard.length > 0 && (
          <ul aria-label="Visible identifier field limitations" className="m-0 mt-1 list-disc pl-5 text-small text-muted" data-testid="identity-guard">
            {view.identityGuard.map((line) => (
              <li key={line}>{line}</li>
            ))}
          </ul>
        )}
        <p role="note" aria-label="Identifier source context" className="m-0 mt-2 text-small text-muted">{view.identityContext}</p>
        <button
          type="button"
          aria-expanded={why}
          aria-controls={`why-held-${suffix}`}
          onClick={() => setWhy(!why)}
          className="-ml-2 mt-1 inline-flex min-h-11 items-center gap-1.5 rounded-control px-2 text-small font-semibold text-jade-800 hover:bg-active"
        >
          <span>{why ? "Hide why" : "Why it was held"}</span>
          <ChevronDown aria-hidden className={cx("size-3.5 transition-transform", why && "rotate-180")} strokeWidth={2} />
        </button>
        {why && (
          <div id={`why-held-${suffix}`} className="mt-2 rounded-control bg-ground p-4">
            <div className="flex flex-wrap items-baseline justify-between gap-x-3">
              <span className="font-medium">Link score</span>
              <span>
                {view.link.score.toFixed(3)}{" "}
                <span className="text-small text-muted">
                  on {view.link.basis}, auto-close line {view.link.line}
                </span>
              </span>
            </div>
            <ScoreTrack score={view.link.score} line={view.link.line} />
            <p className="m-0 mt-2 text-small leading-[18px] text-muted">
              This score describes the displayed link context. The original configured guard result is unavailable.
            </p>
            <p className="m-0 mt-1 text-small leading-[18px] text-muted">
              The rules reader applies the DoH closure checklist word for word. The second reader is a language model ({view.readerModel}). Both must
              agree for a report to close a case.
            </p>
          </div>
        )}
      </div>
      <div className={cx("grid grid-cols-1 border-t border-rule", children ? "@2xl:grid-cols-[minmax(0,0.9fr)_minmax(0,1.55fr)]" : undefined)}>
        <PairChecks view={view} />
        {children && <div className="min-w-0 border-t border-rule bg-limestone-100 @2xl:border-l @2xl:border-t-0">{children}</div>}
      </div>

    </section>
  );
}

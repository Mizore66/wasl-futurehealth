import { Mark, Pill } from "@/components/review/marks";
import { HonestyLabel } from "@/components/wasl/honesty-label";
import { TechTerm } from "@/components/wasl/tech-term";
import { apiMode } from "@/lib/api/source";
import type { Audit, Programme } from "@/lib/api/schema";
import { Card } from "./sections";
import { atWords, bodySummary, entryLabel, reviewWords, roleWords, shortHash } from "./words";

/** Full hash on a disclosure; the short form is for comparing by eye. */
function Hash({ label, value }: { label: string; value: string }) {
  return (
    <span className="block">
      <span className="text-muted">{label} </span>
      <span className="font-mono" title={value}>
        {shortHash(value)}
      </span>
    </span>
  );
}

/** Whether the server's check passed, with honest wording about what that check is. */
export function VerifiedCard({ audit }: { audit: Audit }) {
  const intact = audit.chain.length === audit.chain_length;
  return (
    <section aria-labelledby="verified-h" className="rounded-card border border-line bg-surface p-4 md:p-pad">
      <h2 id="verified-h" className="m-0 text-section font-medium">
        Chain check
      </h2>
      <p className="m-0 mt-3 flex flex-wrap items-center gap-3" role="status" data-testid="chain-verified">
        {audit.chain_verified ? (
          <Pill tone="positive" shape="tick">
            Verified · {audit.chain.length} of {audit.chain_length} intact
          </Pill>
        ) : (
          <Pill tone="negative" shape="cross">
            Chain broken
          </Pill>
        )}
        {!intact && (
          <span className="text-negative">
            Only {audit.chain.length} of {audit.chain_length} entries were returned.
          </span>
        )}
      </p>
      <p className="m-0 mt-3 max-w-[72ch]">
        {audit.chain_verified
          ? "The server recomputed every entry's hash from its contents and the hash of the entry before it, and they all matched. So no entry was changed or removed after it was sealed."
          : "The server recomputed the hashes and at least one did not match. Treat this case's record as altered until it is checked."}
      </p>
      <p className="m-0 mt-2 max-w-[72ch] text-small text-muted">
        This proves the record is unaltered. It does not prove the decisions were clinically right.{" "}
        {apiMode() === "live"
          ? "The Wasl API ran the check when this page loaded; your browser does not re-run it."
          : "The Wasl API ran the check when this snapshot was published (the run metadata above says when); this page shows that result and does not re-run it."}
      </p>
    </section>
  );
}

/** Each sealed entry: what it was, what it says, and how it chains to the one before. */
export function ChainCard({ audit }: { audit: Audit }) {
  return (
    <Card id="chain" title="Sealed entries" note="Each entry carries the hash of the one before it. Break one and every later hash stops matching.">
      <ol className="m-0 mt-3 list-none p-0" aria-label="Chain entries">
        {audit.chain.map((e, i) => (
          <li key={e.hash} className="border-t border-rule py-3" data-entry={e.type}>
            <div className="flex items-baseline gap-3">
              <span className="w-5 flex-none text-right tabular-nums text-muted">{i + 1}</span>
              <div className="min-w-0 flex-1">
                <p className="m-0 font-medium">
                  <TechTerm id={e.type} label={entryLabel(e.type)} />
                </p>
                <p className="m-0 mt-1">{bodySummary(e)}</p>
                <p className="m-0 mt-1 text-small leading-5">
                  <Hash label={e.prev === "genesis" ? "Starts the chain" : "After"} value={e.prev === "genesis" ? "genesis" : e.prev} />
                  <Hash label="Hash" value={e.hash} />
                </p>
                <details className="mt-1 text-small text-muted">
                  <summary className="min-h-6 cursor-pointer">Full hashes and entry body</summary>
                  <p className="m-0 mt-1 break-all font-mono">prev {e.prev}</p>
                  <p className="m-0 mt-1 break-all font-mono">hash {e.hash}</p>
                  <pre className="m-0 mt-2 overflow-x-auto whitespace-pre-wrap break-all rounded-control bg-ground p-3 font-mono">
                    {JSON.stringify(e.body, null, 2)}
                  </pre>
                </details>
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
      title="Decision signatures"
      aside={
        <span className="text-small text-muted" data-testid="signature-count">
          {matching} of {rows.length} match
        </span>
      }
      note="Each decision is signed when made. The server signs it again from the stored decision; the two must be equal."
    >
      {rows.length === 0 ? (
        <p className="m-0 mt-3 text-muted">No decisions are signed for this case.</p>
      ) : (
        <ol className="m-0 mt-3 list-none p-0" aria-label="Decision signatures">
          {rows.map((s, i) => {
            const ok = s.sig === s.recomputed;
            return (
              <li key={s.decision_id} className="border-t border-rule py-3" data-match={ok}>
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
                <p className="m-0 mt-1 text-small leading-5">
                  <Hash label="Stored" value={s.sig} />
                  <Hash label="Recomputed" value={s.recomputed} />
                </p>
                <details className="mt-1 text-small text-muted">
                  <summary className="min-h-6 cursor-pointer">Full values</summary>
                  <p className="m-0 mt-1 break-all font-mono">{s.decision_id}</p>
                  <p className="m-0 mt-1 break-all">Policy {s.policy}</p>
                  <p className="m-0 mt-1 break-all font-mono">stored {s.sig}</p>
                  <p className="m-0 mt-1 break-all font-mono">recomputed {s.recomputed}</p>
                </details>
              </li>
            );
          })}
        </ol>
      )}
    </Card>
  );
}

/** Review actions appended through the API: append-only, each linked to the one before. */
export function HistoryCard({ audit, programme }: { audit: Audit; programme: Programme }) {
  return (
    <Card
      id="history"
      title="Review history"
      aside={<HonestyLabel kind="simulated">Demo actions, not clinical sign-offs</HonestyLabel>}
      note="Append-only. A new answer is added below; an old one is never edited."
    >
      {audit.history.length === 0 ? (
        <p className="m-0 mt-3 text-muted">
          Nobody has signed this case through the API yet. The simulated reviewers&rsquo; sign-offs from the run are on the case page.
        </p>
      ) : (
        <ol className="m-0 mt-3 list-none p-0" aria-label="Review history">
          {audit.history.map((r, i) => (
            <li key={r.hash} className="border-t border-rule py-3">
              <p className="m-0 font-medium">{r.decision ? reviewWords(r.decision, programme) : roleWords(r.action)}</p>
              {/* The auditor's redacted history never names the actor or quotes the note. */}
              <p className="m-0 text-muted">
                {r.actor ? `${r.actor} · ` : ""}
                {roleWords(r.role)} · {atWords(r.at)}
              </p>
              {r.note && <p className="m-0 mt-1">&ldquo;{r.note}&rdquo;</p>}
              <p className="m-0 mt-1 text-small leading-5">
                <Hash label={i === 0 ? "Starts the log" : "After"} value={r.prev} />
                <Hash label="Hash" value={r.hash} />
              </p>
            </li>
          ))}
        </ol>
      )}
    </Card>
  );
}

const STREAM_WORDS: Record<string, string> = { fhir: "Incoming records (FHIR)" };
const streamWords = (k: string) => STREAM_WORDS[k] ?? (/^out\.p\d+$/.test(k) ? `Outgoing messages, partition ${k.slice(5)}` : k);

/** The daily heads the run reports, as the API gave them. */
export function AnchorsCard({ audit }: { audit: Audit }) {
  const entries = Object.entries(audit.anchors);
  return (
    <Card id="anchors" title="Anchors" aside={<HonestyLabel kind="designed">External store: designed, not checked here</HonestyLabel>}>
      {entries.length === 0 ? (
        <p className="m-0 mt-3 text-muted">The API sent no anchors for this case.</p>
      ) : (
        <ul className="m-0 mt-3 list-none p-0" aria-label="Anchors">
          {entries.map(([k, v]) => (
            <li key={k} className="border-t border-rule py-2">
              <span className="block">{streamWords(k)}</span>
              <Hash label="Head" value={v} />
              <details className="text-small text-muted">
                <summary className="min-h-6 cursor-pointer">Full value</summary>
                <code className="break-all font-mono">
                  {k} {v}
                </code>
              </details>
            </li>
          ))}
        </ul>
      )}
      {audit.anchor_note && (
        <p className="m-0 mt-3 max-w-[72ch] text-muted">{audit.anchor_note}. This page shows the heads the API reports; it does not query that store.</p>
      )}
    </Card>
  );
}

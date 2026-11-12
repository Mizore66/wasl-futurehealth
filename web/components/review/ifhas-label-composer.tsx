"use client";

import { useState } from "react";
import { Chips } from "./chips";
import {
  composeIfhasLabel,
  IFHAS_DOMAIN_WORDS,
  IFHAS_DOMAINS,
  IFHAS_FOLLOWUP_STATUSES,
  IFHAS_KINDS,
  IFHAS_SCREEN_CELLS,
  parseIfhasLabel,
  type IfhasDomain,
  type IfhasDraft,
  type IfhasKind,
} from "./ifhas-label";

/**
 * One control per IFHAS domain instead of one list of 304 label strings. It reports the canonical label once the
 * draft is complete and "" (no answer) until then; the parent's draft and the server action's check are unchanged.
 */
export function IfhasLabelComposer({
  name,
  value,
  onPick,
  disabled,
}: {
  name: string;
  value: string | undefined;
  onPick: (label: string) => void;
  disabled: boolean;
}) {
  const [draft, setDraft] = useState<IfhasDraft>(() => parseIfhasLabel(value));
  const [seen, setSeen] = useState(value);
  const label = composeIfhasLabel(draft);
  /* The parent cleared or replaced the answer (a submit, an undo, a new identity answer): show what it now holds. */
  if (seen !== value) {
    setSeen(value);
    if ((value ?? "") !== label) setDraft(parseIfhasLabel(value));
  }

  function update(next: IfhasDraft) {
    setDraft(next);
    const composed = composeIfhasLabel(next);
    setSeen(composed);
    onPick(composed);
  }

  const kind = draft.kind;
  const perDomain = kind === "screening" || kind === "followup";
  return (
    <div data-testid="ifhas-composer">
      <div role="radiogroup" aria-label="IFHAS document">
        <Chips<IfhasKind> name={`${name}-kind`} options={IFHAS_KINDS} value={kind} onPick={(k) => update({ kind: k, values: {} })} disabled={disabled} />
      </div>
      {perDomain && (
        <div className="mt-2 grid gap-2 sm:grid-cols-2">
          {IFHAS_DOMAINS.map((domain: IfhasDomain) => {
            const options = kind === "screening" ? IFHAS_SCREEN_CELLS[domain].map(([cell, words]) => [cell, `${cell}, ${words}`] as const) : IFHAS_FOLLOWUP_STATUSES;
            return (
              <label key={domain} className="flex min-w-0 flex-col gap-1 text-small">
                <span>{IFHAS_DOMAIN_WORDS[domain]}</span>
                <select
                  name={`${name}-${domain}`}
                  value={draft.values[domain] ?? ""}
                  disabled={disabled}
                  onChange={(event) => update({ kind, values: { ...draft.values, [domain]: event.target.value } })}
                  className="min-h-11 w-full rounded-control border border-control bg-surface px-3 text-body text-ink"
                >
                  <option value="" disabled={kind === "screening"}>
                    {kind === "screening" ? "Choose a result" : "Not mentioned"}
                  </option>
                  {options.map(([v, words]) => (
                    <option key={v} value={v}>
                      {words}
                    </option>
                  ))}
                </select>
              </label>
            );
          })}
        </div>
      )}
      {kind && (
        <p className="m-0 mt-2 text-small text-muted" aria-live="polite" data-testid="ifhas-label">
          {label ? (
            <>
              Evidence label <code className="font-mono text-ink [overflow-wrap:anywhere]">{label}</code>
            </>
          ) : kind === "screening" ? (
            "Choose a result for every domain."
          ) : (
            "Choose a status for at least one domain the document mentions."
          )}
        </p>
      )}
    </div>
  );
}

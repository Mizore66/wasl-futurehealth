"use client";

import Link from "next/link";
import { useRef, useSyncExternalStore } from "react";
import { Avatar } from "@/components/avatar/avatar";
import { Dialog, DialogContent, DialogDescription, DialogTitle } from "@/components/ui/dialog";
import { HonestyLabel } from "@/components/wasl/honesty-label";
import { cn } from "@/lib/utils";
import type { Profile } from "./profile";

const PARAM = "person";
const CHANGE = "wasl:person";

function subscribe(onChange: () => void) {
  window.addEventListener("popstate", onChange);
  window.addEventListener(CHANGE, onChange);
  return () => {
    window.removeEventListener("popstate", onChange);
    window.removeEventListener(CHANGE, onChange);
  };
}

const read = () => new URLSearchParams(window.location.search).get(PARAM);

function writeUrl(id: string | null, how: "push" | "replace") {
  const url = new URL(window.location.href);
  if (id) url.searchParams.set(PARAM, id);
  else url.searchParams.delete(PARAM);
  window.history[how === "push" ? "pushState" : "replaceState"](null, "", url);
  window.dispatchEvent(new Event(CHANGE));
}

/**
 * The open profile lives in the URL (`?person=<case id>`), so a profile can be shared and Back closes it.
 * Closing one this page opened goes back; closing one the page loaded with removes the parameter instead.
 */
export function useProfileParam() {
  const id = useSyncExternalStore(subscribe, read, () => null);
  const pushed = useRef(false);
  return {
    id,
    open(next: string) {
      writeUrl(next, "push");
      pushed.current = true;
    },
    close() {
      if (pushed.current) {
        pushed.current = false;
        window.history.back();
      } else writeUrl(null, "replace");
    },
  };
}

/** A person chip's link: a real href for new tabs and sharing, and an in-page open for a plain click. */
export function profileLink(id: string, open: (id: string) => void) {
  return {
    href: `?${PARAM}=${encodeURIComponent(id)}`,
    onClick(e: React.MouseEvent) {
      if (e.metaKey || e.ctrlKey || e.shiftKey || e.altKey || e.button !== 0) return;
      e.preventDefault();
      open(id);
    },
  };
}

const VALUE_WORD: Record<string, string> = { yes: "yes", no: "no", not_stated: "not stated", not_asked: "not asked", adequate: "adequate", inadequate: "inadequate" };
const word = (v: string) => VALUE_WORD[v] ?? v.replaceAll("_", " ");

export function PersonSheet({ profile, onClose }: { profile: Profile | null; onClose: () => void }) {
  const p = profile;
  return (
    <Dialog open={!!p} onOpenChange={(open) => !open && onClose()}>
      {p && (
        <DialogContent
          data-testid="person-sheet"
          className="left-auto right-0 top-0 flex h-dvh max-h-dvh w-full max-w-[min(480px,100vw)] translate-x-0 translate-y-0 flex-col gap-0 overflow-y-auto rounded-none border-y-0 border-r-0 border-line bg-surface p-0 text-ink shadow-none data-[state=closed]:zoom-out-100 data-[state=open]:zoom-in-100 data-[state=open]:slide-in-from-right-8 sm:max-w-[480px]"
        >
          <div className="flex items-center gap-4 border-b border-rule px-6 pb-5 pt-6 pr-12">
            <Avatar person={{ key: p.person.key, sex: p.person.sex }} state={p.matched ? "known" : "unknown"} size={64} label="" />
            <div className="min-w-0">
              <DialogTitle className="text-title font-medium leading-tight">{p.person.name}</DialogTitle>
              <div lang="ar" dir="rtl" className="text-left text-body text-ink-soft">
                {p.person.nameAr}
              </div>
              <DialogDescription className="m-0 mt-1 text-small text-muted">
                As written on the report: <span className="text-ink-soft" data-testid="report-name">{p.person.reportName}</span>
              </DialogDescription>
            </div>
          </div>

          <div className="flex flex-col gap-6 px-6 py-5">
            <HonestyLabel kind="replay">
              Replay of a recorded run · {p.person.source === "generated" ? "generated name" : "demo cast name"}
            </HonestyLabel>

            <Section title="Now">
              <p className="m-0 text-body" data-testid="profile-now">
                {p.now}
              </p>
              {!p.matched && <p className="m-0 mt-1 text-small text-muted">Not matched to a referral yet at this step, so the face shows &ldquo;?&rdquo;.</p>}
            </Section>

            <Section title="Decision">
              <p className="m-0 text-body font-medium">{p.decision.word}</p>
              <p className="m-0 text-small text-muted">
                {p.decision.label} · safety gate {p.decision.gatePassed ? "passed" : "held"}
              </p>
            </Section>

            <Section title="The readers" note={`Gemma 3 4B answered in ${p.latencyS}, as recorded. Model ${p.model}.`}>
              <table className="w-full border-collapse text-left text-small">
                <thead>
                  <tr className="text-muted">
                    <th scope="col" className="pb-1 pr-2 font-normal">Field</th>
                    {p.rulesOnRecord && <th scope="col" className="pb-1 pr-2 font-normal">Rules reader</th>}
                    <th scope="col" className="pb-1 font-normal">AI reader</th>
                  </tr>
                </thead>
                <tbody>
                  {p.fields.map((f) => (
                    <tr key={f.key} className="border-t border-rule align-top">
                      <th scope="row" className="py-2 pr-2 font-normal">
                        <div className="text-body">{f.name}</div>
                        <div className="text-muted">{f.key}</div>
                      </th>
                      {p.rulesOnRecord && <td className="py-2 pr-2 text-body">{f.rules ? word(f.rules) : "—"}</td>}
                      <td className="py-2">
                        <div className="text-body">{word(f.value)}</div>
                        {f.quote ? (
                          <q className="mt-0.5 block text-ink-soft">{f.quote}</q>
                        ) : (
                          <span className="text-muted">no quote</span>
                        )}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
              {!p.rulesOnRecord && (
                <p className="m-0 mt-2 text-small text-muted">This run&apos;s ledger keeps only the AI reader&apos;s answer for this report. The rules reader&apos;s answer is recorded for the demo cast&apos;s reports.</p>
              )}
            </Section>

            <Section title="Steps in this run">
              <ol className="m-0 list-none p-0" aria-label="Steps in this run">
                {p.history.map((h) => (
                  <li key={h.step} className={cn("grid grid-cols-[56px_minmax(0,1fr)] gap-x-2 border-t border-rule py-1.5", !h.played && "text-muted")}>
                    <span className="text-small tabular-nums text-muted">step {h.step}</span>
                    <span className="min-w-0">
                      <span className="block text-body">{h.text}</span>
                      <span className="block truncate text-small text-muted">{h.played ? h.tech : "not reached yet in this loop"}</span>
                    </span>
                  </li>
                ))}
              </ol>
            </Section>

            {p.record ? (
              <Link
                href={p.record}
                className="inline-flex h-9 items-center self-start rounded-control bg-cta px-4 text-body font-medium text-white no-underline hover:text-white"
              >
                Open full record
              </Link>
            ) : (
              <p className="m-0 text-small text-muted">
                No patient record: this person was generated for the replay, which is a different run from the one the records come from.
              </p>
            )}
          </div>
        </DialogContent>
      )}
    </Dialog>
  );
}

function Section({ title, note, children }: { title: string; note?: string; children: React.ReactNode }) {
  return (
    <section>
      <h3 className="m-0 text-small font-medium text-muted">{title}</h3>
      <div className="mt-1.5">{children}</div>
      {note && <p className="m-0 mt-2 text-small text-muted">{note}</p>}
    </section>
  );
}

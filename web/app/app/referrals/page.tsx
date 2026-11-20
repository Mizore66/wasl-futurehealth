import Link from "next/link";
import { ChevronRight } from "lucide-react";
import { ReferralInbox } from "@/components/worklist/referral-inbox";
import { REFERRAL_VIEWS, handoffOf, packetReferral, referralFrom, referralView, viewScope, type Referral, type ReferralView } from "@/components/worklist/referrals";
import { hasRecord, runClock } from "@/components/worklist/run-clock";
import { syntheticHandoffs, syntheticReferrals } from "@/components/worklist/synthetic-referrals";
import { getAllCases } from "@/lib/api/all-cases";
import type { CaseSummary } from "@/lib/api/schema";
import { apiMode, getCase, getDashboard } from "@/lib/api/source";
import { SegmentedFilter } from "@/components/ui/filters/toolbar";
import { respondToReferral } from "./actions";

export const metadata = { title: "Referrals" };

const first = (v: string | string[] | undefined) => (Array.isArray(v) ? v[0] : v)?.trim() ?? "";

/*
 * Which open cases need their canonical packet from the detail read. The live list carries a hand-off marker
 * (pipeline handoffMarker), so only returned cases are read; the static fixtures predate it and keep the dropoff rule.
 */
const needsPacket = (c: CaseSummary, fallback: boolean) => !c.closed && (apiMode() === "live" ? !!c.handoff : fallback);

/* A unit return's note to the GP is free text, so the list only says it exists; the detail carries it. */
const needsNote = (c: CaseSummary) => !c.closed && !!c.api_overlay?.referral_response?.has_note;

async function crcReferrals(view: ReferralView, q: string, now: number): Promise<Referral[]> {
  const [{ cases: listed }, extra] = await Promise.all([getAllCases({ provider: view.provider, programme: view.programme, ...(q && { q }) }), syntheticReferrals(q)]);
  const cases = [...listed, ...extra];
  const details = await Promise.all(cases.map(async (c) => (needsPacket(c, c.dropoff === "referral_rejected") || needsNote(c) ? await getCase(c.id) : null)));
  return cases
    .map((c, i) => {
      const d = details[i];
      const current = needsPacket(c, c.dropoff === "referral_rejected") ? (d?.current ?? null) : null;
      const read = needsNote(c) && d?.api_overlay ? { ...c, api_overlay: d.api_overlay } : c;
      return referralFrom(read, now, hasRecord(c), current && handoffOf(current));
    })
    .filter((r) => r !== null);
}

/* Live mode reads the canonical packet only of cases whose list summary marks a returned referral. */
async function packetReferrals(view: ReferralView, q: string, now: number): Promise<Referral[]> {
  const [{ cases: listed }, extra] = await Promise.all([getAllCases({ provider: view.provider, programme: view.programme, ...(q && { q }) }), syntheticHandoffs(view.programme, view.provider, q)]);
  const live = apiMode() === "live" ? await Promise.all(listed.filter((c) => needsPacket(c, false)).map(async (c) => ({ case: c, current: (await getCase(c.id))?.current ?? null }))) : [];
  return [...live, ...extra].map((x) => packetReferral(x.case, x.current, now, hasRecord(x.case))).filter((r) => r !== null);
}

export default async function Page({ searchParams }: PageProps<"/app/referrals">) {
  const params = await searchParams;
  const q = first(params.q);
  const programme = first(params.programme);
  const provider = first(params.provider);
  const view = referralView(programme, provider);
  const [dashboard, now] = await Promise.all([getDashboard(), runClock()]);
  const referrals = !view ? [] : view.programme === "crc" ? await crcReferrals(view, q, now) : await packetReferrals(view, q, now);
  const live = apiMode() === "live";
  return (
    <ReferralInbox
      referrals={referrals}
      window={dashboard.sla.referral_response_days}
      now={now}
      q={q}
      scope={viewScope(programme, provider)}
      referrer={view?.referrer ?? "referrer"}
      respond={live && view?.programme === "crc" ? respondToReferral : undefined}
      heading={view ? { title: view.title, indication: view.indication } : { title: "Referrals", indication: "" }}
      scopeBar={
        <>
          <SegmentedFilter
            label="Referral programme"
            items={REFERRAL_VIEWS.map((v) => {
              const scope = v === REFERRAL_VIEWS[0] ? "" : viewScope(v.programme, v.provider);
              return { label: v.label, href: scope ? `/app/referrals?${scope}` : "/app/referrals", current: v === view };
            })}
          />
          {view ? (
            <Link href={view.programme === "crc" ? "/app/referrals/returned" : `/app/referrals/returned?${viewScope(view.programme, view.provider)}`} className="inline-flex min-h-11 items-center gap-1 text-small font-medium text-jade-800 no-underline hover:underline">
              Returned referrals, referring {view.referrer === "GP" ? "GP" : "clinician"} view <ChevronRight aria-hidden className="size-4" />
            </Link>
          ) : (
            <p className="m-0 text-small text-muted">No referral workflow for this programme and provider.</p>
          )}
        </>
      }
    />
  );
}

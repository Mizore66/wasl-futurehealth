import Link from "next/link";
import { HonestyLabel } from "@/components/wasl/honesty-label";
import { ReferralInbox } from "@/components/worklist/referral-inbox";
import { REFERRAL_VIEWS, handoffOf, packetReferral, referralFrom, referralView, viewScope, type Referral, type ReferralView } from "@/components/worklist/referrals";
import { hasRecord, runClock } from "@/components/worklist/run-clock";
import { syntheticHandoffs, syntheticReferrals } from "@/components/worklist/synthetic-referrals";
import { getAllCases } from "@/lib/api/all-cases";
import type { CaseSummary } from "@/lib/api/schema";
import { apiMode, getCase, getDashboard } from "@/lib/api/source";
import { cn } from "@/lib/utils";
import { respondToReferral } from "./actions";
import { simulatedRunDate } from "@/lib/domain/staff-time";

export const metadata = { title: "Referrals" };

const first = (v: string | string[] | undefined) => (Array.isArray(v) ? v[0] : v)?.trim() ?? "";

/*
 * Which open cases need their canonical packet from the detail read. The live list carries a hand-off marker
 * (pipeline handoffMarker), so only returned cases are read; the static fixtures predate it and keep the dropoff rule.
 */
const needsPacket = (c: CaseSummary, fallback: boolean) => !c.closed && (apiMode() === "live" ? !!c.handoff : fallback);

async function crcReferrals(view: ReferralView, q: string, now: number): Promise<Referral[]> {
  const [{ cases: listed }, extra] = await Promise.all([getAllCases({ provider: view.provider, programme: view.programme, ...(q && { q }) }), syntheticReferrals(q)]);
  const cases = [...listed, ...extra];
  const currents = await Promise.all(cases.map(async (c) => (needsPacket(c, c.dropoff === "referral_rejected") ? ((await getCase(c.id))?.current ?? null) : null)));
  return cases.map((c, i) => referralFrom(c, now, hasRecord(c), currents[i] && handoffOf(currents[i]))).filter((r) => r !== null);
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
      aside={
        <>
          <nav aria-label="Referral programme" className="flex flex-wrap gap-2">
            {REFERRAL_VIEWS.map((v) => {
              const scope = viewScope(v === REFERRAL_VIEWS[0] ? "" : v.programme, v === REFERRAL_VIEWS[0] ? "" : v.provider);
              return (
                <Link
                  key={v.programme}
                  href={scope ? `/app/referrals?${scope}` : "/app/referrals"}
                  aria-current={v === view ? "page" : undefined}
                  className={cn("inline-flex min-h-11 items-center rounded-chip border px-3 text-small no-underline", v === view ? "border-jade-700 bg-jade-100 text-jade-900" : "border-control bg-surface text-muted hover:bg-ground")}
                >
                  {v.label}
                </Link>
              );
            })}
          </nav>
          <div className="flex flex-col items-start gap-1.5 sm:items-end">
            {view && (
              <Link href={view.programme === "crc" ? "/app/referrals/returned" : `/app/referrals/returned?${viewScope(view.programme, view.provider)}`} className="inline-flex min-h-11 items-center text-small underline">
                Returned referrals, referring {view.referrer === "GP" ? "GP" : "clinician"} view
              </Link>
            )}
            <span className="text-small text-muted">
              {view ? view.unit : "No referral workflow for this programme and provider"} · {simulatedRunDate(now)}
            </span>
            {view?.programme !== "crc" ? (
              // The longest label on the page: let it wrap so a phone does not scroll sideways (#155).
              <HonestyLabel kind="designed" className="h-auto min-h-6 max-w-full whitespace-normal py-0.5">
                Returned packets: canonical, synthetic. Referrer answers: {live ? "built, on the referrer's page" : "need the live synthetic API"}. Re-referral here: designed, session only
              </HonestyLabel>
            ) : live ? (
              <HonestyLabel kind="built">Referral answers: built, saved to the synthetic case ledger</HonestyLabel>
            ) : (
              <HonestyLabel kind="designed">Referral inbox: designed, not built</HonestyLabel>
            )}
          </div>
        </>
      }
    />
  );
}

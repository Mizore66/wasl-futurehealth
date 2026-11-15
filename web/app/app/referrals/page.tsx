import Link from "next/link";
import { HonestyLabel } from "@/components/wasl/honesty-label";
import { ReferralInbox } from "@/components/worklist/referral-inbox";
import { REFERRAL_VIEWS, handoffOf, packetReferral, referralFrom, referralView, viewScope, type Referral, type ReferralView } from "@/components/worklist/referrals";
import { hasRecord, runClock } from "@/components/worklist/run-clock";
import { syntheticHandoffs, syntheticReferrals } from "@/components/worklist/synthetic-referrals";
import { getAllCases } from "@/lib/api/all-cases";
import { apiMode, getCase, getDashboard } from "@/lib/api/source";
import { cn } from "@/lib/utils";
import { respondToReferral } from "./actions";
import { formatDay } from "@/lib/domain/day";

export const metadata = { title: "Referrals" };

const first = (v: string | string[] | undefined) => (Array.isArray(v) ? v[0] : v)?.trim() ?? "";

async function crcReferrals(view: ReferralView, q: string, now: number): Promise<Referral[]> {
  const [{ cases: listed }, extra] = await Promise.all([getAllCases({ provider: view.provider, programme: view.programme, ...(q && { q }) }), syntheticReferrals(q)]);
  const cases = [...listed, ...extra];
  const currents = await Promise.all(cases.map(async (c) => (!c.closed && c.dropoff === "referral_rejected" ? ((await getCase(c.id))?.current ?? null) : null)));
  return cases.map((c, i) => referralFrom(c, now, hasRecord(c), currents[i] && handoffOf(currents[i]))).filter((r) => r !== null);
}

/* A returned lung or DR referral carries no list-level marker, so live mode reads each open case's canonical packet. */
async function packetReferrals(view: ReferralView, q: string, now: number): Promise<Referral[]> {
  const [{ cases: listed }, extra] = await Promise.all([getAllCases({ provider: view.provider, programme: view.programme, ...(q && { q }) }), syntheticHandoffs(view.programme, view.provider, q)]);
  const live = apiMode() === "live" ? await Promise.all(listed.filter((c) => !c.closed).map(async (c) => ({ case: c, current: (await getCase(c.id))?.current ?? null }))) : [];
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
      aside={
        <div className="flex flex-col items-end gap-1.5">
          {/* Below lg an open referral fills the screen, so the views give way to its controls; the list shows them. */}
          <div className="flex flex-col items-end gap-1.5 max-lg:group-data-[open=true]/inbox:hidden">
            <nav aria-label="Referral programme" className="flex flex-wrap justify-end gap-2">
              {REFERRAL_VIEWS.map((v) => {
                const scope = viewScope(v === REFERRAL_VIEWS[0] ? "" : v.programme, v === REFERRAL_VIEWS[0] ? "" : v.provider);
                return (
                  <Link
                    key={v.programme}
                    href={scope ? `/app/referrals?${scope}` : "/app/referrals"}
                    aria-current={v === view ? "page" : undefined}
                    className={cn("inline-flex min-h-11 items-center rounded-chip border px-3 text-small", v === view ? "border-ink bg-active text-ink" : "border-control text-muted hover:bg-ground")}
                  >
                    {v.label}
                  </Link>
                );
              })}
            </nav>
            {view?.programme === "crc" && (
              <Link href="/app/referrals/returned" className="inline-flex min-h-11 items-center text-small underline">
                Returned referrals, referring GP view
              </Link>
            )}
          </div>
          <span className="text-small text-muted">
            {view ? view.unit : "No referral workflow for this programme and provider"} · run day {Math.floor(now)}, {formatDay(now)}
          </span>
          {view?.programme !== "crc" ? (
            <HonestyLabel kind="designed">Returned packets: canonical, synthetic. Re-referral: designed, session only</HonestyLabel>
          ) : live ? (
            <HonestyLabel kind="built">Referral answers: built, saved to the synthetic case ledger</HonestyLabel>
          ) : (
            <HonestyLabel kind="designed">Referral inbox: designed, not built</HonestyLabel>
          )}
        </div>
      }
    />
  );
}

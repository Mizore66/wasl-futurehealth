import { HonestyLabel } from "@/components/wasl/honesty-label";
import { ReferralInbox } from "@/components/worklist/referral-inbox";
import { handoffOf, referralFrom } from "@/components/worklist/referrals";
import { hasRecord, runClock } from "@/components/worklist/run-clock";
import { syntheticReferrals } from "@/components/worklist/synthetic-referrals";
import { getAllCases } from "@/lib/api/all-cases";
import { apiMode, getCase, getDashboard } from "@/lib/api/source";
import { respondToReferral } from "./actions";
import { formatDay } from "@/lib/domain/day";

export const metadata = { title: "Referrals" };

export default async function Page({ searchParams }: PageProps<"/app/referrals">) {
  const raw = (await searchParams).q;
  const q = (Array.isArray(raw) ? raw[0] : raw)?.trim() ?? "";
  const [{ cases: listed }, extra, dashboard, now] = await Promise.all([
    getAllCases({ provider: "ahs-mushrif", programme: "crc", ...(q && { q }) }),
    syntheticReferrals(q),
    getDashboard(),
    runClock(),
  ]);
  const live = apiMode() === "live";
  const cases = [...listed, ...extra];
  const currents = await Promise.all(cases.map(async (c) => (!c.closed && c.dropoff === "referral_rejected" ? ((await getCase(c.id))?.current ?? null) : null)));
  const referrals = cases.map((c, i) => referralFrom(c, now, hasRecord(c), currents[i] && handoffOf(currents[i]))).filter((r) => r !== null);
  return (
    <ReferralInbox
      referrals={referrals}
      window={dashboard.sla.referral_response_days}
      now={now}
      q={q}
      respond={live ? respondToReferral : undefined}
      aside={
        <div className="flex flex-col items-end gap-1.5">
          <span className="text-small text-muted">
            SKMC endoscopy · run day {Math.floor(now)}, {formatDay(now)}
          </span>
          {live ? (
            <HonestyLabel kind="built">Referral answers: built, saved to the synthetic case ledger</HonestyLabel>
          ) : (
            <HonestyLabel kind="designed">Referral inbox: designed, not built</HonestyLabel>
          )}
        </div>
      }
    />
  );
}

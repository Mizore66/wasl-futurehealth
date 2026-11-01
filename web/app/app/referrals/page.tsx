import { HonestyLabel } from "@/components/wasl/honesty-label";
import { ReferralInbox } from "@/components/worklist/referral-inbox";
import { referralFrom } from "@/components/worklist/referrals";
import { hasRecord, runClock } from "@/components/worklist/run-clock";
import { getCases, getDashboard } from "@/lib/api/source";
import { formatDay } from "@/lib/domain/day";

export const metadata = { title: "Referrals" };

/** Days the GP has to act on a returned referral. */
const ACT_BY_DAYS = 3;

export default async function Page({ searchParams }: PageProps<"/app/referrals">) {
  const raw = (await searchParams).q;
  const q = (Array.isArray(raw) ? raw[0] : raw)?.trim() ?? "";
  const [{ cases }, dashboard, now] = await Promise.all([
    getCases({ provider: "ahs-mushrif", programme: "crc", limit: 500, ...(q && { q }) }),
    getDashboard(),
    runClock(),
  ]);
  const referrals = cases.map((c) => referralFrom(c, now, hasRecord(c))).filter((r) => r !== null);
  return (
    <ReferralInbox
      key={q}
      referrals={referrals}
      window={dashboard.sla.referral_response_days}
      actBy={formatDay(now + ACT_BY_DAYS)}
      q={q}
      aside={
        <div className="flex flex-col items-end gap-1.5">
          <span className="text-small text-muted">
            SKMC endoscopy · run day {Math.floor(now)}, {formatDay(now)}
          </span>
          <HonestyLabel kind="designed">Referral inbox: designed, not built</HonestyLabel>
        </div>
      }
    />
  );
}

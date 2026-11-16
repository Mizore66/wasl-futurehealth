import Link from "next/link";
import { HonestyLabel } from "@/components/wasl/honesty-label";
import { ReferrerHandoffs } from "@/components/worklist/referrer-handoffs";
import { returnedPacket } from "@/components/worklist/returned-packets";
import { getAllCases } from "@/lib/api/all-cases";
import { apiMode } from "@/lib/api/source";
import { answerAsReferrer } from "../actions";

export const metadata = { title: "Returned referrals" };

/** The referring GP's view: CRC referrals the endoscopy unit returned to Al Mushrif, answered with the GP's own credential. */
export default async function Page() {
  const live = apiMode() === "live";
  const packets = live
    ? (await getAllCases({ provider: "ahs-mushrif", programme: "crc" })).cases
        .map(returnedPacket)
        .filter((p) => p !== null)
        .sort((a, b) => Number(!!a.answer) - Number(!!b.answer) || a.actBy - b.actBy)
    : [];
  return (
    <div>
      <header className="mb-6 flex flex-wrap items-end justify-between gap-x-6 gap-y-2">
        <div>
          <h1 className="m-0 text-title font-semibold">Returned referrals</h1>
          <p className="m-0 mt-1 text-muted">Referring GP, Al Mushrif. Referrals the endoscopy unit handed back to you.</p>
        </div>
        <div className="flex flex-col items-end gap-1.5">
          <Link href="/app/referrals" className="inline-flex min-h-11 items-center text-small underline">
            Receiving unit inbox
          </Link>
          {live ? (
            <HonestyLabel kind="built">GP answers: built, saved to the synthetic case ledger</HonestyLabel>
          ) : (
            <HonestyLabel kind="designed">GP answers: need the live synthetic API</HonestyLabel>
          )}
        </div>
      </header>
      <ReferrerHandoffs packets={packets} answer={live ? answerAsReferrer : undefined} />
    </div>
  );
}

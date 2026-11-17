import Link from "next/link";
import { HonestyLabel } from "@/components/wasl/honesty-label";
import { ReferrerHandoffs } from "@/components/worklist/referrer-handoffs";
import { REFERRAL_VIEWS, referralView, viewScope, type ReferralView } from "@/components/worklist/referrals";
import { returnedPacket, type ReturnedPacket } from "@/components/worklist/returned-packets";
import { syntheticHandoffs } from "@/components/worklist/synthetic-referrals";
import { getAllCases } from "@/lib/api/all-cases";
import { apiMode, getCase } from "@/lib/api/source";
import { cn } from "@/lib/utils";
import { answerAsReferrerWith } from "../actions";

export const metadata = { title: "Returned referrals" };

const first = (v: string | string[] | undefined) => (Array.isArray(v) ? v[0] : v)?.trim() ?? "";
const order = (a: ReturnedPacket, b: ReturnedPacket) => Number(!!a.answer) - Number(!!b.answer) || a.actBy - b.actBy;

/*
 * The returned packets of one referring provider. Live: the list projection carries every packet's binding; a canonical
 * packet's detail is read only for its recommendation and advice. Fixtures: the synthetic lung and DR controls, read-only.
 */
async function packets(view: ReferralView): Promise<ReturnedPacket[]> {
  if (apiMode() !== "live") {
    const controls = view.programme === "crc" ? [] : await syntheticHandoffs(view.programme, view.provider, "");
    return controls.map((x) => returnedPacket(x.case, x.current)).filter((p) => p !== null);
  }
  const { cases } = await getAllCases({ provider: view.provider, programme: view.programme });
  const open = cases.filter((c) => !c.closed && c.api_overlay?.referral_handoff?.event);
  const read = await Promise.all(open.map(async (c) => (c.api_overlay?.referral_handoff?.source === "canonical" ? ((await getCase(c.id))?.current ?? null) : null)));
  return open.map((c, i) => returnedPacket(c, read[i])).filter((p) => p !== null);
}

/** The responsible referrer's view: referrals a receiving unit returned, answered with the referring provider's own credential. */
export default async function Page({ searchParams }: PageProps<"/app/referrals/returned">) {
  const params = await searchParams;
  const programme = first(params.programme);
  const provider = first(params.provider);
  const view = referralView(programme, provider);
  const live = apiMode() === "live";
  const list = view ? (await packets(view)).sort(order) : [];
  const scope = viewScope(programme, provider);
  return (
    <div>
      <header className="mb-6 flex flex-wrap items-end justify-between gap-x-6 gap-y-2">
        <div>
          <h1 className="m-0 text-title font-semibold">Returned referrals</h1>
          <p className="m-0 mt-1 text-muted">{view ? view.returned : "No referral workflow for this programme and provider."}</p>
        </div>
        <div className="flex flex-col items-end gap-1.5">
          <nav aria-label="Referring provider" className="flex flex-wrap justify-end gap-2">
            {REFERRAL_VIEWS.map((v) => {
              const s = v === REFERRAL_VIEWS[0] ? "" : viewScope(v.programme, v.provider);
              return (
                <Link
                  key={v.programme}
                  href={s ? `/app/referrals/returned?${s}` : "/app/referrals/returned"}
                  aria-current={v === view ? "page" : undefined}
                  className={cn("inline-flex min-h-11 items-center rounded-chip border px-3 text-small", v === view ? "border-ink bg-active text-ink" : "border-control text-muted hover:bg-ground")}
                >
                  {v.label}
                </Link>
              );
            })}
          </nav>
          <Link href={scope ? `/app/referrals?${scope}` : "/app/referrals"} className="inline-flex min-h-11 items-center text-small underline">
            Receiving unit inbox
          </Link>
          {live ? (
            <HonestyLabel kind="built">Referrer answers: built, saved to the synthetic case ledger</HonestyLabel>
          ) : (
            <HonestyLabel kind="designed">Referrer answers: need the live synthetic API</HonestyLabel>
          )}
        </div>
      </header>
      {view && <ReferrerHandoffs packets={list} referrer={view.referrer} answer={live ? answerAsReferrerWith.bind(null, view.credential) : undefined} />}
    </div>
  );
}

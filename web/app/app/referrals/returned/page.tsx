import Link from "next/link";
import { ChevronRight } from "lucide-react";
import { PageHeader } from "@/components/shell/page-header";
import { HonestyLabel } from "@/components/wasl/honesty-label";
import { safeInboxBack } from "@/components/worklist/inbox-url";
import { ReferrerHandoffs, SessionReferrerHandoffs } from "@/components/worklist/referrer-handoffs";
import { REFERRAL_VIEWS, referralView, referrerAnswerLabel, viewScope, type ReferralView } from "@/components/worklist/referrals";
import { needsDetail, returnedPacket, type ReturnedPacket } from "@/components/worklist/returned-packets";
import { syntheticHandoffs } from "@/components/worklist/synthetic-referrals";
import { getAllCases } from "@/lib/api/all-cases";
import { apiMode, getCase, hasCredential } from "@/lib/api/source";
import { mapSettledBounded } from "@/lib/domain/bounded";
import { cn } from "@/lib/utils";
import { answerAsReferrerWith } from "../actions";

export const metadata = { title: "Returned referrals" };

/** Concurrent case-detail reads this page may hold open against the API. */
const DETAIL_READS = 4;

const first = (v: string | string[] | undefined) => (Array.isArray(v) ? v[0] : v)?.trim() ?? "";
const order = (a: ReturnedPacket, b: ReturnedPacket) => Number(!!a.answer) - Number(!!b.answer) || a.actBy - b.actBy;

/*
 * The returned packets of one referring provider. Live: the list projection carries every packet's binding; a canonical
 * packet's detail is read only for its recommendation and advice, a unit return's only for its note to the GP.
 * Fixtures: the synthetic lung and DR controls, read-only; CRC shows this browser's session returns instead.
 */
async function packets(view: ReferralView): Promise<ReturnedPacket[]> {
  if (apiMode() !== "live") {
    const controls = view.programme === "crc" ? [] : await syntheticHandoffs(view.programme, view.provider, "");
    return controls.map((x) => returnedPacket(x.case, x.current)).filter((p) => p !== null);
  }
  const { cases } = await getAllCases({ provider: view.provider, programme: view.programme });
  const open = cases.filter((c) => !c.closed && c.api_overlay?.referral_handoff?.event);
  /*
   * Only canonical packets still waiting on the referrer (recommendation and advice) and noted unit returns need their
   * detail. Reads run at most
   * DETAIL_READS at a time and settle per card: a slow or failed case read marks that card, never fails the page.
   */
  const read = await mapSettledBounded(open, DETAIL_READS, async (c) => (needsDetail(c) ? await getCase(c.id) : null));
  return open
    .map((c, i) => {
      const r = read[i];
      const d = r.ok ? r.value : null;
      // A unit return's note to the GP is in the detail only; its overlay replaces the list's for that packet.
      const packet = returnedPacket(d?.api_overlay ? { ...c, api_overlay: d.api_overlay } : c, d?.current ?? null);
      return packet && !r.ok ? { ...packet, detailUnavailable: true } : packet;
    })
    .filter((p) => p !== null);
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
  /* Fixtures mode, CRC: the unit's returns from this browser session, which the GP can acknowledge in this browser. */
  const session = !live && view?.programme === "crc";
  const label = referrerAnswerLabel(live, view ? hasCredential(view.credential) : false, session);
  /* Opened from a referral's hand-off: that referral leads, and "Back to inbox" returns to the exact inbox state. */
  const focus = first(params.case);
  const back = safeInboxBack(first(params.back)) ?? (scope ? `/app/referrals?${scope}` : "/app/referrals");
  const shown = focus ? [...list.filter((p) => p.id === focus), ...list.filter((p) => p.id !== focus)] : list;
  return (
    <div>
      <Link href={back} className="mb-2 inline-flex min-h-11 items-center gap-1.5 font-semibold text-jade-800 no-underline hover:underline">
        <ChevronRight aria-hidden className="size-4 rotate-180" /> {first(params.back) ? "Back to inbox" : "Receiving unit inbox"}
      </Link>
      {/* Not clipped: an empty list is shorter than the run notice's drop-down, so the band rounds its own corners. */}
      <section aria-labelledby="returned-title" className="rounded-card border border-line bg-surface">
        <PageHeader
          attached
          run
         
          title={<span id="returned-title">Returned referrals</span>}
          sub={view ? view.returned : "No referral workflow for this programme and provider."}
          aside={<HonestyLabel kind={label.kind}>{label.text}</HonestyLabel>}
        />
        <div className="rounded-b-[7px] bg-limestone-100 p-4 sm:p-pad">
          <nav aria-label="Referring provider" className="mb-3 flex flex-wrap gap-2">
            {REFERRAL_VIEWS.map((v) => {
              const s = v === REFERRAL_VIEWS[0] ? "" : viewScope(v.programme, v.provider);
              return (
                <Link
                  key={v.programme}
                  href={s ? `/app/referrals/returned?${s}` : "/app/referrals/returned"}
                  aria-current={v === view ? "page" : undefined}
                  className={cn("inline-flex min-h-11 items-center rounded-chip border px-3 text-small no-underline", v === view ? "border-jade-700 bg-jade-100 text-jade-900" : "border-control bg-surface text-muted hover:bg-ground")}
                >
                  {v.label}
                </Link>
              );
            })}
          </nav>
          {view &&
            (session ? (
              <SessionReferrerHandoffs referrer={view.referrer} focus={focus || undefined} />
            ) : (
              <ReferrerHandoffs packets={shown} referrer={view.referrer} focus={focus || undefined} answer={live ? answerAsReferrerWith.bind(null, view.credential) : undefined} />
            ))}
        </div>
      </section>
    </div>
  );
}

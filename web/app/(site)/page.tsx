import Link from "next/link";
import { ContactCta } from "@/components/site/contact";
import { SiteFooter } from "@/components/site/footer";
import { QUOTED, missedInFive } from "@/components/site/figures";
import { NameJoiner } from "@/components/site/name-joiner";
import { Reveal } from "@/components/site/reveal";
import { body, cta, display, figure, h2, h3, lead, link, section, small, wrap } from "@/components/site/styles";
import { result } from "@/lib/results";
import { cn } from "@/lib/utils";

export const metadata = { title: { absolute: "Wasl · Nobody lost between hospitals" } };

const twoUp = "grid grid-cols-[repeat(auto-fit,minmax(min(100%,420px),1fr))] gap-[clamp(2.5rem,6vw,8rem)]";

const RECEIPT = [
  { label: "Follow-up check", value: "Complete" },
  { label: "Proof, in the hospital’s words", value: "“All surfaces were well seen”" },
  { label: "Read by", value: "Two readers, and both agree" },
  { label: "Matched by", value: "Her name and her birth date" },
  { label: "Looked after by", value: "Her clinic nurse" },
];

const WHO = [
  { who: "The person waiting", what: "A message in her own language, a named nurse to call, and no reminders until she says yes to them." },
  { who: "The nurse", what: "One list of who still needs a check, so calls go to people who need them, not to people already done." },
  { who: "The health system", what: "A follow-up rate it can trust, because every case marked done carries its proof." },
  { who: "The auditor", what: "Every decision written down with who made it, in a record where any later change shows." },
];

/** The receipt's torn edge: a zigzag along the bottom. */
const ZIG = (() => {
  let pts = "0,0 400,0";
  for (let x = 400; x > 0; x -= 20) pts += ` ${x - 10},16 ${x - 20},0`;
  return pts;
})();

export default function Page() {
  const closures = result("v2.safety.crc_false_closures");
  const closed = closures.denominator!.toLocaleString("en-US");

  return (
    <>
      <header className={cn(wrap, "pb-20 pt-12 md:pb-[120px] md:pt-[72px]")}>
        <h1 className={cn(display, "-ml-[0.04em]")}>
          Nobody lost
          <br /> between hospitals
        </h1>
        <div className={cn(twoUp, "mt-14 items-stretch")}>
          <div className="flex max-w-[34rem] flex-col justify-between gap-12">
            <div>
              <p className={lead}>
                When a screening test asks for a closer look, Wasl makes sure that person gets the follow-up check, and marks it done only
                with proof.
              </p>
              <div className="mt-10 flex flex-wrap items-center gap-7">
                <a href="#pilot" className={cta}>
                  Request a pilot
                </a>
                <Link href="/how-it-works" className={link}>
                  See how it works
                </Link>
              </div>
            </div>
            <p className={cn(body, "max-w-[30rem] leading-[1.55]")}>
              A similar name is never enough. Wasl also checks a second detail, such as a birth date. When the two disagree, a person
              decides.
            </p>
          </div>
          <NameJoiner />
        </div>
      </header>

      <section aria-labelledby="lost-title" className="bg-(--mint)">
        <Reveal className={cn(wrap, section, twoUp, "items-start")}>
          <div>
            <h2 id="lost-title" className={h2}>
              Done, and nobody knew
            </h2>
            <p className={cn(lead, "mt-9 max-w-[36ch]")}>
              Fatima had her check. It happened at a hospital across the city, where her name was spelled “F.&nbsp;Alzabi” and her ID
              number was left off. To her clinic, she never went.
            </p>
            <p className={cn(lead, "mt-6 max-w-[36ch]")}>
              So the nurse keeps calling her, while someone who truly missed their check waits further down the list.
            </p>
          </div>
          <div className="flex flex-col gap-16">
            <div>
              <div className={cn(figure, "text-(--teal)")} data-figure="missed">
                {missedInFive()}
              </div>
              <p className={cn(body, "mt-5 max-w-[44ch] leading-[1.55]")}>
                people who had their check looked as if they had not, when records were matched by ID number alone. A rounded figure, from
                testing on simulated patients.
              </p>
            </div>
            <div>
              <div className={figure} data-figure="wait">
                {QUOTED.medianWaitDays.value} days
              </div>
              <p className={cn(body, "mt-5 max-w-[44ch] leading-[1.55]")}>
                was the typical wait for the follow-up check in Abu Dhabi, against a {QUOTED.standardDays.value}-day standard. From a
                published study, 2021.
              </p>
            </div>
          </div>
        </Reveal>
      </section>

      <section aria-labelledby="proof-title" className="bg-(--sand)">
        <Reveal className={cn(wrap, section, twoUp, "items-center")}>
          <div
            role="img"
            aria-label="Drawing of a Wasl receipt for Fatima Al Zaabi: follow-up check complete, with the hospital's own words as proof, read by two readers who agree, matched by name and birth date, owned by her clinic nurse, marked done."
            className="w-full max-w-[560px]"
          >
            <div className="bg-(--card) px-[clamp(1.5rem,3vw,2.5rem)] pb-8 pt-10">
              <div className="flex items-center gap-3">
                <span className="text-[24px] font-bold leading-none tracking-[-0.02em]">Wasl</span>
                <span className="h-[22px] w-[3px] bg-(--teal)" />
                <span lang="ar" dir="rtl" className="font-arabic text-[22px] font-semibold leading-none text-(--teal)">
                  وصل
                </span>
              </div>
              <div className="mt-8 text-[clamp(2rem,1rem_+_3vw,4rem)] font-bold leading-[1.05] tracking-[-0.03em]">Fatima Al Zaabi</div>
              <dl className="mt-7 flex flex-col gap-[18px]">
                {RECEIPT.map((r) => (
                  <div key={r.label}>
                    <dt className={small}>{r.label}</dt>
                    <dd className="m-0 mt-0.5 text-[18px] font-semibold leading-[1.45]">{r.value}</dd>
                  </div>
                ))}
              </dl>
              <div className="mt-8 flex items-center gap-3 bg-(--teal) px-5 py-4 text-(--paper)">
                <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
                  <path d="M5 12l5 5 9-10" />
                </svg>
                <span className="text-[24px] font-bold tracking-[-0.01em]">Done, with proof</span>
              </div>
            </div>
            <svg width="100%" height="16" viewBox="0 0 400 16" preserveAspectRatio="none" className="block">
              <polygon points={ZIG} fill="#fffdf8" />
            </svg>
          </div>

          <div>
            <h2 id="proof-title" className={h2}>
              Done only when it is proven
            </h2>
            <p className={cn(lead, "mt-9 max-w-[36ch]")}>
              In Arabic,{" "}
              <span lang="ar" dir="rtl" className="font-arabic">
                وصل
              </span>{" "}
              means a connection. It is also a word for a receipt.
            </p>
            <p className={cn(body, "mt-6 max-w-[56ch]")}>
              Wasl marks a case done only when the hospital’s own report shows the check was complete. Two separate readers, one following
              fixed medical rules and one AI, read every report, and both must agree. If they do not, or anything is unclear, two people look
              at it before anything is closed.
            </p>
            <div className="mt-14">
              <div className={cn(figure, "text-(--teal)")}>None wrong</div>
              <p className={cn(body, "mt-5 max-w-[48ch] leading-[1.55]")} data-figure="closures">
                of {closed} cases Wasl marked done. In testing on simulated patients.
              </p>
            </div>
          </div>
        </Reveal>
      </section>

      <section aria-labelledby="who-title">
        <Reveal className={cn(wrap, section, "flex flex-wrap items-start gap-[clamp(2rem,4vw,4rem)]")}>
          <h2 id="who-title" className={cn(h2, "max-w-[14ch] flex-[1_1_320px]")}>
            Who notices the difference
          </h2>
          <ul className="m-0 flex min-w-0 flex-[2_1_640px] list-none flex-col gap-2 p-0">
            {WHO.map((w, i) => (
              <li
                key={w.who}
                className={cn(
                  "grid grid-cols-[repeat(auto-fit,minmax(min(100%,220px),1fr))] items-baseline gap-x-10 gap-y-3 px-[clamp(1.25rem,3vw,2.5rem)] py-8",
                  i % 2 === 0 ? "bg-(--sand)" : "bg-(--paper)",
                )}
              >
                <h3 className={h3}>{w.who}</h3>
                <p className={cn(body, "max-w-[52ch] leading-[1.55]")}>{w.what}</p>
              </li>
            ))}
          </ul>
        </Reveal>
      </section>

      <section id="pilot" aria-labelledby="start-title" className="scroll-mt-4 bg-(--mint)">
        <Reveal className={cn(wrap, section)}>
          <h2 id="start-title" className={cn(display, "-ml-[0.04em]")}>
            Try it at one clinic first
          </h2>
          <div className="mt-10 flex flex-wrap items-end justify-between gap-10">
            <p className={cn(lead, "max-w-[38ch]")}>
              Three months, free, alongside your team. A clinician checks every case before Wasl may mark it done.
            </p>
            <div className="flex flex-wrap items-center gap-7">
              <ContactCta />
              <Link href="/pilot" className={cn(link, "text-(--ink)")}>
                See how a pilot works
              </Link>
            </div>
          </div>
        </Reveal>
      </section>

      <SiteFooter
        big
        honesty="simulated"
        qualifier="Fatima and every other patient shown here are made up."
        rest="Test results come from simulated patients; Wasl claims no results on real patients yet. The 72-day figure is from a published Abu Dhabi study (BMC Cancer, 2021)."
      />
    </>
  );
}


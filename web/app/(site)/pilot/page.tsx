import Link from "next/link";
import { ContactCta } from "@/components/site/contact";
import { QUOTED } from "@/components/site/figures";
import { SiteFooter } from "@/components/site/footer";
import { Reveal } from "@/components/site/reveal";
import { body, cta, display, figure, h2, h3, lead, link, section, small, wrap } from "@/components/site/styles";
import { cn } from "@/lib/utils";

export const metadata = { title: "Pilot" };

const twoUp = "grid grid-cols-[repeat(auto-fit,minmax(min(100%,420px),1fr))] gap-[clamp(2rem,6vw,8rem)]";

const TERMS = [
  { head: "Free for three months", text: "No cost to your health system during the trial." },
  { head: "One clinic, one hospital", text: "A small, real setting where every case can be followed by hand." },
  { head: "A clinician checks every case", text: "Wasl suggests. A clinician confirms before any case is marked done." },
  { head: "One wrong “done” ends it", text: "If a single case is marked done when it should not be, the trial stops." },
];

/** Bar widths follow each phase's length in months (3, 6, 3, then open-ended). */
const PHASES = [
  {
    when: "Months 1–3",
    title: "Run alongside",
    text: "Wasl works next to your team at one clinic and one hospital. It decides nothing alone.",
    gate: "No case wrongly marked done, and it finds the cases your team finds.",
    fill: "bg-(--ink)",
  },
  {
    when: "Months 4–9",
    title: "Grow clinic by clinic",
    text: "Six to eight clinics join one at a time, so each can be compared with how it did before it joined.",
    gate: "More people get their check on time, and still no case wrongly marked done.",
    fill: "bg-(--teal)",
  },
  {
    when: "Months 10–12",
    title: "Agree terms",
    text: "If the tests are passed, we agree a contract and a price with the health system.",
    gate: "A signed agreement.",
    fill: "bg-[#4f8a84]",
  },
  {
    when: "After a year",
    title: "Other screenings",
    text: "The same approach for cervical, then breast screening.",
    gate: "Each new screening starts again with a clinician checking every case.",
    fill: "bg-[#b8d3ce]",
  },
];

const COSTS = [
  {
    k: "What you provide",
    v: "Access to the referral and hospital report records for one clinic and one hospital, a clinician to check Wasl’s work, and a data protection review that runs alongside.",
  },
  { k: "What we provide", v: "Wasl, set up and run by our team, with every decision it makes open to your review." },
  {
    k: "After the trial",
    v: "For one screening programme alone, the cost is hard to justify. Shared across several, such as bowel, cervical and breast, it becomes good value. We plan that with you.",
  },
];

const LIMITS = [
  {
    limit: "It has only met made-up patients",
    detail: "Every result so far comes from testing on simulated patients. The trial is its first time with real records.",
    test: "In the trial, its safety threshold is re-tuned on your own cases, with a clinician checking every one.",
  },
  {
    limit: "It cannot add hospital appointments",
    detail: "If there are too few slots for follow-up checks, people still wait, however well they are tracked.",
    test: "In the trial, it shows where slots run short, so the health system can act.",
  },
  {
    limit: "Some cases will always need people",
    detail: "When Wasl is unsure, it asks. That is on purpose, and it means your team keeps part of the work.",
    test: "In the trial, it counts how many cases it hands back, so you can plan staff time.",
  },
  {
    limit: "Some names go to a person more often",
    detail: "In testing, names from some backgrounds needed a person’s review more often than others. We are working to even this out.",
    test: "In the trial, this is reported for every site, against a set limit.",
  },
  {
    limit: "Its translations need a human check",
    detail: "The messages patients receive in their own language are drafted by machine, and machines make mistakes with medical words.",
    test: "In the trial, messages go out only after a professional translator has approved them.",
  },
  {
    limit: "It is not yet live in a hospital",
    detail: "The connection to hospital records is built and tested, but not yet running anywhere.",
    test: "The trial is its first live connection, at one clinic and one hospital.",
  },
];

export default function Page() {
  return (
    <>
      <header className={cn(wrap, twoUp, "items-start pb-20 pt-12 md:pb-[120px] md:pt-[72px]")}>
        <div>
          <h1 className={cn(display, "-ml-[0.04em]")}>
            Start with
            <br /> one clinic
          </h1>
          <p className={cn(lead, "mt-10 max-w-[34ch]")}>
            For three months, Wasl runs alongside your team at one clinic and the hospital it sends people to. It is free, and it closes
            nothing on its own.
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
        <section aria-labelledby="terms-title" className="bg-(--sand) p-[clamp(1.5rem,3vw,2.5rem)]">
          <h2 id="terms-title" className={h3}>
            The terms of the trial
          </h2>
          <ul className="m-0 mt-7 flex list-none flex-col gap-7 p-0">
            {TERMS.map((t) => (
              <li key={t.head} className="flex items-start gap-4">
                <span aria-hidden className="mt-1.5 size-4 flex-none bg-(--teal)" />
                <div>
                  <div className={h3}>{t.head}</div>
                  <div className="mt-1 text-[18px] leading-[1.55]">{t.text}</div>
                </div>
              </li>
            ))}
          </ul>
        </section>
      </header>

      <section aria-labelledby="steps-title">
        <Reveal className={cn(wrap, "pb-20 md:pb-[120px]")}>
          <h2 id="steps-title" className={h2}>
            Each step must pass first
          </h2>
          <ol className="m-0 mt-16 grid min-w-0 list-none grid-cols-1 gap-x-2 gap-y-10 p-0 md:grid-cols-[3fr_6fr_3fr_4fr]">
            {PHASES.map((p) => (
              <li key={p.when} className="min-w-0">
                <div aria-hidden className={cn("h-4", p.fill)} />
                <div className="pr-6 pt-6">
                  <div className={small}>{p.when}</div>
                  <h3 className={cn(h3, "mt-1.5")}>{p.title}</h3>
                  <p className={cn(body, "mt-3 max-w-[46ch] leading-[1.55]")}>{p.text}</p>
                  <div className={cn(small, "mt-6")}>To move on</div>
                  <p className="m-0 mt-1 max-w-[46ch] text-[18px] font-semibold leading-normal">{p.gate}</p>
                </div>
              </li>
            ))}
          </ol>
          <p className={cn(lead, "mt-14 max-w-[52ch]")} data-figure="streak">
            Wasl may close a case without a clinician only after it has been right {QUOTED.pilotStreak.value} times in a row. Even then, a
            person still checks one case in ten. This is a rule of the trial, not a result.
          </p>
        </Reveal>
      </section>

      <section aria-labelledby="cost-title" className="bg-(--sand)">
        <Reveal className={cn(wrap, section, twoUp, "items-start")}>
          <div>
            <h2 id="cost-title" className={h2}>
              What it costs to try
            </h2>
            <div className={cn(figure, "mt-12 text-(--teal)")}>Free</div>
            <p className={cn(lead, "mt-6 max-w-[30ch]")}>
              The three-month trial costs your health system nothing. You pay only if it passes and you choose to go on.
            </p>
          </div>
          <dl className="m-0 flex flex-col gap-2">
            {COSTS.map((c) => (
              <div key={c.k} className="bg-(--paper) px-[clamp(1.25rem,2.5vw,2rem)] py-7">
                <dt className={h3}>{c.k}</dt>
                <dd className={cn(body, "mt-2 max-w-[56ch]")}>{c.v}</dd>
              </div>
            ))}
          </dl>
        </Reveal>
      </section>

      <section aria-labelledby="limits-title">
        <Reveal className={cn(wrap, section)}>
          <div className="flex flex-wrap items-end justify-between gap-x-16 gap-y-6">
            <h2 id="limits-title" className={h2}>
              What it cannot do yet
            </h2>
            <p className={cn(lead, "max-w-[34ch]")}>We found these ourselves. Each one has a way the trial puts it to the test.</p>
          </div>
          <ul className="m-0 mt-16 grid list-none grid-cols-[repeat(auto-fit,minmax(min(100%,460px),1fr))] gap-x-[clamp(2rem,6vw,8rem)] gap-y-14 p-0">
            {LIMITS.map((l) => (
              <li key={l.limit}>
                <h3 className={h3}>{l.limit}</h3>
                <p className={cn(body, "mt-2.5 max-w-[52ch]")}>{l.detail}</p>
                <p className={cn(body, "mt-3 max-w-[52ch] font-semibold text-(--teal)")}>{l.test}</p>
              </li>
            ))}
          </ul>
        </Reveal>
      </section>

      <section id="pilot" aria-labelledby="request-title" className="scroll-mt-4 bg-(--mint)">
        <Reveal className={cn(wrap, section)}>
          <h2 id="request-title" className={cn(display, "-ml-[0.04em]")}>
            Tell us where to start
          </h2>
          <div className="mt-10 flex flex-wrap items-end justify-between gap-10">
            <p className={cn(lead, "max-w-[38ch]")}>
              Name one clinic and the hospital it sends people to. We will plan the trial with your team, and agree what success looks like
              before it begins.
            </p>
            <ContactCta />
          </div>
        </Reveal>
      </section>

      <SiteFooter
        honesty="designed"
        qualifier="The trial plan and its tests are a proposal, to be agreed with you."
        rest="Everything Wasl has shown so far comes from testing on simulated patients. No results on real patients are claimed."
      />
    </>
  );
}

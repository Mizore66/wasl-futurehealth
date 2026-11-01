import Link from "next/link";
import { Avatar } from "@/components/avatar/avatar";
import { SiteFooter } from "@/components/site/footer";
import { Reveal } from "@/components/site/reveal";
import { body, cta, display, h2, h3, lead, section, small, wrap } from "@/components/site/styles";
import { CAST } from "@/components/story/script";
import { cn } from "@/lib/utils";

export const metadata = { title: "How it works" };

/** Who the step's dot shows: Fatima, or the hospital's record before Wasl has joined it to her. */
type Face = "fatima" | "record";

type Step = {
  face: Face;
  title: string;
  text: string;
  cardLabel: string;
  arabic?: string;
  main: string;
  mainKind: "big" | "quote" | "plain" | "done";
  lines: { k: string; v: string }[];
};

const STEPS: Step[] = [
  {
    face: "fatima",
    title: "Her test asks for a closer look",
    text: "At her clinic in Al Mushrif, Fatima’s screening test comes back asking for a follow-up check. Wasl opens her case the same day and gives it to a named nurse, so it always has an owner.",
    cardLabel: "Her case",
    main: "Opened",
    mainKind: "big",
    lines: [
      { k: "Looked after by", v: "Her clinic nurse" },
      { k: "Next", v: "Tell Fatima, then book her check" },
    ],
  },
  {
    face: "fatima",
    title: "She is told, and asked to say it back",
    text: "She gets a message in Arabic. The nurse asks her to explain it in her own words. The first time, she had it wrong, so the nurse explained again. Reminders wait until she agrees to receive them.",
    cardLabel: "The message she receives",
    arabic: "نتيجة فحصك تحتاج إلى فحص إضافي. ستتصل بك الممرضة لحجز موعد.",
    main: "Your result needs a follow-up check. Your nurse will call you to book it.",
    mainKind: "plain",
    lines: [
      { k: "First answer", v: "“So I don’t need to do anything?”" },
      { k: "After the nurse", v: "“I need to book a check at the hospital.”" },
    ],
  },
  {
    face: "record",
    title: "Her check happens across the city",
    text: "She has the check at a hospital in another part of Abu Dhabi. Their record spells her name differently and leaves out her ID number. On its own, her clinic would never see it.",
    cardLabel: "The hospital’s record",
    main: "F. Alzabi",
    mainKind: "big",
    lines: [
      { k: "Born", v: "14 March 1972" },
      { k: "ID number", v: "Missing" },
    ],
  },
  {
    face: "fatima",
    title: "Wasl recognises her",
    text: "Wasl knows an Arabic name can be written many ways in English letters. It suggests the match, then joins the records only when a second detail, her birth date, agrees too. If it is not sure, a person decides.",
    cardLabel: "Two records, one person",
    arabic: "فاطمة الزعابي",
    main: "F. Alzabi",
    mainKind: "big",
    lines: [
      { k: "Birth date", v: "The same on both" },
      { k: "Decision", v: "Joined" },
    ],
  },
  {
    face: "fatima",
    title: "Two readers check the report",
    text: "Two separate readers, one following fixed medical rules and one AI, read the hospital’s report. Both must find proof that the check was complete before anything moves on.",
    cardLabel: "From the hospital’s report",
    main: "“The preparation was excellent and all surfaces were well seen.”",
    mainKind: "quote",
    lines: [
      { k: "Rules reader", v: "Complete" },
      { k: "AI reader", v: "Complete, they agree" },
    ],
  },
  {
    face: "fatima",
    title: "Done, with the proof attached",
    text: "Only now is her case marked done. The proof, the match and both readings are written into a record where any later change shows.",
    cardLabel: "Her case",
    main: "Done, with proof",
    mainKind: "done",
    lines: [
      { k: "Proof", v: "The hospital’s own words" },
      { k: "Checked by", v: "Two readers who agree" },
    ],
  },
];

const BIG = "mt-2.5 text-[clamp(2rem,1rem_+_3vw,4rem)] font-bold leading-[1.1] tracking-[-0.02em]";
const MAIN_CLASS: Record<Step["mainKind"], string> = {
  big: BIG,
  done: cn(BIG, "text-(--teal)"),
  quote: "mt-2.5 text-[24px] font-semibold leading-[1.4]",
  plain: "mt-1.5 text-[18px] leading-normal",
};

const FATIMA = CAST.fatima.seed;
const OTHER = CAST.aisha.seed;

/** A face on the thread: teal ring once Wasl knows who it is, grey ring around the "?" record. */
function Face({ face, size }: { face: Face; size: number }) {
  const known = face === "fatima";
  return (
    <span className={cn("relative inline-flex flex-none rounded-full p-[3px] ring-[3px] ring-inset", known ? "ring-(--teal)" : "ring-(--stub)")}>
      <Avatar person={known ? FATIMA : undefined} size={size} label="" />
    </span>
  );
}

function Artefact({ s }: { s: Step }) {
  return (
    <div className="border-t border-(--line) pt-5">
      <div className={small}>{s.cardLabel}</div>
      {s.arabic && (
        <div lang="ar" dir="rtl" className="font-arabic mt-2.5 text-left text-[24px] font-semibold leading-normal">
          {s.arabic}
        </div>
      )}
      <div className={MAIN_CLASS[s.mainKind]}>{s.main}</div>
      {s.lines.map((l) => (
        <div key={l.k} className="mt-3 flex flex-wrap items-baseline gap-x-3">
          <span className={cn(small, "min-w-36 flex-none")}>{l.k}</span>
          <span className="text-[18px] font-semibold leading-[1.45]">{l.v}</span>
        </div>
      ))}
    </div>
  );
}

const badge = "inline-flex size-16 flex-none items-center justify-center rounded-full";

export default function Page() {
  return (
    <>
      <header className={cn(wrap, "pb-20 pt-12 md:pb-[120px] md:pt-[72px]")}>
        <h1 className={cn(display, "-ml-[0.04em]")}>
          One person’s
          <br /> path to done
        </h1>
        <div className="mt-14 grid grid-cols-[repeat(auto-fit,minmax(min(100%,420px),1fr))] items-center gap-[clamp(2rem,6vw,8rem)]">
          <p className={cn(lead, "max-w-[34ch]")}>
            Follow Fatima from a screening result at her local clinic to a follow-up check across the city, proven complete.
          </p>
          <div
            role="img"
            aria-label="Drawing of Fatima’s route: she goes from her clinic in Al Mushrif to a hospital across the city for her check, and Wasl brings the proof back to her clinic."
            className="grid grid-cols-[auto_1fr_auto] items-center gap-y-6"
          >
            <span className="row-span-2">
              <Face face="fatima" size={72} />
            </span>
            <div className="px-4">
              <div className="text-[18px] leading-[1.45]">She goes for her check</div>
              <div className="mt-2.5 border-t-[3px] border-dashed border-(--stub)" />
            </div>
            <span className="col-start-3 row-span-2 row-start-1">
              <Face face="record" size={72} />
            </span>
            <div className="col-start-2 px-4">
              <div className="h-2 rounded-full bg-(--teal)" />
              <div className="mt-2.5 text-[18px] font-bold leading-[1.45] text-(--teal)">Wasl brings the proof back</div>
            </div>
            <div className="col-span-3 flex justify-between gap-6 text-[clamp(1.125rem,1rem_+_0.6vw,1.5rem)] font-bold leading-[1.3]">
              <span>Her clinic, Al Mushrif</span>
              <span className="text-right">A hospital across the city</span>
            </div>
          </div>
        </div>
      </header>

      <section aria-labelledby="path-title" className="bg-(--sand)">
        <div className={cn(wrap, section)}>
          <h2 id="path-title" className={h2}>
            From a result to a receipt
          </h2>
          <ol className="m-0 mt-16 list-none p-0">
            {STEPS.map((s, i) => (
              <li
                key={s.title}
                data-face={s.face}
                className="grid grid-cols-[56px_minmax(0,1fr)] gap-x-[clamp(1rem,3vw,3rem)] md:grid-cols-[56px_minmax(0,5fr)_minmax(0,6fr)]"
              >
                <div aria-hidden className="relative row-span-2 flex justify-center md:row-span-1">
                  <span
                    className={cn(
                      "absolute left-1/2 w-[3px] -translate-x-1/2 bg-(--teal)",
                      i === 0 ? "top-7" : "top-0",
                      i === STEPS.length - 1 ? "h-7" : "bottom-0",
                    )}
                  />
                  <span className="relative self-start rounded-full bg-(--sand)">
                    <Face face={s.face} size={44} />
                  </span>
                </div>
                <Reveal className="pb-6 md:pb-[72px]">
                  <h3 className={h3}>{s.title}</h3>
                  <p className={cn(body, "mt-3 max-w-[48ch]")}>{s.text}</p>
                </Reveal>
                <Reveal className="col-start-2 min-w-0 pb-14 md:col-start-3 md:pb-[72px]">
                  <Artefact s={s} />
                </Reveal>
              </li>
            ))}
          </ol>
        </div>
      </section>

      <section aria-labelledby="trap-title">
        <Reveal className={cn(wrap, section)}>
          <h2 id="trap-title" className={h2}>
            When it looks done but isn’t
          </h2>
          <div className="mt-7 flex items-center gap-5">
            <Avatar person={OTHER} size={64} label="" />
            <p className={cn(lead, "max-w-[46ch]")}>Another patient’s report says her check began. It never says the check was finished.</p>
          </div>
          <div className="mt-14 grid grid-cols-[repeat(auto-fit,minmax(min(100%,420px),1fr))] gap-2">
            <div className="bg-(--sand-deep) p-[clamp(1.5rem,4vw,3.5rem)]" data-side="tracker">
              <h3 className={h3}>A simple tracker</h3>
              <div className="mt-7 flex items-center gap-5">
                <span aria-hidden className={cn(badge, "bg-(--ink) text-(--paper)")}>
                  <svg width="32" height="32" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
                    <path d="M5 12l5 5 9-10" />
                  </svg>
                </span>
                <span className={h2}>Ticked done</span>
              </div>
              <p className={cn(body, "mt-7 max-w-[44ch]")}>It sees a report from the hospital and closes the case. Nobody calls her again.</p>
            </div>
            <div className="bg-(--held-bg) p-[clamp(1.5rem,4vw,3.5rem)]" data-side="wasl">
              <h3 className={h3}>Wasl</h3>
              <div className="mt-7 flex items-center gap-5">
                <span aria-hidden className={cn(badge, "bg-(--held) text-(--held-bg)")}>
                  <svg width="32" height="32" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
                    <path d="M9 6v12M15 6v12" />
                  </svg>
                </span>
                <span className={h2}>Kept open</span>
              </div>
              <p className={cn(body, "mt-7 max-w-[44ch]")}>
                A check that stops halfway has not looked everywhere. Wasl keeps her case open and asks a person to look, so she can be called
                back.
              </p>
            </div>
          </div>
        </Reveal>
      </section>

      <section id="decide" aria-labelledby="decide-title" className="scroll-mt-4 bg-(--sand)">
        <Reveal className={cn(wrap, section, "flex flex-col items-center text-center")}>
          <h2 id="decide-title" className={h2}>
            Who decides
          </h2>
          <p className="m-0 mt-8 max-w-[36ch] text-balance text-[clamp(1.5rem,1rem_+_1.4vw,2.25rem)] font-medium leading-[1.3] tracking-[-0.01em]">
            Wasl suggests. When it is unsure, two people decide. Every step is written down, so anyone can see later who did what.
          </p>
          <Link href="/app" className={cn(cta, "mt-12")}>
            Open the dashboard
          </Link>
        </Reveal>
      </section>

      <SiteFooter
        honesty="illustrative"
        qualifier="Fatima and the other patient are made up."
        rest="Their stories are built from the cases Wasl is tested on; no results on real patients are claimed."
      />
    </>
  );
}

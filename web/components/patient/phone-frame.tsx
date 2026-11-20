import Link from "next/link";
import { Mark } from "@/components/brand/mark";
import { HonestyLabel } from "@/components/wasl/honesty-label";
import { cn } from "@/lib/utils";
import type { Lang } from "./i18n";
import { ALL_DICTS as DICTS } from "./i18n/drafts";
import { phoneHref, STEPS, surfaceLangs, type PhoneQuery, type Step } from "./journey";
import { LanguageSheet } from "./language-sheet";
import { PHONE_FONT } from "./parts";

const STEP_NAMES: Record<Step, string> = { notice: "Notice", check: "Check", book: "Book", outcome: "Outcome" };
const LANG_NAMES_EN: Record<Lang, string> = { ar: "Arabic", en: "English", ur: "Urdu", fil: "Filipino" };

/**
 * Shown above every step of the internal translation-review preview (#133). The integration badge says nothing
 * about translation review, so the preview states it in staff English, outside the phone's own language.
 */
function UnreviewedTranslationWarning({ lang }: { lang: Lang }) {
  return (
    <div role="note" data-testid="unreviewed-translation" lang="en" dir="ltr" className="mx-5 mt-3 rounded-control border border-control bg-raised p-3 text-small leading-[1.6] text-ink">
      <p className="m-0 font-medium">Internal translation review preview. Not for patients.</p>
      <p className="m-0 mt-1">
        This {LANG_NAMES_EN[lang]} medical copy is an unreviewed draft. It has not been reviewed by a qualified medical translator and is not approved for patients.
      </p>
    </div>
  );
}

/** The phone column: a demo strip (English, honesty label, step links), then the patient's screen in her language. */
export function PhoneFrame({ step, query, live = false, children }: { step: Step; query: PhoneQuery; live?: boolean; children: React.ReactNode }) {
  const t = DICTS[query.lang];
  const review = query.surface === "review";
  const options = surfaceLangs(query.surface).map((lang) => ({
    lang,
    dir: DICTS[lang].dir,
    label: DICTS[lang].langName,
    href: phoneHref(step, { ...query, lang }),
    font: PHONE_FONT[lang],
  }));
  return (
    <div className="min-h-dvh bg-ground">
      <div className="mx-auto flex min-h-dvh w-full max-w-[390px] flex-col bg-phone-ground min-[420px]:border-x min-[420px]:border-line">
        <nav aria-label="Demo steps" lang="en" dir="ltr" className="flex flex-wrap items-center gap-3 px-5 pt-3 text-small">
          {live ? <HonestyLabel kind="built">Simulated · answers saved</HonestyLabel> : <HonestyLabel kind="designed">Design preview · answers not saved</HonestyLabel>}
          <ol className="m-0 ms-auto flex flex-wrap list-none gap-3 p-0">
            {STEPS.map((s, i) => (
              <li key={s}>
                <Link
                  href={phoneHref(s, query)}
                  aria-current={s === step ? "page" : undefined}
                  className={cn(
                    "inline-flex min-h-6 items-center hover:text-ink",
                    s === step ? "font-medium text-ink underline underline-offset-4" : "text-muted",
                  )}
                >
                  {i + 1} {STEP_NAMES[s]}
                </Link>
              </li>
            ))}
          </ol>
        </nav>
        {review && <UnreviewedTranslationWarning lang={query.lang} />}
        <div lang={query.lang} dir={t.dir} style={{ fontFamily: PHONE_FONT[query.lang] }} className="flex flex-1 flex-col text-ink">
          <header className="flex min-h-11 items-center gap-2.5 px-5 pt-4 pb-2">
            <Mark size={24} />
            <span className="text-[15px] font-medium">{t.clinic}</span>
            <LanguageSheet current={query.lang} title={t.langTitle} closeLabel={t.close} currentLabel={t.langName} options={options} />
          </header>
          {children}
        </div>
      </div>
    </div>
  );
}

import Link from "next/link";
import { Mark } from "@/components/brand/mark";
import { HonestyLabel } from "@/components/wasl/honesty-label";
import { cn } from "@/lib/utils";
import { DICTS, LANGS } from "./i18n";
import { phoneHref, STEPS, type PhoneQuery, type Step } from "./journey";
import { LanguageSheet } from "./language-sheet";
import { PHONE_FONT } from "./parts";

const STEP_NAMES: Record<Step, string> = { notice: "Notice", check: "Check", book: "Book" };

/** The phone column: a demo strip (English, honesty label, step links), then the patient's screen in her language. */
export function PhoneFrame({ step, query, live = false, children }: { step: Step; query: PhoneQuery; live?: boolean; children: React.ReactNode }) {
  const t = DICTS[query.lang];
  const options = LANGS.map((lang) => ({
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
          {live ? <HonestyLabel kind="built">Synthetic · server saved</HonestyLabel> : <HonestyLabel kind="designed">Designed, not built · no API</HonestyLabel>}
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

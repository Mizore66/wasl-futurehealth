import { BottomSheet } from "./bottom-sheet";
import { DICTS, type Lang } from "./i18n";
import { CallBar, PhoneNumber, phoneType } from "./parts";

export function NoticeScreen({ lang }: { lang: Lang }) {
  const t = DICTS[lang];
  const n = t.notice;
  const type = phoneType(lang);
  return (
    <>
      <main className="flex flex-1 flex-col gap-3 px-5 pt-5 pb-4">
        <div className="pb-3">
          <p className="m-0 mb-1.5 text-[16px] text-muted">{n.greet}</p>
          <h1 className={type.h1}>{n.title}</h1>
          <p className={type.body}>{n.body}</p>
        </div>

        <section aria-labelledby="next-step" className="rounded-card border border-line bg-surface p-5">
          <div className="mb-2 flex items-center gap-2">
            <span aria-hidden="true" className="size-2.5 flex-none rounded-full bg-hue" />
            <h2 id="next-step" className="m-0 text-[15px] font-medium text-hue">
              {n.nextHead}
            </h2>
          </div>
          <p className={type.body}>{n.next}</p>
          <div className="mt-4 flex items-center gap-3">
            <span aria-hidden="true" className="flex size-11 flex-none items-center justify-center rounded-full bg-raised">
              <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="var(--color-ink-soft)" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
                <circle cx="12" cy="8" r="4" />
                <path d="M4 21c1.5-4 4.5-6 8-6s6.5 2 8 6" />
              </svg>
            </span>
            <div>
              <div className="text-[17px] font-medium">{n.nurse}</div>
              <div className="text-[14px] leading-[1.7] text-muted">
                {n.alsoCall} <PhoneNumber />
              </div>
            </div>
          </div>
        </section>

        <BottomSheet
          title={n.rights}
          closeLabel={t.close}
          trigger={
            <button
              type="button"
              className="flex min-h-11 w-full cursor-pointer items-center gap-2 border-0 bg-transparent px-1 text-start text-ink-soft"
            >
              <svg aria-hidden="true" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" className="flex-none">
                <path d="M12 3l8 3v6c0 4.5-3.2 8-8 9-4.8-1-8-4.5-8-9V6z" />
              </svg>
              <span className="flex-1 text-[16px]">{n.rights}</span>
              <svg aria-hidden="true" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="var(--color-muted)" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" className="flex-none ltr:-scale-x-100">
                <path d="M15 6l-6 6 6 6" />
              </svg>
            </button>
          }
        >
          <ul className="m-0 flex list-none flex-col gap-3 p-0">
            {n.rightsItems.map((r) => (
              <li key={r} className={type.body}>
                {r}
              </li>
            ))}
          </ul>
          <p className="m-0 mt-4 text-[14px] leading-[1.8] text-muted">{n.rightsNote}</p>
        </BottomSheet>
      </main>
      <CallBar label={t.call} />
    </>
  );
}

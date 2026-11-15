import type { PatientScript } from "./programme";

/** Marks gated English programme copy so screen readers and fonts treat it as English inside a translated phone. */
export function scriptLang(script: PatientScript): { lang?: "en"; dir?: "ltr" } {
  return script.gated ? { lang: "en", dir: "ltr" } : {};
}

export function GatedNote({ script }: { script: PatientScript }) {
  if (!script.gated) return null;
  return (
    <p role="note" lang="en" dir="ltr" className="m-0 rounded-control bg-raised p-3 text-[14px] leading-[1.6] text-muted">
      This information is in English until a reviewed translation is ready. Call the care team for help in your language.
    </p>
  );
}

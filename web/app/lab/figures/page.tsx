import type { Metadata } from "next";
import { Turntable } from "./turntable";

export const metadata: Metadata = { title: "Figure lab", robots: { index: false } };

export default function FigureLab() {
  return (
    <main className="min-h-dvh bg-(--paper) px-6 py-8 text-(--ink)">
      <p className="m-0 inline-block bg-(--sand) px-2 py-0.5 text-[13px] font-semibold">Lab · not part of the product</p>
      <h1 className="m-0 mt-2 text-[28px] font-bold leading-tight">Figures for the landing story</h1>
      <p className="m-0 mt-1 max-w-[70ch] text-[15px] text-(--muted)">
        Straight-cylinder proportions on the real kit and matcap. Each row is Fatima, Khalid, a grey unknown and six random seeds, close up and at road-shot size (about 40px tall). A is the one the story uses.
      </p>
      <Turntable />
    </main>
  );
}

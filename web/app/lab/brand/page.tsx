import type { Metadata } from "next";
import { BRAND, BrandLockup, BrandSymbol, CONCEPTS, type Concept } from "@/components/brand/concepts";

export const metadata: Metadata = { title: "Brand lab", robots: { index: false } };

const SIZES = [16, 32, 64, 256] as const;

const CSS = `
.brand { background: ${BRAND.paper}; color: ${BRAND.ink}; min-height: 100dvh; padding: 32px 40px 96px; font-size: 14px; line-height: 20px; }
.brand h1 { font-size: 28px; line-height: 36px; font-weight: 600; letter-spacing: -0.01em; margin: 4px 0; }
.brand h2 { font-size: 20px; line-height: 28px; font-weight: 600; margin: 0; }
.brand h3 { font-size: 13px; font-weight: 600; color: #56666a; margin: 18px 0 8px; text-transform: uppercase; letter-spacing: 0.04em; }
.brand .badge { display: inline-block; padding: 2px 8px; border-radius: 4px; background: #f5e8cc; color: #8a5a12; font-weight: 600; font-size: 12px; }
.brand .note { color: #56666a; max-width: 72ch; }
.brand .concept { background: ${BRAND.surface}; border: 1px solid #e2d9c9; border-radius: 12px; padding: 24px; margin-top: 24px; }
.brand .sizes { display: flex; align-items: flex-end; gap: 28px; padding: 20px; border-radius: 8px; flex-wrap: wrap; }
.brand .sizes figure { margin: 0; display: flex; flex-direction: column; align-items: center; gap: 6px; }
.brand .sizes figcaption { font-size: 11px; color: #56666a; }
.brand .on-ink { background: ${BRAND.ink}; }
.brand .on-ink figcaption { color: #9fb3b1; }
.brand .on-paper { background: ${BRAND.paper}; }
.brand .grid2 { display: grid; grid-template-columns: 1fr 1fr; gap: 12px; }
.brand .lockbox { padding: 28px 24px; border-radius: 8px; display: flex; align-items: center; gap: 28px; flex-wrap: wrap; }
.brand .mocks { display: grid; grid-template-columns: 340px 260px 240px; gap: 16px; align-items: start; }
.brand .tab-bar { background: #dfe3e6; border-radius: 8px 8px 0 0; padding: 8px 8px 0; }
.brand .tab { display: flex; align-items: center; gap: 8px; background: #fff; border-radius: 8px 8px 0 0; padding: 8px 12px; width: 220px; font-size: 12px; color: #1f2328; }
.brand .tab.dark { background: #35363a; color: #e8eaed; }
.brand .tab-bar.dark { background: #202124; }
.brand .side { background: ${BRAND.paper}; border: 1px solid #e2d9c9; border-radius: 8px; padding: 14px 12px; }
.brand .side-head { display: flex; align-items: center; gap: 10px; }
.brand .side-head b { display: block; font-size: 14px; line-height: 18px; }
.brand .side-head span { display: block; font-size: 12px; color: #56666a; }
.brand .side-nav { margin-top: 12px; display: grid; gap: 2px; font-size: 14px; }
.brand .side-nav div { padding: 6px 10px; border-radius: 6px; }
.brand .side-nav div:first-child { background: #e6dccb; }
.brand .phone { width: 240px; border: 8px solid #1d2b2d; border-radius: 30px; overflow: hidden; background: ${BRAND.paper}; }
.brand .phone-head { display: flex; align-items: center; justify-content: space-between; padding: 14px 14px 12px; border-bottom: 1px solid #e2d9c9; background: ${BRAND.surface}; }
.brand .phone-body { padding: 14px; font-size: 13px; color: #34494b; height: 120px; }
.brand .og-wrap { width: 600px; height: 315px; overflow: hidden; border-radius: 8px; border: 1px solid #e2d9c9; margin-top: 12px; }
.brand .og { width: 1200px; height: 630px; transform: scale(0.5); transform-origin: 0 0; background: ${BRAND.paper}; display: flex; flex-direction: column; justify-content: space-between; padding: 72px 80px; box-sizing: border-box; }
.brand .og p { margin: 0; font-size: 60px; line-height: 1.04; letter-spacing: -0.03em; font-weight: 700; max-width: 900px; }
.brand .og small { font-size: 22px; color: #56666a; }
.brand .checks { display: flex; gap: 24px; align-items: center; flex-wrap: wrap; }
.brand .checks [data-check] { display: inline-block; line-height: 0; }
`;

function SizeRow({ concept, ground }: { concept: Concept; ground: "paper" | "ink" }) {
  const onInk = ground === "ink";
  return (
    <div className={`sizes ${onInk ? "on-ink" : "on-paper"}`}>
      {SIZES.map((s) => (
        <figure key={s}>
          <BrandSymbol
            concept={concept}
            size={s}
            color={onInk ? BRAND.mint : BRAND.teal}
            knockout={onInk ? BRAND.ink : BRAND.paper}
          />
          <figcaption>{s}px tile</figcaption>
        </figure>
      ))}
      {[16, 32, 64].map((s) => (
        <figure key={`m${s}`}>
          <BrandSymbol concept={concept} size={s} variant="mark" color={onInk ? BRAND.paper : BRAND.ink} />
          <figcaption>{s}px one-colour</figcaption>
        </figure>
      ))}
    </div>
  );
}

function Mocks({ concept }: { concept: Concept }) {
  return (
    <>
      <div className="mocks">
        <div>
          <div className="tab-bar">
            <div className="tab">
              <BrandSymbol concept={concept} size={16} />
              Programme · Wasl
            </div>
          </div>
          <div className="tab-bar dark" style={{ marginTop: 8 }}>
            <div className="tab dark">
              <BrandSymbol concept={concept} size={16} />
              Programme · Wasl
            </div>
          </div>
        </div>
        <div className="side">
          <div className="side-head">
            <BrandSymbol concept={concept} size={28} />
            <div>
              <b>Wasl</b>
              <span>ADPHC · CRC programme</span>
            </div>
          </div>
          <div className="side-nav">
            <div>Programme</div>
            <div>Today</div>
            <div>Review</div>
          </div>
        </div>
        <div className="phone">
          <div className="phone-head">
            <BrandLockup concept={concept} height={20} />
            <span style={{ fontSize: 12, color: "#56666a" }}>EN · ع</span>
          </div>
          <div className="phone-body">Your follow-up check is booked. We will tell your clinic for you.</div>
        </div>
      </div>
      <div className="og-wrap">
        <div className="og">
          <BrandLockup concept={concept} height={64} />
          <p>Every positive screening test, followed to a documented end.</p>
          <small>Abu Dhabi public health programme · synthetic data demo</small>
        </div>
      </div>
    </>
  );
}

export default function BrandLab() {
  return (
    <main className="brand">
      <style>{CSS}</style>
      <span className="badge">Lab · not part of the product</span>
      <h1>Wasl mark: four concepts</h1>
      <p className="note">
        Designed, not shipped. Each symbol is hand-drawn SVG on a 32-unit grid. The tile is the favicon and app icon; the bare mark is the
        one-colour version used in lockups. The Arabic in each lockup is live text in IBM Plex Sans Arabic.
      </p>

      <h3>Current mark, for comparison</h3>
      <div className="sizes on-paper">
        {[16, 32].map((s) => (
          <figure key={s}>
            <span
              style={{ width: s, height: s, borderRadius: s * 0.25, background: BRAND.ink, color: "#fff", display: "flex", alignItems: "center", justifyContent: "center", fontSize: s * 0.62, fontWeight: 600, fontFamily: "var(--font-arabic)" }}
            >
              و
            </span>
            <figcaption>{s}px و tile</figcaption>
          </figure>
        ))}
      </div>

      {CONCEPTS.map((c, i) => (
        <section key={c.id} className="concept" id={c.id}>
          <h2>
            {String.fromCharCode(97 + i)}) {c.name}
          </h2>
          <p className="note">{c.idea}</p>
          <h3>Sizes</h3>
          <div className="grid2">
            <SizeRow concept={c} ground="paper" />
            <SizeRow concept={c} ground="ink" />
          </div>
          <h3>Lockups</h3>
          <div className="grid2">
            <div className="lockbox on-paper">
              <BrandLockup concept={c} height={40} />
              <BrandLockup concept={c} height={22} oneColour={BRAND.ink} />
            </div>
            <div className="lockbox on-ink">
              <BrandLockup concept={c} height={40} tone="dark" />
              <BrandLockup concept={c} height={22} oneColour={BRAND.paper} />
            </div>
          </div>
          <h3>In place</h3>
          <Mocks concept={c} />
        </section>
      ))}

      <section className="concept">
        <h2>Raster checks</h2>
        <p className="note">Exact-size elements for screenshot checks at 16 and 32 pixels.</p>
        <div className="checks">
          {CONCEPTS.flatMap((c) =>
            [16, 32].map((s) => (
              <span key={`${c.id}-${s}`} data-check={`${c.id}-${s}`}>
                <BrandSymbol concept={c} size={s} />
              </span>
            )),
          )}
          {CONCEPTS.flatMap((c) =>
            [16, 32].map((s) => (
              <span key={`${c.id}-${s}-mark`} data-check={`${c.id}-${s}-mark`}>
                <BrandSymbol concept={c} size={s} variant="mark" color={BRAND.ink} />
              </span>
            )),
          )}
        </div>
      </section>
    </main>
  );
}

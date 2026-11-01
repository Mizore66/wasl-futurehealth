import { readFile } from "node:fs/promises";
import { join } from "node:path";
import { ImageResponse } from "next/og";
import { BRAND, Logo } from "./mark";

export const OG_ALT = "Wasl. Every positive screening test, followed to a documented end.";
export const OG_SIZE = { width: 1200, height: 630 };

const fonts = Promise.all(
  (["Geist-Medium.ttf", "Geist-Bold.ttf"] as const).map((f) => readFile(join(process.cwd(), "assets/fonts", f))),
);

/** The share card for opengraph-image and twitter-image: paper ground, the logo, the promise, the honesty line. */
export async function renderOgImage() {
  const [medium, bold] = await fonts;
  return new ImageResponse(
    (
      <div
        style={{
          width: "100%",
          height: "100%",
          display: "flex",
          flexDirection: "column",
          justifyContent: "space-between",
          padding: "72px 80px",
          background: BRAND.paper,
          color: BRAND.ink,
          fontFamily: "Geist",
        }}
      >
        <Logo style={{ fontSize: 64 }} />
        <div style={{ fontSize: 68, fontWeight: 700, lineHeight: 1.04, letterSpacing: "-0.03em", maxWidth: 940 }}>
          Every positive screening test, followed to a documented end.
        </div>
        <div style={{ display: "flex", alignItems: "center", gap: 16, fontSize: 26, fontWeight: 500, color: "#4f5f63" }}>
          <div style={{ width: 12, height: 12, borderRadius: 6, background: BRAND.teal }} />
          Abu Dhabi colorectal screening · synthetic data demo
        </div>
      </div>
    ),
    {
      ...OG_SIZE,
      fonts: [
        { name: "Geist", data: medium, weight: 500, style: "normal" },
        { name: "Geist", data: bold, weight: 700, style: "normal" },
      ],
    },
  );
}

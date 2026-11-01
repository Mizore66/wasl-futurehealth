/**
 * Builds every brand raster and the static icon.svg from <Mark>, so no icon is hand-exported.
 * Run: pnpm exec tsx scripts/build-brand.tsx
 */
import { writeFile } from "node:fs/promises";
import { join } from "node:path";
import { chromium } from "@playwright/test";
import { renderToStaticMarkup } from "react-dom/server";
import { BRAND, Mark } from "../components/brand/mark";

const root = join(__dirname, "..");
const svg = (el: React.ReactElement) => renderToStaticMarkup(el).replace(/ (class|data-brand|aria-hidden)="[^"]*"/g, "");

/** The bare mark's stroked bounds span 29.2 of its 32 units. */
const BARE_FILL = 29.2 / 32;

function fullBleed(side: number, markShare: number) {
  const size = Math.round((side * markShare) / BARE_FILL);
  return `<div style="width:${side}px;height:${side}px;background:${BRAND.teal};display:flex;align-items:center;justify-content:center">${svg(
    <Mark size={size} variant="bare" color={BRAND.paper} />,
  )}</div>`;
}

/** An ICO is a 6-byte header, one 16-byte entry per image, then the PNGs as-is. */
function ico(pngs: { size: number; png: Buffer }[]) {
  const header = Buffer.alloc(6 + pngs.length * 16);
  header.writeUInt16LE(1, 2);
  header.writeUInt16LE(pngs.length, 4);
  let offset = header.length;
  pngs.forEach(({ size, png }, i) => {
    const at = 6 + i * 16;
    header.writeUInt8(size >= 256 ? 0 : size, at);
    header.writeUInt8(size >= 256 ? 0 : size, at + 1);
    header.writeUInt16LE(1, at + 4);
    header.writeUInt16LE(32, at + 6);
    header.writeUInt32LE(png.length, at + 8);
    header.writeUInt32LE(offset, at + 12);
    offset += png.length;
  });
  return Buffer.concat([header, ...pngs.map((p) => p.png)]);
}

async function main() {
  const browser = await chromium.launch();
  const page = await browser.newPage({ deviceScaleFactor: 1 });
  async function png(html: string) {
    await page.setContent(`<body style="margin:0;background:transparent"><div id="a" style="display:inline-block;line-height:0">${html}</div></body>`);
    return page.locator("#a").screenshot({ omitBackground: true });
  }

  const favicon = [];
  for (const size of [16, 32, 48]) favicon.push({ size, png: await png(svg(<Mark size={size} />)) });
  await writeFile(join(root, "app/favicon.ico"), ico(favicon));

  const darkTab = `<style>@media (prefers-color-scheme:dark){rect{fill:${BRAND.mint}}path{stroke:${BRAND.ink}}}</style>`;
  await writeFile(join(root, "app/icon.svg"), svg(<Mark size={16} />).replace(/(<svg[^>]*>)/, `$1${darkTab}`) + "\n");

  await writeFile(join(root, "app/apple-icon.png"), await png(fullBleed(180, 0.6)));
  await writeFile(join(root, "public/icon-192.png"), await png(svg(<Mark size={192} />)));
  await writeFile(join(root, "public/icon-512.png"), await png(svg(<Mark size={512} />)));
  await writeFile(join(root, "public/icon-maskable-512.png"), await png(fullBleed(512, 0.56)));

  await browser.close();
  console.log("brand assets written");
}

void main();

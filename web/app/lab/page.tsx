import type { Metadata } from "next";
import { Avatar, AvatarPair } from "@/components/avatar/avatar";
import { SYSTEMS, type SystemId } from "@/components/avatar/traits";
import { CAST, FATIMA, FATMA } from "./cast";
import { PipelineMock } from "./pipeline-mock";
import { ResolveLoop } from "./resolve-loop";

export const metadata: Metadata = { title: "Lab", robots: { index: false } };

/** One token set for the landing and the app: the landing's warm paper and teal, with the app's density. */
const TOKENS = [
  ["--paper", "#f6f3ee", "page ground, landing and app"],
  ["--surface", "#fffdf8", "cards, table rows"],
  ["--sand", "#efe8dc", "raised fields, hover"],
  ["--sand-deep", "#e6dccb", "active, selected"],
  ["--line", "#e2d9c9", "borders"],
  ["--ink", "#0b2427", "text"],
  ["--ink-soft", "#34494b", "secondary text"],
  ["--muted", "#56666a", "captions"],
  ["--teal", "#0f4f4c", "primary, links, the thread"],
  ["--mint", "#dbe8e6", "teal tint, landing bands"],
  ["--positive", "#2f6b3a", "closed with evidence"],
  ["--held", "#8a5a12", "waiting for people"],
  ["--negative", "#a8322a", "kept open, tamper"],
] as const;

const CSS = `
.lab {
  ${TOKENS.map(([k, v]) => `${k}: ${v};`).join("\n  ")}
  --positive-bg: #e3eedf;
  --held-bg: #f5e8cc;
  --negative-bg: #f6e0da;
  --radius: 10px;
  --avatar-join: var(--teal);
  --avatar-paper: var(--surface);
  --avatar-muted: var(--muted);
  background: var(--paper);
  color: var(--ink);
  min-height: 100dvh;
  padding: 32px 40px 80px;
  font-size: 14px;
  line-height: 20px;
}
.lab h1 { font-size: 28px; line-height: 36px; font-weight: 600; letter-spacing: -0.01em; }
.lab h2 { font-size: 18px; line-height: 26px; font-weight: 600; margin: 40px 0 4px; }
.lab h3 { font-size: 15px; font-weight: 600; }
.lab .note { color: var(--muted); max-width: 72ch; }
.lab .badge { display: inline-block; padding: 2px 8px; border-radius: 4px; background: var(--held-bg); color: var(--held); font-weight: 600; font-size: 12px; margin-bottom: 8px; }
.lab .systems { display: grid; grid-template-columns: repeat(3, 1fr); gap: 16px; margin-top: 16px; }
.lab .card { background: var(--surface); border: 1px solid var(--line); border-radius: var(--radius); padding: 20px; }
.lab .card + .card { margin: 0; }
.lab .seeds { display: grid; grid-template-columns: repeat(8, 40px); gap: 8px; margin: 12px 0; }
.lab .strip { display: flex; gap: 4px; flex-wrap: wrap; margin: 6px 0; align-items: center; }
.lab .row { display: flex; gap: 16px; align-items: center; margin: 10px 0; flex-wrap: wrap; }
.lab .label { color: var(--muted); font-size: 12px; min-width: 64px; }
.lab .pairs { display: grid; grid-template-columns: auto 1fr; gap: 10px 16px; align-items: center; margin-top: 8px; }
.lab .tokens { display: grid; grid-template-columns: repeat(auto-fill, minmax(190px, 1fr)); gap: 8px; margin: 12px 0 20px; }
.lab .swatch { display: flex; gap: 10px; align-items: center; font-size: 12px; }
.lab .swatch i { width: 28px; height: 28px; border-radius: 6px; border: 1px solid var(--line); flex: none; }
.lab .swatch b { display: block; font-family: ui-monospace, monospace; font-weight: 500; }
.lab .swatch span { color: var(--muted); }
.lab .demo { display: grid; grid-template-columns: 1.25fr 1fr; gap: 16px; align-items: start; }
.lab .app-head { display: flex; justify-content: space-between; align-items: baseline; margin-bottom: 12px; }
.lab .switch { display: inline-flex; gap: 4px; float: right; }
.lab .switch button { border: 1px solid var(--line); background: var(--surface); border-radius: 6px; padding: 2px 10px; font-weight: 600; color: var(--ink-soft); cursor: pointer; }
.lab .switch button[aria-pressed="true"] { background: var(--teal); color: var(--surface); border-color: var(--teal); }
.lab .flow { list-style: none; margin: 8px 0 0; padding: 0; clear: both; }
.lab .flow-row { display: grid; grid-template-columns: 190px 1fr 24px; gap: 12px; align-items: center; min-height: 52px; border-top: 1px solid var(--line); padding: 6px 0; }
.lab .flow-row[data-outcome="true"] { padding-left: 14px; border-left: 3px solid var(--line); }
.lab .flow-row[data-tone="positive"] { border-left-color: var(--positive); }
.lab .flow-row[data-tone="held"] { border-left-color: var(--held); }
.lab .flow-row[data-tone="negative"] { border-left-color: var(--negative); }
.lab .flow-name { display: block; font-weight: 500; }
.lab .flow-sub { display: block; color: var(--muted); font-size: 12px; line-height: 16px; }
.lab .flow-chips { display: flex; flex-wrap: wrap; gap: 6px; }
.lab .flow-count { text-align: right; font-variant-numeric: tabular-nums; color: var(--ink-soft); }
.lab .chip { display: inline-flex; align-items: center; gap: 6px; padding: 3px 10px 3px 3px; border-radius: 999px; background: var(--sand); font-size: 13px; white-space: nowrap; }
.lab .flow-row[data-tone="positive"] .chip { background: var(--positive-bg); }
.lab .flow-row[data-tone="held"] .chip { background: var(--held-bg); }
.lab .flow-row[data-tone="negative"] .chip { background: var(--negative-bg); }
.lab .hero { background: var(--paper); border: 1px solid var(--line); border-radius: var(--radius); padding: 36px 32px; }
.lab .hero h4 { font-size: 44px; line-height: 48px; font-weight: 600; letter-spacing: -0.02em; margin: 0 0 16px; }
.lab .hero p { font-size: 17px; line-height: 26px; color: var(--ink-soft); margin: 0 0 20px; }
.lab .hero .cta { display: inline-block; background: var(--ink); color: var(--surface); padding: 10px 16px; border-radius: 6px; font-weight: 600; }
.lab .records { display: grid; grid-template-columns: 1fr auto 1fr; gap: 10px; align-items: center; margin-top: 28px; background: var(--sand); border-radius: var(--radius); padding: 16px; }
.lab .record { background: var(--surface); border-radius: 8px; padding: 12px; display: flex; gap: 10px; align-items: center; }
.lab .record small { display: block; color: var(--muted); font-size: 11px; }
.lab .record strong { font-size: 18px; font-weight: 600; }
.lab .thread { width: 28px; height: 3px; background: var(--teal); border-radius: 2px; }
`;

const SAMPLE = CAST[0];

function SystemCard({ id }: { id: SystemId }) {
  return (
    <section className="card" aria-labelledby={`sys-${id}`}>
      <h3 id={`sys-${id}`}>{SYSTEMS[id].name}</h3>
      <div className="seeds">
        {CAST.map((p) => (
          <Avatar key={p.key} system={id} person={p} size={40} label={p.name} />
        ))}
      </div>
      <div className="row">
        <span className="label">24px</span>
        <span className="strip">
          {CAST.slice(0, 10).map((p) => (
            <Avatar key={p.key} system={id} person={p} size={24} label={p.name} />
          ))}
        </span>
      </div>
      <div className="row">
        <span className="label">16px</span>
        <span className="strip">
          {CAST.slice(0, 10).map((p) => (
            <Avatar key={p.key} system={id} person={p} size={16} label={p.name} />
          ))}
        </span>
      </div>
      <div className="row">
        <span className="label">Unknown</span>
        <Avatar system={id} size={24} />
        <Avatar system={id} size={64} />
        <span className="label">Resolving</span>
        <ResolveLoop system={id} person={SAMPLE} size={24} />
        <ResolveLoop system={id} person={SAMPLE} size={64} />
      </div>
      <div className="pairs">
        <AvatarPair system={id} a={FATIMA} b={FATIMA} size={24} />
        <span>Same person: Fatima Al Zaabi and F. Alzabi</span>
        <AvatarPair system={id} a={FATIMA} b={FATMA} size={24} />
        <span>Two people: Fatima vs Fatma Al Zaabi</span>
        <AvatarPair system={id} a={FATIMA} b={FATIMA} size={64} />
        <span>Same, 64px</span>
        <AvatarPair system={id} a={FATIMA} b={FATMA} size={64} />
        <span>Different, 64px</span>
      </div>
    </section>
  );
}

export default function LabPage() {
  return (
    <main className="lab">
      <style>{CSS}</style>
      <span className="badge">Lab · not part of the product</span>
      <h1>Patient avatars and one theme</h1>
      <p className="note">
        Three competing avatar systems, each a pure function of a person key. A record no link has reached shows a
        question mark. All people here are made up.
      </p>

      <div className="systems">
        {(Object.keys(SYSTEMS) as SystemId[]).map((id) => (
          <SystemCard key={id} id={id} />
        ))}
      </div>

      <h2>One theme: sand and thread</h2>
      <p className="note">
        The landing&apos;s warm paper and teal become the app&apos;s ground and primary. Status keeps three muted
        hues. The app keeps its density: 14px body, 52px rows.
      </p>
      <div className="tokens">
        {TOKENS.map(([k, v, use]) => (
          <div className="swatch" key={k}>
            <i style={{ background: v }} />
            <div>
              <b>
                {k} {v}
              </b>
              <span>{use}</span>
            </div>
          </div>
        ))}
      </div>

      <div className="demo">
        <section className="card" aria-label="Pipeline mock">
          <div className="app-head">
            <h3>Case flow · replay mock</h3>
          </div>
          <PipelineMock initial="portrait" />
        </section>
        <section className="hero" aria-label="Landing hero mock">
          <h4>Nobody lost between hospitals</h4>
          <p>
            When a screening test asks for a closer look, Wasl makes sure that person gets the follow-up check, and
            marks it done only with proof.
          </p>
          <span className="cta">Request a pilot</span>
          <div className="records">
            <div className="record">
              <Avatar system="portrait" person={FATIMA} size={40} label="Fatima Al Zaabi" />
              <div>
                <small>Her clinic</small>
                <strong>فاطمة الزعابي</strong>
              </div>
            </div>
            <span className="thread" aria-hidden />
            <div className="record">
              <ResolveLoop system="portrait" person={FATIMA} size={40} />
              <div>
                <small>Hospital across the city</small>
                <strong>F. Alzabi</strong>
              </div>
            </div>
          </div>
        </section>
      </div>
    </main>
  );
}

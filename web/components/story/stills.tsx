import { Avatar } from "@/components/avatar/avatar";
import type { PersonSeed } from "@/components/avatar/traits";
import { CAST, SUPPORT, type BeatId, type StoryFigures } from "./beats";
import { COLOR } from "./palette";

/**
 * One drawn key frame per beat, for visitors without the 3D room. Same objects, same palette: pebbles
 * carry the app's avatar, the teal thread is the only saturated thing.
 */

const FATIMA = CAST.fatima.seed;

/** A pebble seen face-on: a stone rim and the app's avatar, or the "?" when no link has reached it. */
function Pebble({ x, y, r, person }: { x: number; y: number; r: number; person?: PersonSeed }) {
  return (
    <g>
      <ellipse cx={x} cy={y + r + 4} rx={r * 1.1} ry={r * 0.18} fill="#2a1e14" opacity="0.12" />
      <circle cx={x} cy={y} r={r} fill={COLOR.surface} stroke={COLOR.stone} strokeWidth={r * 0.12} />
      <foreignObject x={x - r * 0.8} y={y - r * 0.8} width={r * 1.6} height={r * 1.6}>
        <Avatar person={person} size={Math.round(r * 1.6)} label="" />
      </foreignObject>
    </g>
  );
}

function Frame({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <svg viewBox="0 0 480 340" role="img" aria-label={label} className="block h-auto w-full max-w-[640px] justify-self-center">
      <rect width="480" height="340" fill={COLOR.ground} />
      <rect y="250" width="480" height="90" fill={COLOR.raised} />
      {children}
    </svg>
  );
}

const plinth = (x: number) => <rect x={x - 46} y={236} width={92} height={18} rx={6} fill={COLOR.stone} />;
const thread = (d: string, color: string = COLOR.teal) => <path d={d} fill="none" stroke={color} strokeWidth="5" strokeLinecap="round" />;

export function Still({ id, figures }: { id: BeatId; figures: StoryFigures }) {
  switch (id) {
    case "lost": {
      const pairs = Array.from({ length: 12 }, (_, i) => ({ x: 50 + (i % 6) * 76, y: 120 + Math.floor(i / 6) * 70, joined: i % 3 === 0 }));
      return (
        <Frame label="Many pairs of pebbles, each pair one person's two records. Only some pairs are joined by a teal thread.">
          {pairs.map((p, i) => (
            <g key={i} opacity={i === 7 ? 1 : 0.55}>
              {p.joined && thread(`M${p.x} ${p.y}H${p.x + 30}`)}
              <circle cx={p.x} cy={p.y} r="13" fill={COLOR.surface} stroke={COLOR.stone} strokeWidth="2" />
              <circle cx={p.x + 30} cy={p.y} r="13" fill={p.joined ? COLOR.surface : COLOR.unknown} stroke={COLOR.stone} strokeWidth="2" />
            </g>
          ))}
          <Pebble x={190} y={270} r={30} person={FATIMA} />
          <Pebble x={290} y={270} r={30} />
        </Frame>
      );
    }
    case "unknown":
      return (
        <Frame label="Fatima's clinic on the left, the hospital under an arch on the right. At the hospital her record shows a question mark.">
          <rect x={30} y={150} width={140} height={86} rx={6} fill={COLOR.raised} stroke={COLOR.stone} />
          {plinth(100)}
          <Pebble x={100} y={196} r={34} person={FATIMA} />
          <circle cx={100} cy={196} r={50} fill="none" stroke={COLOR.teal} strokeOpacity="0.35" strokeWidth="2" strokeDasharray="4 6" />
          <path d="M318 236V150A62 62 0 0 1 442 150V236" fill="none" stroke={COLOR.raised} strokeWidth="18" />
          {plinth(380)}
          <Pebble x={380} y={196} r={34} />
        </Frame>
      );
    case "wait": {
      const x = (d: number) => 40 + (d / figures.waitDays) * 400;
      return (
        <Frame label={`A long beam marked at day ${figures.standardDays}. The pebble rolls past the mark and stops at day ${figures.waitDays}.`}>
          <rect x={40} y={210} width={400} height={26} rx={8} fill={COLOR.stone} />
          <rect x={x(figures.standardDays) - 2} y={196} width={4} height={40} fill={COLOR.ink} />
          <Pebble x={x(figures.waitDays) - 22} y={180} r={24} person={FATIMA} />
        </Frame>
      );
    }
    case "join":
      return (
        <Frame label="A teal thread runs from Fatima's clinic record through two rings, name and birth date, to her hospital record, which now shows her face.">
          {plinth(70)}
          {plinth(410)}
          {thread("M104 200C150 180 170 220 200 200S260 180 280 200S340 220 376 200")}
          <ellipse cx={200} cy={200} rx={14} ry={40} fill="none" stroke={COLOR.tealBright} strokeWidth="6" />
          <ellipse cx={280} cy={200} rx={14} ry={40} fill="none" stroke={COLOR.tealBright} strokeWidth="6" />
          <Pebble x={70} y={200} r={34} person={FATIMA} />
          <Pebble x={410} y={200} r={34} person={FATIMA} />
        </Frame>
      );
    case "proof":
      return (
        <Frame label={`A report slab between two readers, one faceted and one smooth; both light the same line. Behind, a wall of ${figures.closuresDone.toLocaleString("en-US")} teal dots, none red.`}>
          <g opacity="0.9">
            {Array.from({ length: 22 * 9 }, (_, i) => (
              <circle key={i} cx={30 + (i % 22) * 20} cy={24 + Math.floor(i / 22) * 13} r="3.4" fill={COLOR.teal} />
            ))}
          </g>
          <polygon points="120,200 140,166 180,166 200,200 180,234 140,234" fill="none" stroke={COLOR.stone} strokeWidth="10" />
          <circle cx={340} cy={200} r={38} fill="none" stroke={COLOR.stone} strokeWidth="10" />
          <rect x={222} y={146} width={36} height={102} rx={5} fill={COLOR.surface} stroke={COLOR.stone} strokeWidth="2" />
          <rect x={190} y={192} width={100} height={8} fill={COLOR.teal} />
        </Frame>
      );
    case "receipt":
      return (
        <Frame label="The thread tied into a knot, pressed flat into a receipt with the knot as its seal.">
          <rect x={170} y={40} width={140} height={196} rx={6} fill={COLOR.surface} stroke={COLOR.stone} strokeWidth="2" />
          {[70, 92, 114, 136, 158].map((y) => (
            <rect key={y} x={188} y={y} width={y === 70 ? 70 : 104} height={7} rx={3} fill={COLOR.stone} />
          ))}
          <circle cx={240} cy={206} r={17} fill={COLOR.teal} />
          {thread("M240 206C200 250 150 230 120 250", COLOR.teal)}
        </Frame>
      );
    case "who":
      return (
        <Frame label="Four lit figures in a ring around the receipt's seal: the person waiting, the nurse, the health system and the auditor, each tied to the seal by a thread.">
          <circle cx={240} cy={210} r={13} fill={COLOR.teal} />
          {[
            [240, 90],
            [110, 200],
            [370, 200],
            [240, 296],
          ].map(([x, y]) => (
            <path key={`${x}${y}`} d={`M240 210L${x} ${y}`} stroke={COLOR.teal} strokeWidth="3" />
          ))}
          <Pebble x={240} y={90} r={26} person={FATIMA} />
          <Pebble x={110} y={200} r={26} person={SUPPORT.nurse} />
          <g>
            {Array.from({ length: 9 }, (_, i) => (
              <circle key={i} cx={350 + (i % 3) * 18} cy={182 + Math.floor(i / 3) * 18} r={7} fill={COLOR.surface} stroke={COLOR.teal} strokeWidth="2" />
            ))}
          </g>
          <Pebble x={240} y={296} r={26} person={SUPPORT.auditor} />
          <circle cx={262} cy={276} r={12} fill="none" stroke={COLOR.ink} strokeWidth="3" />
        </Frame>
      );
    case "start": {
      const pairs = Array.from({ length: 12 }, (_, i) => ({ x: 50 + (i % 6) * 76, y: 110 + Math.floor(i / 6) * 70, held: i === 4 || i === 9 }));
      return (
        <Frame label="The room again: most pairs now joined by teal threads, a few held open in amber with two people beside them.">
          {pairs.map((p, i) => (
            <g key={i}>
              {thread(`M${p.x} ${p.y}H${p.x + 30}`, p.held ? COLOR.held : COLOR.teal)}
              <circle cx={p.x} cy={p.y} r="13" fill={COLOR.surface} stroke={COLOR.stone} strokeWidth="2" />
              <circle cx={p.x + 30} cy={p.y} r="13" fill={p.held ? COLOR.unknown : COLOR.surface} stroke={COLOR.stone} strokeWidth="2" />
              {p.held && <circle cx={p.x + 15} cy={p.y + 30} r="8" fill="none" stroke={COLOR.unknown} strokeWidth="2" />}
            </g>
          ))}
        </Frame>
      );
    }
  }
}

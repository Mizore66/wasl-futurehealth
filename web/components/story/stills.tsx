import { Avatar } from "@/components/avatar/avatar";
import type { PersonSeed } from "@/components/avatar/traits";
import { COLOR } from "./palette";
import { CAST, SUPPORT, type BeatId, type StoryFigures } from "./script";

/**
 * One drawn key frame per beat, for visitors without the 3D story. The same objects in the same
 * palette: stone buildings, pebbles carrying the app's avatar, and the teal thread as the only
 * saturated thing.
 */

const FATIMA = CAST.fatima.seed;

/** A pebble seen face-on: a stone rim and the app's avatar, or the "?" when no link has reached it. */
function Pebble({ x, y, r, person }: { x: number; y: number; r: number; person?: PersonSeed }) {
  return (
    <g>
      <ellipse cx={x} cy={y + r + 3} rx={r * 1.1} ry={r * 0.18} fill="#2a1e14" opacity="0.12" />
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
      <rect y="250" width="480" height="90" fill={COLOR.floor} />
      <path d="M0 290C120 280 360 300 480 288" stroke={COLOR.paving} strokeWidth="16" fill="none" />
      {children}
    </svg>
  );
}

/** The clinic: a low block with an arched door and a window. */
function Clinic({ x, nurse = false }: { x: number; nurse?: boolean }) {
  return (
    <g>
      <rect x={x} y={170} width={170} height={84} rx={4} fill={COLOR.building} stroke={COLOR.stone} />
      <path d={`M${x + 40} 254V214A16 16 0 0 1 ${x + 72} 214V254Z`} fill={COLOR.recess} />
      <rect x={x + 104} y={196} width={36} height={26} fill={COLOR.recess} />
      {nurse && <Pebble x={x + 122} y={212} r={11} person={SUPPORT.nurse} />}
    </g>
  );
}

/** The hospital: three stepped masses and a canopy over the door. */
function Hospital({ x }: { x: number }) {
  return (
    <g>
      <rect x={x + 40} y={92} width={70} height={162} rx={4} fill={COLOR.building} stroke={COLOR.stone} />
      <rect x={x + 20} y={130} width={120} height={124} rx={4} fill={COLOR.buildingShade} stroke={COLOR.stone} />
      <rect x={x} y={176} width={170} height={78} rx={4} fill={COLOR.building} stroke={COLOR.stone} />
      <rect x={x + 30} y={204} width={60} height={6} fill={COLOR.building} stroke={COLOR.stone} />
      <rect x={x + 60} y={212} width={22} height={42} fill={COLOR.recess} />
    </g>
  );
}

const thread = (d: string, color: string = COLOR.teal) => <path d={d} fill="none" stroke={color} strokeWidth="5" strokeLinecap="round" />;

const ring = (cx: number, cy: number, c: string) => <ellipse cx={cx} cy={cy} rx={10} ry={26} fill="none" stroke={c} strokeWidth="5" />;

export function Still({ id, figures }: { id: BeatId; figures: StoryFigures }) {
  switch (id) {
    case "clinic":
      return (
        <Frame label="Fatima stands at the arched door of her clinic. The nurse is behind the window.">
          <Clinic x={150} nurse />
          <Pebble x={206} y={262} r={26} person={FATIMA} />
        </Frame>
      );
    case "city": {
      const x31 = 40 + (figures.standardDays / figures.waitDays) * 400;
      return (
        <Frame label={`Fatima walks the long road across the city, past a stone marking day ${figures.standardDays}, toward the hospital ${figures.waitDays} days away.`}>
          {[30, 70, 120, 160, 300, 350].map((x, i) => (
            <rect key={x} x={x} y={200 - (i % 3) * 18} width={34} height={60 + (i % 3) * 18} rx={3} fill={i % 2 ? COLOR.block2 : COLOR.block1} stroke={COLOR.stone} />
          ))}
          <rect x={x31 - 4} y={262} width={8} height={20} rx={2} fill={COLOR.stone} />
          <Pebble x={x31 - 60} y={262} r={18} person={FATIMA} />
          <rect x={400} y={160} width={70} height={100} rx={3} fill={COLOR.building} stroke={COLOR.stone} opacity="0.6" />
        </Frame>
      );
    }
    case "door":
      return (
        <Frame label="At the hospital door her record is written F. Alzabi with no ID number, and her face shows only a question mark.">
          <Hospital x={160} />
          <rect x={218} y={150} width={30} height={40} rx={2} fill={COLOR.surface} stroke={COLOR.stone} />
          <Pebble x={236} y={262} r={24} />
        </Frame>
      );
    case "gap":
      return (
        <Frame label="From above: the clinic on the left, the hospital on the right, open sand between. The nurse's calls fade before they cross.">
          <Clinic x={10} nurse />
          <Hospital x={300} />
          {[30, 60, 90].map((r) => (
            <path key={r} d={`M${132 + r} 200A${r} ${r} 0 0 1 ${132 + r} 250`} fill="none" stroke={COLOR.teal} strokeOpacity={0.5 - r / 220} strokeWidth="2" />
          ))}
          <Pebble x={384} y={274} r={14} />
        </Frame>
      );
    case "thread":
      return (
        <Frame label="The teal thread leaves the clinic window and runs along the road Fatima walked.">
          <Clinic x={10} nurse />
          {thread("M132 210C170 250 210 280 300 284S400 286 440 284")}
          <circle cx={440} cy={284} r={7} fill={COLOR.teal} />
        </Frame>
      );
    case "join":
      return (
        <Frame label="At the hospital door the thread passes a name ring and a birth-date ring, and her face comes back.">
          <Hospital x={280} />
          {thread("M20 270C120 270 200 268 336 266")}
          {ring(250, 262, COLOR.tealBright)}
          {ring(300, 264, COLOR.tealBright)}
          <Pebble x={350} y={262} r={24} person={FATIMA} />
        </Frame>
      );
    case "proof":
      return (
        <Frame label="The hospital's report between two readers, one faceted and one smooth. Both light the same line.">
          <rect x={200} y={110} width={80} height={120} rx={4} fill={COLOR.surface} stroke={COLOR.stone} strokeWidth="2" />
          {[130, 150, 170, 190, 210].map((y) => (
            <rect key={y} x={214} y={y} width={52} height={6} rx={3} fill={y === 170 ? COLOR.teal : COLOR.stone} />
          ))}
          <polygon points="90,170 108,140 144,140 162,170 144,200 108,200" fill="none" stroke={COLOR.tealBright} strokeWidth="9" />
          <circle cx={364} cy={170} r={32} fill="none" stroke={COLOR.tealBright} strokeWidth="9" />
          <path d="M162 172L214 173M332 172L266 173" stroke={COLOR.teal} strokeWidth="3" />
        </Frame>
      );
    case "home":
      return (
        <Frame label="The receipt rides the thread back to the clinic window, and the nurse stops calling.">
          <Clinic x={20} nurse />
          {thread("M142 212C200 250 300 284 470 286")}
          <rect x={250} y={196} width={36} height={48} rx={3} fill={COLOR.surface} stroke={COLOR.stone} />
          <circle cx={268} cy={232} r={6} fill={COLOR.teal} />
        </Frame>
      );
    case "end":
      return (
        <Frame label="The city from above: most people now have a teal thread from clinic to hospital; one is held amber with two people beside it.">
          <Clinic x={10} />
          <Hospital x={300} />
          {[0, 1, 2, 3, 4].map((i) => (
            <g key={i}>{thread(`M${60 + i * 14} 170Q240 ${30 + i * 14} ${330 + i * 22} 236`, i === 3 ? COLOR.held : COLOR.teal)}</g>
          ))}
        </Frame>
      );
  }
}

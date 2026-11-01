import type { PersonSeed, Top } from "@/components/avatar/traits";
import { lookOf } from "./look";
import { CITY, COLOR } from "./palette";
import { CAST, SUPPORT, type BeatId, type StoryFigures } from "./script";

/**
 * One drawn key frame per beat, for visitors without the 3D story. The same world as the scene: a
 * straight-cylinder figure with a Lego head and its person's colours, the muted city, the white and
 * teal hospital, and the bright thread.
 */

const FATIMA = CAST.fatima.seed;

/** Hair or a head covering over a head of half-width 1 standing 1.9 tall, top at y = 0, in head units. */
const TOP_PIECE: Record<Top, string | null> = {
  bald: null,
  short: "M-1.1 .55V-.05Q-1.1-.3-.7-.3H.7Q1.1-.3 1.1-.05V.55H.75V.25H-.75V.55Z",
  crop: "M-1.05 .3V-.02Q-1.05-.18-.7-.18H.7Q1.05-.18 1.05-.02V.3H.8V.12H-.8V.3Z",
  curly: "M-1.25.45A.5.5 0 0 1-1 -.2A.55.55 0 0 1-.3-.5A.55.55 0 0 1 .4-.5A.55.55 0 0 1 1-.2A.5.5 0 0 1 1.25.45L.8.15H-.8Z",
  wavy: "M-1.25 1.9V0Q-1.25-.3-.8-.3H.8Q1.25-.3 1.25 0V1.9H.8V.25H-.8V1.9Z",
  long: "M-1.3 3.5V0Q-1.3-.3-.8-.3H.8Q1.3-.3 1.3 0V3.5H.85V.25H-.85V3.5Z",
  bun: "M-1.1.55V-.05Q-1.1-.3-.7-.3H-.45A.55.55 0 1 1 .45-.3H.7Q1.1-.3 1.1-.05V.55H.75V.25H-.75V.55Z",
  hijab: "M-1.25 2.5V.1Q-1.25-.35-.8-.35H.8Q1.25-.35 1.25.1V2.5ZM0 .22A.76.82 0 1 0 .01.22Z",
  shayla: "M-1.45 3.5L-1.25 2V.1Q-1.25-.35-.8-.35H.8Q1.25-.35 1.25.1V2L1.45 3.5ZM0 .22A.76.82 0 1 0 .01.22Z",
  ghutra: "M-1.5 3L-1.25 1.6V.1Q-1.25-.35-.8-.35H.8Q1.25-.35 1.25.1V1.6L1.5 3ZM0 .22A.76.82 0 1 0 .01.22Z",
  kufi: "M-1.08.16V-.3Q-1.08-.5-.8-.5H.8Q1.08-.5 1.08-.3V.16Z",
};

/** A figure standing with its base at (x, y): a straight cylinder and a Lego head. No seed draws the "?". */
function Figure({ x, y, r, person }: { x: number; y: number; r: number; person?: PersonSeed }) {
  const look = person ? lookOf(person) : null;
  const cloth = look?.cloth ?? COLOR.unknown;
  const skin = look?.skin ?? COLOR.unknown;
  const bodyH = r * 2.65;
  const headH = r * 1.65;
  const headR = r * 0.88;
  const top = y - bodyH - headH;
  const piece = look && TOP_PIECE[look.top];
  const ink = look && look.skin < "#6" ? "#f3e6d8" : "#1b2224";
  return (
    <g>
      <ellipse cx={x} cy={y} rx={r * 1.75} ry={r * 0.42} fill="#261a10" opacity="0.32" />
      <rect x={x - r} y={y - bodyH} width={r * 2} height={bodyH} rx={r * 0.12} fill={cloth} />
      <ellipse cx={x - r * 0.35} cy={y - bodyH * 0.55} rx={r * 0.28} ry={bodyH * 0.38} fill="#fff" opacity="0.14" />
      <rect x={x - headR} y={top} width={headR * 2} height={headH} rx={headR * 0.26} fill={skin} />
      {look ? (
        <g fill={ink}>
          <circle cx={x - headR * 0.38} cy={top + headH * 0.42} r={r * 0.11} />
          <circle cx={x + headR * 0.38} cy={top + headH * 0.42} r={r * 0.11} />
          <rect x={x - headR * 0.28} y={top + headH * 0.68} width={headR * 0.56} height={r * 0.09} rx={r * 0.045} />
        </g>
      ) : (
        <text x={x} y={top + headH * 0.76} textAnchor="middle" fontSize={headH * 0.8} fontWeight="800" fill={COLOR.unknownInk}>
          ?
        </text>
      )}
      {piece && <path d={piece} transform={`translate(${x} ${top}) scale(${headR})`} fill={look.topColor} fillRule="evenodd" />}
    </g>
  );
}

function Frame({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <svg viewBox="0 0 480 340" role="img" aria-label={label} className="block h-auto w-full max-w-[640px] justify-self-center">
      <rect width="480" height="340" fill={COLOR.ground} />
      <rect y="250" width="480" height="90" fill={COLOR.floor} />
      <path d="M0 290C120 280 360 300 480 288" stroke={COLOR.kerb} strokeWidth="22" fill="none" />
      <path d="M0 290C120 280 360 300 480 288" stroke={COLOR.road} strokeWidth="17" fill="none" />
      {children}
    </svg>
  );
}

function Palm({ x, y, h }: { x: number; y: number; h: number }) {
  return (
    <g>
      <path d={`M${x} ${y}L${x + 2} ${y - h}`} stroke={COLOR.trunk} strokeWidth="3" />
      <path
        d={`M${x + 2} ${y - h}q-18 -8 -22 8q10 -8 22 -8q-14 -14 -8 -20q6 8 8 20q2 -14 12 -18q2 10 -12 18q18 -6 22 8q-12 -8 -22 -8`}
        fill={COLOR.leaf}
      />
    </g>
  );
}

/** The clinic: a low warm block with a striped awning over the arched door, a lattice and a palm. */
function Clinic({ x, nurse = false }: { x: number; nurse?: boolean }) {
  return (
    <g>
      <Palm x={x - 8} y={256} h={68} />
      <rect x={x} y={170} width={170} height={84} rx={3} fill={COLOR.clinic} />
      <rect x={x} y={246} width={170} height={8} fill={COLOR.awning} />
      <path d={`M${x + 40} 254V214A16 16 0 0 1 ${x + 72} 214V254Z`} fill={COLOR.recess} />
      {[0, 1, 2, 3, 4].map((i) => (
        <rect key={i} x={x + 32 + i * 9.6} y={198} width={9.6} height={12} fill={i % 2 ? COLOR.surface : COLOR.awning} />
      ))}
      <rect x={x + 8} y={190} width={22} height={30} fill={COLOR.recess} />
      <path
        d={`M${x + 13} 190V220M${x + 19} 190V220M${x + 25} 190V220M${x + 8} 197H${x + 30}M${x + 8} 205H${x + 30}M${x + 8} 213H${x + 30}`}
        stroke={COLOR.lattice}
        strokeWidth="1.6"
      />
      <rect x={x + 104} y={196} width={36} height={26} fill={COLOR.recess} />
      {nurse && <Figure x={x + 122} y={230} r={7} person={SUPPORT.nurse} />}
      {nurse && <rect x={x + 104} y={222} width={36} height={12} fill={COLOR.clinic} />}
    </g>
  );
}

/** The hospital: a white podium with a teal band, a glass tower behind fins, a helipad, a raised wing and a columned portico. */
function Hospital({ x }: { x: number }) {
  return (
    <g>
      <rect x={x + 42} y={62} width={84} height={120} rx={2} fill={COLOR.hospital} />
      <rect x={x + 50} y={82} width={68} height={100} fill={COLOR.glass} />
      {[0, 1, 2, 3, 4, 5, 6, 7, 8].map((i) => (
        <rect key={i} x={x + 50 + i * 8.25} y={82} width={1.8} height={100} fill={COLOR.hospital} />
      ))}
      <rect x={x + 41} y={68} width={86} height={6} fill={COLOR.hospitalTeal} />
      <ellipse cx={x + 84} cy={60} rx={30} ry={5} fill={COLOR.hospital} />
      <ellipse cx={x + 84} cy={59} rx={20} ry={3} fill="none" stroke={COLOR.hospitalTeal} strokeWidth="1.6" />
      <rect x={x + 148} y={148} width={38} height={46} rx={2} fill={COLOR.hospital} />
      <rect x={x + 151} y={158} width={35} height={22} fill={COLOR.hospitalTeal} />
      {[152, 180].map((c) => (
        <rect key={c} x={x + c} y={194} width={3} height={60} fill={COLOR.hospitalShade} />
      ))}
      <rect x={x} y={176} width={150} height={78} rx={2} fill={COLOR.hospital} />
      <rect x={x} y={182} width={150} height={5} fill={COLOR.hospitalTeal} />
      <rect x={x + 6} y={212} width={38} height={36} fill={COLOR.glass} />
      <rect x={x + 112} y={212} width={32} height={36} fill={COLOR.glass} />
      <rect x={x + 76} y={214} width={22} height={40} fill={COLOR.glassDeep} />
      <rect x={x + 42} y={200} width={68} height={7} rx={1.5} fill={COLOR.hospital} stroke={COLOR.hospitalShade} />
      <rect x={x + 42} y={205} width={68} height={2} fill={COLOR.hospitalTeal} />
      {[47, 104].map((c) => (
        <rect key={c} x={x + c} y={207} width={3.4} height={49} fill={COLOR.hospital} stroke={COLOR.hospitalShade} strokeWidth=".6" />
      ))}
    </g>
  );
}

const thread = (d: string, color: string = COLOR.thread) => <path d={d} fill="none" stroke={color} strokeWidth="5" strokeLinecap="round" />;

const ring = (cx: number, cy: number, c: string) => <ellipse cx={cx} cy={cy} rx={10} ry={26} fill="none" stroke={c} strokeWidth="5" />;

export function Still({ id, figures }: { id: BeatId; figures: StoryFigures }) {
  switch (id) {
    case "clinic":
      return (
        <Frame label="Fatima stands at the arched door of her clinic. The nurse is behind the window.">
          <Clinic x={150} nurse />
          <Figure x={206} y={296} r={15} person={FATIMA} />
        </Frame>
      );
    case "city": {
      const x31 = 40 + (figures.standardDays / figures.waitDays) * 400;
      return (
        <Frame label={`Fatima walks the long road across the city, past a stone marking day ${figures.standardDays}, toward the hospital ${figures.waitDays} days away.`}>
          {[30, 70, 120, 160, 300, 350].map((x, i) => (
            <rect key={x} x={x} y={200 - (i % 3) * 18} width={34} height={60 + (i % 3) * 18} rx={3} fill={CITY[i]} />
          ))}
          <rect x={x31 - 4} y={262} width={8} height={20} rx={2} fill={COLOR.stone} />
          <Figure x={x31 - 60} y={290} r={11} person={FATIMA} />
          <rect x={400} y={140} width={70} height={120} rx={3} fill={COLOR.hospital} opacity="0.75" />
          <rect x={400} y={148} width={70} height={5} fill={COLOR.hospitalTeal} opacity="0.75" />
        </Frame>
      );
    }
    case "door":
      return (
        <Frame label="At the hospital door her record is written F. Alzabi with no ID number, and she shows only a grey figure with a question mark.">
          <Hospital x={160} />
          <rect x={218} y={150} width={30} height={40} rx={2} fill={COLOR.surface} stroke={COLOR.stone} />
          <Figure x={236} y={296} r={14} />
        </Frame>
      );
    case "gap":
      return (
        <Frame label="From above: the clinic on the left, the hospital on the right, open sand between. The nurse's calls fade before they cross.">
          <Clinic x={10} nurse />
          <Hospital x={300} />
          {[30, 60, 90].map((r) => (
            <path key={r} d={`M${132 + r} 200A${r} ${r} 0 0 1 ${132 + r} 250`} fill="none" stroke={COLOR.tealBright} strokeOpacity={0.6 - r / 220} strokeWidth="2" />
          ))}
          <Figure x={384} y={294} r={8} />
        </Frame>
      );
    case "thread":
      return (
        <Frame label="The teal thread leaves the clinic window and runs along the road Fatima walked.">
          <Clinic x={10} nurse />
          {thread("M132 210C170 250 210 280 300 284S400 286 440 284")}
          <circle cx={440} cy={284} r={7} fill={COLOR.thread} />
        </Frame>
      );
    case "join":
      return (
        <Frame label="At the hospital door the thread passes a name ring and a birth-date ring, and she is herself again.">
          <Hospital x={280} />
          {thread("M20 270C120 270 200 268 336 266")}
          {ring(250, 262, COLOR.tealBright)}
          {ring(300, 264, COLOR.tealBright)}
          <Figure x={350} y={296} r={14} person={FATIMA} />
        </Frame>
      );
    case "proof":
      return (
        <Frame label="The hospital's report between two readers, one faceted and one smooth. Both light the same line.">
          <rect x={200} y={110} width={80} height={120} rx={4} fill={COLOR.surface} stroke={COLOR.stone} strokeWidth="2" />
          {[130, 150, 170, 190, 210].map((y) => (
            <rect key={y} x={214} y={y} width={52} height={6} rx={3} fill={y === 170 ? COLOR.thread : COLOR.stone} />
          ))}
          <polygon points="90,170 108,140 144,140 162,170 144,200 108,200" fill="none" stroke={COLOR.tealBright} strokeWidth="9" />
          <circle cx={364} cy={170} r={32} fill="none" stroke={COLOR.tealBright} strokeWidth="9" />
          <path d="M162 172L214 173M332 172L266 173" stroke={COLOR.thread} strokeWidth="3" />
        </Frame>
      );
    case "home":
      return (
        <Frame label="The receipt rides the thread back to the clinic window, and the nurse stops calling.">
          <Clinic x={20} nurse />
          {thread("M142 212C200 250 300 284 470 286")}
          <rect x={250} y={196} width={36} height={48} rx={3} fill={COLOR.surface} stroke={COLOR.stone} />
          <circle cx={268} cy={232} r={6} fill={COLOR.thread} />
        </Frame>
      );
    case "end":
      return (
        <Frame label="The city from above: most people now have a teal thread from clinic to hospital; one is held amber with two people beside it.">
          <Clinic x={10} />
          <Hospital x={300} />
          {[0, 1, 2, 3, 4].map((i) => (
            <g key={i}>{thread(`M${60 + i * 14} 170Q240 ${30 + i * 14} ${330 + i * 22} 236`, i === 3 ? COLOR.held : COLOR.thread)}</g>
          ))}
        </Frame>
      );
  }
}

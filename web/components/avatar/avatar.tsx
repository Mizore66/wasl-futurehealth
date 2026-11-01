import type { ReactNode } from "react";
import "./avatar.css";
import {
  BG,
  CLOTH,
  COVER,
  GHUTRA,
  HAIR,
  MOTIFS,
  SKIN,
  portrait,
  sadu,
  token,
  type PersonSeed,
  type PortraitFeatures,
  type SaduFeatures,
  type SystemId,
  type TokenFeatures,
  type Top,
} from "./traits";

export type AvatarState = "known" | "unknown" | "resolving";

const INK = "#1b2224";
const VOID = "#e9e5de";
const VOID_INK = "#8b8f8c";

function topFill(top: Top, color: number): string {
  if (top === "ghutra") return GHUTRA[color];
  if (top === "shayla" || top === "hijab" || top === "kufi") return COVER[color];
  return HAIR[color];
}

/** Hair or headwear drawn behind the head. */
function TopBehind({ top, fill }: { top: Top; fill: string }) {
  switch (top) {
    case "short":
      return <circle cx="32" cy="27" r="13.6" fill={fill} />;
    case "wavy":
      return <path d="M18 31C17 15 47 15 46 31L47 42C42 46 22 46 17 42Z" fill={fill} />;
    case "long":
      return <path d="M18 30C18 13 46 13 46 30L48 54L16 54Z" fill={fill} />;
    case "bun":
      return (
        <>
          <circle cx="32" cy="12.5" r="6.5" fill={fill} />
          <circle cx="32" cy="27" r="13.4" fill={fill} />
        </>
      );
    default:
      return null;
  }
}

/** Hair or headwear drawn over the head. Coverings leave an oval opening for the face (even-odd). */
function TopFront({ top, fill }: { top: Top; fill: string }) {
  switch (top) {
    case "short":
    case "wavy":
    case "long":
    case "bun":
      return <path d="M19.6 29C19 17 45 17 44.4 29C40 22.5 25 21.5 19.6 29Z" fill={fill} />;
    case "crop":
      return <path d="M20.8 25.5C22.5 16 41.5 16 43.2 25.5C38 21.2 26 21.2 20.8 25.5Z" fill={fill} />;
    case "curly":
      return (
        <g fill={fill}>
          <circle cx="22" cy="24" r="5.6" />
          <circle cx="27" cy="18.5" r="6" />
          <circle cx="33" cy="17" r="6" />
          <circle cx="39" cy="19" r="6" />
          <circle cx="42.5" cy="24.5" r="5.2" />
        </g>
      );
    case "shayla":
      return (
        <>
          <path
            fillRule="evenodd"
            d="M12 66C11 44 15 14 32 14C49 14 53 44 52 66ZM32 20.5A9.6 11.4 0 1 0 32.01 20.5Z"
            fill={fill}
          />
          <path d="M22 44C28 50 40 50 47 41" stroke={INK} strokeOpacity="0.18" strokeWidth="1.4" fill="none" />
        </>
      );
    case "hijab":
      return (
        <path
          fillRule="evenodd"
          d="M15 66C15 50 16 15 32 15C48 15 49 50 49 66ZM32 20.8A9.3 11 0 1 0 32.01 20.8Z"
          fill={fill}
        />
      );
    case "ghutra":
      return (
        <>
          <path
            fillRule="evenodd"
            d="M14 60L19 29C19 14 45 14 45 29L50 60ZM32 21.5A9.6 11 0 1 0 32.01 21.5Z"
            fill={fill}
            stroke={INK}
            strokeOpacity="0.12"
          />
          <ellipse cx="32" cy="19.5" rx="12.6" ry="3.2" fill="none" stroke={INK} strokeWidth="2.4" />
        </>
      );
    case "kufi":
      return <path d="M20.5 24.5C20.5 12.5 43.5 12.5 43.5 24.5Z" fill={fill} stroke={INK} strokeOpacity="0.15" />;
    default:
      return null;
  }
}

const FACE_RY = [12.6, 13.6, 14.6];
const FACE_RX = [12.4, 11.4, 10.6];

function PortraitSvg({ f }: { f: PortraitFeatures }) {
  const skin = SKIN[f.skin];
  const fill = topFill(f.top, f.topColor);
  const hairInk = HAIR[f.topColor % HAIR.length];
  const covered = f.top === "shayla" || f.top === "hijab" || f.top === "ghutra";
  const eyeInk = f.skin >= 4 ? "#120c09" : INK;
  return (
    <svg viewBox="0 0 64 64" aria-hidden>
      <rect width="64" height="64" fill={BG[f.bg]} />
      <TopBehind top={f.top} fill={fill} />
      <ellipse cx="32" cy="66" rx="23" ry="17" fill={CLOTH[f.cloth]} />
      <rect x="27.5" y="38" width="9" height="12" rx="3" fill={skin} />
      <ellipse cx="32" cy="31" rx={FACE_RX[f.face]} ry={FACE_RY[f.face]} fill={skin} />
      <TopFront top={f.top} fill={fill} />
      {f.accent === "earrings" && !covered && (
        <g fill="#c9a54a">
          <circle cx={32 - FACE_RX[f.face] - 0.4} cy="36" r="1.6" />
          <circle cx={32 + FACE_RX[f.face] + 0.4} cy="36" r="1.6" />
        </g>
      )}
      {f.accent === "blush" && (
        <g fill="#d46a5a" opacity="0.28">
          <circle cx="25.5" cy="36" r="2.4" />
          <circle cx="38.5" cy="36" r="2.4" />
        </g>
      )}
      {f.accent === "beard" && (
        <path d="M21.5 34C22 44 27 46.5 32 46.5C37 46.5 42 44 42.5 34C39 40 25 40 21.5 34Z" fill={hairInk} />
      )}
      {f.accent === "moustache" && <path d="M27.5 37.4C30 35.6 34 35.6 36.5 37.4C34 37 30 37 27.5 37.4Z" fill={hairInk} stroke={hairInk} strokeWidth="1.4" />}
      <Eyes kind={f.eyes} ink={eyeInk} />
      <Mouth kind={f.mouth} ink={eyeInk} />
    </svg>
  );
}

function Eyes({ kind, ink }: { kind: number; ink: string }) {
  switch (kind) {
    case 0:
      return (
        <g fill={ink}>
          <circle cx="27.4" cy="31" r="1.7" />
          <circle cx="36.6" cy="31" r="1.7" />
        </g>
      );
    case 1:
      return <path d="M25 31.6Q27.4 29 29.8 31.6M34.2 31.6Q36.6 29 39 31.6" stroke={ink} strokeWidth="1.5" fill="none" strokeLinecap="round" />;
    case 2:
      return <path d="M25.4 31H29.4M34.6 31H38.6" stroke={ink} strokeWidth="1.6" strokeLinecap="round" />;
    default:
      return (
        <g stroke={ink} strokeWidth="1.2" fill="none">
          <circle cx="27.4" cy="31" r="3.3" />
          <circle cx="36.6" cy="31" r="3.3" />
          <path d="M30.7 31H33.3" />
          <circle cx="27.4" cy="31" r="0.9" fill={ink} />
          <circle cx="36.6" cy="31" r="0.9" fill={ink} />
        </g>
      );
  }
}

function Mouth({ kind, ink }: { kind: number; ink: string }) {
  if (kind === 0) return <path d="M28.6 38.4Q32 41.2 35.4 38.4" stroke={ink} strokeWidth="1.5" fill="none" strokeLinecap="round" />;
  if (kind === 1) return <path d="M29.4 39H34.6" stroke={ink} strokeWidth="1.5" strokeLinecap="round" />;
  return <ellipse cx="32" cy="39" rx="2.2" ry="1.5" fill={ink} />;
}

/** Point on a circle centred on (32,32); angle in degrees, 0 at twelve o'clock. */
function polar(r: number, deg: number): string {
  const a = ((deg - 90) * Math.PI) / 180;
  return `${(32 + r * Math.cos(a)).toFixed(2)} ${(32 + r * Math.sin(a)).toFixed(2)}`;
}

function arc(r: number, from: number, to: number): string {
  const large = to - from > 180 ? 1 : 0;
  return `M${polar(r, from)}A${r} ${r} 0 ${large} 1 ${polar(r, to)}`;
}

const ORBIT_SPANS = [110, 170, 250];

function TokenSvg({ f }: { f: TokenFeatures }) {
  const rot = f.rotation * 45;
  const seg = 360 / f.segments;
  const gap = f.segments === 1 ? 0 : 22;
  return (
    <svg viewBox="0 0 64 64" aria-hidden>
      <rect width="64" height="64" fill={BG[f.bg]} />
      {f.segments === 1 ? (
        <circle cx="32" cy="32" r="26" fill="none" stroke={CLOTH[f.ring]} strokeWidth="8" />
      ) : (
        Array.from({ length: f.segments }, (_, i) => (
          <path
            key={i}
            d={arc(26, rot + i * seg + gap / 2, rot + (i + 1) * seg - gap / 2)}
            fill="none"
            stroke={CLOTH[f.ring]}
            strokeWidth="8"
            strokeLinecap="round"
          />
        ))
      )}
      <path d={arc(17.5, rot + 200, rot + 200 + ORBIT_SPANS[f.orbitSpan])} fill="none" stroke={CLOTH[f.orbit]} strokeWidth="5" strokeLinecap="round" />
      <circle cx="32" cy="32" r="12" fill={SKIN[f.skin]} />
      {f.glyph === 1 && <circle cx="32" cy="32" r="3.4" fill={INK} />}
      {f.glyph === 2 && <path d="M32 27L37 32L32 37L27 32Z" fill={INK} />}
      {f.glyph === 3 && <circle cx="32" cy="32" r="5" fill="none" stroke={INK} strokeWidth="1.8" />}
    </svg>
  );
}

function Motif({ kind, ink }: { kind: number; ink: string }) {
  const xs = [0, 8, 16, 24, 32, 40, 48, 56];
  switch (MOTIFS[kind]) {
    case "triangles":
      return <path d={xs.map((x) => `M${x} 64L${x + 4} 54L${x + 8} 64Z`).join("")} fill={ink} />;
    case "diamonds":
      return <path d={xs.map((x) => `M${x + 4} 52L${x + 8} 58L${x + 4} 64L${x} 58Z`).join("")} fill={ink} />;
    case "chevrons":
      return <path d={xs.map((x) => `M${x} 61L${x + 4} 55L${x + 8} 61`).join("")} stroke={ink} strokeWidth="2.6" fill="none" />;
    case "checks":
      return <path d={xs.map((x, i) => `M${x} ${i % 2 ? 52 : 58}h8v6h-8Z`).join("")} fill={ink} />;
    case "stripes":
      return (
        <g fill={ink}>
          <rect x="0" y="53" width="64" height="2.5" />
          <rect x="0" y="58.5" width="64" height="2.5" />
        </g>
      );
    default:
      return <g fill={ink}>{xs.map((x) => <circle key={x} cx={x + 4} cy="58" r="2.2" />)}</g>;
  }
}

function SaduSvg({ f }: { f: SaduFeatures }) {
  const fill = topFill(f.top, f.topColor);
  return (
    <svg viewBox="0 0 64 64" aria-hidden>
      <rect width="64" height="64" fill={BG[f.bg]} />
      <TopBehind top={f.top} fill={fill} />
      <path d="M8 64C8 52 18 46 32 46C46 46 56 52 56 64Z" fill={SKIN[f.skin]} opacity="0.55" />
      <ellipse cx="32" cy="30" rx="11.6" ry="13.4" fill={SKIN[f.skin]} />
      <TopFront top={f.top} fill={fill} />
      <rect x="0" y="50" width="64" height="14" fill={CLOTH[f.band]} />
      <Motif kind={f.motif} ink={CLOTH[f.thread]} />
    </svg>
  );
}

function UnknownSvg({ system }: { system: SystemId }) {
  const q = (
    <text x="32" y={system === "token" ? 39.5 : 37} textAnchor="middle" fontSize="20" fontWeight="700" fontFamily="system-ui, sans-serif" fill={VOID_INK}>
      ?
    </text>
  );
  if (system === "token") {
    return (
      <svg viewBox="0 0 64 64" aria-hidden>
        <rect width="64" height="64" fill={VOID} />
        <circle cx="32" cy="32" r="26" fill="none" stroke={VOID_INK} strokeOpacity="0.45" strokeWidth="3" strokeDasharray="5 6" />
        {q}
      </svg>
    );
  }
  return (
    <svg viewBox="0 0 64 64" aria-hidden>
      <rect width="64" height="64" fill={VOID} />
      <ellipse cx="32" cy="66" rx="23" ry="17" fill={VOID_INK} opacity="0.22" />
      <ellipse cx="32" cy="30" rx="12" ry="13.6" fill={VOID_INK} opacity="0.22" />
      {q}
    </svg>
  );
}

export function Face({ system, person }: { system: SystemId; person: PersonSeed }) {
  if (system === "portrait") return <PortraitSvg f={portrait(person)} />;
  if (system === "token") return <TokenSvg f={token(person)} />;
  return <SaduSvg f={sadu(person)} />;
}

export const RADIUS: Record<SystemId, string> = { portrait: "999px", token: "999px", sadu: "24%" };

export interface AvatarProps {
  system: SystemId;
  /** The person the system believes in. Omit for a record no link has reached. */
  person?: PersonSeed;
  state?: AvatarState;
  size?: number;
  label?: string;
}

export function Avatar({ system, person, state = person ? "known" : "unknown", size = 32, label }: AvatarProps) {
  const radius = RADIUS[system];
  return (
    <span
      className="wav-root"
      data-state={person ? state : "unknown"}
      role="img"
      aria-label={label ?? (state === "unknown" || !person ? "Identity not yet confirmed" : "Patient")}
      style={{ width: size, height: size, borderRadius: radius }}
    >
      {person && (
        <span className="wav-layer wav-face" style={{ borderRadius: radius, overflow: "hidden" }}>
          <Face system={system} person={person} />
        </span>
      )}
      <span className="wav-layer wav-unknown" style={{ borderRadius: radius, overflow: "hidden" }}>
        <UnknownSvg system={system} />
      </span>
    </span>
  );
}

/** Two records side by side: overlapped and ringed when they are one person, apart with ≠ when they are two. */
export function AvatarPair({ system, a, b, size = 32, caption }: { system: SystemId; a: PersonSeed; b: PersonSeed; size?: number; caption?: ReactNode }) {
  const same = a.key === b.key;
  return (
    <span className="wav-pair" data-same={same} style={{ ["--pair-size" as string]: `${size}px` }}>
      <Avatar system={system} person={a} size={size} />
      <span className="wav-link" aria-hidden>
        ≠
      </span>
      <Avatar system={system} person={b} size={size} label={same ? "Same person" : "A different person"} />
      {caption}
    </span>
  );
}

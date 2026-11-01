import type { ReactNode } from "react";
import { Avatar, AvatarFrame, INK, PortraitSvg, TopBehind, TopFront, UnknownSvg, topFill, type AvatarState } from "@/components/avatar/avatar";
import { BG, CLOTH, SKIN, portrait, type PersonSeed } from "@/components/avatar/traits";
import { MOTIFS, sadu, token, type SaduFeatures, type SystemId, type TokenFeatures } from "./systems";

/* Systems B and C, kept in the lab beside the product portrait for comparison. */

const VOID = "#e9e5de";
const VOID_INK = "#8b8f8c";

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

function UnknownToken() {
  return (
    <svg viewBox="0 0 64 64" aria-hidden>
      <rect width="64" height="64" fill={VOID} />
      <circle cx="32" cy="32" r="26" fill="none" stroke={VOID_INK} strokeOpacity="0.45" strokeWidth="3" strokeDasharray="5 6" />
      <text x="32" y="39.5" textAnchor="middle" fontSize="20" fontWeight="700" fontFamily="system-ui, sans-serif" fill={VOID_INK}>
        ?
      </text>
    </svg>
  );
}
function Face({ system, person }: { system: SystemId; person: PersonSeed }) {
  if (system === "portrait") return <PortraitSvg f={portrait(person)} />;
  if (system === "token") return <TokenSvg f={token(person)} />;
  return <SaduSvg f={sadu(person)} />;
}

const RADIUS: Record<SystemId, string> = { portrait: "999px", token: "999px", sadu: "24%" };

export function LabAvatar({
  system,
  person,
  state = person ? "known" : "unknown",
  size = 32,
  label,
}: {
  system: SystemId;
  person?: PersonSeed;
  state?: AvatarState;
  size?: number;
  label?: string;
}) {
  if (system === "portrait") return <Avatar person={person} state={state} size={size} label={label} />;
  return (
    <AvatarFrame
      face={person && <Face system={system} person={person} />}
      unknown={system === "token" ? <UnknownToken /> : <UnknownSvg />}
      state={state}
      size={size}
      radius={RADIUS[system]}
      label={label ?? (state === "unknown" || !person ? "Identity not yet confirmed" : "Patient")}
    />
  );
}

/** Two records side by side: overlapped and ringed when they are one person, apart with ≠ when they are two. */
export function LabAvatarPair({ system, a, b, size = 32, caption }: { system: SystemId; a: PersonSeed; b: PersonSeed; size?: number; caption?: ReactNode }) {
  const same = a.key === b.key;
  return (
    <span className="wav-pair" data-same={same} style={{ ["--pair-size" as string]: `${size}px` }}>
      <LabAvatar system={system} person={a} size={size} />
      <span className="wav-link" aria-hidden>
        ≠
      </span>
      <LabAvatar system={system} person={b} size={size} label={same ? "Same person" : "A different person"} />
      {caption}
    </span>
  );
}

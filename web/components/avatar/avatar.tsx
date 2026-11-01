import type { ReactNode } from "react";
import "./avatar.css";
import { BG, CLOTH, COVER, GHUTRA, HAIR, SKIN, portrait, type PersonSeed, type PortraitFeatures, type Top } from "./traits";

/** `resolving` plays the joining moment once: "?" shrinks away and the face opens in its place. */
export type AvatarState = "known" | "unknown" | "resolving";

export const INK = "#1b2224";

export function topFill(top: Top, color: number): string {
  if (top === "ghutra") return GHUTRA[color];
  if (top === "shayla" || top === "hijab" || top === "kufi") return COVER[color];
  return HAIR[color];
}

/** Hair or headwear drawn behind the head. */
export function TopBehind({ top, fill }: { top: Top; fill: string }) {
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
export function TopFront({ top, fill }: { top: Top; fill: string }) {
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

export function PortraitSvg({ f }: { f: PortraitFeatures }) {
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

/** The "?" face: a faint bust behind a question mark, for a record whose link is not trusted yet. */
export function UnknownSvg() {
  return (
    <svg viewBox="0 0 64 64" aria-hidden>
      <rect width="64" height="64" fill="var(--color-raised, #efe8dc)" />
      <ellipse cx="32" cy="66" rx="23" ry="17" fill="var(--color-unknown, #8c9496)" opacity="0.3" />
      <ellipse cx="32" cy="30" rx="12" ry="13.6" fill="var(--color-unknown, #8c9496)" opacity="0.3" />
      <text x="32" y="37" textAnchor="middle" fontSize="20" fontWeight="700" fontFamily="system-ui, sans-serif" fill="var(--color-muted, #56666a)">
        ?
      </text>
    </svg>
  );
}

/** The two stacked layers every avatar system shares, so the "?" → face moment is one CSS rule. */
export function AvatarFrame({
  face,
  unknown,
  state,
  size,
  radius = "999px",
  label,
}: {
  face?: ReactNode;
  unknown: ReactNode;
  state: AvatarState;
  size: number;
  radius?: string;
  label: string;
}) {
  return (
    <span
      className="wav-root"
      data-state={face ? state : "unknown"}
      role={label ? "img" : undefined}
      aria-label={label || undefined}
      aria-hidden={label ? undefined : true}
      style={{ width: size, height: size, borderRadius: radius }}
    >
      {face && (
        <span className="wav-layer wav-face" style={{ borderRadius: radius, overflow: "hidden" }}>
          {face}
        </span>
      )}
      <span className="wav-layer wav-unknown" style={{ borderRadius: radius, overflow: "hidden" }}>
        {unknown}
      </span>
    </span>
  );
}

export interface AvatarProps {
  /** The person the system believes in. Omit for a record no link has reached. */
  person?: PersonSeed;
  state?: AvatarState;
  size?: number;
  /** Accessible name. Pass "" when a visible name sits beside the avatar and it is decoration. */
  label?: string;
}

/** The patient avatar: a flat portrait drawn from a person key, or "?" while the record's link is not trusted. */
export function Avatar({ person, state = person ? "known" : "unknown", size = 32, label }: AvatarProps) {
  const unknown = state === "unknown" || !person;
  return (
    <AvatarFrame
      face={person && <PortraitSvg f={portrait(person)} />}
      unknown={<UnknownSvg />}
      state={state}
      size={size}
      label={label ?? (unknown ? "Identity not yet confirmed" : "Patient")}
    />
  );
}
